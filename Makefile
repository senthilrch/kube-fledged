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

.PHONY: clean clean-fledged clean-client clean-operator fledged-amd64 fledged-image client-image operator-image build-images push-images test deploy update remove
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
  RELEASE_VERSION=v0.7.0
endif

ifndef DOCKER_VERSION
  DOCKER_VERSION=19.03.8
endif

ifndef CRICTL_VERSION
  CRICTL_VERSION=v1.18.0
endif

ifndef GOLANG_VERSION
  GOLANG_VERSION=1.14.2
endif

ifndef ALPINE_VERSION
  ALPINE_VERSION=3.11.6
endif

ifndef OPERATORSDK_VERSION
  OPERATORSDK_VERSION=v0.17.0
endif

ifndef GIT_BRANCH
  GIT_BRANCH=master
endif

ifndef TARGET_PLATFORMS
  TARGET_PLATFORMS=linux/amd64,linux/arm/v7,linux/arm64/v8
endif

ifndef OPERATOR_TARGET_PLATFORMS
  OPERATOR_TARGET_PLATFORMS=linux/amd64,linux/arm64
endif

ifndef BUILD_OUTPUT
  BUILD_OUTPUT=--push
endif

HTTP_PROXY_CONFIG=
ifdef HTTP_PROXY
  HTTP_PROXY_CONFIG=--build-arg http_proxy=${HTTP_PROXY}
endif

HTTPS_PROXY_CONFIG=
ifdef HTTPS_PROXY
  HTTPS_PROXY_CONFIG=--build-arg https_proxy=${HTTPS_PROXY}
endif


### BUILD
clean: clean-fledged clean-client clean-operator

clean-fledged:
	-rm -f build/fledged
	-docker image rm ${FLEDGED_IMAGE_REPO}:${RELEASE_VERSION}
	-docker image rm `docker image ls -f dangling=true -q`

clean-client:
	-docker image rm ${FLEDGED_DOCKER_CLIENT_IMAGE_REPO}:${RELEASE_VERSION}
	-docker image rm `docker image ls -f dangling=true -q`

clean-operator:
	-docker image rm ${OPERATOR_IMAGE_REPO}:${RELEASE_VERSION}
	-docker image rm `docker image ls -f dangling=true -q`

fledged-amd64: clean-fledged
	CGO_ENABLED=0 go build -o build/fledged -ldflags '-s -w -extldflags "-static"' cmd/fledged.go && \
	cd build && docker build -t ${FLEDGED_IMAGE_REPO}:${RELEASE_VERSION} -f Dockerfile.fledged_dev \
	--build-arg ALPINE_VERSION=${ALPINE_VERSION} .

fledged-dev: fledged-amd64
	docker push ${FLEDGED_IMAGE_REPO}:${RELEASE_VERSION}

fledged-image: clean-fledged
	cd build && docker buildx build --platform=${TARGET_PLATFORMS} -t ${FLEDGED_IMAGE_REPO}:${RELEASE_VERSION} \
	-f Dockerfile.fledged ${HTTP_PROXY_CONFIG} ${HTTPS_PROXY_CONFIG} --build-arg GIT_BRANCH=${GIT_BRANCH} \
	--build-arg GOLANG_VERSION=${GOLANG_VERSION} --build-arg ALPINE_VERSION=${ALPINE_VERSION} --progress=plain ${BUILD_OUTPUT} .

client-image: clean-client
	cd build && docker buildx build --platform=${TARGET_PLATFORMS} -t ${FLEDGED_DOCKER_CLIENT_IMAGE_REPO}:${RELEASE_VERSION} \
	-f Dockerfile.docker_client  ${HTTP_PROXY_CONFIG} ${HTTPS_PROXY_CONFIG} \
	--build-arg DOCKER_VERSION=${DOCKER_VERSION} --build-arg CRICTL_VERSION=${CRICTL_VERSION} \
	--build-arg ALPINE_VERSION=${ALPINE_VERSION} --progress=plain ${BUILD_OUTPUT} .

operator-image: clean-operator
	cd deploy/kubefledged-operator && \
	docker buildx build --platform=${OPERATOR_TARGET_PLATFORMS} -t ${OPERATOR_IMAGE_REPO}:${RELEASE_VERSION} \
	-f ./build/Dockerfile --build-arg OPERATORSDK_VERSION=${OPERATORSDK_VERSION} --progress=plain ${BUILD_OUTPUT} .

push-images:
	-docker push ${FLEDGED_IMAGE_REPO}:${RELEASE_VERSION}
	-docker push ${FLEDGED_DOCKER_CLIENT_IMAGE_REPO}:${RELEASE_VERSION}
	-docker push ${OPERATOR_IMAGE_REPO}:${RELEASE_VERSION}

release: install-buildx fledged-image client-image operator-image

