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

package images

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	fledgedv1alpha2 "github.com/senthilrch/kube-fledged/pkg/apis/kubefledged/v1alpha2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/util/workqueue"
)

const fledgedNameSpace = "kube-fledged"

var node = corev1.Node{
	ObjectMeta: metav1.ObjectMeta{
		Labels: map[string]string{"kubernetes.io/hostname": "bar"},
	},
}

func newTestImageManager(kubeclientset kubernetes.Interface, imagepullpolicy string,
	serviceaccountname string, imagedeletejobhostnetwork bool,
	jobpriorityclassname string, candeletejob bool, criSocketPath string) (*ImageManager, coreinformers.PodInformer) {
	imagePullDeadlineDuration := time.Millisecond * 10
	criClientImage := "senthilrch/fledged-docker-client:latest"
	busyboxImage := "senthilrch/busybox:1.35.0"
	imagePullPolicy := imagepullpolicy
	serviceAccountName := serviceaccountname
	imageDeleteJobHostNetwork := imagedeletejobhostnetwork
	jobPriorityClassName := jobpriorityclassname
	canDeleteJob := candeletejob
	criSocketPath := criSocketPath
	imagecacheworkqueue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ImageCaches")
	imageworkqueue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ImagePullerStatus")

	imagemanager, podInformer := NewImageManager(imagecacheworkqueue, imageworkqueue, kubeclientset,
		fledgedNameSpace, imagePullDeadlineDuration, criClientImage, busyboxImage, imagePullPolicy,
		serviceAccountName, imageDeleteJobHostNetwork, jobPriorityClassName, canDeleteJob, criSocketPath)
	imagemanager.podsSynced = func() bool { return true }

	return imagemanager, podInformer
}

