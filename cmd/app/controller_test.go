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
	"fmt"
	"strings"
	"testing"
	"time"

	fledgedv1alpha1 "github.com/senthilrch/kube-fledged/pkg/apis/fledged/v1alpha1"
	clientset "github.com/senthilrch/kube-fledged/pkg/client/clientset/versioned"
	fledgedclientsetfake "github.com/senthilrch/kube-fledged/pkg/client/clientset/versioned/fake"
	informers "github.com/senthilrch/kube-fledged/pkg/client/informers/externalversions"
	fledgedinformers "github.com/senthilrch/kube-fledged/pkg/client/informers/externalversions/fledged/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubeinformers "k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
)

// noResyncPeriodFunc returns 0 for resyncPeriod in case resyncing is not needed.
func noResyncPeriodFunc() time.Duration {
	return 0
}

func newTestController(kubeclientset kubernetes.Interface, fledgedclientset clientset.Interface) (*Controller, coreinformers.NodeInformer, fledgedinformers.ImageCacheInformer) {
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeclientset, noResyncPeriodFunc())
	fledgedInformerFactory := informers.NewSharedInformerFactory(fledgedclientset, noResyncPeriodFunc())
	nodeInformer := kubeInformerFactory.Core().V1().Nodes()
	imagecacheInformer := fledgedInformerFactory.Fledged().V1alpha1().ImageCaches()
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

	controller := NewController(kubeclientset, fledgedclientset, nodeInformer, imagecacheInformer,
		imageCacheRefreshFrequency, imagePullDeadlineDuration, dockerClientImage, imagePullPolicy)
	controller.nodesSynced = func() bool { return true }
	controller.imageCachesSynced = func() bool { return true }
	return controller, nodeInformer, imagecacheInformer
}

