# 1. Problem Statement:-
Kubernetes is the Container Orchestration Engine of choice for a wide variety of Applications. When a Pod is successfully
scheduled to a worker node, the container images required for the Pod are pulled down from a image registry. Depending on the
size of the image and network latency, the image pull will consume considerable time duration. Ultimately this increases the time
taken to start the containers and Pod reaching ready status.

There are several use cases where Pods are expected to be up and running instantly. The considerable time taken in pulling the
container image becomes a major bottleneck in achieving this. And there are use cases where
the connectivity to the image registry might not be available all the time
(E.g. IoT/Edge computing where the Edge nodes are running on a moving Cruise vessel).

We will need a robust solution to solve these problems

# 2. Existing Solution:-
The existing solution to tackle the problem of reducing the Pod startup time is to have a Registry mirror running inside the
Cluster. The first time you request an image from your local registry mirror, it pulls the
image from the Master image registry and stores it locally before sending it to the client.
On subsequent requests, the local registry mirror is able to serve the image from its own
storage. See https://docs.docker.com/registry/recipes/mirror/

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

# 3. Proposed Solution - *Distributed* Cluster Image Cache:-
The proposed solution is to have a ** *distributed* ** cluster image cache. The image cache is distributed across all/multiple worker nodes and not in a centralized local repository mirror.
Applications that
require instant Pod startup or that cannot tolerate loss of connectivity to image registry
will have the container images stored in the cluster image cache and made available directly in the node. When a Pod is scheduled to the
node that has image pull policy either "Never" or "IfNotPresent", the image from the image cache in the node
will be used. This eliminates the delay incurred in downloading the image.

### 3.1. Challenges with the Proposed Solution:-
Kubernetes has an in-built image garbage collection mechanism. On a periodic basis, the kubelet in the node
check if the disk usage has reached a certain threshold (configurable via flags). Once this threshold is
reached kubelet automatically deletes all unused images in the node. This is a much needed
functionality of the kubelet. However, this can result in deletion of images present in the node image cache that
is proposed in this solution. Many users in the k8s community have raised issues to have the
kubelet configurable to have a list of whitelisted images that will not be affected by garbage collection. One user has already
implemented a solution for this that has been tested in Production and has raised a PR. Please refer below:-

https://github.com/kubernetes/kubernetes/pull/68549

### 3.2. Temporary workaround:-
(Document the workaround proposal to prevent kubelet gc from removing the images in node image cache)

# High level design:-
The Cluster Image Cache proposed in the solution will be implemented as an extension API resource. This
gives the advantage of managing (i.e. CRUD operations) the Image cache using Kubernetes-style APIs. Kubernetes offers
two mechanism for implementing extension APIs viz. Extension API server and Custom Resource Definition. We will
use Custom Resource Definition (CRD) to implement the extension API.

A new controller needs to be written to act on the CRD resources. The controller will need to watch for ClusterImageCache API
resource and react accordingly.

### Create action:-
A new Image Cache needs to be created. This will result in pulling the container images specified in the API resource on to
the nodes specified in the API resource. Nodes will be specified in the form of Node Selector that selects a set of Nodes.

### Remove action:-
(The Image Cache needs to be removed. The previously pulled images will be deleted, provided there are no containers
using the image. If the images become unused in a future point in time, the Controller will not delete such images. Such
images will be left in the nodes so that Kubelet's GC action will remove them. If user requires complete wiping of
the images in the Cache, user needs to ensure there are no containers using the images, before issuing the API operation.)

No action required since the Kubelet's image GC action will be responsible for removing the images. The controller
will not delete the images in the nodes when the user deletes the Image Cache API resource.

### Update action:-
New images can be added or existing images removed. Also it is possible that node Selectors are modified. The controller
needs to take appropriate action to keep the current state of the Image Cache as per the Desired state.

### Display action:-
No action required by the controller.

