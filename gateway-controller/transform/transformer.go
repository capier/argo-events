package transform

import (
	"net/http"
	"io/ioutil"
	"fmt"
	"github.com/argoproj/argo-events/common"
	suuid "github.com/satori/go.uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
	zlog "github.com/rs/zerolog"
	"k8s.io/client-go/kubernetes"
	"encoding/json"
	"bytes"
	"os"
	sv1alpha "github.com/argoproj/argo-events/pkg/apis/sensor/v1alpha1"
)

type EventConfig struct {
	// EventType is type of the event
	EventType string

	// EventTypeVersion is the version of the `eventType`
	EventTypeVersion string

	// Source of the event
	EventSource string

	// Sensors to dispatch the event to
	Sensors []string
}

type tOperationCtx struct {
	// Namespace is namespace where gateway-controller is deployed
	Namespace string

	// Event logger
	log zlog.Logger

	// Event configuration
	Config EventConfig

	// Kubernetes clientset
	kubeClientset kubernetes.Interface
}

func NewTransformOperationContext(name string, namespace string, clientset kubernetes.Interface) *tOperationCtx {
	return &tOperationCtx{
		Namespace:     namespace,
		kubeClientset: clientset,
		log:           zlog.New(os.Stdout).With().Str("gateway-controller-name", name).Logger(),
	}
}

// Transform request transforms http request payload into CloudEvent
func (toc *tOperationCtx) transform(r *http.Request) (*sv1alpha.Event, error) {
	// Generate an event id
	eventId := suuid.Must(suuid.NewV4(), nil)
	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Errorf("failed to parse request payload. Err %+v", err)
		return nil, err
	}

	// Create an CloudEvent
	ce := &sv1alpha.Event{
		Context: sv1alpha.EventContext{
			CloudEventsVersion: common.CloudEventsVersion,
			EventID:            fmt.Sprintf("%x", eventId),
			ContentType:        r.Header.Get(common.HeaderContentType),
			EventTime:          metav1.Time{Time: time.Now().UTC()},
			EventType:          toc.Config.EventType,
			EventTypeVersion:   toc.Config.EventTypeVersion,
			Source:             &sv1alpha.URI{
									Host:   toc.Config.EventSource,
								},
		},
		Payload: payload,
	}

	toc.log.Info().Msg("request transformed")
	return ce, nil
}

// dispatches the event to configured sensor
func (toc *tOperationCtx) dispatchTransformedEvent(ce *sv1alpha.Event) error {
	for _, sensor := range toc.Config.Sensors {
		sensorService, err := toc.kubeClientset.CoreV1().Services(toc.Namespace).Get(sensor + "-svc", metav1.GetOptions{})
		if err != nil {
			toc.log.Error().Str("sensor-svc", sensor).Err(err).Msg("failed to connect to sensor service")
			return err
		}

		if sensorService.Spec.ClusterIP == "" {
			toc.log.Error().Str("sensor-service", sensor).Err(err).Msg("failed to connect to sensor service")
			return err
		}
		toc.log.Info().Str("sensor-service-ip", sensorService.Spec.ClusterIP).Msg("sensor service ip")

		eventBytes, err := json.Marshal(ce)
		if err != nil {
			toc.log.Error().Err(err).Msg("failed to get event bytes")
			return err
		}
		toc.log.Info().Interface("event bytes", eventBytes).Msg("event bytes")
		toc.log.Info().Msg("sending event to sensor")
		req, err := http.NewRequest("POST", fmt.Sprintf("http://%s:%d", sensorService.Spec.ClusterIP, common.SensorServicePort), bytes.NewBuffer(eventBytes))
		if err != nil {
			toc.log.Error().Msg("unable to create a http request")
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		client := &http.Client{}
		_, err = client.Do(req)
		if err != nil {
			return err
		}
	}
	return nil
}

// transforms the event into cloudevent
func (toc *tOperationCtx) HandleTransformRequest(w http.ResponseWriter, r *http.Request) {
	toc.log.Info().Msg("transforming incoming request")
	ce, err := toc.transform(r)
	if err != nil {
		toc.log.Error().Err(err).Msg("failed to transform user event into CloudEvent")
		return
	}
	err = toc.dispatchTransformedEvent(ce)
	if err != nil {
		toc.log.Error().Err(err).Msg("failed to send cloud event to sensor")
	}
}