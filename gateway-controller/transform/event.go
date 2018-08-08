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

package transform

import (
	zlog "github.com/rs/zerolog"
	"k8s.io/client-go/kubernetes"
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
	// Namespace is namespace where gateway-controller is deployed
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
		log:           zlog.New(os.Stdout).With().Str("gateway-controller-name", name).Logger(),
	}
}
