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

.PHONY: clean clean-controller clean-cri-client clean-operator controller-amd64 controller-image cri-client-image operator-image build-images push-images test deploy update remove
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
DOCKER_CLI_EXPERIMENTAL=enabled

ifndef CONTROLLER_IMAGE_REPO
  CONTROLLER_IMAGE_REPO=docker.io/senthilrch/kubefledged-controller
endif

ifndef WEBHOOK_SERVER_IMAGE_REPO
  WEBHOOK_SERVER_IMAGE_REPO=docker.io/senthilrch/kubefledged-webhook-server
endif

ifndef CRI_CLIENT_IMAGE_REPO
  CRI_CLIENT_IMAGE_REPO=docker.io/senthilrch/kubefledged-cri-client
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
  OPERATORSDK_VERSION=v0.17.1
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

ifndef OPERATOR_NAMESPACE
  OPERATOR_NAMESPACE=kubefledged-operator
endif

ifndef KUBEFLEDGED_NAMESPACE
  KUBEFLEDGED_NAMESPACE=kube-fledged
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
clean: clean-controller clean-webhook-server clean-cri-client clean-operator

clean-controller:
	-rm -f build/kubefledged-controller
	-docker image rm ${CONTROLLER_IMAGE_REPO}:${RELEASE_VERSION}
	-docker image rm `docker image ls -f dangling=true -q`

clean-webhook-server:
	-rm -f build/kubefledged-webhook-server
	-docker image rm ${WEBHOOK_SERVER_IMAGE_REPO}:${RELEASE_VERSION}
	-docker image rm `docker image ls -f dangling=true -q`

clean-cri-client:
	-docker image rm ${CRI_CLIENT_IMAGE_REPO}:${RELEASE_VERSION}
	-docker image rm `docker image ls -f dangling=true -q`

clean-operator:
	-docker image rm ${OPERATOR_IMAGE_REPO}:${RELEASE_VERSION}
	-docker image rm `docker image ls -f dangling=true -q`

controller-image: clean-controller
	docker buildx build --platform=${TARGET_PLATFORMS} -t ${CONTROLLER_IMAGE_REPO}:${RELEASE_VERSION} \
	-f build/Dockerfile.controller ${HTTP_PROXY_CONFIG} ${HTTPS_PROXY_CONFIG} \
	--build-arg GOLANG_VERSION=${GOLANG_VERSION} --build-arg ALPINE_VERSION=${ALPINE_VERSION} --progress=plain ${BUILD_OUTPUT} .

controller-amd64: TARGET_PLATFORMS=linux/amd64
controller-amd64: install-buildx controller-image

controller-dev: clean-controller
	CGO_ENABLED=0 go build -o build/kubefledged-controller -ldflags '-s -w -extldflags "-static"' cmd/controller/main.go && \
	docker build -t ${CONTROLLER_IMAGE_REPO}:${RELEASE_VERSION} -f build/Dockerfile.controller_dev \
	--build-arg ALPINE_VERSION=${ALPINE_VERSION} .
	docker push ${CONTROLLER_IMAGE_REPO}:${RELEASE_VERSION}

webhook-server-image: clean-webhook-server
	docker buildx build --platform=${TARGET_PLATFORMS} -t ${WEBHOOK_SERVER_IMAGE_REPO}:${RELEASE_VERSION} \
	-f build/Dockerfile.webhook_server ${HTTP_PROXY_CONFIG} ${HTTPS_PROXY_CONFIG} \
	--build-arg GOLANG_VERSION=${GOLANG_VERSION} --build-arg ALPINE_VERSION=${ALPINE_VERSION} --progress=plain ${BUILD_OUTPUT} .

webhook-server-amd64: TARGET_PLATFORMS=linux/amd64
webhook-server-amd64: install-buildx webhook-server-image

webhook-server-dev: clean-webhook-server
	CGO_ENABLED=0 go build -o build/kubefledged-webhook-server -ldflags '-s -w -extldflags "-static"' cmd/webhook-server/main.go && \
	docker build -t ${WEBHOOK_SERVER_IMAGE_REPO}:${RELEASE_VERSION} -f build/Dockerfile.webhook_server_dev \
	--build-arg ALPINE_VERSION=${ALPINE_VERSION} .
	docker push ${WEBHOOK_SERVER_IMAGE_REPO}:${RELEASE_VERSION}