func TestPullDeleteImage(t *testing.T) {
	job := batchv1.Job{}
	defaultImageCache := fledgedv1alpha2.ImageCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "kube-fledged",
		},
		Spec: fledgedv1alpha2.ImageCacheSpec{
			CacheSpec: []fledgedv1alpha2.CacheSpecImages{
				{
					Images: []string{"foo"},
				},
			},
		},
	}
	tests := []struct {
		name                string
		action              string
		iwr                 ImageWorkRequest
		expectError         bool
		expectedErrorString string
	}{
		{
			name:   "#1 Successful creation of image pull job",
			action: "pullimage",
			iwr: ImageWorkRequest{
				Image:      "foo",
				Node:       &node,
				WorkType:   ImageCacheCreate,
				Imagecache: &defaultImageCache,
			},
			expectError:         false,
			expectedErrorString: "",
		},
		{
			name:   "#2 Unsuccessful - imagecache pointer is nil",
			action: "pullimage",
			iwr: ImageWorkRequest{
				Image:      "foo",
				Node:       &node,
				WorkType:   ImageCacheCreate,
				Imagecache: nil,
			},
			expectError:         true,
			expectedErrorString: "imagecache pointer is nil",
		},
		{
			name:   "#3 Unsuccessful - Internal error occurred: fake error",
			action: "pullimage",
			iwr: ImageWorkRequest{
				Image:      "foo",
				Node:       &node,
				WorkType:   ImageCacheCreate,
				Imagecache: &defaultImageCache,
			},
			expectError:         true,
			expectedErrorString: "Internal error occurred: fake error",
		},
		{
			name:   "#4 Successful creation of image delete job",
			action: "deleteimage",
			iwr: ImageWorkRequest{
				Image:      "foo",
				Node:       &node,
				WorkType:   ImageCachePurge,
				Imagecache: &defaultImageCache,
			},
			expectError:         false,
			expectedErrorString: "",
		},
		{
			name:   "#5 Unsuccessful - imagecache pointer is nil",
			action: "deleteimage",
			iwr: ImageWorkRequest{
				Image:      "foo",
				Node:       &node,
				WorkType:   ImageCachePurge,
				Imagecache: nil,
			},
			expectError:         true,
			expectedErrorString: "imagecache pointer is nil",
		},
		{
			name:   "#6 Unsuccessful - Internal error occurred: fake error",
			action: "deleteimage",
			iwr: ImageWorkRequest{
				Image:      "foo",
				Node:       &node,
				WorkType:   ImageCachePurge,
				Imagecache: &defaultImageCache,
			},
			expectError:         true,
			expectedErrorString: "Internal error occurred: fake error",
		},
		{
			name:   "#7 Successful creation of image delete job (runtime: containerd)",
			action: "deleteimage",
			iwr: ImageWorkRequest{
				Image:                   "foo",
				Node:                    &node,
				ContainerRuntimeVersion: "containerd://1.0.0",
				WorkType:                ImageCachePurge,
				Imagecache:              &defaultImageCache,
			},
			expectError:         false,
			expectedErrorString: "",
		},
		{
			name:   "#8 Successful creation of image delete job (runtime: cri-o)",
			action: "deleteimage",
			iwr: ImageWorkRequest{
				Image:                   "foo",
				Node:                    &node,
				ContainerRuntimeVersion: "cri-o://1.0.0",
				WorkType:                ImageCachePurge,
				Imagecache:              &defaultImageCache,
			},
			expectError:         false,
			expectedErrorString: "",
		},
		{
			name:   "#9 Successful creation of image delete job (runtime: docker)",
			action: "deleteimage",
			iwr: ImageWorkRequest{
				Image:                   "foo",
				Node:                    &node,
				ContainerRuntimeVersion: "docker://1.0.0",
				WorkType:                ImageCachePurge,
				Imagecache:              &defaultImageCache,
			},
			expectError:         false,
			expectedErrorString: "",
		},
	}
	for _, test := range tests {
		fakekubeclientset := &fakeclientset.Clientset{}
		if test.expectedErrorString == "Internal error occurred: fake error" {
			fakekubeclientset.AddReactor("create", "jobs", func(action core.Action) (handled bool, ret runtime.Object, err error) {
				return true, nil, apierrors.NewInternalError(fmt.Errorf("fake error"))
			})
		} else {
			fakekubeclientset.AddReactor("create", "jobs", func(action core.Action) (handled bool, ret runtime.Object, err error) {
				return true, &job, nil
			})
		}

		imagemanager, _ := newTestImageManager(fakekubeclientset, "IfNotPresent", "sa-kube-fledged", false, "priority-class-kube-fledged", false)
		var err error
		if test.action == "pullimage" {
			_, err = imagemanager.pullImage(test.iwr)
		}
		if test.action == "deleteimage" {
			_, err = imagemanager.deleteImage(test.iwr)
		}
		if test.expectError {
			if err == nil {
				t.Errorf("Test: %s failed: expectedError=%s, actualError=nil", test.name, test.expectedErrorString)
			}
			if err != nil && !strings.HasPrefix(err.Error(), test.expectedErrorString) {
				t.Errorf("Test: %s failed: expectedError=%s, actualError=%s", test.name, test.expectedErrorString, err.Error())
			}
		} else if err != nil {
			t.Errorf("Test: %s failed. expectedError=nil, actualError=%s", test.name, err.Error())
		}
	}
}

