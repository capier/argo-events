package main

import (
	"os"
	"github.com/argoproj/argo-events/common"
	"k8s.io/client-go/kubernetes"
	"github.com/argoproj/argo-events/gateway-controller/transform"
	"net/http"
	"fmt"
	"golang.org/x/net/context"
	"log"
)

func main() {
	kubeConfig, _ := os.LookupEnv(common.EnvVarKubeConfig)
	restConfig, err := common.GetClientConfig(kubeConfig)
	if err != nil {
		panic(err)
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

	configmap, _ := os.LookupEnv(common.GatewayConfigMapEnvVar)
	if configmap == "" {
		panic("No configmap provided")
	}

	// Create an operation context
	eoc := transform.NewTransformOperationContext(name, namespace, kubeClient)
	eoc.Config.Source = name

	ctx := context.Background()
	_, err = eoc.WatchEventConfigMap(ctx, configmap)
	if err != nil {
		log.Fatalf("failed to register watch for store config map: %+v", err)
	}

	http.HandleFunc("/", eoc.HandleTransformRequest)
	log.Fatal(http.ListenAndServe(":" + fmt.Sprintf("%d", common.TransformerPort), nil))
}
