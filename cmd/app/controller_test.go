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

package app

import (
	"testing"
	"time"

	fakefledgedclientset "github.com/senthilrch/kube-fledged/pkg/client/clientset/versioned/fake"
	fledgedinformers "github.com/senthilrch/kube-fledged/pkg/client/informers/externalversions"
	"k8s.io/apimachinery/pkg/runtime"
	kubeinformers "k8s.io/client-go/informers"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
)

//const controllerAgentName = "fledged"
//const fledgedNameSpace = "kube-fledged"
//const fledgedFinalizer = "fledged"
const noResync time.Duration = time.Second * 0
const imageCacheRefreshFrequency time.Duration = time.Second * 0
const imagePullDeadlineDuration time.Duration = time.Second * 5

func TestNewController(t *testing.T) {
	fakekubeclientset := fakeclientset.NewSimpleClientset([]runtime.Object{}...)
	fakefledgedclientset := fakefledgedclientset.NewSimpleClientset([]runtime.Object{}...)
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(fakekubeclientset, noResync)
	fledgedInformerFactory := fledgedinformers.NewSharedInformerFactory(fakefledgedclientset, noResync)

	controller := NewController(fakekubeclientset, fakefledgedclientset,
		kubeInformerFactory.Core().V1().Nodes(),
		fledgedInformerFactory.Fledged().V1alpha1().ImageCaches(),
		imageCacheRefreshFrequency, imagePullDeadlineDuration)
	t.Logf("New controller created successfully: %v", controller)

	//stopCh := make(chan struct{})
	//if err := controller.Run(1, stopCh); err != nil {
	//	t.Errorf("Error running controller: %s", err.Error())
	//}
}