func TestPreFlightChecks(t *testing.T) {
	tests := []struct {
		name                  string
		jobList               *batchv1.JobList
		jobListError          error
		jobDeleteError        error
		imageCacheList        *fledgedv1alpha1.ImageCacheList
		imageCacheListError   error
		imageCacheUpdateError error
		expectErr             bool
		errorString           string
	}{
		{
			name:                  "#1: No dangling jobs. No imagecaches",
			jobList:               &batchv1.JobList{Items: []batchv1.Job{}},
			jobListError:          nil,
			jobDeleteError:        nil,
			imageCacheList:        &fledgedv1alpha1.ImageCacheList{Items: []fledgedv1alpha1.ImageCache{}},
			imageCacheListError:   nil,
			imageCacheUpdateError: nil,
			expectErr:             false,
			errorString:           "",
		},
		{
			name:           "#2: No dangling jobs. No dangling imagecaches",
			jobList:        &batchv1.JobList{Items: []batchv1.Job{}},
			jobListError:   nil,
			jobDeleteError: nil,
			imageCacheList: &fledgedv1alpha1.ImageCacheList{
				Items: []fledgedv1alpha1.ImageCache{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "foo",
						},
						Status: fledgedv1alpha1.ImageCacheStatus{
							Status: fledgedv1alpha1.ImageCacheActionStatusSucceeded,
						},
					},
				},
			},
			imageCacheListError:   nil,
			imageCacheUpdateError: nil,
			expectErr:             false,
			errorString:           "",
		},
		{
			name: "#3: One dangling job. One dangling image cache. Successful list and delete",
			//imageCache:    nil,
			jobList: &batchv1.JobList{
				Items: []batchv1.Job{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "foo",
						},
					},
				},
			},
			jobListError:   nil,
			jobDeleteError: nil,
			imageCacheList: &fledgedv1alpha1.ImageCacheList{
				Items: []fledgedv1alpha1.ImageCache{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "foo",
						},
						Status: fledgedv1alpha1.ImageCacheStatus{
							Status: fledgedv1alpha1.ImageCacheActionStatusProcessing,
						},
					},
				},
			},
			imageCacheListError:   nil,
			imageCacheUpdateError: nil,
			expectErr:             false,
			errorString:           "",
		},
		{
			name:           "#4: Unsuccessful listing of jobs",
			jobList:        nil,
			jobListError:   fmt.Errorf("fake error"),
			jobDeleteError: nil,
			expectErr:      true,
			errorString:    "Internal error occurred: fake error",
		},
		{
			name:                  "#5: Unsuccessful listing of imagecaches",
			jobList:               &batchv1.JobList{Items: []batchv1.Job{}},
			jobListError:          nil,
			jobDeleteError:        nil,
			imageCacheList:        nil,
			imageCacheListError:   fmt.Errorf("fake error"),
			imageCacheUpdateError: nil,
			expectErr:             true,
			errorString:           "Internal error occurred: fake error",
		},
		{
			name: "#6: One dangling job. Successful list. Unsuccessful delete",
			jobList: &batchv1.JobList{
				Items: []batchv1.Job{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "foo",
						},
					},
				},
			},
			jobListError:   nil,
			jobDeleteError: fmt.Errorf("fake error"),
			expectErr:      true,
			errorString:    "Internal error occurred: fake error",
		},
		{
			name:           "#7: One dangling image cache. Successful list. Unsuccessful delete",
			jobList:        &batchv1.JobList{Items: []batchv1.Job{}},
			jobListError:   nil,
			jobDeleteError: nil,
			imageCacheList: &fledgedv1alpha1.ImageCacheList{
				Items: []fledgedv1alpha1.ImageCache{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "foo",
						},
						Status: fledgedv1alpha1.ImageCacheStatus{
							Status: fledgedv1alpha1.ImageCacheActionStatusProcessing,
						},
					},
				},
			},
			imageCacheListError:   nil,
			imageCacheUpdateError: fmt.Errorf("fake error"),
			expectErr:             true,
			errorString:           "Internal error occurred: fake error",
		},
	}
	for _, test := range tests {
		fakekubeclientset := &fakeclientset.Clientset{}
		fakefledgedclientset := &fledgedclientsetfake.Clientset{}
		if test.jobListError != nil {
			listError := apierrors.NewInternalError(test.jobListError)
			fakekubeclientset.AddReactor("list", "jobs", func(action core.Action) (handled bool, ret runtime.Object, err error) {
				return true, nil, listError
			})
		} else {
			fakekubeclientset.AddReactor("list", "jobs", func(action core.Action) (handled bool, ret runtime.Object, err error) {
				return true, test.jobList, nil
			})
		}
		if test.jobDeleteError != nil {
			deleteError := apierrors.NewInternalError(test.jobDeleteError)
			fakekubeclientset.AddReactor("delete", "jobs", func(action core.Action) (handled bool, ret runtime.Object, err error) {
				return true, nil, deleteError
			})
		} else {
			fakekubeclientset.AddReactor("delete", "jobs", func(action core.Action) (handled bool, ret runtime.Object, err error) {
				return true, nil, nil
			})
		}

		if test.imageCacheListError != nil {
			listError := apierrors.NewInternalError(test.imageCacheListError)
			fakefledgedclientset.AddReactor("list", "imagecaches", func(action core.Action) (handled bool, ret runtime.Object, err error) {
				return true, nil, listError
			})
		} else {
			fakefledgedclientset.AddReactor("list", "imagecaches", func(action core.Action) (handled bool, ret runtime.Object, err error) {
				return true, test.imageCacheList, nil
			})
		}
		if test.imageCacheUpdateError != nil {
			updateError := apierrors.NewInternalError(test.imageCacheUpdateError)
			fakefledgedclientset.AddReactor("update", "imagecaches", func(action core.Action) (handled bool, ret runtime.Object, err error) {
				return true, nil, updateError
			})
		} else {
			fakefledgedclientset.AddReactor("update", "imagecaches", func(action core.Action) (handled bool, ret runtime.Object, err error) {
				return true, nil, nil
			})
		}

		controller, _, _ := newTestController(fakekubeclientset, fakefledgedclientset)

		err := controller.PreFlightChecks()
		if test.expectErr {
			if !(err != nil && strings.HasPrefix(err.Error(), test.errorString)) {
				t.Errorf("Test: %s failed", test.name)
			}
		} else {
			if err != nil {
				t.Errorf("Test: %s failed. err received = %s", test.name, err.Error())
			}
		}
	}
	t.Logf("%d tests passed", len(tests))
}

