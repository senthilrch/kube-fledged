/*
Copyright 2016 The Kubernetes Authors.

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
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/flowcontrol"
)

type PullResult struct {
	image string
	err      error
}

type ImagePuller interface {
	limiter flowcontrol.RateLimiter
	pullImage(string, chan<- pullResult)
}

var _ imagePuller = &parallelImagePuller{}

type parallelImagePuller struct {
	imagePuller
}

func newParallelImagePuller(qps float32, burst int) imagePuller {
	limiter := flowcontrol.NewTokenBucketRateLimiter(qps, burst)
	return &parallelImagePuller{limiter: limiter}
}

func (pip *parallelImagePuller) pullImage(image string, pullChan chan<- PullResult) error {
	if !ts.limiter.TryAccept() {
		return fmt.Errorf("pull QPS exceeded.")
	}
	go func() {
		imageRef, err := pip.imageService.PullImage(image)
		pullChan <- PullResult{
			imageRef: imageRef,
			err:      err,
		}
	}()
	return nil
}
