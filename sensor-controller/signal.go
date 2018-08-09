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
)

func (soc *sOperationCtx) processSignal(signal v1alpha1.Signal) (*v1alpha1.NodeStatus, error) {
	soc.log.Debugf("evaluating signal '%s'", signal.Name)
	node := soc.getNodeByName(signal.Name)

	if node == nil {
		node = soc.initializeNode(signal.Name, v1alpha1.NodeTypeSignal, v1alpha1.NodePhaseNew)
	}

	if node.Phase == v1alpha1.NodePhaseNew {
		err := soc.watchSignal(&signal)
		if err != nil {
			return nil, err
		}
	}

	// let's check the latest event to see if node has completed?
	if node.LatestEvent != nil {
		if !node.LatestEvent.Seen {
			soc.s.Status.Nodes[node.ID].LatestEvent.Seen = true
			soc.updated = true
		}
		return soc.markNodePhase(signal.Name, v1alpha1.NodePhaseComplete), nil
	}

	return soc.markNodePhase(signal.Name, v1alpha1.NodePhaseActive, "stream established"), nil
}

// waits for signal notification from sensor
func (soc *sOperationCtx) watchSignal(signal *v1alpha1.Signal) error {
	<- soc.controller.sensorChs[signal.Name]
	return nil
}
