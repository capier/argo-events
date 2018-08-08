/*
Copyright 2018 BlackRock, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"fmt"
	"github.com/argoproj/argo-events/common"
	zlog "github.com/rs/zerolog"
	suuid "github.com/satori/go.uuid"
	"io/ioutil"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"net/http"
	"os"
	"time"
)

type EventConfig struct {
	// EventType is type of the event
	EventType string

	// EventTypeVersion is the version of the `eventType`
	EventTypeVersion string
}

type eOperationCtx struct {
	// Namespace is namespace where gateway is deployed
	Namespace string

	// Event logger
	log zlog.Logger

	// Event configuration
	Config EventConfig

	// Kubernetes clientset
	kubeClientset kubernetes.Interface
}

// Event contains context about event and payload
type Event struct {
	ctx     EventContext
	payload []byte
}

// EventContext contains standard metadata about an event.
// See https://github.com/cloudevents/spec/blob/v0.1/spec.md#context-attributes
type EventContext struct {
	// The version of the CloudEvents specification used by the event.
	CloudEventsVersion string `json:"cloudEventsVersion,omitempty"`
	// ID of the event; must be non-empty and unique within the scope of the producer.
	EventID string `json:"eventID"`
	// Timestamp when the event happened.
	EventTime time.Time `json:"eventTime,omitempty"`
	// Type of occurrence which has happened.
	EventType string `json:"eventType"`
	// The version of the `eventType`; this is producer-specific.
	EventTypeVersion string `json:"eventTypeVersion,omitempty"`
	// A link to the schema that the `data` attribute adheres to.
	SchemaURL string `json:"schemaURL,omitempty"`
	// A MIME (RFC 2046) string describing the media type of `data`.
	ContentType string `json:"contentType,omitempty"`
	// A URI describing the event producer.
	Source string `json:"source"`
	// Additional metadata without a well-defined structure.
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

func NewEventOperationContext(name string, namespace string, clientset kubernetes.Interface) *eOperationCtx {
	return &eOperationCtx{
		Namespace:     namespace,
		kubeClientset: clientset,
		log:           zlog.New(os.Stdout).With().Str("gateway-name", name).Logger(),
	}
}

// Transform request transforms http request payload into CloudEvent
func (eoc *eOperationCtx) TransformRequest(source string, eventType string, eventTypeVersion string, r *http.Request) (*Event, error) {
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
			EventType:          eventType,
			EventTypeVersion:   eventTypeVersion,
			Source:             source,
		},
		payload: payload,
	}

	return ce, nil
}

func (e *eOperationCtx) WatchEventConfigMap(ctx context.Context, name string) (cache.Controller, error) {
	source := e.newStoreConfigMapWatch(name)
	_, controller := cache.NewInformer(
		source,
		&apiv1.ConfigMap{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				if cm, ok := obj.(*apiv1.ConfigMap); ok {
					e.log.Info().Str("config-map", name).Msg("detected ConfigMap update. Updating the controller config.")
					err := e.updateConfig(cm)
					if err != nil {
						e.log.Error().Err(err).Msg("update of config failed")
					}
				}
			},
			UpdateFunc: func(old, new interface{}) {
				if newCm, ok := new.(*apiv1.ConfigMap); ok {
					e.log.Info().Msg("detected ConfigMap update. Updating the controller config.")
					err := e.updateConfig(newCm)
					if err != nil {
						e.log.Error().Err(err).Msg("update of config failed")
					}
				}
			},
		})

	go controller.Run(ctx.Done())
	return controller, nil
}

func (e *eOperationCtx) newStoreConfigMapWatch(name string) *cache.ListWatch {
	x := e.kubeClientset.CoreV1().RESTClient()
	resource := "configmaps"
	fieldSelector := fields.ParseSelectorOrDie(fmt.Sprintf("metadata.name=%s", name))

	listFunc := func(options metav1.ListOptions) (runtime.Object, error) {
		options.FieldSelector = fieldSelector.String()
		req := x.Get().
			Namespace(e.Namespace).
			Resource(resource).
			VersionedParams(&options, metav1.ParameterCodec)
		return req.Do().Get()
	}
	watchFunc := func(options metav1.ListOptions) (watch.Interface, error) {
		options.Watch = true
		options.FieldSelector = fieldSelector.String()
		req := x.Get().
			Namespace(e.Namespace).
			Resource(resource).
			VersionedParams(&options, metav1.ParameterCodec)
		return req.Watch()
	}
	return &cache.ListWatch{ListFunc: listFunc, WatchFunc: watchFunc}
}

func (e *eOperationCtx) updateConfig(cm *apiv1.ConfigMap) error {
	eventType, ok := cm.Data[common.EventType]
	if !ok {
		return fmt.Errorf("configMap '%s' does not have key '%s'", cm.Name, common.EventType)
	}
	eventTypeVersion, ok := cm.Data[common.EventTypeVersion]
	if !ok {
		return fmt.Errorf("configMap '%s' does not have key '%s'", cm.Name, common.EventTypeVersion)
	}
	e.Config = EventConfig{
		EventType:        eventType,
		EventTypeVersion: eventTypeVersion,
	}
	return nil
}
