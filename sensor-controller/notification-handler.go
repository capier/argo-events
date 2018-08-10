package sensor_controller

import (
	"encoding/json"
	"github.com/argoproj/argo-events/common"
	v1alpha1 "github.com/argoproj/argo-events/pkg/apis/sensor/v1alpha1"
	"github.com/argoproj/argo-events/pkg/event"
	sv1 "github.com/argoproj/argo-events/pkg/sensor-client/clientset/versioned/typed/sensor/v1alpha1"
	pb "github.com/argoproj/argo-events/proto"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/context"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"net/http"
	"github.com/rs/zerolog"
	"fmt"
)

type sensorCtx struct {
	sensorClient sv1.ArgoprojV1alpha1Interface

	kubeClient kubernetes.Interface

	config  *rest.Config

	stream *pb.SensorUpdate_UpdateSensorClient

	sensor *v1alpha1.Sensor

	server *http.Server

	log zerolog.Logger
}

func NewSensorContext(sensorClient sv1.ArgoprojV1alpha1Interface, kubeClient kubernetes.Interface, config  *rest.Config,
	stream *pb.SensorUpdate_UpdateSensorClient, sensor *v1alpha1.Sensor, server *http.Server, log zerolog.Logger) *sensorCtx {
	return &sensorCtx{
		sensorClient: sensorClient,
		kubeClient: kubeClient,
		config: config,
		stream: stream,
		sensor: sensor,
		server: server,
		log: log,
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