/*
Copyright 2018 The kube-fledged authors.

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
	"flag"
	"time"

	"github.com/golang/glog"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd" // Uncomment the following line to load the gcp plugin (only required to authenticate against GKE clusters).

	// _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"github.com/senthilrch/kube-fledged/cmd/app"
	clientset "github.com/senthilrch/kube-fledged/pkg/client/clientset/versioned"
	informers "github.com/senthilrch/kube-fledged/pkg/client/informers/externalversions"
	"github.com/senthilrch/kube-fledged/pkg/signals"
)

var (
	masterURL                  string
	kubeconfig                 string
	imageCacheRefreshFrequency time.Duration
	imagePullDeadlineDuration  time.Duration
	dockerClientImage          string
	imagePullPolicy            string
)

func main() {
	flag.Parse()

	// set up signals so we handle the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		glog.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	fledgedClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building fledged clientset: %s", err.Error())
	}

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Second*30)
	fledgedInformerFactory := informers.NewSharedInformerFactory(fledgedClient, time.Second*30)

	controller := app.NewController(kubeClient, fledgedClient,
		kubeInformerFactory.Core().V1().Nodes(),
		fledgedInformerFactory.Fledged().V1alpha1().ImageCaches(),
		imageCacheRefreshFrequency, imagePullDeadlineDuration, dockerClientImage, imagePullPolicy)

	glog.Info("Starting pre-flight checks")
	if err = controller.PreFlightChecks(); err != nil {
		glog.Fatalf("Error running pre-flight checks: %s", err.Error())
	}
	glog.Info("Pre-flight checks completed")

	go kubeInformerFactory.Start(stopCh)
	go fledgedInformerFactory.Start(stopCh)

	if err = controller.Run(1, stopCh); err != nil {
		glog.Fatalf("Error running controller: %s", err.Error())
	}
}

func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig.")
	flag.DurationVar(&imagePullDeadlineDuration, "image-pull-deadline-duration", time.Minute*5, "Maximum duration allowed for pulling an image. After this duration, image pull is considered to have failed")
	flag.DurationVar(&imageCacheRefreshFrequency, "image-cache-refresh-frequency", time.Minute*15, "The image cache is refreshed periodically to ensure the cache is up to date. Setting this flag to 0s will disable refresh")
	flag.StringVar(&dockerClientImage, "docker-client-image", "senthilrch/fledged-docker-client:latest", "The image name of the docker client. the docker client is used when deleting images during purging the cache")
	flag.StringVar(&imagePullPolicy, "image-pull-policy", "", "Image pull policy default value is IfNotPresent. Possible values are IfNotPresent and Always")	
        flag.StringVar(&imagePullPolicy, "image-pull-policy", "", " Image pull policy for pulling images into the cache. Possible values are 'IfNotPresent' and 'Always'. Default value is 'IfNotPresent'. Default value for Images with ':latest' tag is 'Always'")
}
