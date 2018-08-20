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

package sensor_controller

import (
	"github.com/argoproj/argo-events/pkg/apis/sensor/v1alpha1"
	v1alpha "github.com/argoproj/argo-events/pkg/apis/sensor/v1alpha1"
	"fmt"
	"strings"
	"encoding/json"
	"github.com/ghodss/yaml"
	"time"
	log "github.com/rs/zerolog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// various supported media types
// TODO: add support for XML
const (
	MediaTypeJSON string = "application/json"
	//MediaTypeXML  string = "application/xml"
	MediaTypeYAML string = "application/yaml"
)


// getNodeByName returns a copy of the node from this sensor for the nodename
// for signals this nodename should be the name of the signal
func getNodeByName(sensor *v1alpha1.Sensor, nodename string) *v1alpha1.NodeStatus {
	nodeID := sensor.NodeID(nodename)
	node, ok := sensor.Status.Nodes[nodeID]
	if !ok {
		return nil
	}
	return node.DeepCopy()
}

// util method to render an event's data as a JSON []byte
// json is a subset of yaml so this should work...
func renderEventDataAsJSON(e *v1alpha.Event) ([]byte, error) {
	if e == nil {
		return nil, fmt.Errorf("event is nil")
	}
	raw := e.Payload
	// contentType is formatted as: '{type}; charset="xxx"'
	contents := strings.Split(e.Context.ContentType, ";")
	switch contents[0] {
	case MediaTypeJSON:
		if isJSON(raw) {
			return raw, nil
		}
		return nil, fmt.Errorf("event data is not valid JSON")
	case MediaTypeYAML:
		data, err := yaml.YAMLToJSON(raw)
		if err != nil {
			return nil, fmt.Errorf("failed converting yaml event data to JSON: %s", err)
		}
		return data, nil
	default:
		return nil, fmt.Errorf("unsupported event content type: %s", e.Context.ContentType)
	}
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func isJSON(b []byte) bool {
	var js json.RawMessage
	return json.Unmarshal(b, &js) == nil
}


// mark the node with a phase, retuns the node
func markNodePhase(sensor *v1alpha1.Sensor, nodeName string, phase v1alpha1.NodePhase, log log.Logger, message ...string) *v1alpha1.NodeStatus {
	node := getNodeByName(sensor, nodeName)
	if node == nil {
		log.Panic().Str("node-name" , nodeName).Msg("node is uninitialized")
	}
	if node.Phase != phase {
		log.Info().Str("type", string(node.Type)).Str("node-name", node.Name).Str("phase", string(node.Phase))
		node.Phase = phase
	}
	if len(message) > 0 && node.Message != message[0] {
		log.Info().Str("type", string(node.Type)).Str("node-name", node.Name).Str("phase", string(node.Phase)).Str("message", message[0])
		node.Message = message[0]
	}
	if node.IsComplete() && node.CompletedAt.IsZero() {
		node.CompletedAt = metav1.Time{Time: time.Now().UTC()}
		log.Info().Str("type", string(node.Type)).Str("node-name", node.Name).Msg("completed")
	}
	sensor.Status.Nodes[node.ID] = *node
	return node
}