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

	"github.com/argoproj/argo-events/common"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/intstr"
	"fmt"
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
		gw:            gw.DeepCopy(),
		updated:       false,
		log:           zlog.New(os.Stdout).With().Str("name", gw.Name).Str("namespace", gw.Namespace).Logger(),
		controller:    controller,
	}
}

func (gwc *gwOperationCtx) operate() error {
	gwc.log.Info().Str("name", gwc.gw.Name).Msg("Operating on gateway-controller")

	// validate gateway
	err := gwc.validate()
	if err != nil {
		gwc.log.Error().Err(err).Msg("gateway validation failed")
		return err
	}

	gatewayClient := gwc.controller.gatewayClientset.ArgoprojV1alpha1().Gateways(gwc.gw.Namespace)

	switch gwc.gw.Status {
	case v1alpha1.NodePhaseNew:
		// Update node phase to running
		gwc.gw.Status = v1alpha1.NodePhaseRunning
		gatewayDeployment := &k8v1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      gwc.gw.Name + "-deployment",
				Namespace: gwc.gw.Namespace,
				Labels: map[string]string{
					"gateway-name": gwc.gw.Name,
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						Name: gwc.gw.Name,
					},
				},
			},
			Spec: k8v1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						ServiceAccountName: gwc.gw.Spec.ServiceAccountName,
						Containers: []corev1.Container{
							{
								Name:            "event-processor",
								ImagePullPolicy: corev1.PullIfNotPresent,
								Image:           gwc.gw.Spec.Image,
								Env: []corev1.EnvVar{
									{
										Name: common.TransformerPortEnvVar,
										Value: fmt.Sprintf("%d", common.TransformerPort),
									},
									{
										Name: common.GatewayConfigMapEnvVar,
										Value: gwc.gw.Spec.ConfigMap,
									},
								},
							},
							{
								Name:            "event-transformer",
								ImagePullPolicy: corev1.PullIfNotPresent,
								Image:           common.GatewayEventTransformerImage,
								Env: []corev1.EnvVar{
									{
										Name: common.GatewayName,
										Value: gwc.gw.Name,
									},
									{
										Name: common.EnvVarNamespace,
										Value: gwc.gw.Namespace,
									},
									{
										Name: common.EventSource,
										Value: gwc.gw.Name,
									},
								},
							},
						},
					},
				},
			},
		}

		_, err := gwc.kubeClientset.AppsV1().Deployments(gwc.gw.Namespace).Create(gatewayDeployment)
		if err != nil {
			gwc.log.Error().Str("gateway-controller", gwc.gw.Name).Err(err).Msg("failed gateway deployment")
			gwc.gw.Status = v1alpha1.NodePhaseError
		} else {
			gwc.gw.Status = v1alpha1.NodePhaseRunning
			gwc.exposeGateway()
		}

		err = gwc.reapplyUpdate(gatewayClient)
		if err != nil {
			gwc.log.Error().Str("gateway-controller", gwc.gw.Name).Msg("failed to update gateway")
			return err
		}
		return nil

	case v1alpha1.NodePhaseError:
		gDeployment, err := gwc.kubeClientset.AppsV1().Deployments(gwc.gw.Namespace).Get(gwc.gw.Name, metav1.GetOptions{})
		if err != nil {
			gwc.log.Error().Str("gateway-controller-name", gwc.gw.Name).Err(err).Msg("Error occurred retrieving gateway-controller deployment")
			return err
		}

		// If image has been updated
		gDeployment.Spec.Template.Spec.Containers[0].Image = gwc.gw.Spec.Image
		_, err = gwc.kubeClientset.AppsV1().Deployments(gwc.gw.Namespace).Update(gDeployment)

		if err != nil {
			gwc.log.Error().Str("gateway-controller-name", gwc.gw.Name).Err(err).Msg("Error occurred updating gateway-controller deployment")
			return err
		}

		// Update node phase to running
		gwc.gw.Status = v1alpha1.NodePhaseRunning
		err = gwc.reapplyUpdate(gatewayClient)
		if err != nil {
			gwc.log.Error().Str("gateway-controller-name", gwc.gw.Name).Msg("failed to update gateway-controller")
			return err
		}
		return nil

	case v1alpha1.NodePhaseRunning:
		gwc.exposeGateway()
		gwc.log.Warn().Str("name", gwc.gw.Name).Msg("Gateway is already running")
	default:
		gwc.log.Panic().Str("name", gwc.gw.Name).Str("phase", string(gwc.gw.Status)).Msg("Unknown gateway-controller phase.")
	}
	return nil
}

// Exposes gateway if configured
func (gwc *gwOperationCtx) exposeGateway() {
	if gwc.gw.Spec.Service.Port != 0 {
		gatewaySvc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: gwc.gw.Name + "-svc",
				Namespace: gwc.gw.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						Name: gwc.gw.Name,
					},
				},
			},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{
					"gateway-name": gwc.gw.Name,
				},
				Ports: []corev1.ServicePort{
					{
						Port: gwc.gw.Spec.Service.Port,
						TargetPort: intstr.FromInt(gwc.gw.Spec.Service.TargetPort),
					},
				},
				Type: corev1.ServiceType(gwc.gw.Spec.Service.Type),
			},
		}

		_, err := gwc.controller.kubeClientset.CoreV1().Services(gwc.gw.Namespace).Create(gatewaySvc)
		// Fail silently
		if err != nil {
			gwc.log.Error().Err(err).Msg("failed to create service for gateway deployment")
		}
	}
}

func (gwc *gwOperationCtx) reapplyUpdate(gatewayClient client.GatewayInterface) error {
	return wait.ExponentialBackoff(common.DefaultRetry, func() (bool, error) {
		g, err := gatewayClient.Get(gwc.gw.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		g.Status = gwc.gw.Status
		gwc.gw, err = gatewayClient.Update(g)
		if err != nil {
			if !common.IsRetryableKubeAPIError(err) {
				return false, err
			}
			return false, nil
		}
		return true, nil
	})
}
