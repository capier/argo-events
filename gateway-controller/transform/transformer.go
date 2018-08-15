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

type GatewayConfig struct {
	// EventType is type of the event
	EventType string

	// EventTypeVersion is the version of the `eventType`
	EventTypeVersion string

	// Source
	Source string

	// Sensor
	Sensor string
}

type tOperationCtx struct {
	// Namespace is namespace where gateway-controller is deployed
	Namespace string

	// Event logger
	log zlog.Logger

	// Event configuration
	Config GatewayConfig

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
	eventId := suuid.Must(suuid.NewV4())
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
									Host:   toc.Config.Source,
								},
		},
		Payload: payload,
	}

	return ce, nil
}

func (toc *tOperationCtx) dispatchTransformedEvent(ce *sv1alpha.Event) error {
	sensorService, err := toc.kubeClientset.CoreV1().Services(toc.Namespace).Get(toc.Config.Sensor, metav1.GetOptions{})
	if err != nil {
		toc.log.Error().Str("sensor-svc", toc.Config.Sensor).Err(err).Msg("failed to connect to sensor service")
		return err
	}
	if sensorService.Spec.ClusterIP == "" {
		toc.log.Error().Str("sensor-service", toc.Config.Sensor).Err(err).Msg("failed to connect to sensor service")
		return err
	}
	toc.log.Debug().Str("sensor-service-ip", sensorService.Spec.ClusterIP).Msg("sensor service ip")
	eventBytes, err := json.Marshal(ce)
	if err != nil {
		toc.log.Error().Err(err).Msg("failed to get event bytes")
		return err
	}
	_, err = http.Post(sensorService.Spec.ClusterIP, "application/json", bytes.NewReader(eventBytes))
	if err != nil {
		toc.log.Error().Err(err).Msg("failed to dispatch event to the sensor")
		return err
	}
	return nil
}

func (toc *tOperationCtx) HandleTransformRequest(w http.ResponseWriter, r *http.Request) {
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