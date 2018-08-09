package v1alpha1

import "k8s.io/apimachinery/pkg/apis/meta/v1"

// Gateway is the definition of a gateway-controller resource
// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Gateway struct {
	v1.TypeMeta   `json:",inline"`
	v1.ObjectMeta `json:"metadata" protobuf:"bytes,1,opt,name=metadata"`
	Status        NodePhase   `json:"type" protobuf:"bytes,2,opt,name=type"`
	Spec          GatewaySpec `json:"spec" protobuf:"bytes,3,opt,name=spec"`
}

// GatewayList is the list of Gateway resources
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type GatewayList struct {
	v1.TypeMeta `json:",inline"`
	v1.ListMeta `json:"metadata" protobuf:"bytes,1,opt,name=metadata"`
	Items       []Gateway `json:"items" protobuf:"bytes,2,opt,name=items"`
}

// GatewaySpec represents gateway-controller specifications
type GatewaySpec struct {
	Image     string `json:"image" protobuf:"bytes,1,opt,name=image"`
	Command   string `json:"command" protobuf:"bytes,2,opt,name=command"`
	ConfigMap string `json:"config_map" protobuf:"bytes,3,opt,name=configmap"`
	Secret    string `json:"secret" protobuf:"bytes,4,opt,name=secret"`
	Type      string `json:"type" protobuf:"bytes,5,opt,name=type"`
	Service   Service   `json:"exposed" protobuf:"bytes,6,opt,name=service"`
	Sensor    string `json:"sensor" protobuf:"bytes,7,opt,name=sensor"`
	ServiceAccountName string `json:"service_account_name" protobuf:"bytes,8,opt,name=service_account_name"`
}

// NodePhase is the label for the condition of a node
type NodePhase string

// possible types of node phases
const (
	NodePhaseRunning NodePhase = "Running" // the node is running
	NodePhaseError   NodePhase = "Error"   // the node has encountered an error in processing
	NodePhaseNew     NodePhase = ""        // the node is new
)

// SensorStatus contains information about the status of a gateway-controller.
type GatewayStatus struct {
	// Phase is the high-level summary of the gateway-controller
	Phase NodePhase `json:"phase" protobuf:"bytes,1,opt,name=phase"`

	// StartedAt is the time at which this gateway-controller was initiated
	StartedAt v1.Time `json:"startedAt,omitempty" protobuf:"bytes,2,opt,name=startedAt"`

	// CompletedAt is the time at which this gateway-controller was completed
	CompletedAt v1.Time `json:"completedAt,omitempty" protobuf:"bytes,3,opt,name=completedAt"`

	// Message is a human readable string indicating details about a gateway-controller in its phase
	Message string `json:"message,omitempty" protobuf:"bytes,4,opt,name=message"`
}

// Service exposed gateway to outside cluster or in cluster components depending on it's type.
type Service struct {
	Type string `json:"type" protobuf:"bytes,1,opt,name=type"`

	Port int32 `json:"port" protobuf:"bytes,2,opt,name=port"`

	TargetPort int `json:"target_port" protobuf:"bytes,3,opt,name=target_port"`
}