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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	stream *pb.SensorUpdate_UpdateSensorClient, sensor *v1alpha1.Sensor, log zerolog.Logger) *sensorCtx {
	return &sensorCtx{
		sensorClient: sensorClient,
		kubeClient: kubeClient,
		config: config,
		stream: stream,
		sensor: sensor,
		log: log,
	}
}

// to resync
func (sc *sensorCtx) watchSensorUpdates() {
	watcher, err := sc.sensorClient.Sensors(sc.sensor.Namespace).Watch(metav1.ListOptions{})
	if err != nil {
		sc.log.Panic().Err(err).Msg("failed to watch sensor object")
	}
	for update := range watcher.ResultChan() {
		// do we need a deep copy?
		sc.sensor = update.Object.(*v1alpha1.Sensor).DeepCopy()
	}
}

func (sc *sensorCtx) StartHttpServer() *http.Server {
	// watch sensor updates
	go sc.watchSensorUpdates()
	srv := &http.Server{Addr: fmt.Sprintf(":%d", common.SensorServicePort)}
	http.HandleFunc("/", sc.handleSignalNotification)
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			// cannot panic, because this probably is an intentional close
			log.Warn().Err(err).Msg("sensor server stopped")
		}
	}()
	return srv
}

// Hanldes notifications from gateway/s
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

	// check if signal is already completed
	if getNodeByName(sc.sensor, gatewayNotification.Ctx.Source).Phase == v1alpha1.NodePhaseComplete {
		log.Error().Str("signal-source", gatewayNotification.Ctx.Source).Msg("signal is already completed")
		return
	}

	// update the signal state
	sc.updateNodePhase(gatewayNotification.Ctx.Source, v1alpha1.NodePhaseComplete)
	sc.sensor, err = sc.updateSensor()
	if err != nil {
		sc.log.Error().Str("signal-name", gatewayNotification.Ctx.Source).Msg("failed to update signal node state")
		return
	}

	// check if all signals are completed
	completed := sc.sensor.AreAllNodesSuccess(v1alpha1.NodeTypeSignal) && sc.sensor.Status.Phase != v1alpha1.NodePhaseComplete
	if completed {
		sc.log.Info().Msg("all signals are processed")
		// trigger action/s
		for _, trigger := range sc.sensor.Spec.Triggers {
			sc.updateNodePhase(trigger.Name, v1alpha1.NodePhaseActive)
			sc.sensor, err = sc.updateSensor()
			err := sc.executeTrigger(trigger)
			if err != nil {
				sc.log.Error().Str("trigger-name", trigger.Name).Err(err).Msg("trigger failed to execute")
				sc.updateNodePhase(trigger.Name, v1alpha1.NodePhaseError)
				sc.updateNodePhase(sc.sensor.Name, v1alpha1.NodePhaseError)
				// update the sensor object with error state
				sc.sensor, err = sc.sensorClient.Sensors(sc.sensor.Namespace).Update(sc.sensor)
				if err != nil {
					sc.log.Error().Err(err).Msg("failed to update sensor phase")
				}
				return
			}
			// update trigger state
			sc.updateNodePhase(trigger.Name, v1alpha1.NodePhaseComplete)
			sc.sensor, err = sc.updateSensor()
			if err != nil {
				sc.log.Error().Err(err).Msg("failed to update sensor after trigger completion")
			}
		}
	}
}

func (sc *sensorCtx) updateSensor() (*v1alpha1.Sensor, error) {
	return sc.sensorClient.Sensors(sc.sensor.Namespace).Update(sc.sensor)
}

func (sc *sensorCtx) updateNodePhase(name string, phase v1alpha1.NodePhase) {
	node := sc.sensor.Status.Nodes[sc.sensor.NodeID(name)]
	node.Phase = phase
	sc.sensor.Status.Nodes[sc.sensor.NodeID(name)] = node
}

func (sc *sensorCtx) performAction() {
	var action *pb.SensorEvent
	err := (*sc.stream).RecvMsg(action)
	if err != nil {
		log.Error().Err(err).Msg("failed to get action message from sensor controller")
	}
	switch common.TriggerAction(action.Type) {
	case common.TriggerAndRepeat:
		log.Debug().Msg("keep sensor server running")
	case common.TriggerAndStop:
		// Close the gRPC stream and shutdown sensor http server
		(*sc.stream).CloseSend()
		sc.server.Shutdown(context.Background())
	default:
		log.Warn().Str("action-type", action.Type).Msg("unknown action type")
	}
}