package main

import (
	"flag"
	"time"

	"github.com/golang/glog"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	// Uncomment the following line to load the gcp plugin (only required to authenticate against GKE clusters).
	// _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"k8s.io/kube-fledged/cmd/app"
	clientset "k8s.io/kube-fledged/pkg/client/clientset/versioned"
	informers "k8s.io/kube-fledged/pkg/client/informers/externalversions"
	"k8s.io/kube-fledged/pkg/signals"
)

var (
	masterURL    string
	kubeconfig   string
	backoffLimit int
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
		fledgedInformerFactory.Fledged().V1alpha1().ImageCaches())

	go kubeInformerFactory.Start(stopCh)
	go fledgedInformerFactory.Start(stopCh)

	if err = controller.Run(2, stopCh); err != nil {
		glog.Fatalf("Error running controller: %s", err.Error())
	}
}

func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig.")
	flag.IntVar(&backoffLimit, "backoff-limit", 6, "Backoff limit refers to number of retries when image pull fails.")
}
