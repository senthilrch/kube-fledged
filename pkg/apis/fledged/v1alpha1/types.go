/*
Copyright 2017 The Kubernetes Authors.

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

package v1alpha1

import (
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ImageCache is a specification for a ImageCache resource
type ImageCache struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ImageCacheSpec   `json:"spec"`
	Status ImageCacheStatus `json:"status"`
}

// CacheSpecImages specifies the Images to be cached
type CacheSpecImages struct {
	Images       []string        `json:"images"`
	NodeSelector v1.NodeSelector `json:"nodeSelector,omitempty"`
}

// ImageCacheSpec is the spec for a ImageCache resource
type ImageCacheSpec struct {
	CacheSpec []CacheSpecImages `json:"cacheSpec"`
}

// ImageCacheStatus is the status for a ImageCache resource
type ImageCacheStatus struct {
	Status  ImageCacheActionStatus `json:"status"`
	Reason  string                 `json:"reason"`
	Message string                 `json:"message"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ImageCacheList is a list of ImageCache resources
type ImageCacheList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []ImageCache `json:"items"`
}

// ImageCacheActionStatus defines the status of ImageCacheAction
type ImageCacheActionStatus string

// List of constants for ImageCacheActionStatus
const (
	ImageCacheActionStatusSucceeded ImageCacheActionStatus = "Succeeded"
	ImageCacheActionStatusFailed    ImageCacheActionStatus = "Failed"
	ImageCacheActionStatusUnknown   ImageCacheActionStatus = "Unknown"
)

// List of constants for ImageCacheReason
const (
	ImageCacheReasonImagesPulledSuccessfully   = "ImagesPulledSuccessfully"
	ImageCacheReasonImagePullFailedOnAllNodes  = "ImagePullFailedOnAllNodes"
	ImageCacheReasonImagePullFailedOnSomeNodes = "ImagePullFailedOnSomeNodes"
	ImageCacheReasonImagePullStatusUnknown     = "ImagePullStatusUnknown"
)

// List of constants for ImageCacheMessage
const (
	ImageCacheMessageImagesPulledSuccessfully   = "All requested images pulled succesfuly to respective nodes"
	ImageCacheMessageImagePullFailedOnAllNodes  = "Image pull failed on all nodes. Please query the jobs using label selector"
	ImageCacheMessageImagePullFailedOnSomeNodes = "Image pull failed on some nodes. Please query the jobs using label selector"
	ImageCacheMessageImagePullStatusUnknown     = "Unable to get the status of Image pull. Retry after some time or contact cluster administrator"
)
