/*
Copyright 2022 The kube-fledged authors.

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

package e2etest

import (
	"fmt"
	"os"
	"testing"

	kubefledgedv1alpha2 "github.com/senthilrch/kube-fledged/pkg/apis/kubefledged/v1alpha2"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
)

const (
	kindClusterName string = "kube-fledged-e2etest"

	//image names from ENV
	controllerImageEnv    string = "CONTROLLER_IMAGE"
	criClientImageEnv     string = "CRI_CLIENT_IMAGE"
	operatorImageEnv      string = "OPERATOR_IMAGE"
	webhookServerImageEnv string = "WEBHOOK_SERVER_IMAGE"
	imageTagEnv           string = "IMAGE_TAG"

	//default images
	defaultImageRegistry     string = "docker.io"
	controllerImagePrefix    string = defaultImageRegistry + "/" + "senthilrch/kubefledged-controller"
	criClientImagePrefix     string = defaultImageRegistry + "/" + "senthilrch/kubefledged-cri-client"
	operatorImagePrefix      string = defaultImageRegistry + "/" + "senthilrch/kubefledged-operator"
	webhookServerImagePrefix string = defaultImageRegistry + "/" + "senthilrch/kubefledged-webhook-server"

	//directory path for controlplane components' manifests
	yamlDir      string = "../deploy"
	helmChartDir string = "../deploy/kubefledged-operator/helm-charts/kubefledged"
	operatorDir  string = "../deploy/kubefledged-operator/deploy"
)

var (
	testenv               env.Environment
	testNamespace         string = envconf.RandomName("kube-fledged-test", 23)
	controlPlaneNamespace string = envconf.RandomName("kube-fledged", 18)
	operatorNamespace     string = envconf.RandomName("kube-fledged-operator", 27)
	controllerImage       string = ""
	criClientImage        string = ""
	operatorImage         string = ""
	webhookServerImage    string = ""
)

func TestMain(m *testing.M) {
	utilruntime.Must(kubefledgedv1alpha2.AddToScheme(scheme.Scheme))

	//Initialize the image names which have to loaded
	// on to the KinD cluster
	tag, ok := os.LookupEnv(imageTagEnv)
	if !ok {
		fmt.Fprintf(os.Stderr, "%s environment variable is not set."+
			" This environment variable is required to load"+
			" docker images in to the KinD cluster", imageTagEnv)
		os.Exit(1)
	}
	controllerImage = controllerImagePrefix + ":" + tag
	criClientImage = criClientImagePrefix + ":" + tag
	operatorImage = operatorImagePrefix + ":" + tag
	webhookServerImage = webhookServerImagePrefix + ":" + tag

	testenv = env.New()
	testenv.Setup(
		//creates kind cluster
		envfuncs.CreateKindCluster(kindClusterName),
		//creates namespaces for the kube-fledged controller
		// and e2e tests in the above KinD cluster
		envfuncs.CreateNamespace(controlPlaneNamespace),
		envfuncs.CreateNamespace(operatorNamespace),
		envfuncs.CreateNamespace(testNamespace),
		//loads the kube-fledged images into the cluster
		envfuncs.LoadDockerImageToCluster(kindClusterName, controllerImage),
		envfuncs.LoadDockerImageToCluster(kindClusterName, criClientImage),
		envfuncs.LoadDockerImageToCluster(kindClusterName, operatorImage),
		envfuncs.LoadDockerImageToCluster(kindClusterName, webhookServerImage),
	).Finish(
		envfuncs.DeleteNamespace(testNamespace),
		envfuncs.DeleteNamespace(operatorNamespace),
		envfuncs.DeleteNamespace(controlPlaneNamespace),
		envfuncs.DestroyKindCluster(kindClusterName),
	)

	os.Exit(testenv.Run(m))
}