func TestRunRefreshWorker(t *testing.T) {
	now := metav1.Now()
	tests := []struct {
		name                string
		imageCacheList      *fledgedv1alpha1.ImageCacheList
		imageCacheListError error
		workqueueItems      int
	}{
		{
			name: "#1: Do not refresh if status is not yet updated",
			imageCacheList: &fledgedv1alpha1.ImageCacheList{
				Items: []fledgedv1alpha1.ImageCache{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "kube-fledged",
						},
					},
				},
			},
			imageCacheListError: nil,
			workqueueItems:      0,
		},
		{
			name: "#2: Do not refresh if image cache is already under processing",
			imageCacheList: &fledgedv1alpha1.ImageCacheList{
				Items: []fledgedv1alpha1.ImageCache{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "kube-fledged",
						},
						Status: fledgedv1alpha1.ImageCacheStatus{
							Status: fledgedv1alpha1.ImageCacheActionStatusProcessing,
						},
					},
				},
			},
			imageCacheListError: nil,
			workqueueItems:      0,
		},
		{
			name: "#3: Do not refresh image cache if cache spec validation failed",
			imageCacheList: &fledgedv1alpha1.ImageCacheList{
				Items: []fledgedv1alpha1.ImageCache{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "kube-fledged",
						},
						Status: fledgedv1alpha1.ImageCacheStatus{
							Status: fledgedv1alpha1.ImageCacheActionStatusFailed,
							Reason: fledgedv1alpha1.ImageCacheReasonCacheSpecValidationFailed,
						},
					},
				},
			},
			imageCacheListError: nil,
			workqueueItems:      0,
		},
		{
			name: "#4: Do not refresh image cache marked for deletion",
			imageCacheList: &fledgedv1alpha1.ImageCacheList{
				Items: []fledgedv1alpha1.ImageCache{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "foo",
							Namespace:         "kube-fledged",
							DeletionTimestamp: &now,
						},
						Status: fledgedv1alpha1.ImageCacheStatus{
							Status: fledgedv1alpha1.ImageCacheActionStatusSucceeded,
						},
					},
				},
			},
			imageCacheListError: nil,
			workqueueItems:      0,
		},
		{
			name: "#5: Successfully queued 1 imagecache for refresh",
			imageCacheList: &fledgedv1alpha1.ImageCacheList{
				Items: []fledgedv1alpha1.ImageCache{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "kube-fledged",
						},
						Status: fledgedv1alpha1.ImageCacheStatus{
							Status: fledgedv1alpha1.ImageCacheActionStatusSucceeded,
						},
					},
				},
			},
			imageCacheListError: nil,
			workqueueItems:      1,
		},
		{
			name: "#6: Successfully queued 2 imagecaches for refresh",
			imageCacheList: &fledgedv1alpha1.ImageCacheList{
				Items: []fledgedv1alpha1.ImageCache{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "kube-fledged",
						},
						Status: fledgedv1alpha1.ImageCacheStatus{
							Status: fledgedv1alpha1.ImageCacheActionStatusSucceeded,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "bar",
							Namespace: "kube-fledged",
						},
						Status: fledgedv1alpha1.ImageCacheStatus{
							Status: fledgedv1alpha1.ImageCacheActionStatusFailed,
						},
					},
				},
			},
			imageCacheListError: nil,
			workqueueItems:      2,
		},
		{
			name:                "#7: No imagecaches to refresh",
			imageCacheList:      nil,
			imageCacheListError: nil,
			workqueueItems:      0,
		},
	}

	for _, test := range tests {
		if test.workqueueItems > 0 {
			//TODO: How to check if workqueue contains the added item?
			continue
		}
		fakekubeclientset := &fakeclientset.Clientset{}
		fakefledgedclientset := &fledgedclientsetfake.Clientset{}

		controller, _, imagecacheInformer := newTestController(fakekubeclientset, fakefledgedclientset)
		if test.imageCacheList != nil && len(test.imageCacheList.Items) > 0 {
			for _, imagecache := range test.imageCacheList.Items {
				imagecacheInformer.Informer().GetIndexer().Add(&imagecache)
			}
		}
		controller.runRefreshWorker()
		if test.workqueueItems == controller.workqueue.Len() {
		} else {
			t.Errorf("Test: %s failed: expected %d, actual %d", test.name, test.workqueueItems, controller.workqueue.Len())
		}
	}
}

