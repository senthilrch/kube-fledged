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

	fledgedv1alpha1 "github.com/senthilrch/kube-fledged/pkg/apis/fledged/v1alpha1"
	fledgedclientsetfake "github.com/senthilrch/kube-fledged/pkg/client/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
)

var (
	alwaysReady = func() bool { return true }
)

func TestValidateCacheSpec(t *testing.T) {
	//var fakekubeclientset *fakeclientset.Clientset
	//var fakefledgedclientset *fledgedclientsetfake.Clientset

	tests := []struct {
		name          string
		imageCache    *fledgedv1alpha1.ImageCache
		nodeList      *corev1.NodeList
		nodeListError error
		expectErr     bool
		errorString   string
	}{
		{
			name:          "Unable to obtain reference to image cache",
			imageCache:    nil,
			nodeList:      nil,
			nodeListError: nil,
			expectErr:     true,
			errorString:   "Unable to obtain reference to image cache",
		},
		{
			name: "No images specified within image list",
			imageCache: &fledgedv1alpha1.ImageCache{
				Spec: fledgedv1alpha1.ImageCacheSpec{
					CacheSpec: []fledgedv1alpha1.CacheSpecImages{
						{
							Images: []string{},
						},
					},
				},
			},
			nodeList:      nil,
			nodeListError: nil,
			expectErr:     true,
			errorString:   "No images specified within image list",
		},
		{
			name: "Duplicate image names within image list",
			imageCache: &fledgedv1alpha1.ImageCache{
				Spec: fledgedv1alpha1.ImageCacheSpec{
					CacheSpec: []fledgedv1alpha1.CacheSpecImages{
						{
							Images: []string{"foo", "foo"},
						},
					},
				},
			},
			nodeList:      nil,
			nodeListError: nil,
			expectErr:     true,
			errorString:   "Duplicate image names within image list",
		},
		{
			name: "Error listing nodes using nodeselector",
			imageCache: &fledgedv1alpha1.ImageCache{
				Spec: fledgedv1alpha1.ImageCacheSpec{
					CacheSpec: []fledgedv1alpha1.CacheSpecImages{
						{
							Images:       []string{"foo"},
							NodeSelector: map[string]string{"foo": "bar"},
						},
					},
				},
			},
			nodeList:      nil,
			nodeListError: fmt.Errorf("fake error"),
			expectErr:     true,
			errorString:   "Error listing nodes using nodeselector",
		},
		{
			name: "Error listing nodes using nodeselector labels.Everything()",
			imageCache: &fledgedv1alpha1.ImageCache{
				Spec: fledgedv1alpha1.ImageCacheSpec{
					CacheSpec: []fledgedv1alpha1.CacheSpecImages{
						{
							Images: []string{"foo"},
						},
					},
				},
			},
			nodeList:      nil,
			nodeListError: fmt.Errorf("fake error"),
			expectErr:     true,
			errorString:   "Error listing nodes using nodeselector labels.Everything()",
		},
		{
			name: "NodeSelector %s did not match any nodes",
			imageCache: &fledgedv1alpha1.ImageCache{
				Spec: fledgedv1alpha1.ImageCacheSpec{
					CacheSpec: []fledgedv1alpha1.CacheSpecImages{
						{
							Images:       []string{"foo"},
							NodeSelector: map[string]string{"foo": "bar"},
						},
					},
				},
			},
			nodeList:      &corev1.NodeList{},
			nodeListError: nil,
			expectErr:     true,
			errorString:   "NodeSelector %s did not match any nodes",
		},
		{
			name: "Successful validation",
			imageCache: &fledgedv1alpha1.ImageCache{
				Spec: fledgedv1alpha1.ImageCacheSpec{
					CacheSpec: []fledgedv1alpha1.CacheSpecImages{
						{
							Images:       []string{"foo"},
							NodeSelector: map[string]string{"foo": "bar"},
						},
					},
				},
			},
			nodeList:      &corev1.NodeList{Items: []corev1.Node{}},
			nodeListError: nil,
			expectErr:     false,
			errorString:   "",
		},
	}

	for _, test := range tests {
		fakekubeclientset := &fakeclientset.Clientset{}
		fakefledgedclientset := &fledgedclientsetfake.Clientset{}

		if test.nodeList != nil || test.nodeListError != nil {
			/* fakekubeclientset.AddReactor("list", "nodes", func(action core.Action) (handled bool, ret runtime.Object, err error) {
				return true, test.nodeList, apierrors.NewInternalError(test.nodeListError)
			}) */
			continue
		}
		controller := newTestController(fakekubeclientset, fakefledgedclientset)

		err := validateCacheSpec(controller, test.imageCache)
		if !(test.expectErr == true && err != nil && strings.HasPrefix(err.Error(), test.errorString)) {
			t.Errorf("Test: %s failed", test.name)
		}
		if !test.expectErr && err != nil {
			t.Errorf("Test: %s failed. err received = %s", test.name, err.Error())
		}
	}
}
