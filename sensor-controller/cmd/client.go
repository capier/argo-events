package main

import (
	"os"
	"github.com/argoproj/argo-events/common"
	sv1 "github.com/argoproj/argo-events/pkg/sensor-client/clientset/versioned/typed/sensor/v1alpha1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"fmt"
	"net/http"
)

var kubeConfig string

func getClientConfig(kubeConfig string) (*rest.Config, error) {
	if kubeConfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeConfig)
	}
	return rest.InClusterConfig()
}

func handleSignalNotification(w http.ResponseWriter, r *http.Request) {

}

func main() {
	sensorName, _ := os.LookupEnv(common.SensorName)
	if sensorName == "" {
		panic("sensor name is not provided")
	}

	config, err := getClientConfig(kubeConfig)
	if err != nil {
		panic(fmt.Sprintf("failed to get client configuration. Err: %+v", err))
	}

	sensorClient, err := sv1.NewForConfig(config)
	if err != nil {
		panic(fmt.Sprintf("failed to get sensor client. Err: %+v", err))
	}

	http.HandleFunc("/", handleSignalNotification)


}