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
	"github.com/senthilrch/kube-fledged/pkg/images"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
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

func TestSyncHandler(t *testing.T) {
	type ActionReaction struct {
		action   string
		reaction string
	}
	now := metav1.Now()
	defaultImageCache := fledgedv1alpha1.ImageCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "kube-fledged",
		},
		Spec: fledgedv1alpha1.ImageCacheSpec{
			CacheSpec: []fledgedv1alpha1.CacheSpecImages{
				{
					Images: []string{"foo"},
				},
			},
		},
	}
	defaultNodeList := &corev1.NodeList{
		Items: []corev1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "fakenode",
					Labels: map[string]string{"foo": "bar"},
				},
			},
		},
	}

	tests := []struct {
		name              string
		imageCache        fledgedv1alpha1.ImageCache
		wqKey             images.WorkQueueKey
		nodeList          *corev1.NodeList
		expectedActions   []ActionReaction
		expectErr         bool
		expectedErrString string
	}{
		{
			name: "#1: Invalid imagecache resource key",
			wqKey: images.WorkQueueKey{
				ObjKey: "foo/bar/car",
			},
			expectErr:         true,
			expectedErrString: "unexpected key format",
		},
		{
			name: "#2: Create - Invalid imagecache spec (no images specified)",
			imageCache: fledgedv1alpha1.ImageCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "kube-fledged",
				},
				Spec: fledgedv1alpha1.ImageCacheSpec{
					CacheSpec: []fledgedv1alpha1.CacheSpecImages{
						{
							Images: []string{},
						},
					},
				},
			},
			wqKey: images.WorkQueueKey{
				ObjKey:   "kube-fledged/foo",
				WorkType: images.ImageCacheCreate,
			},
			expectedActions:   []ActionReaction{{action: "update", reaction: ""}},
			expectErr:         true,
			expectedErrString: "No images specified within image list",
		},
		{
			name:       "#3: Update - Old imagecache pointer is nil",
			imageCache: defaultImageCache,
			wqKey: images.WorkQueueKey{
				ObjKey:        "kube-fledged/foo",
				WorkType:      images.ImageCacheUpdate,
				OldImageCache: nil,
			},
			nodeList:          defaultNodeList,
			expectedActions:   []ActionReaction{{action: "update", reaction: ""}},
			expectErr:         true,
			expectedErrString: "OldImageCacheNotFound",
		},
		{
			name:       "#4: Update - No. of imagelists not equal",
			imageCache: defaultImageCache,
			wqKey: images.WorkQueueKey{
				ObjKey:   "kube-fledged/foo",
				WorkType: images.ImageCacheUpdate,
				OldImageCache: &fledgedv1alpha1.ImageCache{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: "kube-fledged",
					},
					Spec: fledgedv1alpha1.ImageCacheSpec{
						CacheSpec: []fledgedv1alpha1.CacheSpecImages{
							{
								Images: []string{"foo"},
							},
							{
								Images: []string{"bar"},
							},
						},
					},
				},
			},
			nodeList:          defaultNodeList,
			expectedActions:   []ActionReaction{{action: "update", reaction: ""}},
			expectErr:         true,
			expectedErrString: "NotSupportedUpdates",
		},
		{
			name:       "#5: Update - Change in NodeSelectors",
			imageCache: defaultImageCache,
			wqKey: images.WorkQueueKey{
				ObjKey:   "kube-fledged/foo",
				WorkType: images.ImageCacheUpdate,
				OldImageCache: &fledgedv1alpha1.ImageCache{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: "kube-fledged",
					},
					Spec: fledgedv1alpha1.ImageCacheSpec{
						CacheSpec: []fledgedv1alpha1.CacheSpecImages{
							{
								Images:       []string{"foo"},
								NodeSelector: map[string]string{"foo": "bar"},
							},
						},
					},
				},
			},
			nodeList:          defaultNodeList,
			expectedActions:   []ActionReaction{{action: "update", reaction: ""}},
			expectErr:         true,
			expectedErrString: "NotSupportedUpdates",
		},
		{
			name:       "#6: Create - Error in adding finalizer",
			imageCache: defaultImageCache,
			wqKey: images.WorkQueueKey{
				ObjKey:   "kube-fledged/foo",
				WorkType: images.ImageCacheCreate,
			},
			nodeList:          defaultNodeList,
			expectedActions:   []ActionReaction{{action: "update", reaction: "fake error"}},
			expectErr:         true,
			expectedErrString: "Internal error occurred: fake error",
		},
		{
			name:       "#7: Refresh - Update status to processing",
			imageCache: defaultImageCache,
			wqKey: images.WorkQueueKey{
				ObjKey:   "kube-fledged/foo",
				WorkType: images.ImageCacheRefresh,
			},
			nodeList: defaultNodeList,
			expectedActions: []ActionReaction{
				{action: "get", reaction: ""},
				{action: "update", reaction: "fake error"},
			},
			expectErr:         true,
			expectedErrString: "Internal error occurred: fake error",
		},
		{
			name:       "#8: Purge - Update status to processing",
			imageCache: defaultImageCache,
			wqKey: images.WorkQueueKey{
				ObjKey:   "kube-fledged/foo",
				WorkType: images.ImageCachePurge,
			},
			nodeList: defaultNodeList,
			expectedActions: []ActionReaction{
				{action: "get", reaction: ""},
				{action: "update", reaction: "fake error"},
			},
			expectErr:         true,
			expectedErrString: "Internal error occurred: fake error",
		},
		{
			name:       "#9: Create - Successfully firing imagepull requests",
			imageCache: defaultImageCache,
			wqKey: images.WorkQueueKey{
				ObjKey:   "kube-fledged/foo",
				WorkType: images.ImageCacheCreate,
			},
			nodeList: defaultNodeList,
			expectedActions: []ActionReaction{
				{action: "get", reaction: ""},
				{action: "update", reaction: ""},
			},
			expectErr:         false,
			expectedErrString: "",
		},
		{
			name:       "#10: Update - Successfully firing imagepull & imagedelete requests",
			imageCache: defaultImageCache,
			wqKey: images.WorkQueueKey{
				ObjKey:   "kube-fledged/foo",
				WorkType: images.ImageCacheUpdate,
				OldImageCache: &fledgedv1alpha1.ImageCache{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: "kube-fledged",
					},
					Spec: fledgedv1alpha1.ImageCacheSpec{
						CacheSpec: []fledgedv1alpha1.CacheSpecImages{
							{
								Images: []string{"foo", "bar"},
							},
						},
					},
				},
			},
			nodeList: defaultNodeList,
			expectedActions: []ActionReaction{
				{action: "get", reaction: ""},
				{action: "update", reaction: ""},
			},
			expectErr:         false,
			expectedErrString: "",
		},
		{
			name: "#11: StatusUpdate - ImagesPulledSuccessfully",
			imageCache: fledgedv1alpha1.ImageCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "kube-fledged",
				},
				Spec: fledgedv1alpha1.ImageCacheSpec{
					CacheSpec: []fledgedv1alpha1.CacheSpecImages{
						{
							Images: []string{"foo"},
						},
					},
				},
				Status: fledgedv1alpha1.ImageCacheStatus{
					StartTime: &now,
				},
			},
			wqKey: images.WorkQueueKey{
				ObjKey:   "kube-fledged/foo",
				WorkType: images.ImageCacheStatusUpdate,
				Status: &map[string]images.ImageWorkResult{
					"job1": {
						Status: images.ImageWorkResultStatusSucceeded,
						ImageWorkRequest: images.ImageWorkRequest{
							WorkType: images.ImageCacheCreate,
						},
					},
				},
			},
			expectedActions: []ActionReaction{
				{action: "get", reaction: ""},
				{action: "update", reaction: ""},
			},
			expectErr:         false,
			expectedErrString: "",
		},
		{
			name: "#12: StatusUpdate - ImagesDeletedSuccessfully",
			imageCache: fledgedv1alpha1.ImageCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "kube-fledged",
				},
				Spec: fledgedv1alpha1.ImageCacheSpec{
					CacheSpec: []fledgedv1alpha1.CacheSpecImages{
						{
							Images: []string{"foo"},
						},
					},
				},
				Status: fledgedv1alpha1.ImageCacheStatus{
					StartTime: &now,
				},
			},
			wqKey: images.WorkQueueKey{
				ObjKey:   "kube-fledged/foo",
				WorkType: images.ImageCacheStatusUpdate,
				Status: &map[string]images.ImageWorkResult{
					"job1": {
						Status: images.ImageWorkResultStatusSucceeded,
						ImageWorkRequest: images.ImageWorkRequest{
							WorkType: images.ImageCachePurge,
						},
					},
				},
			},
			expectedActions: []ActionReaction{
				{action: "get", reaction: ""},
				{action: "update", reaction: ""},
			},
			expectErr:         false,
			expectedErrString: "",
		},
		{
			name:       "#13: StatusUpdate - ImagePullFailedForSomeImages",
			imageCache: defaultImageCache,
			wqKey: images.WorkQueueKey{
				ObjKey:   "kube-fledged/foo",
				WorkType: images.ImageCacheStatusUpdate,
				Status: &map[string]images.ImageWorkResult{
					"job1": {
						Status: images.ImageWorkResultStatusFailed,
						ImageWorkRequest: images.ImageWorkRequest{
							WorkType: images.ImageCacheCreate,
						},
					},
				},
			},
			expectedActions: []ActionReaction{
				{action: "get", reaction: ""},
				{action: "update", reaction: ""},
			},
			expectErr:         false,
			expectedErrString: "",
		},
		{
			name:       "#14: StatusUpdate - ImageDeleteFailedForSomeImages",
			imageCache: defaultImageCache,
			wqKey: images.WorkQueueKey{
				ObjKey:   "kube-fledged/foo",
				WorkType: images.ImageCacheStatusUpdate,
				Status: &map[string]images.ImageWorkResult{
					"job1": {
						Status: images.ImageWorkResultStatusFailed,
						ImageWorkRequest: images.ImageWorkRequest{
							WorkType: images.ImageCachePurge,
						},
					},
				},
			},
			expectedActions: []ActionReaction{
				{action: "get", reaction: ""},
				{action: "update", reaction: ""},
			},
			expectErr:         false,
			expectedErrString: "",
		},
		{
			name: "#15: Delete - Remove Finalizer",
			imageCache: fledgedv1alpha1.ImageCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "foo",
					Namespace:         "kube-fledged",
					DeletionTimestamp: &now,
					Finalizers:        []string{"fledged"},
				},
				Spec: fledgedv1alpha1.ImageCacheSpec{
					CacheSpec: []fledgedv1alpha1.CacheSpecImages{
						{
							Images: []string{"foo"},
						},
					},
				},
			},
			wqKey: images.WorkQueueKey{
				ObjKey:   "kube-fledged/foo",
				WorkType: images.ImageCacheDelete,
			},
			expectedActions: []ActionReaction{
				{action: "get", reaction: ""},
				{action: "update", reaction: ""},
			},
			expectErr:         false,
			expectedErrString: "",
		},
	}

	for _, test := range tests {
		fakekubeclientset := &fakeclientset.Clientset{}
		fakefledgedclientset := &fledgedclientsetfake.Clientset{}
		for _, ar := range test.expectedActions {
			if ar.reaction != "" {
				apiError := apierrors.NewInternalError(fmt.Errorf(ar.reaction))
				fakefledgedclientset.AddReactor(ar.action, "imagecaches", func(action core.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, apiError
				})
			}
			fakefledgedclientset.AddReactor(ar.action, "imagecaches", func(action core.Action) (handled bool, ret runtime.Object, err error) {
				return true, &test.imageCache, nil
			})
		}

		controller, nodeInformer, imagecacheInformer := newTestController(fakekubeclientset, fakefledgedclientset)
		if test.nodeList != nil && len(test.nodeList.Items) > 0 {
			for _, node := range test.nodeList.Items {
				nodeInformer.Informer().GetIndexer().Add(&node)
			}
		}
		imagecacheInformer.Informer().GetIndexer().Add(&test.imageCache)
		err := controller.syncHandler(test.wqKey)
		if test.expectErr {
			if err == nil {
				t.Errorf("Test: %s failed: expectedError=%s, actualError=nil", test.name, test.expectedErrString)
			}
			if err != nil && !strings.HasPrefix(err.Error(), test.expectedErrString) {
				t.Errorf("Test: %s failed: expectedError=%s, actualError=%s", test.name, test.expectedErrString, err.Error())
			}
		} else if err != nil {
			t.Errorf("Test: %s failed. expectedError=nil, actualError=%s", test.name, err.Error())
		}
	}
	t.Logf("%d tests passed", len(tests))
}

