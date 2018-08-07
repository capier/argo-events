package gateway_controller

import (
	"github.com/argoproj/argo-events/pkg/apis/gateway/v1alpha1"
	argov1 "github.com/argoproj/argo/pkg/apis/workflow/v1alpha1"
	zlog "github.com/rs/zerolog"
	"os"
)

// the context of an operation on a gateway.
// the gateway-controller creates this context each time it picks a Gateway off its queue.
type gwOperationCtx struct {
	// gw is the gateway object
	gw *v1alpha1.Gateway

	// updated indicates whether the gateway object was updated and needs to be persisted back to k8
	updated bool

	// log is the logger for a gateway
	log zlog.Logger

	// reference to the gateway-controller
	controller *GatewayController
}

// newGatewayOperationCtx creates and initializes a new gOperationCtx object
func newGatewayOperationCtx(gw *v1alpha1.Gateway, controller *GatewayController) *gwOperationCtx {
	return &gwOperationCtx{
		gw:       gw.DeepCopy(),
		updated: false,
		log: zlog.New(os.Stdout).With().Str("name", gw.Name).Str("namespace", gw.Namespace).Logger(),
		controller: controller,
	}
}

func (gwc *gwOperationCtx) operate() error {
	gwc.log.Info().Str("name", gwc.gw.Name).Msg("Operating on gateway")

	// operate on gateway only if it in new state

	switch gwc.gw.Status {
	case v1alpha1.NodePhaseNew:
		// Create two step argo workflow. Step 1 will run user specific code
		// and output to stdout. This output will be feed to Step 2.
		// Step 2 performs converting Step 1 output data into CloudEvents format.
		// See CloudEvents specs: https://github.com/cloudevents/spec
	case v1alpha1.NodePhaseError:

	}

	if gwc.gw.Status == v1alpha1.NodePhaseNew {

	} else {
		gwc.log.Warn().Str("name", gwc.gw.Name).Msg("Gateway is already running")
	}
}

func (gwc *gwOperationCtx) createGatewayWorkflow() *argov1.Workflow {

}