func TestAddRemoveFinalizers(t *testing.T) {
	tests := []struct {
		name        string
		imageCache  *fledgedv1alpha1.ImageCache
		action      string
		expectErr   bool
		errorString string
	}{
		{
			name: "#1: Add 'fledged' finalizer successfully",
			imageCache: &fledgedv1alpha1.ImageCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "foo",
					Namespace:  "kube-fledged",
					Finalizers: []string{},
				},
			},
			action:      "addfinalizer",
			expectErr:   false,
			errorString: "",
		},
		{
			name: "#2: 'fledged' finalizer already exists.",
			imageCache: &fledgedv1alpha1.ImageCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "foo",
					Namespace:  "kube-fledged",
					Finalizers: []string{"fledged"},
				},
			},
			action:      "addfinalizer",
			expectErr:   false,
			errorString: "",
		},
		{
			name: "#3: Remove 'fledged' finalizer successfully",
			imageCache: &fledgedv1alpha1.ImageCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "foo",
					Namespace:  "kube-fledged",
					Finalizers: []string{"fledged"},
				},
			},
			action:      "removefinalizer",
			expectErr:   false,
			errorString: "",
		},
		{
			name: "#4: No 'fledged' finalizer in imagecache",
			imageCache: &fledgedv1alpha1.ImageCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "foo",
					Namespace:  "kube-fledged",
					Finalizers: []string{},
				},
			},
			action:      "removefinalizer",
			expectErr:   false,
			errorString: "",
		},
		{
			name: "#5: Error in updating imagecache during adding finalizer",
			imageCache: &fledgedv1alpha1.ImageCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "foo",
					Namespace:  "kube-fledged",
					Finalizers: []string{},
				},
			},
			action:      "addfinalizer",
			expectErr:   true,
			errorString: "Internal error occurred",
		},
		{
			name: "#6: Error in updating imagecache during removing finalizer",
			imageCache: &fledgedv1alpha1.ImageCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "foo",
					Namespace:  "kube-fledged",
					Finalizers: []string{"fledged"},
				},
			},
			action:      "removefinalizer",
			expectErr:   true,
			errorString: "Internal error occurred",
		},
	}

	for _, test := range tests {
		fakekubeclientset := &fakeclientset.Clientset{}
		fakefledgedclientset := &fledgedclientsetfake.Clientset{}
		if test.expectErr {
			updateError := apierrors.NewInternalError(fmt.Errorf("fake error"))
			fakefledgedclientset.AddReactor("update", "imagecaches", func(action core.Action) (handled bool, ret runtime.Object, err error) {
				return true, nil, updateError
			})
		} else {
			fakefledgedclientset.AddReactor("update", "imagecaches", func(action core.Action) (handled bool, ret runtime.Object, err error) {
				return true, nil, nil
			})
		}

		controller, _, _ := newTestController(fakekubeclientset, fakefledgedclientset)

		var err error
		if test.action == "addfinalizer" {
			err = controller.addFinalizer(test.imageCache)
		}
		if test.action == "removefinalizer" {
			err = controller.removeFinalizer(test.imageCache)
		}
		if test.expectErr {
			if err != nil && strings.HasPrefix(err.Error(), test.errorString) {
			} else {
				t.Errorf("Test: %s failed", test.name)
			}
		} else if err != nil {
			t.Errorf("Test: %s failed. err received = %s", test.name, err.Error())
		}

	}
	t.Logf("%d tests passed", len(tests))
}
