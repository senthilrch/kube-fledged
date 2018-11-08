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

.PHONY: clean clean-fledged clean-client fledged-image client-image push-images deploy update
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

ifndef FLEDGED_DOCKER_CLIENT_IMAGE_NAME
  FLEDGED_DOCKER_CLIENT_IMAGE_NAME=senthilrch/fledged-docker-client:latest
endif

ifndef DOCKER_VERSION
  DOCKER_VERSION=18.06.1-ce
endif

HTTP_PROXY_CONFIG=
ifdef HTTP_PROXY
  HTTP_PROXY_CONFIG=--build-arg http_proxy=${HTTP_PROXY}
endif

HTTPS_PROXY_CONFIG=
ifdef HTTPS_PROXY
  HTTPS_PROXY_CONFIG=--build-arg https_proxy=${HTTPS_PROXY}
endif


### BUILDING
clean: clean-fledged clean-client

clean-fledged:
	-rm -f build/fledged
	-rm -f build/fledged.tar.gz
	-docker image rm $(FLEDGED_IMAGE_NAME)
	-docker image rm `docker image ls -f dangling=true -q`

clean-client:
	-rm -f build/fledged-docker-client.tar.gz
	-docker image rm $(FLEDGED_DOCKER_CLIENT_IMAGE_NAME)
	-docker image rm `docker image ls -f dangling=true -q`

fledged:
	CGO_ENABLED=0 go build -o build/fledged \
	  -ldflags '-s -w -extldflags "-static"' cmd/fledged.go

fledged-image: clean-fledged fledged
	cd build && docker build -t $(FLEDGED_IMAGE_NAME) . && \
	docker save -o fledged.tar $(FLEDGED_IMAGE_NAME) && \
	gzip fledged.tar

client-image: clean-client
	cd build && docker build -t $(FLEDGED_DOCKER_CLIENT_IMAGE_NAME) \
	-f Dockerfile.docker_client  ${HTTP_PROXY_CONFIG} ${HTTPS_PROXY_CONFIG} \
	--build-arg VERSION=${DOCKER_VERSION} . && 	\
	docker save -o fledged-docker-client.tar $(FLEDGED_DOCKER_CLIENT_IMAGE_NAME) && \
	gzip fledged-docker-client.tar 

push-image:
	-docker push $(FLEDGED_IMAGE_NAME)
	-docker push $(FLEDGED_DOCKER_CLIENT_IMAGE_NAME)

deploy:
	kubectl apply -f deploy/fledged-crd.yaml && sleep 2 && \
	kubectl apply -f deploy/fledged-namespace.yaml && sleep 2 && \
	kubectl apply -f deploy/fledged-serviceaccount.yaml && \
	kubectl apply -f deploy/fledged-clusterrole.yaml && \
	kubectl apply -f deploy/fledged-clusterrolebinding.yaml && \
	kubectl apply -f deploy/fledged-deployment.yaml

update:
	kubectl scale deployment fledged --replicas=0 -n kube-fledged && sleep 5 && \
	kubectl scale deployment fledged --replicas=1 -n kube-fledged && sleep 5 && \
	kubectl get pods -l app=fledged -n kube-fledged
