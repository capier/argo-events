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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"fmt"
	"log"
)

type webhook struct {
	srv *http.Server
	clientset *kubernetes.Clientset
	config   string
	namespace string
	targetPort string
}

func main() {
	kubeConfig, _ := os.LookupEnv(common.EnvVarKubeConfig)
	restConfig, err := common.GetClientConfig(kubeConfig)
	if err != nil {
		panic(err)
	}
	config := "webhook-gateway-configmap"
	namespace := common.DefaultGatewayControllerNamespace
	targetPort, _ := os.LookupEnv(common.TransformerPortEnvVar)

	clientset := kubernetes.NewForConfigOrDie(restConfig)
	w := &webhook{
		clientset: clientset,
		config:  config,
		namespace: namespace,
		targetPort: targetPort,
	}
	configmap, err := w.clientset.CoreV1().ConfigMaps(w.namespace).Get(w.config, metav1.GetOptions{})
	if err != nil {
		panic(fmt.Errorf("failed to get webhook configuration. Err: %+v", err))
	}
	port := configmap.Data["port"]
	url := configmap.Data["endpointURL"]
	method := configmap.Data["method"]
	log.Printf("port %s, url %s, method %s", port, url, method)
	http.HandleFunc(url, func(writer http.ResponseWriter, request *http.Request) {
		log.Println(request.Method)
		if request.Method == method {
			log.Printf("recieved a request, forwarding it to http://localhost:%s", w.targetPort)
			http.Post(fmt.Sprintf("http://localhost:%s", w.targetPort), "application/octet-stream", request.Body)
		} else {
			fmt.Errorf("http method is not supported")
		}
	})
	log.Println(fmt.Sprintf("server is going to start listening port %s", port))
	log.Fatal(http.ListenAndServe(":" + fmt.Sprintf("%s", port), nil))
}