install-buildx:
	docker run --rm --privileged multiarch/qemu-user-static --reset -p yes
	-docker buildx rm multibuilder
	docker buildx create --name multibuilder --driver docker-container --use
	docker buildx inspect --bootstrap
	docker buildx ls

test:
	-rm -f coverage.out
	bash hack/run-unit-tests.sh

deploy-using-yaml:
	kubectl apply -f deploy/kubefledged-crd.yaml && \
	kubectl apply -f deploy/kubefledged-namespace.yaml && \
	kubectl apply -f deploy/kubefledged-serviceaccount.yaml && \
	kubectl apply -f deploy/kubefledged-clusterrole.yaml && \
	kubectl apply -f deploy/kubefledged-clusterrolebinding.yaml && \
	kubectl apply -f deploy/kubefledged-deployment.yaml

deploy-using-operator:
	# Deploy the operator to a separate namespace called "operators"
	sed -i "s|OPERATOR_NAMESPACE|operators|g" deploy/kubefledged-operator/deploy/service_account.yaml
	sed -i "s|OPERATOR_NAMESPACE|operators|g" deploy/kubefledged-operator/deploy/clusterrole_binding.yaml
	sed -i "s|OPERATOR_NAMESPACE|operators|g" deploy/kubefledged-operator/deploy/operator.yaml
	-kubectl create namespace operators
	kubectl create -f deploy/kubefledged-operator/deploy/crds/charts.helm.k8s.io_kubefledgeds_crd.yaml
	kubectl create -f deploy/kubefledged-operator/deploy/service_account.yaml
	kubectl create -f deploy/kubefledged-operator/deploy/clusterrole.yaml
	kubectl create -f deploy/kubefledged-operator/deploy/clusterrole_binding.yaml
	kubectl create -f deploy/kubefledged-operator/deploy/operator.yaml
	# Deploy kube-fledged to a separate namespace called "kube-fledged"
	sed -i "s|OPERATOR_NAMESPACE|operators|g" deploy/kubefledged-operator/deploy/crds/charts.helm.k8s.io_v1alpha1_kubefledged_cr.yaml
	sed -i "s|KUBEFLEDGED_NAMESPACE|kube-fledged|g" deploy/kubefledged-operator/deploy/crds/charts.helm.k8s.io_v1alpha1_kubefledged_cr.yaml
	-kubectl create namespace kube-fledged
	kubectl create -f deploy/kubefledged-operator/deploy/crds/charts.helm.k8s.io_v1alpha1_kubefledged_cr.yaml

update:
	kubectl scale deployment kubefledged --replicas=0 -n kube-fledged && sleep 1 && \
	kubectl scale deployment kubefledged --replicas=1 -n kube-fledged && sleep 1 && \
	kubectl get pods -l app=kubefledged -n kube-fledged

remove:
	kubectl delete -f deploy/kubefledged-namespace.yaml && \
	kubectl delete -f deploy/kubefledged-clusterrolebinding.yaml && \
	kubectl delete -f deploy/kubefledged-clusterrole.yaml && \
	kubectl delete -f deploy/kubefledged-crd.yaml

remove-all:
	# Remove kube-fledged and the namespace "kube-fledged"
	kubectl delete -f deploy/kubefledged-operator/deploy/crds/charts.helm.k8s.io_v1alpha1_kubefledged_cr.yaml
	-kubectl delete namespace kube-fledged
	sed -i "s|kube-fledged|KUBEFLEDGED_NAMESPACE|g" deploy/kubefledged-operator/deploy/crds/charts.helm.k8s.io_v1alpha1_kubefledged_cr.yaml
	sed -i "s|operators|OPERATOR_NAMESPACE|g" deploy/kubefledged-operator/deploy/crds/charts.helm.k8s.io_v1alpha1_kubefledged_cr.yaml
	# Remove the operator and the namespace "operators"
	kubectl delete -f deploy/kubefledged-operator/deploy/operator.yaml
	kubectl delete -f deploy/kubefledged-operator/deploy/clusterrole_binding.yaml
	kubectl delete -f deploy/kubefledged-operator/deploy/clusterrole.yaml
	kubectl delete -f deploy/kubefledged-operator/deploy/service_account.yaml
	kubectl delete -f deploy/kubefledged-operator/deploy/crds/charts.helm.k8s.io_kubefledgeds_crd.yaml
	-kubectl delete namespace operators
	sed -i "s|operators|OPERATOR_NAMESPACE|g" deploy/kubefledged-operator/deploy/operator.yaml
	sed -i "s|operators|OPERATOR_NAMESPACE|g" deploy/kubefledged-operator/deploy/clusterrole_binding.yaml
	sed -i "s|operators|OPERATOR_NAMESPACE|g" deploy/kubefledged-operator/deploy/service_account.yaml