func TestEnqueueImageCache(t *testing.T) {
	now := metav1.Now()
	nowplus5s := metav1.NewTime(time.Now().Add(time.Second * 5))
	defaultImageCache := fledgedv1alpha1.ImageCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "kube-fledged",
		},
		Spec: fledgedv1alpha1.ImageCacheSpec{
			CacheSpec: []fledgedv1alpha1.CacheSpecImages{
				{
					Images: []string{"foo"},
				},
			},
		},
	}
	tests := []struct {
		name           string
		workType       images.WorkType
		oldImageCache  fledgedv1alpha1.ImageCache
		newImageCache  fledgedv1alpha1.ImageCache
		expectedResult bool
	}{
		{
			name:           "#1: Create - Imagecache queued successfully",
			workType:       images.ImageCacheCreate,
			newImageCache:  defaultImageCache,
			expectedResult: true,
		},
		{
			name:     "#2: Create - Imagecache with Status field, so no queueing",
			workType: images.ImageCacheCreate,
			newImageCache: fledgedv1alpha1.ImageCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "kube-fledged",
				},
				Spec: fledgedv1alpha1.ImageCacheSpec{
					CacheSpec: []fledgedv1alpha1.CacheSpecImages{
						{
							Images: []string{"foo"},
						},
					},
				},
				Status: fledgedv1alpha1.ImageCacheStatus{
					Status: fledgedv1alpha1.ImageCacheActionStatusSucceeded,
				},
			},
			expectedResult: false,
		},
		{
			name:          "#3: Update - Imagecache purge. Successful queueing",
			workType:      images.ImageCacheUpdate,
			oldImageCache: defaultImageCache,
			newImageCache: fledgedv1alpha1.ImageCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "foo",
					Namespace:         "kube-fledged",
					DeletionTimestamp: &now,
				},
				Spec: fledgedv1alpha1.ImageCacheSpec{
					CacheSpec: []fledgedv1alpha1.CacheSpecImages{
						{
							Images: []string{"foo"},
						},
					},
				},
				Status: fledgedv1alpha1.ImageCacheStatus{
					Status: fledgedv1alpha1.ImageCacheActionStatusSucceeded,
				},
			},
			expectedResult: true,
		},
		{
			name:     "#4: Update - Imagecache delete. Successful queueing",
			workType: images.ImageCacheUpdate,
			oldImageCache: fledgedv1alpha1.ImageCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "foo",
					Namespace:         "kube-fledged",
					DeletionTimestamp: &now,
				},
				Spec: fledgedv1alpha1.ImageCacheSpec{
					CacheSpec: []fledgedv1alpha1.CacheSpecImages{
						{
							Images: []string{"foo"},
						},
					},
				},
				Status: fledgedv1alpha1.ImageCacheStatus{
					Status: fledgedv1alpha1.ImageCacheActionStatusSucceeded,
				},
			},
			newImageCache: fledgedv1alpha1.ImageCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "foo",
					Namespace:         "kube-fledged",
					DeletionTimestamp: &nowplus5s,
				},
				Spec: fledgedv1alpha1.ImageCacheSpec{
					CacheSpec: []fledgedv1alpha1.CacheSpecImages{
						{
							Images: []string{"foo"},
						},
					},
				},
				Status: fledgedv1alpha1.ImageCacheStatus{
					Status: fledgedv1alpha1.ImageCacheActionStatusSucceeded,
				},
			},
			expectedResult: true,
		},
		{
			name:           "#5: Update - No change in Spec. Unsuccessful queueing",
			workType:       images.ImageCacheUpdate,
			oldImageCache:  defaultImageCache,
			newImageCache:  defaultImageCache,
			expectedResult: false,
		},
		{
			name:     "#6: Update - Status processing. Unsuccessful queueing",
			workType: images.ImageCacheUpdate,
			oldImageCache: fledgedv1alpha1.ImageCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "kube-fledged",
				},
				Spec: fledgedv1alpha1.ImageCacheSpec{
					CacheSpec: []fledgedv1alpha1.CacheSpecImages{
						{
							Images: []string{"foo"},
						},
					},
				},
				Status: fledgedv1alpha1.ImageCacheStatus{
					Status: fledgedv1alpha1.ImageCacheActionStatusProcessing,
				},
			},
			expectedResult: false,
		},
		{
			name:          "#7: Update - Successful queueing",
			workType:      images.ImageCacheUpdate,
			oldImageCache: defaultImageCache,
			newImageCache: fledgedv1alpha1.ImageCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "kube-fledged",
				},
				Spec: fledgedv1alpha1.ImageCacheSpec{
					CacheSpec: []fledgedv1alpha1.CacheSpecImages{
						{
							Images: []string{"foo", "bar"},
						},
					},
				},
			},
			expectedResult: true,
		},
		{
			name:           "#7: Delete - Unsuccessful queueing",
			workType:       images.ImageCacheDelete,
			expectedResult: false,
		},
		{
			name:           "#8: Refresh - Successful queueing",
			workType:       images.ImageCacheRefresh,
			oldImageCache:  defaultImageCache,
			expectedResult: true,
		},
	}

	for _, test := range tests {
		fakekubeclientset := &fakeclientset.Clientset{}
		fakefledgedclientset := &fledgedclientsetfake.Clientset{}
		controller, _, _ := newTestController(fakekubeclientset, fakefledgedclientset)
		result := controller.enqueueImageCache(test.workType, &test.oldImageCache, &test.newImageCache)
		if result != test.expectedResult {
			t.Errorf("Test %s failed: expected=%t, actual=%t", test.name, test.expectedResult, result)
		}
	}
}

