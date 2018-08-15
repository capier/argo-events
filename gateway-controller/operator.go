package gateway_controller

import (
	"github.com/argoproj/argo-events/pkg/apis/gateway/v1alpha1"
	client "github.com/argoproj/argo-events/pkg/gateway-client/clientset/versioned/typed/gateway/v1alpha1"
	zlog "github.com/rs/zerolog"
	k8v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"os"

	"fmt"
	"github.com/argoproj/argo-events/common"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
)

// the context of an operation on a gateway-controller.
// the gateway-controller-controller creates this context each time it picks a Gateway off its queue.
type gwOperationCtx struct {
	// gw is the gateway-controller object
	gw *v1alpha1.Gateway

	// updated indicates whether the gateway-controller object was updated and needs to be persisted back to k8
	updated bool

	// log is the logger for a gateway-controller
	log zlog.Logger

	// reference to the gateway-controller-controller
	controller *GatewayController

	// kubernetes clientset
	kubeClientset kubernetes.Clientset
}

// newGatewayOperationCtx creates and initializes a new gOperationCtx object
func newGatewayOperationCtx(gw *v1alpha1.Gateway, controller *GatewayController) *gwOperationCtx {
	return &gwOperationCtx{
		gw:         gw.DeepCopy(),
		updated:    false,
		log:        zlog.New(os.Stdout).With().Str("name", gw.Name).Str("namespace", gw.Namespace).Logger(),
		controller: controller,
	}
}

func (goc *gwOperationCtx) operate() error {
	goc.log.Info().Str("name", goc.gw.Name).Msg("operating on the gateway")

	// validate gateway
	err := goc.validate()
	if err != nil {
		goc.log.Error().Err(err).Msg("gateway validation failed")
		return err
	}
	gatewayClient := goc.controller.gatewayClientset.ArgoprojV1alpha1().Gateways(goc.gw.Namespace)

	// manages states of a gateway
	switch goc.gw.Status {
	case v1alpha1.NodePhaseNew:
		// Update node phase to running
		goc.gw.Status = v1alpha1.NodePhaseRunning
		gatewayDeployment := &k8v1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      goc.gw.Name + "-deployment",
				Namespace: goc.gw.Namespace,
				Labels: map[string]string{
					"gateway-name": goc.gw.Name,
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						Name: goc.gw.Name,
					},
				},
			},
			Spec: k8v1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						ServiceAccountName: goc.gw.Spec.ServiceAccountName,
						Containers: []corev1.Container{
							{
								Name: "event-processor",
								// Add image pull policy to gateway types definition
								ImagePullPolicy: corev1.PullIfNotPresent,
								Image:           goc.gw.Spec.Image,
								Env: []corev1.EnvVar{
									{
										Name:  common.TransformerPortEnvVar,
										Value: fmt.Sprintf("%d", common.TransformerPort),
									},
									{
										Name:  common.GatewayConfigMapEnvVar,
										Value: goc.gw.Spec.ConfigMap,
									},
								},
							},
							{
								Name:            "event-transformer",
								ImagePullPolicy: corev1.PullIfNotPresent,
								Image:           common.GatewayEventTransformerImage,
								Env: []corev1.EnvVar{
									{
										Name:  common.EnvVarNamespace,
										Value: goc.gw.Namespace,
									},
									{
										Name:  common.EventSource,
										Value: goc.gw.Name,
									},
								},
							},
						},
					},
				},
			},
		}

		// we can now create the gateway deployment.
		// depending on user configuration gateway will be exposed outside the cluster or intra-cluster.
		_, err := goc.kubeClientset.AppsV1().Deployments(goc.gw.Namespace).Create(gatewayDeployment)
		if err != nil {
			goc.log.Error().Str("gateway", goc.gw.Name).Err(err).Msg("failed gateway deployment")
			goc.gw.Status = v1alpha1.NodePhaseError
		} else {
			goc.gw.Status = v1alpha1.NodePhaseRunning
			if goc.gw.Spec.Service.Port != 0 {
				goc.exposeGateway()
			}
		}

		// update state of the gateway
		goc.gw, err = gatewayClient.Update(goc.gw)
		if err != nil {
			err = goc.reapplyUpdate(gatewayClient)
			if err != nil {
				goc.log.Error().Str("gateway", goc.gw.Name).Msg("failed to update gateway")
				return err
			}
		}

		// Gateway is in error
	case v1alpha1.NodePhaseError:
		gDeployment, err := goc.kubeClientset.AppsV1().Deployments(goc.gw.Namespace).Get(goc.gw.Name, metav1.GetOptions{})
		if err != nil {
			goc.log.Error().Str("gateway name", goc.gw.Name).Err(err).Msg("Error occurred retrieving gateway deployment")
			return err
		}

		// If image has been updated
		gDeployment.Spec.Template.Spec.Containers[0].Image = goc.gw.Spec.Image
		_, err = goc.kubeClientset.AppsV1().Deployments(goc.gw.Namespace).Update(gDeployment)
		if err != nil {
			goc.log.Error().Str("gateway", goc.gw.Name).Err(err).Msg("Error occurred updating gateway deployment")
			return err
		}

		// Update node phase to running
		goc.gw.Status = v1alpha1.NodePhaseRunning
		// update state of the gateway
		goc.gw, err = gatewayClient.Update(goc.gw)
		if err != nil {
			err = goc.reapplyUpdate(gatewayClient)
			if err != nil {
				goc.log.Error().Str("gateway-controller", goc.gw.Name).Msg("failed to update gateway")
				return err
			}
		}
		return nil

		// Gateway is
	case v1alpha1.NodePhaseRunning:
		// Todo: if the sensor to which event should be dispatched changes then update the configmap for gateway pod
		goc.log.Warn().Str("name", goc.gw.Name).Msg("Gateway is already running")
	default:
		goc.log.Panic().Str("name", goc.gw.Name).Str("phase", string(goc.gw.Status)).Msg("Unknown gateway phase.")
	}
	return nil
}

// Creates a service that exposes gateway outside the cluster
func (goc *gwOperationCtx) exposeGateway() {
	gatewayService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      goc.gw.Name + "-svc",
			Namespace: goc.gw.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					Name: goc.gw.Name,
				},
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"gateway-name": goc.gw.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Port:       goc.gw.Spec.Service.Port,
					TargetPort: intstr.FromInt(goc.gw.Spec.Service.TargetPort),
				},
			},
			Type: corev1.ServiceType(goc.gw.Spec.Service.Type),
		},
	}

	_, err := goc.controller.kubeClientset.CoreV1().Services(goc.gw.Namespace).Create(gatewayService)
	// Fail silently
	if err != nil {
		goc.log.Error().Err(err).Msg("failed to create service for gateway deployment")
	}
}

func (goc *gwOperationCtx) reapplyUpdate(gatewayClient client.GatewayInterface) error {
	return wait.ExponentialBackoff(common.DefaultRetry, func() (bool, error) {
		g, err := gatewayClient.Get(goc.gw.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		g.Status = goc.gw.Status
		goc.gw, err = gatewayClient.Update(g)
		if err != nil {
			if !common.IsRetryableKubeAPIError(err) {
				return false, err
			}
			return false, nil
		}
		return true, nil
	})
}
