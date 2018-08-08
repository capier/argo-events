package main

import (
	"os"
	"github.com/argoproj/argo-events/common"
	"fmt"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"github.com/argoproj/argo-events/gateway-controller/transform"
	"net/http"
)

var(
	kubeConfig string
)

func getClientConfig(kubeConfig string) (*rest.Config, error) {
	if kubeConfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeConfig)
	}
	return rest.InClusterConfig()
}

func main() {
	restConfig, err := getClientConfig(kubeConfig)
	if err != nil {
		fmt.Errorf("%+v", err)
		return
	}
	kubeClient := kubernetes.NewForConfigOrDie(restConfig)

	// name of the gateway
	name, _ := os.LookupEnv(common.GatewayName)
	if name == "" {
		panic("No gateway name provided.")
	}

	// namespace where gateway is deployed
	namespace, _ := os.LookupEnv(common.EnvVarNamespace)
	if namespace == "" {
		panic("No gateway namespace provided.")
	}

	configmap, _ := os.LookupEnv(common.GatewayEnvVarConfigMap)
	if configmap == "" {
		panic("No configmap provided for gateway")
	}

	// Create an operation context
	eoc := transform.NewEventOperationContext(name, namespace, kubeClient)
}
