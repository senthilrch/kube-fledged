# Kube-Fledged helm chart

## Short description of Kube-fledged  
  
  
Kube-fledged is a kubernetes operator for creating and managing a cache of container images directly on the worker nodes of a kubernetes cluster. It allows a user to define a list of images and onto which worker nodes those images should be cached (i.e. pulled). As a result, application pods start almost instantly, since the images need not be pulled from the registry. kube-fledged provides CRUD APIs to manage the lifecycle of the image cache, and supports several configurable parameters to customize the functioning as per one's needs.

## How to install the chart 


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

## Chart parameters


| Parameter | Default value | Description |
| --------- | ------------- | ----------- |
| controllerReplicaCount    | 1        | No. of replicas of kubefledged-controller |
| webhookServerReplicaCount | 1        | No. of replicas of kubefledged-webhook-server |
| controller.hostNetwork    | false    | When set to "true", kubefledged-controller pod runs with "hostNetwork: true" |
| controller.priorityClassName    | ""    | priorityClassName of kubefledged-controller pod |
| webhookServer.enable      | false    | When set to "true", kubefledged-webhook-server is installed |
| webhookServer.hostNetwork | false    | When set to "true", kubefledged-webhook-server pod runs with "hostNetwork: true" |
| webhookServer.priorityClassName    | ""    | priorityClassName of kubefledged-webhook-server pod |
| image.kubefledgedControllerRepository | docker.io/senthilrch/kubefledged-controller | Repository name of kubefledged-controller image |
| image.kubefledgedCRIClientRepository | docker.io/senthilrch/kubefledged-cri-client | Repository name of kubefledged-cri-client image |
| image.kubefledgedWebhookServerRepository | docker.io/senthilrch/kubefledged-webhook-server | Repository name of kubefledged-webhook-server image |
| image.pullPolicy | Always | Image pull policy for kubefledged-controller and kubefledged-webhook-server pods |
| args.controllerImageCacheRefreshFrequency | 15m | The image cache is refreshed periodically to ensure the cache is up to date. Setting this flag to "0s" will disable refresh |
| args.controllerImageDeleteJobHostNetwork | false | Whether the pod for the image delete job should be run with 'HostNetwork: true' |
| args.controllerImagePullDeadlineDuration | 5m | Maximum duration allowed for pulling an image. After this duration, image pull is considered to have failed |
| args.controllerImagePullPolicy | IfNotPresent | Image pull policy for pulling images into and refreshing the cache. Possible values are 'IfNotPresent' and 'Always'. Default value is 'IfNotPresent'. Image with no or ":latest" tag are always pulled |
| args.controllerJobPriorityClassName | "" | priorityClassName of jobs created by kubefledged-controller. If not specified, priorityClassName won't be set |
| args.controllerServiceAccountName | "" | serviceAccountName used in Jobs created for pulling or deleting images. Optional flag. If not specified the default service account of the namespace is used |
| args.controllerLogLevel | INFO | Log level of kubefledged-controller |
| args.webhookServerCertFile | /var/run/secrets/webhook-server/tls.crt | Path of server certificate of kubefledged-webhook-server |
| args.webhookServerKeyFile | /var/run/secrets/webhook-server/tls.key | Path of server key of kubefledged-webhook-server |
| args.webhookServerPort | 443 | Listening port of kubefledged-webhook-server |
| args.webhookServerLogLevel | INFO | Log level of kubefledged-webhook-server |
| nameOverride | "" | nameOverride replaces the name of the chart in Chart.yaml, when this is used to construct Kubernetes object names |
| fullnameOverride | "" | fullnameOverride completely replaces the generated name |
|  |  |  |