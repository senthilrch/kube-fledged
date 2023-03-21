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
	"os"
	"strings"
	"time"

	"github.com/golang/glog"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	// Uncomment the following line to load the gcp plugin (only required to authenticate against GKE clusters).
	// _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"github.com/senthilrch/kube-fledged/cmd/controller/app"
	clientset "github.com/senthilrch/kube-fledged/pkg/client/clientset/versioned"
	informers "github.com/senthilrch/kube-fledged/pkg/client/informers/externalversions"
	"github.com/senthilrch/kube-fledged/pkg/signals"
)

var (
	imageCacheRefreshFrequency time.Duration
	imagePullDeadlineDuration  time.Duration
	criClientImage             string
	busyboxImage               string
	imagePullPolicy            string
	fledgedNameSpace           string
	serviceAccountName         string
	imageDeleteJobHostNetwork  bool
	jobPriorityClassName       string
	kubeconfig                 string
	masterURL                  string
	//Default value for when `--job-retention-policy` flag is not set
	canDeleteJob  bool = true
	criSocketPath string
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

	controller := app.NewController(kubeClient, fledgedClient, fledgedNameSpace,
		kubeInformerFactory.Core().V1().Nodes(),
		fledgedInformerFactory.Kubefledged().V1alpha2().ImageCaches(),
		imageCacheRefreshFrequency, imagePullDeadlineDuration, criClientImage,
		busyboxImage, imagePullPolicy, serviceAccountName, imageDeleteJobHostNetwork,
		jobPriorityClassName, canDeleteJob, criSocketPath)

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

	flag.StringVar(&kubeconfig, "kubeconfig", "",
		"Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "",
		"The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")

	flag.DurationVar(&imagePullDeadlineDuration, "image-pull-deadline-duration", time.Minute*5, "Maximum duration allowed for pulling an image. After this duration, image pull is considered to have failed")
	flag.DurationVar(&imageCacheRefreshFrequency, "image-cache-refresh-frequency", time.Minute*15, "The image cache is refreshed periodically to ensure the cache is up to date. Setting this flag to 0s will disable refresh")
	flag.StringVar(&imagePullPolicy, "image-pull-policy", "IfNotPresent", "Image pull policy for pulling images into the cache. Possible values are 'IfNotPresent' and 'Always'. Default value is 'IfNotPresent'. Images with no or ':latest' tag are always pulled")
	if fledgedNameSpace = os.Getenv("KUBEFLEDGED_NAMESPACE"); fledgedNameSpace == "" {
		fledgedNameSpace = "kube-fledged"
	}
	if criClientImage = os.Getenv("KUBEFLEDGED_CRI_CLIENT_IMAGE"); criClientImage == "" {
		criClientImage = "senthilrch/kubefledged-cri-client:latest"
	}
	if busyboxImage = os.Getenv("BUSYBOX_IMAGE"); busyboxImage == "" {
		busyboxImage = "senthilrch/busybox:1.35.0"
	}
	flag.StringVar(&serviceAccountName, "service-account-name", "", "serviceAccountName used in Jobs created for pulling/deleting images. Optional flag. If not specified the default service account of the namespace is used")
	flag.BoolVar(&imageDeleteJobHostNetwork, "image-delete-job-host-network", false, "whether the pod for the image delete job should be run with 'HostNetwork: true'. Default value: false")
	flag.StringVar(&jobPriorityClassName, "job-priority-class-name", "", "priorityClassName of jobs created by kubefledged-controller")
	flag.Func("job-retention-policy", "sets the retention behavior of finished Image Manager Jobs (default: 'delete')",
		func(val string) error {
			const (
				deletePolicy string = "delete"
				retainPolicy string = "retain"
			)
			switch strings.ToLower(strings.TrimSpace(val)) {
			case deletePolicy:
				canDeleteJob = true
				glog.Infof("Using '%s' Job Retention Policy", deletePolicy)
				return nil
			case retainPolicy:
				canDeleteJob = false
				glog.Infof("Using '%s' Job Retention Policy", retainPolicy)
				return nil
			default:
				//canDeleteJob is initialized to true already
				glog.Infof("Failed to set '%s' Job Retention Policy -- invalid input:"+
					" falling back to '%s' Job Retention Policy", val, deletePolicy)
				return nil
			}
		},
	)
	flag.StringVar(&criSocketPath, "cri-socket-path", "", "path to the cri socket on the node e.g. /run/containerd/containerd.sock (default: /var/run/docker.sock, /run/containerd/containerd.sock, /var/run/crio/crio.sock)")
}
