package main

import (
	"os"
	"github.com/argoproj/argo-events/common"
	"github.com/argoproj/argo-events/gateway-controller"
	"context"
)

func main() {
	// kubernetes configuration
	kubeConfig, _ := os.LookupEnv(common.EnvVarKubeConfig)
	restConfig, err := common.GetClientConfig(kubeConfig)
	if err != nil {
		panic(err)
	}

	// sensor-controller configuration
	configMap, ok := os.LookupEnv(common.EnvVarConfigMap)
	if !ok {
		configMap = common.DefaultConfigMapName(common.DefaultSensorControllerDeploymentName)
	}

	controller := gateway_controller.NewGatewayController(restConfig, configMap)
	err = controller.ResyncConfig()
	if err != nil {
		panic(err)
	}

	go controller.Run(context.Background(), 1, 1)

	// Wait forever
	select {}
}