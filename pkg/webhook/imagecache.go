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

package webhook

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/golang/glog"
	fledgedv1alpha2 "github.com/senthilrch/kube-fledged/pkg/apis/kubefledged/v1alpha2"
	v1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	customResourcePatch1 string = `[
         { "op": "add", "path": "/data/mutation-stage-1", "value": "yes" }
     ]`
	customResourcePatch2 string = `[
         { "op": "add", "path": "/data/mutation-stage-2", "value": "yes" }
     ]`
)

const imageCachePurgeAnnotationKey = "kubefledged.io/purge-imagecache"
const imageCacheRefreshAnnotationKey = "kubefledged.io/refresh-imagecache"

// MutateImageCache modifies image cache resource
/*
func MutateImageCache(ar v1.AdmissionReview) *v1.AdmissionResponse {
	glog.V(4).Info("mutating custom resource")
	cr := struct {
		metav1.ObjectMeta
		Data map[string]string
	}{}

	raw := ar.Request.Object.Raw
	err := json.Unmarshal(raw, &cr)
	if err != nil {
		glog.Error(err)
		return toV1AdmissionResponse(err)
	}

	reviewResponse := v1.AdmissionResponse{}
	reviewResponse.Allowed = true

	if cr.Data["mutation-start"] == "yes" {
		reviewResponse.Patch = []byte(customResourcePatch1)
	}
	if cr.Data["mutation-stage-1"] == "yes" {
		reviewResponse.Patch = []byte(customResourcePatch2)
	}
	if len(reviewResponse.Patch) != 0 {
		pt := v1.PatchTypeJSONPatch
		reviewResponse.PatchType = &pt
	}
	return &reviewResponse
}
*/

// ValidateImageCache validates image cache resource
func ValidateImageCache(ar v1.AdmissionReview) *v1.AdmissionResponse {
	glog.V(4).Info("admitting image cache")
	reviewResponse := v1.AdmissionResponse{}
	reviewResponse.Allowed = true

	oldImageCache, imageCache, err := extractImageCachesFromAdmissionReview(ar)
	if err != nil {
		return toV1AdmissionResponse(err)
	}

	cacheSpec := imageCache.Spec.CacheSpec
	glog.V(4).Infof("cacheSpec: %+v", cacheSpec)

	for _, i := range cacheSpec {
		if len(i.Images) == 0 {
			glog.Error("No images specified within image list")
			return toV1AdmissionResponse(fmt.Errorf("No images specified within image list"))
		}

		for m := range i.Images {
			for p := 0; p < m; p++ {
				if i.Images[p] == i.Images[m] {
					glog.Errorf("Duplicate image names within image list: %s", i.Images[m])
					return toV1AdmissionResponse(fmt.Errorf("Duplicate image names within image list: %s", i.Images[m]))
				}
			}
		}
	}

	if ar.Request.Operation == v1.Update {
		if len(oldImageCache.Spec.CacheSpec) != len(imageCache.Spec.CacheSpec) {
			glog.Errorf("Mismatch in no. of image lists")
			return toV1AdmissionResponse(fmt.Errorf("Mismatch in no. of image lists"))
		}

		for i := range oldImageCache.Spec.CacheSpec {
			if !reflect.DeepEqual(oldImageCache.Spec.CacheSpec[i].NodeSelector, imageCache.Spec.CacheSpec[i].NodeSelector) {
				glog.Errorf("Mismatch in node selector")
				return toV1AdmissionResponse(fmt.Errorf("Mismatch in node selector"))
			}
		}

		if oldImageCache.Status.Status == fledgedv1alpha2.ImageCacheActionStatusProcessing {
			if !reflect.DeepEqual(oldImageCache.Spec, imageCache.Spec) {
				glog.Error("Previous image cache operation under processing. New operation not allowed now")
				return toV1AdmissionResponse(fmt.Errorf("Previous image cache operation under processing. New operation not allowed now"))
			}
			if _, ok := oldImageCache.Annotations[imageCachePurgeAnnotationKey]; !ok {
				if _, ok := imageCache.Annotations[imageCachePurgeAnnotationKey]; ok {
					glog.Error("Previous image cache operation under processing. New operation not allowed now")
					return toV1AdmissionResponse(fmt.Errorf("Previous image cache operation under processing. New operation not allowed now"))
				}
			}
			if _, ok := oldImageCache.Annotations[imageCacheRefreshAnnotationKey]; !ok {
				if _, ok := imageCache.Annotations[imageCacheRefreshAnnotationKey]; ok {
					glog.Error("Previous image cache operation under processing. New operation not allowed now")
					return toV1AdmissionResponse(fmt.Errorf("Previous image cache operation under processing. New operation not allowed now"))
				}
			}
		}
	}

	glog.Info("Image cache creation/update validated successfully")
	return &reviewResponse
}

func toV1AdmissionResponse(err error) *v1.AdmissionResponse {
	return &v1.AdmissionResponse{
		Result: &metav1.Status{
			Message: err.Error(),
		},
	}
}

func extractImageCachesFromAdmissionReview(ar v1.AdmissionReview) (*fledgedv1alpha2.ImageCache, *fledgedv1alpha2.ImageCache, error) {
	var raw, oldraw []byte
	var imageCache, oldImageCache fledgedv1alpha2.ImageCache

	raw = ar.Request.Object.Raw
	err := json.Unmarshal(raw, &imageCache)
	if err != nil {
		glog.Error(err)
		return nil, nil, err
	}

	if ar.Request.Operation == v1.Update {
		oldraw = ar.Request.OldObject.Raw
		err := json.Unmarshal(oldraw, &oldImageCache)
		if err != nil {
			glog.Error(err)
			return nil, nil, err
		}
	}

	return &oldImageCache, &imageCache, nil
}
