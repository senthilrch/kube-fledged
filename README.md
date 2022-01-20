<img src="logo.png">

# 

[![Build Status](https://travis-ci.org/senthilrch/kube-fledged.svg?branch=master)](https://travis-ci.org/senthilrch/kube-fledged)
[![Coverage Status](https://coveralls.io/repos/github/senthilrch/kube-fledged/badge.svg?branch=master)](https://coveralls.io/github/senthilrch/kube-fledged?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/senthilrch/kube-fledged)](https://goreportcard.com/report/github.com/senthilrch/kube-fledged)
[![GitHub release (latest by date)](https://img.shields.io/github/v/release/senthilrch/kube-fledged)](https://img.shields.io/github/v/release/senthilrch/kube-fledged)
[![License](https://img.shields.io/github/license/senthilrch/kube-fledged)](https://img.shields.io/github/license/senthilrch/kube-fledged)

**kube-fledged** is a kubernetes operator for creating and managing a cache of container images directly on the worker nodes of a kubernetes cluster. It allows a user to define a list of images and onto which worker nodes those images should be cached (i.e. pulled). As a result, application pods start almost instantly, since the images need not be pulled from the registry.

kube-fledged provides CRUD APIs to manage the lifecycle of the image cache, and supports several configurable parameters to customize the functioning as per one's needs. 

## Table of contents
<!-- https://github.com/thlorenz/doctoc -->
<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Use cases](#use-cases)
- [Prerequisites](#prerequisites)
- [Quick Install using YAML manifests](#quick-install-using-yaml-manifests)
- [Quick Install using Helm chart](#quick-install-using-helm-chart)
- [Quick Install using Helm operator](#quick-install-using-helm-operator)
- [Helm chart parameters](#helm-chart-parameters)
- [Build and Deploy](#build-and-deploy)
  - [Build](#build)
  - [Deploy](#deploy)
- [How to use](#how-to-use)
  - [Create image cache](#create-image-cache)
  - [View the status of image cache](#view-the-status-of-image-cache)
  - [Add/remove images in image cache](#addremove-images-in-image-cache)
  - [Refresh image cache](#refresh-image-cache)
  - [Delete image cache](#delete-image-cache)
  - [Remove kube-fledged](#remove-kube-fledged)
- [How it works](#how-it-works)
- [Configuration Flags for Kubefledged Controller](#configuration-flags-for-kubefledged-controller)
- [Supported Container Runtimes](#supported-container-runtimes)
- [Supported Platforms](#supported-platforms)
- [Built With](#built-with)
- [Contributing](#contributing)
- [License](#license)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Use cases

- Applications that require rapid start-up. For e.g. an application performing real-time data processing needs to scale rapidly due to a burst in data volume.
- Serverless Functions since they need to react immediately to incoming events.
- IoT applications that run on Edge devices, because the network connectivity between the edge device and image registry would be intermittent.
- If images need to be pulled from a private registry and everyone cannot be granted access to pull images from this registry, then the images can be made available on the nodes of the cluster.
- If a cluster administrator or operator needs to roll-out upgrades to an application and wants to verify before-hand if the new images can be pulled successfully.

## Prerequisites

- A functioning kubernetes cluster (v1.16 or above). It could be a simple development cluster like minikube or a large production cluster.
- Cluster-admin privileges to the kubernetes cluster.
- All master and worker nodes having the ["kubernetes.io/hostname"](https://kubernetes.io/docs/reference/kubernetes-api/labels-annotations-taints/#kubernetes-io-hostname) label.
- git, make, go, docker engine (>= 19.03), openssl, kubectl, helm, gpg and gnu-sed installed on a local linux or mac machine. kubectl configured properly to access the cluster.

## Quick Install using YAML manifests

These instructions install _kube-fledged_ to a separate namespace called "kube-fledged", using YAML manifests and pre-built images in [Docker Hub.](https://hub.docker.com/u/senthilrch)

- Clone the source code repository

  ```
  $ mkdir -p $HOME/src/github.com/senthilrch
  $ git clone https://github.com/senthilrch/kube-fledged.git $HOME/src/github.com/senthilrch/kube-fledged
  $ cd $HOME/src/github.com/senthilrch/kube-fledged
  ```

- Deploy _kube-fledged_ to the cluster

  ```
  $ make deploy-using-yaml
  ```

- Verify if _kube-fledged_ deployed successfully

  ```
  $ kubectl get pods -n kube-fledged -l app=kubefledged
  $ kubectl get imagecaches -n kube-fledged (Output should be: 'No resources found')
  ```

- Optional: Deploy _kube-fledged webhook server_ to the cluster. This component enables validating the ImageCache CR.

  ```
  $ make deploy-webhook-server-using-yaml
  ```

## Quick Install using Helm chart

- Create the namespace where kube-fledged will be installed

  ```
  $ export KUBEFLEDGED_NAMESPACE=kube-fledged
  $ kubectl create namespace ${KUBEFLEDGED_NAMESPACE}
  ```

- Verify and install latest version of kube-fledged helm chart

  ```
  $ helm repo add kubefledged-charts https://senthilrch.github.io/kubefledged-charts/
  $ helm repo update
  $ gpg --keyserver keyserver.ubuntu.com --recv-keys 92D793FA3A6460ED (or) gpg --keyserver pgp.mit.edu --recv-keys 92D793FA3A6460ED
  $ gpg --export >~/.gnupg/pubring.gpg
  $ helm install --verify kube-fledged kubefledged-charts/kube-fledged -n ${KUBEFLEDGED_NAMESPACE} --wait
  ```

- Optional: Verify and install kube-fledged webhook server. This component enables validating the ImageCache CR.

  ```
  $ helm upgrade --verify kube-fledged kubefledged-charts/kube-fledged -n ${KUBEFLEDGED_NAMESPACE} --set webhookServer.enable=true --wait
  ```

## Quick Install using Helm operator

These instructions install _kube-fledged_ to a separate namespace called "kube-fledged", using Helm operator and pre-built images in [Docker Hub.](https://hub.docker.com/u/senthilrch)

- Clone the source code repository

  ```
  $ mkdir -p $HOME/src/github.com/senthilrch
  $ git clone https://github.com/senthilrch/kube-fledged.git $HOME/src/github.com/senthilrch/kube-fledged
  $ cd $HOME/src/github.com/senthilrch/kube-fledged
  ```

- Deploy the helm operator and _kube-fledged_ to namespace "kube-fledged". If you need to deploy to a different namespace, export the variable KUBEFLEDGED_NAMESPACE

  ```
  $ make deploy-using-operator
  ```

- Verify if _kube-fledged_ deployed successfully

  ```
  $ kubectl get pods -n kube-fledged -l app.kubernetes.io/name=kube-fledged
  $ kubectl get imagecaches -n kube-fledged (Output should be: 'No resources found')
  ```

- Optional: Deploy _kube-fledged webhook server_ to the cluster. This component enables validating the ImageCache CR.

  ```
  $ make deploy-webhook-server-using-operator
  ```

## Helm chart parameters

Parameters of the helm chart are documented [here](docs/helm-parameters.md)

## Build and Deploy

These instructions will help you build _kube-fledged_ from source and deploy it to a separate namespace called "kube-fledged". If you need to deploy it to a different namespace, edit the namespace field of the manifests in "kube-fledged/deploy" accordingly.

### Build

- Clone the source code repository

  ```
  $ mkdir -p $HOME/src/github.com/senthilrch
  $ git clone https://github.com/senthilrch/kube-fledged.git $HOME/src/github.com/senthilrch/kube-fledged
  $ cd $HOME/src/github.com/senthilrch/kube-fledged
  ```

- If you are behind a proxy, export the following ENV variables (UPPER case)

  ```
  export HTTP_PROXY=http://proxy_ip_or_hostname:port
  export HTTPS_PROXY=https://proxy_ip_or_hostname:port
  ```

- Build and push the docker images to registry (e.g. Docker hub)

  ```
  $ export RELEASE_VERSION=<your_tag>
  $ export CONTROLLER_IMAGE_REPO=docker.io/<your_dockerhub_username>/kubefledged-controller
  $ export WEBHOOK_SERVER_IMAGE_REPO=docker.io/<your_dockerhub_username>/kubefledged-webhook-server
  $ export CRI_CLIENT_IMAGE_REPO=docker.io/<your_dockerhub_username>/kubefledged-cri-client
  $ export OPERATOR_IMAGE_REPO=docker.io/<your_dockerhub_username>/kubefledged-operator
  $ docker login -u <username> -p <password>
  $ export DOCKER_CLI_EXPERIMENTAL=enabled
  $ make install-buildx && make release-amd64
  ```

### Deploy

_Note:- You need to have 'cluster-admin' privileges to deploy_

- All manifests required for deploying _kube-fledged_ are present in 'kube-fledged/deploy' directory.
- Edit "kubefledged-deployment-controller.yaml".

  Set "image" to "<your_docker_hub_username>/kubefledged-controller:<your_tag>"

  ```
  image: <your_docker_hub_username>/kubefledged-controller:<your_tag>
  ```

- If you pushed the image to a private repository, add 'imagePullSecrets' to the end of "kubefledged-deployment-controller.yaml". Refer to kubernetes documentation on [Specifying ImagePullSecrets on a Pod](https://kubernetes.io/docs/concepts/containers/images/#specifying-imagepullsecrets-on-a-pod). The secret <your_registry_key> should be created in "kube-fledged" namespace.

  ```
  serviceAccountName: kubefledged
  imagePullSecrets:
    - name: <your_registry_key>
  ```
- Edit "kubefledged-deployment-webhook-server.yaml".

  Set "image" to "<your_docker_hub_username>/kubefledged-webhook-server:<your_tag>"

  ```
  image: <your_docker_hub_username>/kubefledged-webhook-server:<your_tag>
  ```
- Deploy _kube-fledged_ to the cluster

  ```
  $ make deploy-using-yaml
  ```

- Verify if _kube-fledged_ deployed successfully

  ```
  $ kubectl get pods -n kube-fledged -l app=kubefledged
  $ kubectl logs -f <pod_name_obtained_from_above_command> -n kube-fledged
  $ kubectl get imagecaches -n kube-fledged (Output should be: 'No resources found')
  ```

## How to use

_kube-fledged_ provides APIs to perform CRUD operations on image cache.  These APIs can be consumed via kubectl or curl

### Create image cache

Refer to sample image cache manifest in "deploy/kubefledged-imagecache.yaml". Edit it as per your needs before creating image cache. If images are in private repositories requiring credentials to pull, add "imagePullSecrets" to the end.

```
  imagePullSecrets:
  - name: myregistrykey
```

Create the image cache using kubectl. Verify successful creation

```
$ kubectl create -f deploy/kubefledged-imagecache.yaml
$ kubectl get imagecaches -n kube-fledged
```

### View the status of image cache

Use following command to view the status of image cache in "json" format.

```
$ kubectl get imagecaches imagecache1 -n kube-fledged -o json
```

### Add/remove images in image cache

Use kubectl edit command to add/remove images in image cache. The edit command opens the manifest in an editor. Edit your changes, save and exit.

```
$ kubectl edit imagecaches imagecache1 -n kube-fledged
$ kubectl get imagecaches imagecache1 -n kube-fledged -o json
```

### Refresh image cache

_kube-fledged_ supports both automatic and on-demand refresh of image cache. Auto refresh is enabled using the flag `--image-cache-refresh-frequency:`. To request for an on-demand refresh, run the following command:-

```
$ kubectl annotate imagecaches imagecache1 -n kube-fledged kubefledged.io/refresh-imagecache=
```

### Delete image cache

Before you could delete the image cache, you need to purge the images in the cache using the following command. This will remove all cached images from the worker nodes.

```
$ kubectl annotate imagecaches imagecache1 -n kube-fledged kubefledged.io/purge-imagecache=
```

View the status of purging the image cache. If any failures, such images should be removed manually or you could decide to leave the images in the worker nodes.

```
$ kubectl get imagecaches imagecache1 -n kube-fledged -o json
```

Finally delete the image cache using following command.

```
$ kubectl delete imagecaches imagecache1 -n kube-fledged
```

### Remove kube-fledged

Run the following command to remove _kube-fledged_ from the cluster. 

```
$ make remove-kubefledged (if you deployed using YAML manifests)
$ helm delete kube-fledged -n ${KUBEFLEDGED_NAMESPACE} (if you deployed using Helm chart)
$ make remove-kubefledged-and-operator (if you deployed using Helm Operator)
```

Note: To remove the _kube-fledged webhook server_ alone. 

```
$ make remove-webhook-server (if you deployed using YAML manifests)
$ helm upgrade kube-fledged deploy/kubefledged-operator/helm-charts/kubefledged -n ${KUBEFLEDGED_NAMESPACE} --set webhookServer.enable=false --wait --debug (if you deployed using Helm chart)
$ make remove-webhook-server-using-operator (if you deployed using Helm Operator)
```

## How it works

Kubernetes allows developers to extend the kubernetes api via [Custom Resources](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/). _kube-fledged_ defines a custom resource of kind “ImageCache” and implements a custom controller (named _kubefledged-controller_). _kubefledged-controller_ does the heavy-lifting for managing image cache. Users can use kubectl commands for creation and deletion of ImageCache resources.

_kubefledged-controller_ has a built-in image manager routine that is responsible for pulling and deleting images. Images are pulled or deleted using kubernetes jobs. If enabled, image cache is refreshed periodically by the refresh worker. _kubefledged-controller_ updates the status of image pulls, refreshes and image deletions in the status field of ImageCache resource.

For more detailed description, go through _kube-fledged's_ [design proposal](docs/design-proposal.md).


## Configuration Flags for Kubefledged Controller

`--image-pull-deadline-duration:` Maximum duration allowed for pulling an image. After this duration, image pull is considered to have failed. default "5m"

`--image-cache-refresh-frequency:` The image cache is refreshed periodically to ensure the cache is up to date. Setting this flag to "0s" will disable refresh. default "15m"

`--image-pull-policy:` Image pull policy for pulling images into and refreshing the cache. Possible values are 'IfNotPresent' and 'Always'. Default value is 'IfNotPresent'. Image with no or ":latest" tag are always pulled.

`--service-account-name:` serviceAccountName used in Jobs created for pulling or deleting images. Optional flag. If not specified the default service account of the namespace is used

`--stderrthreshold:` Log level. set the value of this flag to INFO

## Supported Container Runtimes

- docker
- containerd
- cri-o

## Supported Platforms

- linux/amd64
- linux/arm
- linux/arm64


## Built With

* [kubernetes/sample-controller](https://github.com/kubernetes/sample-controller) - Building our own kubernetes-style controller using CRD.
* [kubebuilder](https://github.com/kubernetes-sigs/kubebuilder) - SDK for building Kubernetes APIs using CRDs
* [operator-sdk](https://github.com/operator-framework/operator-sdk) - SDK for building Kubernetes applications
* [cri-tools](https://github.com/kubernetes-sigs/cri-tools) - CLI and validation tools for Kubelet Container Runtime Interface (CRI).
* [buildx](https://github.com/docker/buildx) - Docker CLI plugin for extended build capabilities with BuildKit 
* [Go Modules](https://golang.org/doc/go1.11#modules) - Go Modules for Dependency Management
* [Make](https://www.gnu.org/software/make/) - GNU Make


## Contributing

Please read [CONTRIBUTING.md](CONTRIBUTING.md) for details on the process for submitting pull requests.

## Code of Conduct

Please read [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for details on our code of conduct, and how to report violations.


## License

This project is licensed under the Apache 2.0 License - see the [LICENSE.md](LICENSE.md) file for details