func TestHandlePodStatusChange(t *testing.T) {
	tests := []struct {
		name     string
		worktype WorkType
		pod      corev1.Pod
	}{
		{
			name:     "#1: Create - Pod succeeded",
			worktype: ImageCacheCreate,
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"job-name": "fakejob"},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodSucceeded,
				},
			},
		},
		{
			name:     "#2: Purge - Pod succeeded",
			worktype: ImageCachePurge,
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"job-name": "fakejob"},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodSucceeded,
				},
			},
		},
		{
			name:     "#3: Create - Pod failed",
			worktype: ImageCacheCreate,
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"job-name": "fakejob"},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodFailed,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							State: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									Reason:  "fakereason",
									Message: "fakemessage",
								},
							},
						},
					},
				},
			},
		},
		{
			name:     "#4: Purge - Pod failed",
			worktype: ImageCachePurge,
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"job-name": "fakejob"},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodFailed,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							State: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									Reason:  "fakereason",
									Message: "fakemessage",
								},
							},
						},
					},
				},
			},
		},
	}
	for _, test := range tests {
		fakekubeclientset := &fakeclientset.Clientset{}
		imagemanager, _ := newTestImageManager(fakekubeclientset, "IfNotPresent", "sa-kube-fledged", false, "priority-class-kube-fledged", false)
		imagemanager.imageworkstatus[test.pod.Labels["job-name"]] = ImageWorkResult{
			Status: ImageWorkResultStatusJobCreated,
			ImageWorkRequest: ImageWorkRequest{
				WorkType: test.worktype,
				Node:     &node,
			},
		}
		imagemanager.handlePodStatusChange(&test.pod)

		if test.pod.Status.Phase == corev1.PodSucceeded {
			if !(imagemanager.imageworkstatus[test.pod.Labels["job-name"]].Status == ImageWorkResultStatusSucceeded) {
				t.Errorf("Test: %s failed: expectedWorkResult=%s, actualWorkResult=%s", test.name, ImageWorkResultStatusSucceeded, imagemanager.imageworkstatus[test.pod.Labels["job-name"]].Status)
			}
		}
		if test.pod.Status.Phase == corev1.PodFailed {
			if !(imagemanager.imageworkstatus[test.pod.Labels["job-name"]].Status == ImageWorkResultStatusFailed) {
				t.Errorf("Test: %s failed: expectedWorkResult=%s, actualWorkResult=%s", test.name, ImageWorkResultStatusFailed, imagemanager.imageworkstatus[test.pod.Labels["job-name"]].Status)
			}
		}
	}
}

