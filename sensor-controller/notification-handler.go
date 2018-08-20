package sensor_controller

import (
	"encoding/json"
	"fmt"
	"github.com/argoproj/argo-events/common"
	"github.com/argoproj/argo-events/pkg/apis/sensor/v1alpha1"
	sv1alpha "github.com/argoproj/argo-events/pkg/apis/sensor/v1alpha1"
	sv1 "github.com/argoproj/argo-events/pkg/sensor-client/clientset/versioned/typed/sensor/v1alpha1"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/context"
	"io/ioutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"net/http"
	"sync"
	"time"
)

// sensorExecutor contains execution context for sensor operation
type sensorExecutor struct {
	// sensorClient is the client for sensor
	sensorClient sv1.ArgoprojV1alpha1Interface
	// kubeClient is the kubernetes client
	kubeClient kubernetes.Interface
	// config is the cluster config
	config *rest.Config
	// sensor object
	sensor *v1alpha1.Sensor
	// http server which exposes the sensor to gateway/s
	server *http.Server
	// logger for the sensor
	log zerolog.Logger
}

func NewSensorExecutor(sensorClient sv1.ArgoprojV1alpha1Interface, kubeClient kubernetes.Interface, config *rest.Config, sensor *v1alpha1.Sensor, log zerolog.Logger) *sensorExecutor {
	return &sensorExecutor{
		sensorClient: sensorClient,
		kubeClient:   kubeClient,
		config:       config,
		sensor:       sensor,
		log:          log,
	}
}

// resyncs the sensor object for status updates
func (se *sensorExecutor) watchSensorUpdates() {
	watcher, err := se.sensorClient.Sensors(se.sensor.Namespace).Watch(metav1.ListOptions{})
	if err != nil {
		se.log.Panic().Err(err).Msg("failed to watch sensor object")
	}
	for update := range watcher.ResultChan() {
		se.sensor = update.Object.(*v1alpha1.Sensor).DeepCopy()
		if se.sensor.Status.Phase == v1alpha1.NodePhaseError {
			// Shutdown server as sensor should not process any event in error state
			se.server.Shutdown(context.Background())
		}
	}
}

// WatchNotifications watches and handles signals sent by the gateway the sensor is interested in.
func (se *sensorExecutor) WatchSignalNotifications(wg sync.WaitGroup) *http.Server {
	// watch sensor updates
	go se.watchSensorUpdates()
	srv := &http.Server{Addr: fmt.Sprintf(":%d", common.SensorServicePort)}
	http.HandleFunc("/", se.handleSignals)
	go func() {
		log.Info().Str("port", string(common.SensorServicePort)).Msg("sensor started listening")
		if err := srv.ListenAndServe(); err != nil {
			// cannot panic, because this probably is an intentional close
			log.Warn().Err(err).Msg("sensor server stopped")
			wg.Done()
		}
	}()
	return srv
}

// Handles signals received from gateway/s
func (se *sensorExecutor) handleSignals(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	gatewaySignal := sv1alpha.Event{}
	err = json.Unmarshal(body, &gatewaySignal)
	if err != nil {
		log.Error().Err(err).Msg("failed to decode notification")
	}

	// validate the signal is from gateway of interest
	for _, signal := range se.sensor.Spec.Signals {
		if signal.Name != gatewaySignal.Context.Source.Host {
			log.Error().Str("signal-source", gatewaySignal.Context.Source.Host).Msg("unknown signal source")
			return
		}
	}

	// if signal is already completed, don't process the new event.
	// Todo: what should be done when rate of signals received exceeds the complete sensor processing time?
	// maybe add queueing logic
	if getNodeByName(se.sensor, gatewaySignal.Context.Source.Host).Phase == v1alpha1.NodePhaseComplete {
		log.Warn().Str("signal-source", gatewaySignal.Context.Source.Host).Msg("signal is already completed")
		return
	}

	// process the signal
	se.processSignal(gatewaySignal.Context.Source.Host, &gatewaySignal)
	se.sensor, err = se.updateSensor()
	if err != nil {
		err = se.reapplyUpdate()
		if err != nil {
			se.log.Error().Str("signal-name", gatewaySignal.Context.Source.Host).Msg("failed to update signal node state")
			return
		}
	}

	// to trigger the sensor action/s we first need to check if all signals are completed and sensor is active
	completed := se.sensor.AreAllNodesSuccess(v1alpha1.NodeTypeSignal) && se.sensor.Status.Phase == v1alpha1.NodePhaseActive
	if completed {
		se.log.Info().Msg("all signals are processed")
		// trigger action/s
		for _, trigger := range se.sensor.Spec.Triggers {
			se.updateNodePhase(trigger.Name, v1alpha1.NodePhaseActive, "trigger is about to run")
			// update the sensor for marking trigger as active
			se.sensor, err = se.updateSensor()
			err := se.executeTrigger(trigger)
			if err != nil {
				// Todo: should we let other triggers to run?
				se.log.Error().Str("trigger-name", trigger.Name).Err(err).Msg("trigger failed to execute")
				se.updateNodePhase(trigger.Name, v1alpha1.NodePhaseError, err.Error())
				se.sensor.Status.Phase = v1alpha1.NodePhaseError
				// update the sensor object with error state
				se.sensor, err = se.updateSensor()
				if err != nil {
					err = se.reapplyUpdate()
					if err != nil {
						se.log.Error().Err(err).Msg("failed to update sensor phase")
						return
					}
				}
			}
			// mark trigger as complete.
			se.updateNodePhase(trigger.Name, v1alpha1.NodePhaseComplete, "trigger completed successfully")
			se.sensor, err = se.updateSensor()
			if err != nil {
				err = se.reapplyUpdate()
				if err != nil {
					se.log.Error().Err(err).Msg("failed to update trigger completion state")
					return
				}
			}
		}
		if !se.sensor.Spec.Repeat {
			// todo: unsubscribe from pub-sub system
			se.server.Shutdown(context.Background())
		}
	}
}

func (sc *sensorExecutor) updateSensor() (*v1alpha1.Sensor, error) {
	return sc.sensorClient.Sensors(sc.sensor.Namespace).Update(sc.sensor)
}

// update the status of the node
func (sc *sensorExecutor) updateNodePhase(name string, phase v1alpha1.NodePhase, msg string) {
	node := getNodeByName(sc.sensor, name)
	node.Message = msg
	node.Phase = phase
	if node.IsComplete() && node.CompletedAt.IsZero() {
		node.CompletedAt = metav1.Time{Time: time.Now().UTC()}
		sc.log.Info().Str("type", string(node.Type)).Str("node-name", node.Name).Str("completed at", node.CompletedAt.String())
	}
	sc.sensor.Status.Nodes[node.ID] = *node
}

func (sc *sensorExecutor) reapplyUpdate() error {
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
