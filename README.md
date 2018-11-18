# kube-fledged

[![Build Status](https://travis-ci.org/senthilrch/kube-fledged.svg?branch=master)](https://travis-ci.org/senthilrch/kube-fledged)
[![Coverage Status](https://coveralls.io/repos/github/senthilrch/kube-fledged/badge.svg?branch=master)](https://coveralls.io/github/senthilrch/kube-fledged?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/senthilrch/kube-fledged)](https://goreportcard.com/report/github.com/senthilrch/kube-fledged)

**_kube-fledged_** is a kubernetes add-on for creating and managing a cache of container images directly on the worker nodes of a kubernetes cluster. It allows a user to define a list
of images and onto which worker nodes those images should be cached (i.e. pre-pulled). As a result, application pods start almost instantly, since the images need not be pulled from the registry.

_kube-fledged_ provides CRUD APIs to manage the lifecycle of the image cache, and supports several configurable parameters to customize the functioning as per one's needs. 

## Use cases

- Applications that require rapid start-up. For e.g. an application performing real-time data processing needs to scale rapidly due to a burst in data volume.
- IoT applications that run on Edge devices when the network connectivity between the edge and image registry is intermittent.
- If a cluster administrator or operator needs to roll-out upgrades to an application and wants to verify before-hand if the new images can be pulled successfully.

## Build and Deploy

These instructions will help you build _kube-fledged_ from source and deploy it on a kubernetes cluster.

### Prerequisites

- A functioning kubernetes cluster (v1.12 or above). It could be a simple development cluster like minikube or a large production cluster.
- All master and worker nodes having the ["kubernetes.io/hostname"](https://kubernetes.io/docs/reference/kubernetes-api/labels-annotations-taints/#kubernetes-io-hostname) label.
- make, go, docker and kubectl installed on a local linux machine. kubectl configured properly to access the cluster.

### Build

Create the source code directories on local linux machine and setup $GOPATH

```
$ mkdir -p $HOME/src/github.com/senthilrch
$ export GOPATH=$HOME
```

Clone the repository

```
$ git clone https://github.com/senthilrch/kube-fledged.git $HOME/src/github.com/senthilrch/kube-fledged
$ cd $HOME/src/github.com/senthilrch/kube-fledged
```

Build and push the docker image to registry (e.g. Docker hub)

```
$ export FLEDGED_IMAGE_NAME=<your_docker_hub_username>/fledged:<your_tag>
$ docker login -u <username> -p <password>
$ make fledged-image && make push-image
```

### Deploy

All manifests required for deploying _kube-fledged_ are present inside 'kube-fledged/deploy'. These steps deploy _kube-fledged_ into a separate namespace called "kube-fledged" with default configuration flags.

Edit "fledged-deployment.yaml":-

- Set the value of KUBERNETES_SERVICE_HOST to the IP/hostname of api server of the cluster 
- Set KUBERNETES_SERVICE_PORT to port number of api server
- Set "image" to "<your_docker_hub_username>/fledged:<your_tag>"

```
      - env:
        - name: KUBERNETES_SERVICE_HOST
          value: "<IP or hostname of api server>"
        - name: KUBERNETES_SERVICE_PORT
          value: "<port number of api server>"
        image: <your_docker_hub_username>/fledged:<your_tag>
```

If you pushed the image to a private repository, add 'imagePullSecrets' to the end of "fledged-deployment.yaml". Refer to kubernetes documentation on [Specifying ImagePullSecrets on a Pod](https://kubernetes.io/docs/concepts/containers/images/#specifying-imagepullsecrets-on-a-pod)

```
      serviceAccountName: fledged
      imagePullSecrets:
        - name: <your_registry_key>
```

Deploy _kube-fledged_ to the cluster

```
$ make deploy
```

Verify if _kube-fledged_ deployed successfully

```
$ kubectl get pods -n kube-fledged -l app=fledged
$ kubectl logs -f <pod_name_obtained_from_above_command> -n kube-fledged
```

## How to use

_kube-fledged_ provides APIs to perform CRUD operations on image cache.  These APIs can be consumed via kubectl or curl

### Create image cache

Refer to sample image cache manifest in "deploy/fledged-imagecache.yaml". Edit it as per your needs before creating image cache. If images are in private repositories requiring credentials to pull, add "imagePullSecrets" to the end.

```
  imagePullSecrets:
  - name: myregistrykey
```

Create the image cache using kubectl. Verify successful creation

```
$ kubectl create -f deploy/fledged-imagecache.yaml
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

### Delete image cache

An existing image cache can be deleted using following command.

```
$ kubectl delete imagecaches imagecache1 -n kube-fledged
```

## How it works

Kubernetes allows developers to extend the kubernetes api via [Custom Resources](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/). _kube-fledged_ defines a custom resource of kind “ImageCache” and implements a custom controller (named _fledged_). _fledged_ does the heavy-lifting for managing image cache. Users can use kubectl commands for creation and deletion of ImageCache resources.

_fledged_ has a built-in image manager routine that is responsible for pulling images. Images are pulled on to nodes using kubernetes jobs. If enabled, image cache is refreshed periodically by the refresh worker. _fledged_ updates the status of image pulls and refreshes in the status field of ImageCache resource.

For more detailed description, go through _kube-fledged's_ [design proposal](docs/cluster-image-cache.md).


## Configuration Flags

`--image-pull-deadline-duration:` Maximum duration allowed for pulling an image. After this duration, image pull is considered to have failed. default "5m"

`--image-cache-refresh-frequency:` The image cache is refreshed periodically to ensure the cache is up to date. Setting this flag to "0s" will disable refresh. default "15m"

`--stderrthreshold:` Log level. set the value of this flag to INFO

## Supported Platforms

- linux/amd64


## Built With

* [kubernetes/sample-controller](https://github.com/kubernetes/sample-controller) - Building our own kubernetes-style controller using CRD.
* [Dep](https://github.com/golang/dep) - Go dependency management tool
* [Make](https://www.gnu.org/software/make/) - GNU Make


## Contributing

Please read [CONTRIBUTING.md](CONTRIBUTING.md) for details on our code of conduct, and the process for submitting pull requests.

## License

This project is licensed under the Apache 2.0 License - see the [LICENSE.md](LICENSE.md) file for details
