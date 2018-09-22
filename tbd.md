# Problem statement:-

An application deployed as a Pod in Kubernetes might need the capability to launch a dependant application. For e.g. A capital market application
might need to start a dependant application at the end of a trading session. the dependant application will perform post processing of
trades executed on that particular day.

Another e.g. is when a master application requires to perform certain tasks that require special hardware resources/devices, it can start a 
dependant application on a node that has the necessary resources.

There are many use cases for having such a capability within applications in Kubernetes.

# Security considerations:-

In principle, a Kubernetes cluster can be setup to provide RBAC access to applications to access the Kubernetes API. Once setup the application
can perform the API calls and operations that are allowed in the Role specification. However this approach has some security weaknesses. So
we would need a more robust solution to solve this problem.

# High level design:-

We need to have a custom controller developed which will act on behalf of the master application. The controller will have all necessary
business logic and checks implemented in it to manage and control the creation of dependent application. Kubernetes Custom Resource Definition
(CRD) can be used to implement the custom controller.

## The role of the Custom Controller:-

1. If an application needs this capability, when the deployment of the master is created a corresponding crd resource is also created.
2. The crd resource should have ownerReference pointing to the master deployment.
3. The crd resource's template section should have the list of resource manifests.
4. When the master application wants to create the dependant application, the controller will create the resources mentioned in the template section.
5. So this way the master application is prevented from creating arbitrary objects by itself. Also this way it becomes transparent to the
developer/administrator what objects would get created for the dependant application.

## TBD:-

1. Instead of CRD can we use API aggregation or the Operator pattern?
2. What is the mechanism by which the master Pod signals the controller when it wants the dependant application to be created?
3. How will the controller return back the result of resource creation and the Endpoint/Credentials of the dependant application to 
the master?
4. How will the master Pod signal the controller to delete the dependant application?
5. IDentify more usecases where this feature can be used.

## Pod Manifest (Annotations):-

If a Pod needs to have the ability to launch a new application then the Pod's manifest should have a suitable annotation. In the absence
of this annotation the controller will not honor any request to create a dependant application. The annotation will also have details as
to what application the Pod is allowed to launch together with other attributes.

When the controller receives a request from a Pod for launching the new application, the Controller will read the annotation. The annotatio
will have all necessary details for launching the new application.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: myapp-pod
  labels:
    app: myapp
  annotations:
    fledged.io: 
	  manifests: <name of configmap containing manifests to be created>
	  helm-chart: <path of helm chart defining the application>
	  duration: <time duration in seconds after which the application should be deleted>
spec:
  containers:
  - name: myapp-container
    image: busybox
    command: ['sh', '-c', 'echo Hello Kubernetes! && sleep 3600']
```
