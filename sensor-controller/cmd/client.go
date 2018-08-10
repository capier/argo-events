package main

import (
	"encoding/json"
	"fmt"
	"github.com/argoproj/argo-events/common"
	v1alpha1 "github.com/argoproj/argo-events/pkg/apis/sensor/v1alpha1"
	"github.com/argoproj/argo-events/pkg/event"
	sv1 "github.com/argoproj/argo-events/pkg/sensor-client/clientset/versioned/typed/sensor/v1alpha1"
	pb "github.com/argoproj/argo-events/proto"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"net/http"
	"os"
	"github.com/rs/zerolog"
	sc "github.com/argoproj/argo-events/sensor-controller"
)

var kubeConfig string

func getClientConfig(kubeConfig string) (*rest.Config, error) {
	if kubeConfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeConfig)
	}
	return rest.InClusterConfig()
}

func (sc *sensorCtx) handleSignalNotification(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	var gatewayNotification *event.Event
	err := decoder.Decode(gatewayNotification)
	if err != nil {
		log.Error().Err(err).Msg("failed to decode gateway notification")
	}
	// validate the notification is from gateway of interest
	for _, signal := range sc.sensor.Spec.Signals {
		if signal.Name != gatewayNotification.Ctx.Source {
			log.Error().Str("signal-source", gatewayNotification.Ctx.Source).Msg("unknown signal source")
			return
		}
	}

	go sc.performAction()

	err = (*sc.stream).Send(&pb.SensorEvent{
		Name: gatewayNotification.Ctx.Source,
		Type: gatewayNotification.Ctx.EventType,
	})

	if err != nil {
		log.Error().Msg("failed to send signal to sensor controller")
	}
}

func (sc *sensorCtx) updateNodePhase(name string, phase v1alpha1.NodePhase) {
	node := sc.sensor.Status.Nodes[sc.sensor.NodeID(name)]
	node.Phase = phase
	sc.sensor.Status.Nodes[sc.sensor.NodeID(name)] = node
}

func (sc *sensorCtx) performAction() {
	log.Info().Msg("waiting for action command from sensor controller")

	var action *pb.SensorEvent
	err := (*sc.stream).RecvMsg(action)
	if err != nil {
		log.Error().Err(err).Msg("failed to get action message from sensor controller")
	}

	for _, trigger := range sc.sensor.Spec.Triggers {
		err := sc.executeTrigger(trigger)
		if err != nil {
			sc.log.Error().Str("trigger-name", trigger.Name).Err(err).Msg("trigger failed to execute")
			sc.updateNodePhase(trigger.Name, v1alpha1.NodePhaseError)
			sc.updateNodePhase(sc.sensor.Name, v1alpha1.NodePhaseError)
			// update the sensor object
			sc.sensor, err = sc.sensorClient.Sensors(sc.sensor.Namespace).Update(sc.sensor)
			if err != nil {
				sc.log.Error().Err(err).Msg("failed to update sensor phase")
			}
			return
		}
	}

	switch common.TriggerAction(action.Type) {
	case common.TriggerAndRepeat:
		log.Debug().Msg("keeping sensor server running")
	case common.TriggerAndStop:
		// Close the gRPC stream and shutdown sensor http server
		(*sc.stream).CloseSend()
		sc.server.Shutdown(context.Background())
	default:
		log.Warn().Str("action-type", action.Type).Msg("unknown action type")
	}
}

func startHttpServer() *http.Server {
	srv := &http.Server{Addr: fmt.Sprintf(":%d", common.SensorServicePort)}
	http.HandleFunc("/", sc.HandleSignalNotification)
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			// cannot panic, because this probably is an intentional close
			log.Warn().Err(err).Msg("sensor server stopped")
		}
	}()
	return srv
}

func main() {
	sensorName, _ := os.LookupEnv(common.SensorName)
	if sensorName == "" {
		panic("sensor name is not provided")
	}

	sensorNamespace, _ := os.LookupEnv(common.SensorNamespace)
	if sensorNamespace == "" {
		panic("sensor namespace is not provided")
	}

	serverAddr, _ := os.LookupEnv(common.DefaultSensorControllerDeploymentName)
	if serverAddr == "" {
		panic("sensor controller name not provided")
	}

	config, err := getClientConfig(kubeConfig)
	if err != nil {
		panic(fmt.Sprintf("failed to get client configuration. Err: %+v", err))
	}

	log := zerolog.New(os.Stdout).With().Str("sensor-name", sensorName).Logger()

	conn, err := grpc.Dial(serverAddr, grpc.WithInsecure())
	if err != nil {
		log.Fatal().Err(err).Msg("sensor did not connect to sensor controller")
	}
	defer conn.Close()

	client := pb.NewSensorUpdateClient(conn)
	stream, err := client.UpdateSensor(context.Background())
	if err != nil {
		log.Fatal().Err(err).Msg("failed to get signal notification stream")
	}
	sensorClient, err := sv1.NewForConfig(config)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to get sensor client")
	}
	kubeClient := kubernetes.NewForConfigOrDie(config)

	// Todo: already created it in operator
	// This service will be removed once Pub-Sub system is implemented.
	// Sensor will then act as subscriber rather than having a http server running
	// to listen to gateway signal notifications
	//sensorSvc := &corev1.Service{
	//	ObjectMeta: metav1.ObjectMeta{
	//		Name:      sensorName,
	//		Namespace: sensorNamespace,
	//	},
	//	Spec: corev1.ServiceSpec{
	//		Selector: map[string]string{
	//			"name": sensorName,
	//		},
	//		Type: corev1.ServiceTypeClusterIP,
	//		Ports: []corev1.ServicePort{
	//			{
	//				Port:       common.SensorServicePort,
	//				TargetPort: intstr.FromInt(common.SensorServicePort),
	//			},
	//		},
	//	},
	//}
	//
	//_, err = kubeClient.CoreV1().Services(sensorNamespace).Create(sensorSvc)
	//if err != nil {
	//	// fail silently
	//	log.Error().Err(err).Msg("failed to create sensor service")
	//}

	sensor, err := sensorClient.Sensors(sensorNamespace).Get(sensorName, metav1.GetOptions{})
	if err != nil {
		log.Panic().Err(err).Msg("failed to retrieve sensor")
	}

	srv := startHttpServer()
	sCtx := sc.NewSensorContext(sensorClient, kubeClient, config, &stream, sensor, srv, log)
}
