package main

import (
	"os"
	"github.com/argoproj/argo-events/common"
	"k8s.io/client-go/kubernetes"
	"github.com/argoproj/argo-events/gateway-controller/transform"
	"net/http"
	"fmt"
	"log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
	"context"
)

func main() {
	kubeConfig, _ := os.LookupEnv(common.EnvVarKubeConfig)
	restConfig, err := common.GetClientConfig(kubeConfig)
	if err != nil {
		panic(err)
	}
	kubeClient := kubernetes.NewForConfigOrDie(restConfig)

	namespace, _ := os.LookupEnv(common.EnvVarNamespace)
	if namespace == "" {
		panic("No namespace provided")
	}

	configmap, _ := os.LookupEnv(common.GatewayConfigMapEnvVar)
	if configmap == "" {
		panic("No config-map provided.")
	}

	tConfigMap, err := kubeClient.CoreV1().ConfigMaps(namespace).Get(configmap, metav1.GetOptions{})

	if err != nil {
		panic(fmt.Errorf("failed to retrieve config map. Err: %+v", err))
	}

	tConfigMapData := tConfigMap.Data
	// create the configuration for transforming and dispatching the event
	eventConfig := transform.EventConfig{
		Sensors: strings.Split(tConfigMapData[common.SensorList], ","),
		EventSource: tConfigMapData[common.EventSource],
		EventType: tConfigMapData[common.EventType],
		EventTypeVersion: tConfigMapData[common.EventTypeVersion],
	}

	// Create an operation context
	eoc := transform.NewTransformOperationContext(eventConfig.EventSource, namespace, kubeClient)
	ctx := context.Background()
	_, err = eoc.WatchEventConfigMap(ctx, configmap)
	if err != nil {
		log.Fatalf("failed to register watch for store config map: %+v", err)
	}

	http.HandleFunc("/", eoc.HandleTransformRequest)
	log.Fatal(http.ListenAndServe(":" + fmt.Sprintf("%d", common.TransformerPort), nil))
}
