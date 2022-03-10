# Helm chart parameters

| Parameter | Default value | Description |
| --------- | ------------- | ----------- |
| controllerReplicaCount    | 1        | No. of replicas of kubefledged-controller |
| webhookServerReplicaCount | 1        | No. of replicas of kubefledged-webhook-server |
| controller.hostNetwork    | false    | When set to "true", kubefledged-controller pod runs with "hostNetwork: true" |
| webhookServer.enable      | false    | When set to "true", kubefledged-webhook-server is installed |
| webhookServer.hostNetwork | false    | When set to "true", kubefledged-webhook-server pod runs with "hostNetwork: true" |
| image.kubefledgedControllerRepository | docker.io/senthilrch/kubefledged-controller | Repository name of kubefledged-controller image |
| image.kubefledgedCRIClientRepository | docker.io/senthilrch/kubefledged-cri-client | Repository name of kubefledged-cri-client image |
| image.kubefledgedWebhookServerRepository | docker.io/senthilrch/kubefledged-webhook-server | Repository name of kubefledged-webhook-server image |
| image.pullPolicy | Always | Image pull policy for kubefledged-controller and kubefledged-webhook-server pods |
| args.controllerImageCacheRefreshFrequency | 15m | The image cache is refreshed periodically to ensure the cache is up to date. Setting this flag to "0s" will disable refresh |
| args.controllerImageDeleteJobHostNetwork | false | Whether the pod for the image delete job should be run with 'HostNetwork: true' |
| args.controllerImagePullDeadlineDuration | 5m | Maximum duration allowed for pulling an image. After this duration, image pull is considered to have failed |
| args.controllerImagePullPolicy | IfNotPresent | Image pull policy for pulling images into and refreshing the cache. Possible values are 'IfNotPresent' and 'Always'. Default value is 'IfNotPresent'. Image with no or ":latest" tag are always pulled |
| args.controllerServiceAccountName | "" | serviceAccountName used in Jobs created for pulling or deleting images. Optional flag. If not specified the default service account of the namespace is used |
| args.controllerLogLevel | INFO | Log level of kubefledged-controller |
| args.webhookServerCertFile | /var/run/secrets/webhook-server/tls.crt | Path of server certificate of kubefledged-webhook-server |
| args.webhookServerKeyFile | /var/run/secrets/webhook-server/tls.key | Path of server key of kubefledged-webhook-server |
| args.webhookServerPort | 443 | Listening port of kubefledged-webhook-server |
| args.webhookServerLogLevel | INFO | Log level of kubefledged-webhook-server |
| nameOverride | "" | nameOverride replaces the name of the chart in Chart.yaml, when this is used to construct Kubernetes object names |
| fullnameOverride | "" | fullnameOverride completely replaces the generated name |
|  |  |  |