func TestUpdateImageCacheStatus(t *testing.T) {
	imageCacheName := "fakeimagecache"
	imageCache := &fledgedv1alpha2.ImageCache{
		ObjectMeta: metav1.ObjectMeta{
			Name: imageCacheName,
		}}
	tests := []struct {
		name                string
		imageworkstatus     map[string]ImageWorkResult
		pods                []corev1.Pod
		eventListErr        bool
		jobDeleteErr        bool
		expectError         bool
		expectedErrorString string
	}{
		{
			name: "#1: Successful",
			imageworkstatus: map[string]ImageWorkResult{
				"fakejob": {
					ImageWorkRequest: ImageWorkRequest{
						Imagecache: &fledgedv1alpha2.ImageCache{
							ObjectMeta: metav1.ObjectMeta{
								Name: imageCacheName,
							},
						},
						Node: &node,
					},
					Status: ImageWorkResultStatusSucceeded,
				},
			},
			pods:        []corev1.Pod{},
			expectError: false,
		},
		{
			name: "#2: Create - Successful",
			imageworkstatus: map[string]ImageWorkResult{
				"fakejob": {
					ImageWorkRequest: ImageWorkRequest{
						Imagecache: &fledgedv1alpha2.ImageCache{
							ObjectMeta: metav1.ObjectMeta{
								Name: imageCacheName,
							},
						},
						Node: &node,
					},
					Status: ImageWorkResultStatusJobCreated,
				},
			},
			pods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: fledgedNameSpace,
						Labels:    map[string]string{"job-name": "fakejob"},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
						ContainerStatuses: []corev1.ContainerStatus{
							{
								State: corev1.ContainerState{
									Waiting: &corev1.ContainerStateWaiting{
										Reason:  "fakereason",
										Message: "fakemessage",
									},
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "#3: Create - Successful (Node not ready, hence empty containerstatuses)",
			imageworkstatus: map[string]ImageWorkResult{
				"fakejob": {
					ImageWorkRequest: ImageWorkRequest{
						Imagecache: &fledgedv1alpha2.ImageCache{
							ObjectMeta: metav1.ObjectMeta{
								Name: imageCacheName,
							},
						},
						Node: &node,
					},
					Status: ImageWorkResultStatusJobCreated,
				},
			},
			pods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: fledgedNameSpace,
						Labels:    map[string]string{"job-name": "fakejob"},
					},
					Status: corev1.PodStatus{
						Phase:             corev1.PodPending,
						ContainerStatuses: []corev1.ContainerStatus{},
					},
				},
			},
			expectError: false,
		},
		{
			name: "#4: Purge - Successful",
			imageworkstatus: map[string]ImageWorkResult{
				"fakejob": {
					ImageWorkRequest: ImageWorkRequest{
						WorkType: ImageCachePurge,
						Imagecache: &fledgedv1alpha2.ImageCache{
							ObjectMeta: metav1.ObjectMeta{
								Name: imageCacheName,
							},
						},
						Node: &node,
					},
					Status: ImageWorkResultStatusJobCreated,
				},
			},
			pods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: fledgedNameSpace,
						Labels:    map[string]string{"job-name": "fakejob"},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
						ContainerStatuses: []corev1.ContainerStatus{
							{
								State: corev1.ContainerState{
									Waiting: &corev1.ContainerStateWaiting{
										Reason:  "fakereason",
										Message: "fakemessage",
									},
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "#5: Purge - Successful",
			imageworkstatus: map[string]ImageWorkResult{
				"fakejob": {
					ImageWorkRequest: ImageWorkRequest{
						WorkType: ImageCachePurge,
						Imagecache: &fledgedv1alpha2.ImageCache{
							ObjectMeta: metav1.ObjectMeta{
								Name: imageCacheName,
							},
						},
						Node: &node,
					},
					Status: ImageWorkResultStatusJobCreated,
				},
			},
			pods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: fledgedNameSpace,
						Labels:    map[string]string{"job-name": "fakejob"},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
						ContainerStatuses: []corev1.ContainerStatus{
							{
								State: corev1.ContainerState{
									Terminated: &corev1.ContainerStateTerminated{
										Reason:  "fakereason",
										Message: "fakemessage",
									},
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "#6: Purge - Successful",
			imageworkstatus: map[string]ImageWorkResult{
				"fakejob": {
					ImageWorkRequest: ImageWorkRequest{
						WorkType: ImageCachePurge,
						Imagecache: &fledgedv1alpha2.ImageCache{
							ObjectMeta: metav1.ObjectMeta{
								Name: imageCacheName,
							},
						},
						Node: &node,
					},
					Status: ImageWorkResultStatusJobCreated,
				},
			},
			pods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: fledgedNameSpace,
					},
				},
			},
			expectError: false,
		},
		{
			name: "#7: Purge - Unsuccessful",
			imageworkstatus: map[string]ImageWorkResult{
				"fakejob": {
					ImageWorkRequest: ImageWorkRequest{
						WorkType: ImageCachePurge,
						Imagecache: &fledgedv1alpha2.ImageCache{
							ObjectMeta: metav1.ObjectMeta{
								Name: imageCacheName,
							},
						},
						Node: &node,
					},
					Status: ImageWorkResultStatusJobCreated,
				},
			},
			pods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: fledgedNameSpace,
						Labels:    map[string]string{"job-name": "fakejob"},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: fledgedNameSpace,
						Labels:    map[string]string{"job-name": "fakejob"},
					},
				},
			},
			expectError:         true,
			expectedErrorString: "more than one pod matched job",
		},
		{
			name: "#8: Create - Successful",
			imageworkstatus: map[string]ImageWorkResult{
				"fakejob": {
					ImageWorkRequest: ImageWorkRequest{
						Imagecache: &fledgedv1alpha2.ImageCache{
							ObjectMeta: metav1.ObjectMeta{
								Name: imageCacheName,
							},
						},
						Node: &node,
					},
					Status: ImageWorkResultStatusSucceeded,
				},
			},
			pods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: fledgedNameSpace,
						Labels:    map[string]string{"job-name": "fakejob"},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
						ContainerStatuses: []corev1.ContainerStatus{
							{
								State: corev1.ContainerState{
									Waiting: &corev1.ContainerStateWaiting{
										Reason:  "fakereason",
										Message: "fakemessage",
									},
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
	}

	for _, test := range tests {
		fakekubeclientset := &fakeclientset.Clientset{}
		if test.eventListErr {
			fakekubeclientset.AddReactor("list", "events", func(action core.Action) (handled bool, ret runtime.Object, err error) {
				return true, nil, apierrors.NewInternalError(fmt.Errorf("fake error"))
			})
		}
		if test.jobDeleteErr {
			fakekubeclientset.AddReactor("delete", "jobs", func(action core.Action) (handled bool, ret runtime.Object, err error) {
				return true, nil, apierrors.NewInternalError(fmt.Errorf("fake error"))
			})
		}
		imagemanager, podInformer := newTestImageManager(fakekubeclientset, "IfNotPresent", "sa-kube-fledged", false, "priority-class-kube-fledged", false)
		for _, pod := range test.pods {
			if !reflect.DeepEqual(pod, corev1.Pod{}) {
				podInformer.Informer().GetIndexer().Add(&pod)
			}
		}
		imagemanager.imageworkstatus = test.imageworkstatus
		errCh := make(chan error)
		go imagemanager.updateImageCacheStatus(imageCache, errCh)
		err := <-errCh
		if err != nil {
			t.Logf("err=%s", err.Error())
		}
		if test.expectError {
			if err == nil {
				t.Errorf("Test: %s failed: expectedError=%s, actualError=nil", test.name, test.expectedErrorString)
			}
			if err != nil && !strings.HasPrefix(err.Error(), test.expectedErrorString) {
				t.Errorf("Test: %s failed: expectedError=%s, actualError=%s", test.name, test.expectedErrorString, err.Error())
			}
		} else if err != nil {
			t.Errorf("Test: %s failed. expectedError=nil, actualError=%s", test.name, err.Error())
		}
	}
}

func TestProcessNextWorkItem(t *testing.T) {
	defaultImageCache := fledgedv1alpha2.ImageCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "kube-fledged",
		},
		Spec: fledgedv1alpha2.ImageCacheSpec{
			CacheSpec: []fledgedv1alpha2.CacheSpecImages{
				{
					Images: []string{"foo"},
				},
			},
		},
	}
	testnode := node
	testnode.Status.Images = []corev1.ContainerImage{
		{
			Names: []string{"foo:v1"},
		},
	}
	tests := []struct {
		name                string
		iwr                 ImageWorkRequest
		imageworkstatus     map[string]ImageWorkResult
		pods                []corev1.Pod
		imagepullpolicy     string
		expectError         bool
		expectedErrorString string
	}{
		{
			name: "#1: Create - Successful",
			iwr: ImageWorkRequest{
				Image:      "fakeimage",
				Node:       &node,
				WorkType:   ImageCacheCreate,
				Imagecache: &defaultImageCache,
			},
			imageworkstatus: map[string]ImageWorkResult{
				"fakejob": {
					ImageWorkRequest: ImageWorkRequest{
						Imagecache: &fledgedv1alpha2.ImageCache{
							ObjectMeta: metav1.ObjectMeta{
								Name: defaultImageCache.Name,
							},
						},
					},
					Status: ImageWorkResultStatusSucceeded,
				},
			},
			pods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: fledgedNameSpace,
						Labels:    map[string]string{"job-name": "fakejob"},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
						ContainerStatuses: []corev1.ContainerStatus{
							{
								State: corev1.ContainerState{
									Terminated: &corev1.ContainerStateTerminated{
										Reason:  "fakereason",
										Message: "fakemessage",
									},
								},
							},
						},
					},
				},
			},
			imagepullpolicy: "IfNotPresent",
			expectError:     false,
		},
		{
			name: "#2: Purge - Successful",
			iwr: ImageWorkRequest{
				Image:      "fakeimage",
				Node:       &node,
				WorkType:   ImageCachePurge,
				Imagecache: &defaultImageCache,
			},
			imageworkstatus: map[string]ImageWorkResult{
				"fakejob": {
					ImageWorkRequest: ImageWorkRequest{
						Imagecache: &fledgedv1alpha2.ImageCache{
							ObjectMeta: metav1.ObjectMeta{
								Name: defaultImageCache.Name,
							},
						},
					},
					Status: ImageWorkResultStatusSucceeded,
				},
			},
			pods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: fledgedNameSpace,
						Labels:    map[string]string{"job-name": "fakejob"},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
						ContainerStatuses: []corev1.ContainerStatus{
							{
								State: corev1.ContainerState{
									Terminated: &corev1.ContainerStateTerminated{
										Reason:  "fakereason",
										Message: "fakemessage",
									},
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "#3: Statusupdate - Successful",
			iwr: ImageWorkRequest{
				WorkType:   ImageCacheCreate,
				Imagecache: &defaultImageCache,
			},
			imageworkstatus: map[string]ImageWorkResult{
				"fakejob": {
					ImageWorkRequest: ImageWorkRequest{
						Imagecache: &fledgedv1alpha2.ImageCache{
							ObjectMeta: metav1.ObjectMeta{
								Name: defaultImageCache.Name,
							},
						},
					},
					Status: ImageWorkResultStatusSucceeded,
				},
			},
			pods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: fledgedNameSpace,
						Labels:    map[string]string{"job-name": "fakejob"},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
						ContainerStatuses: []corev1.ContainerStatus{
							{
								State: corev1.ContainerState{
									Terminated: &corev1.ContainerStateTerminated{
										Reason:  "fakereason",
										Message: "fakemessage",
									},
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name:                "#4: Create - Unsuccessful",
			expectError:         false,
			expectedErrorString: "Unexpected type in workqueue",
		},
		{
			name: "#5: Create - Successful (Image not present in Node)",
			iwr: ImageWorkRequest{
				Image:      "foo:v1",
				Node:       &node,
				WorkType:   ImageCacheCreate,
				Imagecache: &defaultImageCache,
			},
			imageworkstatus: map[string]ImageWorkResult{
				"fakejob": {
					ImageWorkRequest: ImageWorkRequest{
						Imagecache: &fledgedv1alpha2.ImageCache{
							ObjectMeta: metav1.ObjectMeta{
								Name: defaultImageCache.Name,
							},
						},
					},
					Status: ImageWorkResultStatusSucceeded,
				},
			},
			pods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: fledgedNameSpace,
						Labels:    map[string]string{"job-name": "fakejob"},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
						ContainerStatuses: []corev1.ContainerStatus{
							{
								State: corev1.ContainerState{
									Terminated: &corev1.ContainerStateTerminated{
										Reason:  "fakereason",
										Message: "fakemessage",
									},
								},
							},
						},
					},
				},
			},
			imagepullpolicy: "IfNotPresent",
			expectError:     false,
		},
		{
			name: "#6: Create - Successful (Image already present in Node)",
			iwr: ImageWorkRequest{
				Image:      "foo:v1",
				Node:       &testnode,
				WorkType:   ImageCacheCreate,
				Imagecache: &defaultImageCache,
			},
			imageworkstatus: map[string]ImageWorkResult{
				"fakejob": {
					ImageWorkRequest: ImageWorkRequest{
						Imagecache: &fledgedv1alpha2.ImageCache{
							ObjectMeta: metav1.ObjectMeta{
								Name: defaultImageCache.Name,
							},
						},
					},
					Status: ImageWorkResultStatusSucceeded,
				},
			},
			pods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: fledgedNameSpace,
						Labels:    map[string]string{"job-name": "fakejob"},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
						ContainerStatuses: []corev1.ContainerStatus{
							{
								State: corev1.ContainerState{
									Terminated: &corev1.ContainerStateTerminated{
										Reason:  "fakereason",
										Message: "fakemessage",
									},
								},
							},
						},
					},
				},
			},
			imagepullpolicy: "IfNotPresent",
			expectError:     false,
		},
	}
	for _, test := range tests {
		fakekubeclientset := &fakeclientset.Clientset{}
		imagemanager, podInformer := newTestImageManager(fakekubeclientset, test.imagepullpolicy, "sa-kube-fledged", false, "priority-class-kube-fledged", false)
		for _, pod := range test.pods {
			if !reflect.DeepEqual(pod, corev1.Pod{}) {
				podInformer.Informer().GetIndexer().Add(&pod)
			}
		}
		imagemanager.imageworkstatus = test.imageworkstatus
		if test.expectedErrorString == "Unexpected type in workqueue" {
			imagemanager.imageworkqueue.Add(struct{}{})
		}
		imagemanager.imageworkqueue.Add(test.iwr)
		imagemanager.processNextWorkItem()
		var err error
		if test.expectError {
			if err == nil {
				t.Errorf("Test: %s failed: expectedError=%s, actualError=nil", test.name, test.expectedErrorString)
			}
			if err != nil && !strings.HasPrefix(err.Error(), test.expectedErrorString) {
				t.Errorf("Test: %s failed: expectedError=%s, actualError=%s", test.name, test.expectedErrorString, err.Error())
			}
		} else if err != nil {
			t.Errorf("Test: %s failed. expectedError=nil, actualError=%s", test.name, err.Error())
		}
	}
}
