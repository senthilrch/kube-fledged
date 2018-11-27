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

// noResyncPeriodFunc returns 0 for resyncPeriod in case resyncing is not needed.
func noResyncPeriodFunc() time.Duration {
	return 0
}

func newTestController(kubeclientset kubernetes.Interface, fledgedclientset clientset.Interface) *Controller {
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeclientset, noResyncPeriodFunc())
	fledgedInformerFactory := fledgedinformers.NewSharedInformerFactory(fledgedclientset, noResyncPeriodFunc())
	kubeInformer := kubeInformerFactory.Core().V1().Nodes()
	fledgedInformer := fledgedInformerFactory.Fledged().V1alpha1().ImageCaches()
	imageCacheRefreshFrequency := time.Second * 0
	imagePullDeadlineDuration := time.Second * 5
	dockerClientImage := "senthilrch/fledged-docker-client:latest"
	imagePullPolicy := "IfNotPresent"

	/* 	startInformers := true
	   	if startInformers {
	   		stopCh := make(chan struct{})
	   		defer close(stopCh)
	   		kubeInformerFactory.Start(stopCh)
	   		fledgedInformerFactory.Start(stopCh)
	   	} */

	controller := NewController(kubeclientset, fledgedclientset, kubeInformer, fledgedInformer,
		imageCacheRefreshFrequency, imagePullDeadlineDuration, dockerClientImage, imagePullPolicy)
	controller.nodesSynced = func() bool { return true }
	return controller
}

func TestPreFlightChecks(t *testing.T) {
	var fakekubeclientset *fakeclientset.Clientset
	var fakefledgedclientset *fledgedclientsetfake.Clientset
	joblist := &batchv1.JobList{}
	joblist.Items = []batchv1.Job{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo",
			},
		},
	}

	//Test #1: No dangling jobs
	fakekubeclientset = &fakeclientset.Clientset{}
	fakefledgedclientset = &fledgedclientsetfake.Clientset{}
	controller := newTestController(fakekubeclientset, fakefledgedclientset)
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

	controller = newTestController(fakekubeclientset, fakefledgedclientset)
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

	controller = newTestController(fakekubeclientset, fakefledgedclientset)
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

	controller = newTestController(fakekubeclientset, fakefledgedclientset)
	err = controller.PreFlightChecks()
	if err == nil {
		t.Errorf("TestPreFlightChecks failed: %s", fmt.Errorf("error"))
	}
}
