package transform

import (
	"net/http"
	"io/ioutil"
	"fmt"
	"github.com/argoproj/argo-events/common"
	suuid "github.com/satori/go.uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
	"k8s.io/apimachinery/pkg/util/wait"
	"encoding/json"
)

// Transform request transforms http request payload into CloudEvent
func (eoc *eOperationCtx) transform(r *http.Request) (*Event, error) {
	// Generate event id
	eventId := suuid.Must(suuid.NewV4())
	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Errorf("failed to parse request payload. Err %+v", err)
		return nil, err
	}

	// Create an CloudEvent
	ce := &Event{
		ctx: EventContext{
			CloudEventsVersion: common.CloudEventsVersion,
			EventID:            fmt.Sprintf("%x", eventId),
			ContentType:        r.Header.Get(common.HeaderContentType),
			EventTime:          time.Now(),
			EventType:          eoc.Config.EventType,
			EventTypeVersion:   eoc.Config.EventTypeVersion,
			Source:             eoc.Config.Source,
		},
		payload: payload,
	}

	return ce, nil
}

func (eoc *eOperationCtx) sendEvent(ce *Event) error {
	sensorService, err := eoc.kubeClientset.CoreV1().Services(eoc.Namespace).Get(eoc.Config.Sensor, metav1.GetOptions{})
	if err != nil {
		eoc.log.Error().Str("sensor-svc", eoc.Config.Sensor).Err(err).Msg("failed to connect to sensor service")
		return err
	}
	if sensorService.Spec.ClusterIP == "" {
		eoc.log.Error().Str("sensor-svc", eoc.Config.Sensor).Msg("failed to get cluster ip.")
		// Retry to get cluster ip
		err = eoc.connectSensorService()
		if err != nil {
			eoc.log.Error().Str("sensor-svc", eoc.Config.Sensor).Err(err).Msg("failed to connect to sensor service")
			return err
		}
	}
	eoc.log.Debug().Str("sensor-service-ip", sensorService.Spec.ClusterIP).Msg("sensor service cluster ip")
	eventBytes, err := json.Marshal(ce)
	http.Post(sensorService.Spec.ClusterIP, "application/json", byte())
}

func (eoc *eOperationCtx) connectSensorService() error {
	return wait.ExponentialBackoff(common.DefaultRetry, func() (bool, error) {
		sensorService, err := eoc.kubeClientset.CoreV1().Services(eoc.Namespace).Get(eoc.Config.Sensor, metav1.GetOptions{})
		if err != nil {
			eoc.log.Error().Str("sensor-svc", eoc.Config.Sensor).Err(err).Msg("failed to connect to sensor service")
			return false, err
		} else {
			if sensorService.Spec.ClusterIP == "" {
				return false, nil
			} else {
				return true, nil
			}
		}
	})
}

func (eoc *eOperationCtx) HandleTransformRequest(w http.ResponseWriter, r *http.Request) {
	ce, err := eoc.transform(r)
	if err != nil {
		eoc.log.Error().Err(err).Msg("failed to transform user event into CloudEvent")
		return
	}
	err = eoc.sendEvent(ce)
	if err != nil {

	}
}