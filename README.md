# kube-fledged

[![Build Status](https://travis-ci.org/senthilrch/kube-fledged.svg?branch=master)](https://travis-ci.org/senthilrch/kube-fledged)
[![Coverage Status](https://coveralls.io/repos/github/senthilrch/kube-fledged/badge.svg?branch=master)](https://coveralls.io/github/senthilrch/kube-fledged?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/senthilrch/kube-fledged)](https://goreportcard.com/report/github.com/senthilrch/kube-fledged)
[![GitHub release (latest by date)](https://img.shields.io/github/v/release/senthilrch/kube-fledged)](https://img.shields.io/github/v/release/senthilrch/kube-fledged)
[![License](https://img.shields.io/github/license/senthilrch/kube-fledged)](https://img.shields.io/github/license/senthilrch/kube-fledged)

**_kube-fledged_** is a kubernetes add-on for creating and managing a cache of container images directly on the worker nodes of a kubernetes cluster. It allows a user to define a list
of images and onto which worker nodes those images should be cached (i.e. pre-pulled). As a result, application pods start almost instantly, since the images need not be pulled from the registry.

_kube-fledged_ provides CRUD APIs to manage the lifecycle of the image cache, and supports several configurable parameters to customize the functioning as per one's needs. 

## Use cases

- Applications that require rapid start-up. For e.g. an application performing real-time data processing needs to scale rapidly due to a burst in data volume.
- Serverless Functions where functions need to react immediately to incoming events.
- IoT applications that run on Edge devices when the network connectivity between the edge and image registry is intermittent.
- If a cluster administrator or operator needs to roll-out upgrades to an application and wants to verify before-hand if the new images can be pulled successfully.

## Prerequisites

- A functioning kubernetes cluster (v1.9 or above). It could be a simple development cluster like minikube or a large production cluster.
- All master and worker nodes having the ["kubernetes.io/hostname"](https://kubernetes.io/docs/reference/kubernetes-api/labels-annotations-taints/#kubernetes-io-hostname) label.
- Supported container runtimes: docker, containerd, cri-o
- git, make, go, docker and kubectl installed on a local linux machine. kubectl configured properly to access the cluster.

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
  $ make deploy
  ```

- Verify if _kube-fledged_ deployed successfully

  ```
  $ kubectl get pods -n kube-fledged -l app=kubefledged
  $ kubectl logs -f <pod_name_obtained_from_above_command> -n kube-fledged
  $ kubectl get imagecaches -n kube-fledged (Output should be: 'No resources found')
  ```

## Quick Install using Helm operator

These instructions install _kube-fledged_ to a separate namespace called "kube-fledged", using Helm operator and pre-built images in [Docker Hub.](https://hub.docker.com/u/senthilrch)

- Clone the source code repository

  ```
  $ mkdir -p $HOME/src/github.com/senthilrch
  $ git clone https://github.com/senthilrch/kube-fledged.git $HOME/src/github.com/senthilrch/kube-fledged
  $ cd $HOME/src/github.com/senthilrch/kube-fledged
  ```

- Deploy the operator to a separate namespace called "operators"

  ```
  $ sed -i "s|OPERATOR_NAMESPACE|operators|g" deploy/kubefledged-operator/deploy/service_account.yaml
  $ sed -i "s|OPERATOR_NAMESPACE|operators|g" deploy/kubefledged-operator/deploy/clusterrole_binding.yaml
  $ sed -i "s|OPERATOR_NAMESPACE|operators|g" deploy/kubefledged-operator/deploy/operator.yaml
  $ kubectl create namespace operators
  $ kubectl create -f deploy/kubefledged-operator/deploy/crds/charts.helm.k8s.io_kubefledgeds_crd.yaml
  $ kubectl create -f deploy/kubefledged-operator/deploy/service_account.yaml
  $ kubectl create -f deploy/kubefledged-operator/deploy/clusterrole.yaml
  $ kubectl create -f deploy/kubefledged-operator/deploy/clusterrole_binding.yaml
  $ kubectl create -f deploy/kubefledged-operator/deploy/operator.yaml
  ```

- Deploy _kube-fledged_ to a separate namespace called "kube-fledged"

  ```
  $ sed -i "s|OPERATOR_NAMESPACE|operators|g" deploy/kubefledged-operator/deploy/crds/charts.helm.k8s.io_v1alpha1_kubefledged_cr.yaml
  $ sed -i "s|KUBEFLEDGED_NAMESPACE|kube-fledged|g" deploy/kubefledged-operator/deploy/crds/charts.helm.k8s.io_v1alpha1_kubefledged_cr.yaml
  $ kubectl create namespace kube-fledged
  $ kubectl create -f deploy/kubefledged-operator/deploy/crds/charts.helm.k8s.io_v1alpha1_kubefledged_cr.yaml
  ```

- Verify if _kube-fledged_ deployed successfully

  ```
  $ kubectl get pods -n kube-fledged -l app.kubernetes.io/name=kubefledged
  $ kubectl logs -f <pod_name_obtained_from_above_command> -n kube-fledged
  $ kubectl get imagecaches -n kube-fledged (Output should be: 'No resources found')
  ```

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
  $ export FLEDGED_IMAGE_REPO=<your_docker_hub_username>/fledged
  $ export FLEDGED_DOCKER_CLIENT_IMAGE_REPO=<your_docker_hub_username>/fledged-docker-client
  $ docker login -u <username> -p <password>
  $ make fledged-image && make client-image && make push-images
  ```

### Deploy

_Note:- You need to have 'cluster-admin' privileges to deploy_

- All manifests required for deploying _kube-fledged_ are present in 'kube-fledged/deploy' directory. Edit "kubefledged-deployment.yaml".

  Set "image" to "<your_docker_hub_username>/fledged:<your_tag>"

  ```
  image: <your_docker_hub_username>/fledged:<your_tag>
  ```

- If you pushed the image to a private repository, add 'imagePullSecrets' to the end of "kubefledged-deployment.yaml". Refer to kubernetes documentation on [Specifying ImagePullSecrets on a Pod](https://kubernetes.io/docs/concepts/containers/images/#specifying-imagepullsecrets-on-a-pod). The secret <your_registry_key> should be created in "kube-fledged" namespace.

  ```
  serviceAccountName: kubefledged
  imagePullSecrets:
    - name: <your_registry_key>
  ```

- Deploy _kube-fledged_ to the cluster

  ```
  $ make deploy
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
$ kubectl annotate imagecaches imagecache1 -n kube-fledged fledged.k8s.io/refresh-imagecache=
```

### Delete image cache

Before you could delete the image cache, you need to purge the images in the cache using the following command. This will remove all cached images from the worker nodes.

```
$ kubectl annotate imagecaches imagecache1 -n kube-fledged fledged.k8s.io/purge-imagecache=
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
$ make remove
```

## How it works

Kubernetes allows developers to extend the kubernetes api via [Custom Resources](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/). _kube-fledged_ defines a custom resource of kind “ImageCache” and implements a custom controller (named _fledged_). _fledged_ does the heavy-lifting for managing image cache. Users can use kubectl commands for creation and deletion of ImageCache resources.

_fledged_ has a built-in image manager routine that is responsible for pulling and deleting images. Images are pulled or deleted using kubernetes jobs. If enabled, image cache is refreshed periodically by the refresh worker. _fledged_ updates the status of image pulls, refreshes and image deletions in the status field of ImageCache resource.

For more detailed description, go through _kube-fledged's_ [design proposal](docs/cluster-image-cache.md).


## Configuration Flags

`--image-pull-deadline-duration:` Maximum duration allowed for pulling an image. After this duration, image pull is considered to have failed. default "5m"

`--image-cache-refresh-frequency:` The image cache is refreshed periodically to ensure the cache is up to date. Setting this flag to "0s" will disable refresh. default "15m"

`--docker-client-image:` The image name of the docker client. the docker client is used when deleting images during purging the cache".

`--image-pull-policy:` Image pull policy for pulling images into the cache. Possible values are 'IfNotPresent' and 'Always'. Default value is 'IfNotPresent'. Default value for Images with ':latest' tag is 'Always'

`--stderrthreshold:` Log level. set the value of this flag to INFO

## Supported Platforms

- linux/amd64


## Built With

* [kubernetes/sample-controller](https://github.com/kubernetes/sample-controller) - Building our own kubernetes-style controller using CRD.
* [Go Modules](https://golang.org/doc/go1.11#modules) - Go Modules for Dependency Management
* [Make](https://www.gnu.org/software/make/) - GNU Make


## Contributing

Please read [CONTRIBUTING.md](CONTRIBUTING.md) for details on our code of conduct, and the process for submitting pull requests.

## License

This project is licensed under the Apache 2.0 License - see the [LICENSE.md](LICENSE.md) file for details