# CustomResourceDefinition API resource:-
```yaml
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  # name must match the spec fields below, and be in the form: <plural>.<group>
  name: imagecaches.fledged.k8s.io
spec:
  # group name to use for REST API: /apis/<group>/<version>
  group: fledged.k8s.io
  # list of versions supported by this CustomResourceDefinition
  versions:
    - name: v1alpha1
      # Each version can be enabled/disabled by Served flag.
      served: true
      # One and only one version must be marked as the storage version.
      storage: true
  # either Namespaced or Cluster
  scope: Namespaced
  names:
    # plural name to be used in the URL: /apis/<group>/<version>/<plural>
    plural: imagecaches
    # singular name to be used as an alias on the CLI and for display
    singular: imagecache
    # kind is normally the CamelCased singular type. Your resource manifests use this.
    kind: ImageCache
    # shortNames allow shorter string to match your resource on the CLI
    shortNames:
    - cic
```

# ImageCache resource:-

```yaml
apiVersion: fledged.k8s.io/v1alpha1
kind: ImageCache
metadata:
  name: imagecache
  namespace: kube-fledged
spec:
  cacheSpec:
  # Following images are available in a public registry e.g. Docker hub.
  # These images will be cached to all nodes with label zone=asia-south1-a
  - images:
    - nginx:1.15.5
    - redis:4.0.11
    nodeSelector: zone=asia-south1-a
  # Following images are available in an internal private registry e.g. DTR.
  # These images will be cached to all nodes with label zone=asia-south1-b
  # imagePullSecrets must be specified in the default service account of namespace kube-fledged
  - images:
    - myprivateregistry/myapp:1.0
    - myprivateregistry/myapp:1.1.1
    nodeSelector: zone=asia-south1-b
  # Following images are available in an external private registry e.g. Docker store.
  # These images will be cached to all nodes in the cluster
  # imagePullSecrets must be specified in the default service account of namespace kube-fledged
  - images:
    - extprivateregistry/extapp:1.0
    - extprivateregistry/extapp:1.1.1
status:
  status: Succeeded
  reason: ImagesPulled
  message: All requested images pulled succesfuly to respective nodes
```

# ImagePuller Job Spec:-
```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: imagepuller
spec:
  template:
    spec:
      nodeSelector:
        kubernetes.io/hostname: worker1.k8s.cis.com
#      affinity:
#        podAntiAffinity:
#          requiredDuringSchedulingIgnoredDuringExecution:
#          - labelSelector:
#              matchExpressions:
#              - key: job-name
#                operator: In
#                values:
#                - imagepuller
#            topologyKey: kubernetes.io/hostname
      initContainers:
      - name: busybox
        image: busybox:1.29.2
        command: ["cp", "/bin/echo", "/tmp/bin"]
        volumeMounts:
        - name: tmp-bin
          mountPath: /tmp/bin
        imagePullPolicy: IfNotPresent
      containers:
      - name: imagepuller
        image: mysql
        command: ["/tmp/bin/echo", "Image pulled successfully!"]
        securityContext:
          privileged: true
        volumeMounts:
        - name: tmp-bin
          mountPath: /tmp/bin
        imagePullPolicy: IfNotPresent
      volumes:
      - name: tmp-bin
        emptyDir: {}
      restartPolicy: Never
#  completions: 2
#  parallelism: 2
```

# Low level design:-

### Mechanism of pulling images into the Image Cache:-

The Controller first calculates the number of nodes returned by the Node selector. Then creates a Deployment resource
with replicas equal to no. of nodes along with suitable Labels. The container image mentioned in the Spec will be the image that needs to be pulled.
Additionally Pod anti-affinity rules are set in the Annotation such that one node can have only one instance of the Pod.

The creation of Deployment resource results in creation of Pod in all nodes. Kubelet of the node will pull the image on to the node.
The controller will fetch the name of all the Pods using Label selector. Then checks for events in the Pod to see if the
image has been successfully pulled. Once the controller verifies that images have been pulled for the Pods, it deletes
the Deployment which results in deletion of the Pods. The deletion of the Pods will not affect the already pulled
images in the node. These pulled images will form the Image Cache so when a Pod is created by the user, it uses the
image from the Cache.

### Watching the Nodes:-

The Controller needs to constantly watch the Nodes in the cluster to ensure the current state of the Image Cache
equals the desired state. If an image get's removed by Kubelet's GC or an Operator/Administrator deletes the image in the
Image Cache, the controller will identify this since the Node resource will have the list of container images in the node.
The controller needs to remediate by firing Pods.

### Deleting the Image Cache:-

No action required by the Controller.
