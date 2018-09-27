# Problem Statement:-
Kubernetes is the Container Orchestration Engine of choice for a wide variety of Applications. When a Pod is successfully
scheduled to a worker node, the container images required for the Pod are pulled down from a image registry. Depending on the
size of the image and network latency, the image pull will consume considerable time duration. Ultimately this increases the time
taken to start the containers and Pod reaching ready status.

There are several use cases where Pods are expected to be up and running instantly. The considerable time taken in pulling the
container image becomes a major bottleneck in achieving this. And there are use cases where
the connectivity to the image registry might not be available all the time
(E.g. IoT/Edge computing where the Edge nodes are running on a moving Cruise vessel).

We will need a robust solution to solve these problems

# Existing Solution:-
The existing solution to tackle this problem is to have a Registry mirror running inside the
Cluster. The first time you request an image from your local registry mirror, it pulls the
image from the Master image registry and stores it locally before sending it to the client.
On subsequent requests, the local registry mirror is able to serve the image from its own
storage. See ![](https://docs.docker.com/registry/recipes/mirror/)

This is an acceptable solution for most use cases. However it has the following drawbacks:-
1. Setting up and maintaining the Local registry mirror consumes considerable computational
and human resources.
2. For huge clusters spanning multiple regions, we need to have multiple local registry mirrors. This
introduces unnecessary complexities when application instances span multiple regions. You might need
to have multiple Deployment manifests each pointing to the local registry mirror of that region.
3. This approach doesn't fully solves the requirement for achieving rapid starting of a Pod since
there is still a notable delay in pulling the image from the local mirror. There are several
use cases which cannot tolerate this delay.
4. Nodes might lose network connectivity to the local registry mirror so the Pod will be stuck
until the connectivity is restored.

# Proposed Solution:-
The proposed solution is to have an image cache on the kubernetes worker node. Applications that
require near instant Pod startup or that cannot tolerate loss of connectivity to image registry
will have the container images stored in the node image cache. When a Pod is scheduled to the
node that has image pull policy either "Never" or "IfNotPresent", the image from the image cache
will be used. This eliminates the delay incurred in downloading the image.

# Challenges with the Proposed Solution:-
Kubernetes has an in-built image garbage collection mechanism. On a periodic basis, the kubelet in the node
check if the disk usage has reached a certain threshold (configurable via flags). Once this threshold is
reached kubelet automatically deletes all unused images in the node. This is a much needed
functionality of the kubelet. This can result in deletion of images present in the node image cache that
is proposed in this solution. Many users in the k8s community have raised issues to have the
kubelet configured to skip some images during garbage collection. One user has already
implemented a solution for this that has been tested in Production and has raised a PR to
accpet the solution. Please refer below:-

