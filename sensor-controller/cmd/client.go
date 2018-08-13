package main

import (
	"fmt"
	"github.com/argoproj/argo-events/common"
	sv1 "github.com/argoproj/argo-events/pkg/sensor-client/clientset/versioned/typed/sensor/v1alpha1"
	pb "github.com/argoproj/argo-events/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"github.com/rs/zerolog"
	sc "github.com/argoproj/argo-events/sensor-controller"
)

var kubeConfig string

func getClientConfig(kubeConfig string) (*rest.Config, error) {
	if kubeConfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeConfig)
	}
	return rest.InClusterConfig()
}
func main() {
	sensorName, _ := os.LookupEnv(common.SensorName)
	if sensorName == "" {
		panic("sensor name is not provided")
	}

	sensorNamespace, _ := os.LookupEnv(common.SensorNamespace)
	if sensorNamespace == "" {
		panic("sensor namespace is not provided")
	}

	serverAddr, _ := os.LookupEnv(common.DefaultSensorControllerDeploymentName)
	if serverAddr == "" {
		panic("sensor controller name not provided")
	}

	config, err := getClientConfig(kubeConfig)
	if err != nil {
		panic(fmt.Sprintf("failed to get client configuration. Err: %+v", err))
	}

	log := zerolog.New(os.Stdout).With().Str("sensor-name", sensorName).Logger()

	conn, err := grpc.Dial(serverAddr, grpc.WithInsecure())
	if err != nil {
		log.Fatal().Err(err).Msg("sensor did not connect to sensor controller")
	}
	defer conn.Close()

	client := pb.NewSensorUpdateClient(conn)
	stream, err := client.UpdateSensor(context.Background())
	if err != nil {
		log.Fatal().Err(err).Msg("failed to get signal notification stream")
	}
	sensorClient, err := sv1.NewForConfig(config)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to get sensor client")
	}
	kubeClient := kubernetes.NewForConfigOrDie(config)

	sensor, err := sensorClient.Sensors(sensorNamespace).Get(sensorName, metav1.GetOptions{})
	if err != nil {
		log.Panic().Err(err).Msg("failed to retrieve sensor")
	}

	sCtx := sc.NewSensorContext(sensorClient, kubeClient, config, &stream, sensor, log)
	sCtx.StartHttpServer()
}
