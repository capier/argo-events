package main

import (
	"os"
	"github.com/argoproj/argo-events/common"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"github.com/argoproj/argo-events/gateway-controller/transform"
	"net/http"
	"fmt"
	"golang.org/x/net/context"
	"log"
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
		log.Fatalf("%+v", err)
	}
	kubeClient := kubernetes.NewForConfigOrDie(restConfig)

	// name of the gateway
	name, _ := os.LookupEnv(common.GatewayName)
	if name == "" {
		panic("No name provided.")
	}

	// namespace where gateway is deployed
	namespace, _ := os.LookupEnv(common.EnvVarNamespace)
	if namespace == "" {
		panic("No namespace provided.")
	}

	configmap, _ := os.LookupEnv(common.GatewayEnvVarConfigMap)
	if configmap == "" {
		panic("No configmap provided")
	}

	// Create an operation context
	eoc := transform.NewEventOperationContext(name, namespace, kubeClient)
	eoc.Config.Source = name

	ctx := context.Background()
	_, err = eoc.WatchEventConfigMap(ctx, configmap)
	if err != nil {
		log.Fatalf("failed to register watch for store config map: %+v", err)
	}

	http.HandleFunc("/foo", eoc.HandleTransformRequest)
	log.Fatal(http.ListenAndServe(":" + fmt.Sprintf("%s", common.Port), nil))
}
