# kube-fledged

kube-fledged is a kubernetes application for creating and managing a cache of container images in a kubernetes
cluster. It allows a user to define a list of images and onto which worker nodes those images should be cached
(i.e. pre-pulled). This enables application Pods to be up and running almost instantly, since the images need not
be pulled from the registry.

kube-fledged provides CRUD APIs to manage the lifecycle of the image cache, and supports several configurable
parameters to customize the functioning as per one's needs.

## Use cases

- Applications that require rapid start-up. For e.g. an application performing real-time data processing needs to
scale rapidly due to a burst in traffic.
- IoT applications that run on Edge devices where the network connectivity between the edge and image registry is
intermittent.
- If a cluster administrator or operator needs to roll-out upgrades to an application and want to verify before-hand
if the new images can be pulled successfully.

## Build and Deploy

These instructions will help you build kube-fledged from source and deploy it on a kubernetes cluster.

### Prerequisites

- A functioning kubernetes cluster (v1.12 or above). It could be a simple development cluster like minikube or a large
production cluster.
- All master and worker nodes having the ["kubernetes.io/hostname"](https://kubernetes.io/docs/reference/kubernetes-api/labels-annotations-taints/#kubernetes-io-hostname) label.
- make, go (v1.11 or above) and kubectl installed on a local machine. kubectl configured properly to access the cluster.

### Build

Create the source code directories on local machine and setup $GOPATH

```
$ mkdir -p src
$ export GOPATH=$GOPATH:$PWD/src
$ mkdir -p src/k8s.io
```

Clone the repository to src/k8s.io and switch to "fledged_stable" branch

```
$ cd src/k8s.io
$ git clone https://senthilrch@bitbucket.org/senthilrch/kube-fledged.git
$ cd kube-fledged
$ git checkout fledged_stable
```

Build and push the docker image to registry (e.g. Docker hub).

```
$ export FLEDGED_REPOSITORY=<your registry>/fledged:<your tag>
$ make image && make push
```

### Deploy

All manifests required for deploying are present inside kube-fledged/deploy. These steps deploys the
application into a separate namespace called "kube-fledged" with default configuration flags.

Edit "fledged-deployment.yaml":-

- Set the value of KUBERNETES_SERVICE_HOST to the IP/hostname of api server of the cluster 
- Set KUBERNETES_SERVICE_PORT to port number of api server
- Set "image" to "<your registry>/fledged:<your tag>"

```
      - env:
        - name: KUBERNETES_SERVICE_HOST
          value: "<IP or hostname of api server>"
        - name: KUBERNETES_SERVICE_PORT
          value: "<port number of api server>"
        image: <your registry>/fledged:<your tag>
```

Deploy the application to the cluster
```
$ make deploy
```

Verify if the application is up and running
```
$ kubectl get pods -n kube-fledged -l app=fledged
$ kubectl logs <pod name output from above command> -n kube-fledged
```

## Manage image cache

## Documentation

- API reference
- Configuration Flags

## Running the tests

Explain how to run the automated tests for this system

### Break down into end to end tests

Explain what these tests test and why

```
Give an example
```

### And coding style tests

Explain what these tests test and why

```
Give an example
```

## Built With

* [kubernetes/sample-controller](https://github.com/kubernetes/sample-controller) - Building our own kubernetes-style controller using CRD.
* [Dep](https://github.com/golang/dep) - Go dependency management tool

## Contributing

Please read [CONTRIBUTING.md](https://gist.github.com/PurpleBooth/b24679402957c63ec426) for details on our code of conduct, and the process for submitting pull requests to us.

## Authors

* **Senthil Raja Chermapandian** - *v1.0* - [senthilrch](https://github.com/senthilrch)

See also the list of [contributors](https://github.com/your/project/contributors) who participated in this project.

## License

This project is licensed under the Apache 2.0 License - see the [LICENSE.md](LICENSE.md) file for details

## Acknowledgments

* Hat tip to anyone whose code was used
* Inspiration
* etc