cri-client-image: clean-cri-client
	docker buildx build --platform=${TARGET_PLATFORMS} -t ${CRI_CLIENT_IMAGE_REPO}:${RELEASE_VERSION} \
	-f build/Dockerfile.cri_client ${HTTP_PROXY_CONFIG} ${HTTPS_PROXY_CONFIG} \
	--build-arg DOCKER_VERSION=${DOCKER_VERSION} --build-arg CRICTL_VERSION=${CRICTL_VERSION} \
	--build-arg ALPINE_VERSION=${ALPINE_VERSION} --progress=plain ${BUILD_OUTPUT} .

operator-image: clean-operator
	cd deploy/kubefledged-operator && \
	docker buildx build --platform=${OPERATOR_TARGET_PLATFORMS} -t ${OPERATOR_IMAGE_REPO}:${RELEASE_VERSION} \
	-f build/Dockerfile --build-arg OPERATORSDK_VERSION=${OPERATORSDK_VERSION} --progress=plain ${BUILD_OUTPUT} .

release: install-buildx controller-image webhook-server-image cri-client-image operator-image

latest-tag:
	docker pull ${CONTROLLER_IMAGE_REPO}:${RELEASE_VERSION}
	docker tag  ${CONTROLLER_IMAGE_REPO}:${RELEASE_VERSION} ${CONTROLLER_IMAGE_REPO}:latest
	docker push ${CONTROLLER_IMAGE_REPO}:latest
	docker pull ${WEBHOOK_SERVER_IMAGE_REPO}:${RELEASE_VERSION}
	docker tag  ${WEBHOOK_SERVER_IMAGE_REPO}:${RELEASE_VERSION} ${WEBHOOK_SERVER_IMAGE_REPO}:latest
	docker push ${WEBHOOK_SERVER_IMAGE_REPO}:latest
	docker pull ${CRI_CLIENT_IMAGE_REPO}:${RELEASE_VERSION}
	docker tag  ${CRI_CLIENT_IMAGE_REPO}:${RELEASE_VERSION} ${CRI_CLIENT_IMAGE_REPO}:latest
	docker push ${CRI_CLIENT_IMAGE_REPO}:latest
	docker pull ${OPERATOR_IMAGE_REPO}:${RELEASE_VERSION}
	docker tag  ${OPERATOR_IMAGE_REPO}:${RELEASE_VERSION} ${OPERATOR_IMAGE_REPO}:latest
	docker push ${OPERATOR_IMAGE_REPO}:latest

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
	-kubectl apply -f deploy/kubefledged-namespace.yaml
	bash deploy/webhook-create-signed-cert.sh
	bash deploy/webhook-patch-ca-bundle.sh
	kubectl apply -f deploy/kubefledged-crd.yaml
	kubectl apply -f deploy/kubefledged-serviceaccount.yaml
	kubectl apply -f deploy/kubefledged-clusterrole.yaml
	kubectl apply -f deploy/kubefledged-clusterrolebinding.yaml
	kubectl apply -f deploy/kubefledged-deployment-controller.yaml
	kubectl apply -f deploy/kubefledged-deployment-webhook-server.yaml
	kubectl apply -f deploy/kubefledged-service-webhook-server.yaml
	kubectl apply -f deploy/kubefledged-validatingwebhook.yaml

