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

// imageManager provides the functionalities for image pulling and image removal
type ImageManager struct {
	pullChan chan<- PullResult
	puller   ImagePuller
}

func NewImageManager(imageService ImageService, serialized bool, qps float32, burst int) ImageManager {
	var puller ImagePuller
	puller = newParallelImagePuller(imageService)
	pullChan := make(chan PullResult)
	return &ImageManager{
		puller:   puller,
		pullChan: pullChan,
	}
}

// PullImageToNode pulls the image to the node. Performs retries based on error
func (m *imageManager) PullImageToNode(image string) (string, string, error) {
	m.puller.pullImage(image, m.pullChan)
	/*
		if imagePullResult.err != nil {
			if imagePullResult.err == RegistryUnavailable {
				msg := fmt.Sprintf("image pull failed for %s because the registry is unavailable.", container.Image)
				return "", msg, imagePullResult.err
			}

			return "", imagePullResult.err.Error(), ErrImagePull
		}
		return imagePullResult.imageRef, "", nil
	*/
	return "", "", nil
}