func TestProcessNextWorkItem(t *testing.T) {
	type ActionReaction struct {
		action   string
		reaction string
	}
	defaultImageCache := fledgedv1alpha1.ImageCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "kube-fledged",
		},
		Spec: fledgedv1alpha1.ImageCacheSpec{
			CacheSpec: []fledgedv1alpha1.CacheSpecImages{
				{
					Images: []string{"foo"},
				},
			},
		},
	}

	tests := []struct {
		name              string
		imageCache        fledgedv1alpha1.ImageCache
		wqKey             images.WorkQueueKey
		expectedActions   []ActionReaction
		expectErr         bool
		expectedErrString string
	}{
		{
			name:       "#1: StatusUpdate - ImageDeleteFailedForSomeImages",
			imageCache: defaultImageCache,
			wqKey: images.WorkQueueKey{
				ObjKey:   "kube-fledged/foo",
				WorkType: images.ImageCacheStatusUpdate,
				Status: &map[string]images.ImageWorkResult{
					"job1": {
						Status: images.ImageWorkResultStatusFailed,
						ImageWorkRequest: images.ImageWorkRequest{
							WorkType: images.ImageCachePurge,
						},
					},
				},
			},
			expectedActions: []ActionReaction{
				{action: "get", reaction: ""},
				{action: "update", reaction: ""},
			},
			expectErr:         false,
			expectedErrString: "",
		},
		{
			name: "#2: Create - Invalid imagecache spec (no images specified)",
			imageCache: fledgedv1alpha1.ImageCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "kube-fledged",
				},
				Spec: fledgedv1alpha1.ImageCacheSpec{
					CacheSpec: []fledgedv1alpha1.CacheSpecImages{
						{
							Images: []string{},
						},
					},
				},
			},
			wqKey: images.WorkQueueKey{
				ObjKey:   "kube-fledged/foo",
				WorkType: images.ImageCacheCreate,
			},
			expectedActions:   []ActionReaction{{action: "update", reaction: ""}},
			expectErr:         false,
			expectedErrString: "No images specified within image list",
		},
		{
			name:              "#3: Unexpected type in workqueue",
			expectErr:         false,
			expectedErrString: "Unexpected type in workqueue",
		},
	}

	for _, test := range tests {
		fakekubeclientset := &fakeclientset.Clientset{}
		fakefledgedclientset := &fledgedclientsetfake.Clientset{}
		for _, ar := range test.expectedActions {
			if ar.reaction != "" {
				apiError := apierrors.NewInternalError(fmt.Errorf(ar.reaction))
				fakefledgedclientset.AddReactor(ar.action, "imagecaches", func(action core.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, apiError
				})
			}
			fakefledgedclientset.AddReactor(ar.action, "imagecaches", func(action core.Action) (handled bool, ret runtime.Object, err error) {
				return true, &test.imageCache, nil
			})
		}

		controller, _, imagecacheInformer := newTestController(fakekubeclientset, fakefledgedclientset)
		imagecacheInformer.Informer().GetIndexer().Add(&test.imageCache)
		if test.expectedErrString == "Unexpected type in workqueue" {
			controller.workqueue.Add(struct{}{})
		}
		controller.workqueue.Add(test.wqKey)
		controller.processNextWorkItem()
		var err error
		if test.expectErr {
			if err == nil {
				t.Errorf("Test: %s failed: expectedError=%s, actualError=nil", test.name, test.expectedErrString)
			}
			if err != nil && !strings.HasPrefix(err.Error(), test.expectedErrString) {
				t.Errorf("Test: %s failed: expectedError=%s, actualError=%s", test.name, test.expectedErrString, err.Error())
			}
		} else if err != nil {
			t.Errorf("Test: %s failed. expectedError=nil, actualError=%s", test.name, err.Error())
		}
	}
	t.Logf("%d tests passed", len(tests))
}