deploy-using-operator:
	# Create the namespaces for operator and kubefledged
	-kubectl create namespace ${OPERATOR_NAMESPACE}
	-kubectl create namespace ${KUBEFLEDGED_NAMESPACE}
	# Deploy the operator to a separate namespace
	sed -i 's|{{OPERATOR_NAMESPACE}}|${OPERATOR_NAMESPACE}|g' deploy/kubefledged-operator/deploy/service_account.yaml
	sed -i "s|{{OPERATOR_NAMESPACE}}|${OPERATOR_NAMESPACE}|g" deploy/kubefledged-operator/deploy/clusterrole_binding.yaml
	sed -i "s|{{OPERATOR_NAMESPACE}}|${OPERATOR_NAMESPACE}|g" deploy/kubefledged-operator/deploy/operator.yaml
	kubectl apply -f deploy/kubefledged-operator/deploy/crds/charts.helm.k8s.io_kubefledgeds_crd.yaml
	kubectl apply -f deploy/kubefledged-operator/deploy/service_account.yaml
	kubectl apply -f deploy/kubefledged-operator/deploy/clusterrole.yaml
	kubectl apply -f deploy/kubefledged-operator/deploy/clusterrole_binding.yaml
	kubectl apply -f deploy/kubefledged-operator/deploy/operator.yaml
	# Deploy kube-fledged to a separate namespace
	sed -i "s|{{OPERATOR_NAMESPACE}}|${OPERATOR_NAMESPACE}|g" deploy/kubefledged-operator/deploy/crds/charts.helm.k8s.io_v1alpha1_kubefledged_cr.yaml
	sed -i "s|{{KUBEFLEDGED_NAMESPACE}}|${KUBEFLEDGED_NAMESPACE}|g" deploy/kubefledged-operator/deploy/crds/charts.helm.k8s.io_v1alpha1_kubefledged_cr.yaml
	bash deploy/webhook-create-signed-cert.sh --namespace ${KUBEFLEDGED_NAMESPACE}
	bash deploy/webhook-patch-ca-bundle.sh
	kubectl apply -f deploy/kubefledged-operator/deploy/crds/charts.helm.k8s.io_v1alpha1_kubefledged_cr.yaml

update:
	kubectl scale deployment kubefledged-controller --replicas=0 -n kube-fledged
	kubectl scale deployment kubefledged-webhook-server --replicas=0 -n kube-fledged && sleep 1
	kubectl scale deployment kubefledged-controller --replicas=1 -n kube-fledged && sleep 1
	kubectl scale deployment kubefledged-webhook-server --replicas=1 -n kube-fledged && sleep 1
	kubectl get pods -l app=kubefledged -n kube-fledged

remove-kubefledged:
	-kubectl delete -f deploy/kubefledged-namespace.yaml
	-kubectl delete -f deploy/kubefledged-clusterrolebinding.yaml
	-kubectl delete -f deploy/kubefledged-clusterrole.yaml
	-kubectl delete -f deploy/kubefledged-crd.yaml
	-kubectl delete -f deploy/kubefledged-validatingwebhook.yaml
	-git checkout deploy/kubefledged-validatingwebhook.yaml
	-git checkout deploy/kubefledged-operator/deploy/crds/charts.helm.k8s.io_v1alpha1_kubefledged_cr.yaml

remove-operator-and-kubefledged:
	# Remove kubefledged and the namespace
	-kubectl delete -f deploy/kubefledged-operator/deploy/crds/charts.helm.k8s.io_v1alpha1_kubefledged_cr.yaml
	-kubectl delete namespace ${KUBEFLEDGED_NAMESPACE}
	-git checkout deploy/kubefledged-validatingwebhook.yaml
	-git checkout deploy/kubefledged-operator/deploy/crds/charts.helm.k8s.io_v1alpha1_kubefledged_cr.yaml
	# Remove the kubefledged operator and the namespace
	-kubectl delete -f deploy/kubefledged-operator/deploy/operator.yaml
	-kubectl delete -f deploy/kubefledged-operator/deploy/clusterrole_binding.yaml
	-kubectl delete -f deploy/kubefledged-operator/deploy/clusterrole.yaml
	-kubectl delete -f deploy/kubefledged-operator/deploy/service_account.yaml
	-kubectl delete -f deploy/kubefledged-operator/deploy/crds/charts.helm.k8s.io_kubefledgeds_crd.yaml
	-kubectl delete namespace ${OPERATOR_NAMESPACE}
	-git checkout deploy/kubefledged-operator/deploy/operator.yaml
	-git checkout deploy/kubefledged-operator/deploy/clusterrole_binding.yaml
	-git checkout deploy/kubefledged-operator/deploy/service_account.yaml

