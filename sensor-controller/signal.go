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
	sv1alpha "github.com/argoproj/argo-events/pkg/apis/sensor/v1alpha1"
)

func (sc *sensorCtx) processSignal(name string, event *sv1alpha.Event) {
	sc.log.Info().Str("signal-name", name).Msg("processing the signal")
	node := getNodeByName(sc.sensor, name)
	node.LatestEvent = &v1alpha1.EventWrapper{
		Event: *event,
		Seen: true,
	}
	node.Message = "signal processing completed"
	node.Phase = v1alpha1.NodePhaseComplete
	sc.sensor.Status.Nodes[node.ID] = *node
}
