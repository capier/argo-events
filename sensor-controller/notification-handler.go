package sensor_controller

import (
	"encoding/json"
	"github.com/argoproj/argo-events/common"
	"github.com/argoproj/argo-events/pkg/apis/sensor/v1alpha1"
	sv1alpha "github.com/argoproj/argo-events/pkg/apis/sensor/v1alpha1"
	sv1 "github.com/argoproj/argo-events/pkg/sensor-client/clientset/versioned/typed/sensor/v1alpha1"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/context"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"github.com/rs/zerolog"
	"fmt"
	"k8s.io/apimachinery/pkg/util/wait"
)

type sensorCtx struct {
	// sensorClient is the client for sensor
	sensorClient sv1.ArgoprojV1alpha1Interface

	// kubeClient is the kubernetes client
	kubeClient kubernetes.Interface

	// config is the cluster config
	config  *rest.Config

	// sensor object
	sensor *v1alpha1.Sensor

	// http server which exposes the sensor to gateway/s
	server *http.Server

	// logger for the sensor
	log zerolog.Logger
}

func NewSensorContext(sensorClient sv1.ArgoprojV1alpha1Interface, kubeClient kubernetes.Interface, config  *rest.Config, sensor *v1alpha1.Sensor, log zerolog.Logger) *sensorCtx {
	return &sensorCtx{
		sensorClient: sensorClient,
		kubeClient: kubeClient,
		config: config,
		sensor: sensor,
		log: log,
	}
}

// resyncs the sensor object
func (sc *sensorCtx) watchSensorUpdates() {
	watcher, err := sc.sensorClient.Sensors(sc.sensor.Namespace).Watch(metav1.ListOptions{})
	if err != nil {
		sc.log.Panic().Err(err).Msg("failed to watch sensor object")
	}
	for update := range watcher.ResultChan() {
		sc.sensor = update.Object.(*v1alpha1.Sensor).DeepCopy()
	}
}

// WatchNotifications watches and handles signals sent by the gateway the sensor is interested in.
func (sc *sensorCtx) WatchSignalNotifications() *http.Server {
	// watch sensor updates
	go sc.watchSensorUpdates()
	srv := &http.Server{Addr: fmt.Sprintf(":%d", common.SensorServicePort)}
	http.HandleFunc("/", sc.handleSignals)
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			// cannot panic, because this probably is an intentional close
			log.Warn().Err(err).Msg("sensor server stopped")
		}
	}()
	return srv
}

// Handles signals from gateway/s
func (sc *sensorCtx) handleSignals(w http.ResponseWriter, r *http.Request) {
	// decode signals received from the gateway
	decoder := json.NewDecoder(r.Body)
	var gatewayNotification *sv1alpha.Event
	err := decoder.Decode(gatewayNotification)
	if err != nil {
		log.Error().Err(err).Msg("failed to decode gateway notification")
	}

	// validate the signal notification is from gateway of interest
	for _, signal := range sc.sensor.Spec.Signals {
		if signal.Name != gatewayNotification.Context.Source.Host {
			log.Error().Str("signal-source", gatewayNotification.Context.Source.Host).Msg("unknown signal source")
			return
		}
	}

	// check if signal is already completed
	if getNodeByName(sc.sensor, gatewayNotification.Context.Source.Host).Phase == v1alpha1.NodePhaseComplete {
		log.Error().Str("signal-source", gatewayNotification.Context.Source.Host).Msg("signal is already completed")
		return
	}

	// process the signal
	sc.processSignal(gatewayNotification.Context.Source.Host, gatewayNotification)
	sc.sensor, err = sc.updateSensor()
	if err != nil {
		err = sc.reapplyUpdate()
		if err != nil {
			sc.log.Error().Str("signal-name", gatewayNotification.Context.Source.Host).Msg("failed to update signal node state")
			return
		}
	}

	// to trigger the sensor action/s we first need to check if all signals are completed and sensor is active
	completed := sc.sensor.AreAllNodesSuccess(v1alpha1.NodeTypeSignal) && sc.sensor.Status.Phase == v1alpha1.NodePhaseActive
	if completed {
		sc.log.Info().Msg("all signals are processed")
		// trigger action/s
		for _, trigger := range sc.sensor.Spec.Triggers {
			sc.updateNodePhase(trigger.Name, v1alpha1.NodePhaseActive)
			// update the sensor for marking trigger as active
			sc.sensor, err = sc.updateSensor()
			err := sc.executeTrigger(trigger)
			if err != nil {
				sc.log.Error().Str("trigger-name", trigger.Name).Err(err).Msg("trigger failed to execute")
				sc.updateNodePhase(trigger.Name, v1alpha1.NodePhaseError)
				sc.updateNodePhase(sc.sensor.Name, v1alpha1.NodePhaseError)
				// update the sensor object with error state
				sc.sensor, err = sc.updateSensor()
				if err != nil {
					err = sc.reapplyUpdate()
					if err != nil {
						sc.log.Error().Err(err).Msg("failed to update sensor phase")
						return
					}
				}
			}

			// mark trigger as complete.
			sc.updateNodePhase(trigger.Name, v1alpha1.NodePhaseComplete)
			sc.sensor, err = sc.updateSensor()
			if err != nil {
				err = sc.reapplyUpdate()
				if err != nil {
					sc.log.Error().Err(err).Msg("failed to update trigger completion state")
					return
				}
			}
		}

		if !sc.sensor.Spec.Repeat {
			// todo: unsubscribe from pub-sub system
			sc.server.Shutdown(context.Background())
		}
	}
}

func (sc *sensorCtx) updateSensor() (*v1alpha1.Sensor, error) {
	return sc.sensorClient.Sensors(sc.sensor.Namespace).Update(sc.sensor)
}

func (sc *sensorCtx) updateNodePhase(name string, phase v1alpha1.NodePhase) {
	// Todo: add information like update message, update time to node
	node := getNodeByName(sc.sensor, name)
	node.Phase = phase
	sc.sensor.Status.Nodes[node.ID] = *node
}

func (sc *sensorCtx) reapplyUpdate() error {
	return wait.ExponentialBackoff(common.DefaultRetry, func() (bool, error) {
		sClient := sc.sensorClient.Sensors(sc.sensor.Namespace)
		s, err := sClient.Get(sc.sensor.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		s.Status = sc.sensor.Status
		sc.sensor, err = sClient.Update(s)
		if err != nil {
			if !common.IsRetryableKubeAPIError(err) {
				return false, err
			}
			return false, nil
		}
		return true, nil
	})
}
