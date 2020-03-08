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

.PHONY: clean clean-fledged clean-client clean-operator fledged-image client-image operator-image build-images push-images test deploy update remove
# Default tag and architecture. Can be overridden
TAG?=$(shell git describe --tags --dirty)
ARCH?=amd64
# Only enable CGO (and build the UDP backend) on AMD64
ifeq ($(ARCH),amd64)
	CGO_ENABLED=1
else
	CGO_ENABLED=0
endif

GOARM=7

ifndef FLEDGED_IMAGE_REPO
  FLEDGED_IMAGE_REPO=docker.io/senthilrch/fledged
endif

ifndef FLEDGED_DOCKER_CLIENT_IMAGE_REPO
  FLEDGED_DOCKER_CLIENT_IMAGE_REPO=docker.io/senthilrch/fledged-docker-client
endif

ifndef OPERATOR_IMAGE_REPO
  OPERATOR_IMAGE_REPO=docker.io/senthilrch/kubefledged-operator
endif

ifndef RELEASE_VERSION
  RELEASE_VERSION=v0.6.0
endif

ifndef DOCKER_VERSION
  DOCKER_VERSION=19.03.7
endif

ifndef CRICTL_VERSION
  CRICTL_VERSION=v1.17.0
endif

ifndef GOLANG_VERSION
  GOLANG_VERSION=1.13.8
endif

ifndef ALPINE_VERSION
  ALPINE_VERSION=3.11
endif

ifndef OPERATORSDK_VERSION
  OPERATORSDK_VERSION=v0.15.2
endif

ifndef GIT_BRANCH
  GIT_BRANCH=master
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
clean: clean-fledged clean-client clean-operator

clean-fledged:
	-rm -f build/fledged
	-docker image rm $(FLEDGED_IMAGE_REPO):$(RELEASE_VERSION)
	-docker image rm `docker image ls -f dangling=true -q`

clean-client:
	-docker image rm $(FLEDGED_DOCKER_CLIENT_IMAGE_REPO):$(RELEASE_VERSION)
	-docker image rm `docker image ls -f dangling=true -q`

clean-operator:
	-docker image rm $(OPERATOR_IMAGE_REPO):$(RELEASE_VERSION)
	-docker image rm `docker image ls -f dangling=true -q`

fledged:
	CGO_ENABLED=0 go build -o build/fledged \
	-ldflags '-s -w -extldflags "-static"' cmd/fledged.go

fledged-image: clean-fledged
	cd build && docker build -t $(FLEDGED_IMAGE_REPO):$(RELEASE_VERSION) -f Dockerfile.fledged \
	${HTTP_PROXY_CONFIG} ${HTTPS_PROXY_CONFIG} --build-arg GIT_BRANCH=${GIT_BRANCH} \
	--build-arg GOLANG_VERSION=${GOLANG_VERSION} --build-arg ALPINE_VERSION=${ALPINE_VERSION} .

client-image: clean-client
	cd build && docker build -t $(FLEDGED_DOCKER_CLIENT_IMAGE_REPO):$(RELEASE_VERSION) \
	-f Dockerfile.docker_client  ${HTTP_PROXY_CONFIG} ${HTTPS_PROXY_CONFIG} \
	--build-arg DOCKER_VERSION=${DOCKER_VERSION} --build-arg CRICTL_VERSION=${CRICTL_VERSION} \
	--build-arg ALPINE_VERSION=${ALPINE_VERSION} .

operator-image: clean-operator
	cd deploy/kubefledged-operator && \
	docker build -t $(OPERATOR_IMAGE_REPO):$(RELEASE_VERSION) -f ./build/Dockerfile \
	--build-arg OPERATORSDK_VERSION=${OPERATORSDK_VERSION} .

build-images: fledged-image client-image operator-image

push-images:
	-docker push $(FLEDGED_IMAGE_REPO):$(RELEASE_VERSION)
	-docker push $(FLEDGED_DOCKER_CLIENT_IMAGE_REPO):$(RELEASE_VERSION)
	-docker push $(OPERATOR_IMAGE_REPO):$(RELEASE_VERSION)

release: build-images push-images

test:
	-rm -f coverage.out
	bash hack/run-unit-tests.sh

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

remove:
	kubectl delete -f deploy/fledged-namespace.yaml && \
	kubectl delete -f deploy/fledged-clusterrolebinding.yaml && \
	kubectl delete -f deploy/fledged-clusterrole.yaml && \
	kubectl delete -f deploy/fledged-crd.yaml

