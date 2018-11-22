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
	"errors"
	"fmt"
	"testing"
	"time"

	clientset "github.com/senthilrch/kube-fledged/pkg/client/clientset/versioned"
	fledgedclientsetfake "github.com/senthilrch/kube-fledged/pkg/client/clientset/versioned/fake"
	fledgedinformers "github.com/senthilrch/kube-fledged/pkg/client/informers/externalversions"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
)

//const controllerAgentName = "fledged"
//const fledgedNameSpace = "kube-fledged"
//const fledgedFinalizer = "fledged"
const noResync time.Duration = time.Second * 0
const imageCacheRefreshFrequency time.Duration = time.Second * 0
const imagePullDeadlineDuration time.Duration = time.Second * 5
const dockerClientImage = "senthilrch/fledged-docker-client:latest"

//var alwaysReady = func() bool { return true }

func newController(kubeclientset kubernetes.Interface, fledgedclientset clientset.Interface) *Controller {
	//fakekubeclientset := fakeclientset.NewSimpleClientset([]runtime.Object{}...)
	//fakefledgedclientset := fakefledgedclientset.NewSimpleClientset([]runtime.Object{}...)
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeclientset, noResync)
	fledgedInformerFactory := fledgedinformers.NewSharedInformerFactory(fledgedclientset, noResync)

	controller := NewController(kubeclientset, fledgedclientset,
		kubeInformerFactory.Core().V1().Nodes(),
		fledgedInformerFactory.Fledged().V1alpha1().ImageCaches(),
		imageCacheRefreshFrequency, imagePullDeadlineDuration, dockerClientImage)

	return controller
}

func TestPreFlightChecks(t *testing.T) {
	var fakekubeclientset *fakeclientset.Clientset
	var fakefledgedclientset *fledgedclientsetfake.Clientset
	joblist := &batchv1.JobList{}
	joblist.Items = []batchv1.Job{
		batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo",
			},
		},
	}

	//Test #1: No dangling jobs
	fakekubeclientset = &fakeclientset.Clientset{}
	fakefledgedclientset = &fledgedclientsetfake.Clientset{}
	controller := newController(fakekubeclientset, fakefledgedclientset)
	err := controller.PreFlightChecks()
	if err != nil {
		t.Errorf("TestPreFlightChecks failed: %s", err.Error())
	}

	//Test #2: 1 dangling job. successful list. successful delete
	fakekubeclientset = &fakeclientset.Clientset{}
	fakefledgedclientset = &fledgedclientsetfake.Clientset{}
	fakekubeclientset.AddReactor("list", "jobs", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		return true, joblist, nil
	})
	fakekubeclientset.AddReactor("delete", "jobs", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, nil
	})

	controller = newController(fakekubeclientset, fakefledgedclientset)
	err = controller.PreFlightChecks()
	if err != nil {
		t.Errorf("TestPreFlightChecks failed: %s", err.Error())
	}

	//Test #3: unsuccessful list
	fakekubeclientset = &fakeclientset.Clientset{}
	fakefledgedclientset = &fledgedclientsetfake.Clientset{}
	fakekubeclientset.AddReactor("list", "jobs", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, apierrors.NewInternalError(errors.New("API server down"))
	})

	controller = newController(fakekubeclientset, fakefledgedclientset)
	err = controller.PreFlightChecks()
	if err == nil {
		t.Errorf("TestPreFlightChecks failed: %s", fmt.Errorf("error"))
	}

	//Test #4: 1 dangling job. successful list. unsuccessful delete
	fakekubeclientset = &fakeclientset.Clientset{}
	fakefledgedclientset = &fledgedclientsetfake.Clientset{}
	fakekubeclientset.AddReactor("list", "jobs", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		return true, joblist, nil
	})

	fakekubeclientset.AddReactor("delete", "jobs", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, apierrors.NewInternalError(errors.New("API server down"))
	})

	controller = newController(fakekubeclientset, fakefledgedclientset)
	err = controller.PreFlightChecks()
	if err == nil {
		t.Errorf("TestPreFlightChecks failed: %s", fmt.Errorf("error"))
	}
}
