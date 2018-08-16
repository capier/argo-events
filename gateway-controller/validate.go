package gateway_controller

import "fmt"

func (gwc *gwOperationCtx) validate() error {
	if gwc.gw.Spec.Image == "" {
		return fmt.Errorf("gateway image is not specified")
	}
	if gwc.gw.Spec.Type == "" {
		return fmt.Errorf("gateway type is not specified")
	}
	if len(gwc.gw.Spec.Sensors) <= 0 {
		return fmt.Errorf("no associated sensor with gateway")
	}
	if gwc.gw.Spec.ServiceAccountName == "" {
		return fmt.Errorf("no service account specified")
	}
	return nil
}