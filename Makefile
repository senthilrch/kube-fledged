# Copyright 2018 The kube-fledged authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

.PHONY: clean image push deploy update
# Default tag and architecture. Can be overridden
TAG?=$(shell git describe --tags --dirty)
ARCH?=amd64
# Only enable CGO (and build the UDP backend) on AMD64
ifeq ($(ARCH),amd64)
	CGO_ENABLED=1
else
	CGO_ENABLED=0
endif

# Go version to use for builds
GO_VERSION=1.11.1

# K8s version used for Makefile helpers
K8S_VERSION=v1.12.0

GOARM=7

ifndef FLEDGED_IMAGE_NAME
  FLEDGED_IMAGE_NAME=senthilrch/fledged:latest
endif

### BUILDING
clean:
	rm -f build/fledged* && \
	rm -f dist/*.tar.gz || \
	docker image rm $(FLEDGED_IMAGE_NAME)

fledged:
	CGO_ENABLED=0 go build -o build/fledged \
	  -ldflags '-s -w -extldflags "-static"' cmd/fledged.go

image: clean fledged
	cd build && docker build -t $(FLEDGED_IMAGE_NAME) . && \
	docker save -o fledged.tar $(FLEDGED_IMAGE_NAME) && \
	gzip fledged.tar

push:
	docker push $(FLEDGED_IMAGE_NAME)

deploy:
	kubectl apply -f deploy/fledged-crd.yaml && sleep 2 && \
	kubectl apply -f deploy/fledged-namespace.yaml && sleep 2 && \
	kubectl apply -f deploy/fledged-serviceaccount.yaml && \
	kubectl apply -f deploy/fledged-clusterrole.yaml && \
	kubectl apply -f deploy/fledged-clusterrolebinding.yaml && \
	kubectl apply -f deploy/fledged-deployment.yaml

update:
	kubectl scale deployment fledged --replicas=0 -n kube-fledged && sleep 5 && \
	kubectl delete jobs -l app=imagecache -n kube-fledged && sleep 5 && \
	kubectl scale deployment fledged --replicas=1 -n kube-fledged && sleep 5 && \
	kubectl get pods -l app=fledged -n kube-fledged
