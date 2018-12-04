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
	"strings"
	"testing"
	"time"

	fledgedv1alpha1 "github.com/senthilrch/kube-fledged/pkg/apis/fledged/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/util/workqueue"
)

func newTestImageManager(kubeclientset kubernetes.Interface) *ImageManager {
	imagePullDeadlineDuration := time.Second * 5
	dockerClientImage := "senthilrch/fledged-docker-client:latest"
	imagePullPolicy := "IfNotPresent"
	imagecacheworkqueue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ImageCaches")
	imageworkqueue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ImagePullerStatus")

	imagemanager := NewImageManager(imagecacheworkqueue, imageworkqueue, kubeclientset, fledgedNameSpace,
		imagePullDeadlineDuration, dockerClientImage, imagePullPolicy)
	imagemanager.podsSynced = func() bool { return true }

	return imagemanager
}

func TestPullDeleteImage(t *testing.T) {
	job := batchv1.Job{}
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
				Node:       "bar",
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
				Node:       "bar",
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
				Node:       "bar",
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
				Node:       "bar",
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
				Node:       "bar",
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
				Node:       "bar",
				WorkType:   ImageCachePurge,
				Imagecache: &defaultImageCache,
			},
			expectError:         true,
			expectedErrorString: "Internal error occurred: fake error",
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

		imagemanager := newTestImageManager(fakekubeclientset)
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
		imagemanager := newTestImageManager(fakekubeclientset)
		imagemanager.imageworkstatus[test.pod.Labels["job-name"]] = ImageWorkResult{
			Status: ImageWorkResultStatusJobCreated,
			ImageWorkRequest: ImageWorkRequest{
				WorkType: test.worktype,
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
