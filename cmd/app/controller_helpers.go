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

	"github.com/golang/glog"
	fledgedv1alpha1 "github.com/senthilrch/kube-fledged/pkg/apis/fledged/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func validateCacheSpec(c *Controller, imageCache *fledgedv1alpha1.ImageCache) error {
	if imageCache == nil {
		glog.Errorf("Unable to obtain reference to image cache")
		return fmt.Errorf("Unable to obtain reference to image cache")
	}

	cacheSpec := imageCache.Spec.CacheSpec
	glog.V(4).Infof("cacheSpec: %+v", cacheSpec)
	var nodes []*corev1.Node
	var err error

	for _, i := range cacheSpec {
		if len(i.NodeSelector) > 0 {
			if nodes, err = c.nodesLister.List(labels.Set(i.NodeSelector).AsSelector()); err != nil {
				glog.Errorf("Error listing nodes using nodeselector %+v: %v", i.NodeSelector, err)
				return err
			}
		} else {
			if nodes, err = c.nodesLister.List(labels.Everything()); err != nil {
				glog.Errorf("Error listing nodes using nodeselector labels.Everything(): %v", err)
				return err
			}
		}
		glog.V(4).Infof("No. of nodes in %+v is %d", i.NodeSelector, len(nodes))
		if len(nodes) == 0 {
			glog.Errorf("NodeSelector %s did not match any nodes.", labels.Set(i.NodeSelector).String())
			return fmt.Errorf("NodeSelector %s did not match any nodes", labels.Set(i.NodeSelector).String())
		}

		if len(i.Images) == 0 {
			glog.Error("No images specified within image list")
			return fmt.Errorf("No images specified within image list")

		}

		for m := range i.Images {
			for p := 0; p < m; p++ {
				if i.Images[p] == i.Images[m] {
					glog.Errorf("Duplicate image names within image list: %s", i.Images[m])
					return fmt.Errorf("Duplicate image names within image list: %s", i.Images[m])
				}
			}
		}
	}

	return nil
}
