/*
Copyright 2018 BlackRock, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"net/http"
	"os"
	"github.com/argoproj/argo-events/common"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"fmt"
	"encoding/json"
	"log"
)

type webhook struct {
	port string
	endpoint endpoint
	srv *http.Server
	clientset *kubernetes.Clientset
	config   string
	namespace string
	targetPort string
}

type endpointConfig struct {
	endpoints []endpoint `json:"endpoints"`
}

type endpoint struct {
	port int `json:"port"`
	method string `json:"method"`
}

func (w *webhook) configureServer() {
	configmap, err := w.clientset.CoreV1().ConfigMaps(w.namespace).Get(w.config, metav1.GetOptions{})
	if err != nil {
		panic(fmt.Errorf("failed to get webhook configuration. Err: %+v", err))
	}
	w.port = configmap.Data["port"]
	w.addRoutes(configmap.Data)
}

func (w *webhook) resyncConfig() {
	watcher, err := w.clientset.CoreV1().ConfigMaps(w.namespace).Watch(metav1.ListOptions{})
	if err != nil {
		fmt.Errorf("failed to set watcher webhook routes updates. Err: %+v", err)
		return
	}
	for update := range watcher.ResultChan() {
		configmap := update.Object.(*corev1.ConfigMap)
		w.addRoutes(configmap.Data)
	}
}

func (w *webhook) addRoutes(config map[string]string) error {
	endpoints := config["endpoints"]
	var endpointConfig *endpointConfig
	err := json.Unmarshal([]byte(endpoints), endpointConfig)
	if err != nil {
		fmt.Errorf("server endpoint/s not configured correctly. Err %+v", err)
		return err
	}
	for _, endpoint := range endpointConfig.endpoints {
		http.HandleFunc(string(endpoint.port), func(writer http.ResponseWriter, request *http.Request) {
			http.Post(fmt.Sprintf("http://localhost:%s", w.targetPort), "application/octet-stream", request.Body)
		})
	}
	return nil
}

func (w *webhook) startWebhook() {
	log.Fatal(http.ListenAndServe(":" + fmt.Sprintf("%s", w.port), nil))
}

func main() {
	kubeConfig, _ := os.LookupEnv(common.EnvVarKubeConfig)
	restConfig, err := common.GetClientConfig(kubeConfig)
	if err != nil {
		panic(err)
	}
	config, _ := os.LookupEnv(common.GatewayConfigMapEnvVar)
	namespace, _ := os.LookupEnv(common.DefaultGatewayControllerNamespace)
	targetPort, _ := os.LookupEnv(common.TransformerPortEnvVar)

	clientset := kubernetes.NewForConfigOrDie(restConfig)
	w := &webhook{
		clientset: clientset,
		config:  config,
		namespace: namespace,
		targetPort: targetPort,
	}
	w.configureServer()
	go w.resyncConfig()
	w.startWebhook()
}
