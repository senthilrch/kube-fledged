/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package app

import (
	"fmt"
	"time"

	"github.com/golang/glog"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	fledgedv1alpha1 "k8s.io/kube-fledged/pkg/apis/fledged/v1alpha1"
	clientset "k8s.io/kube-fledged/pkg/client/clientset/versioned"
	fledgedscheme "k8s.io/kube-fledged/pkg/client/clientset/versioned/scheme"
	informers "k8s.io/kube-fledged/pkg/client/informers/externalversions/fledged/v1alpha1"
	listers "k8s.io/kube-fledged/pkg/client/listers/fledged/v1alpha1"
	"k8s.io/kube-fledged/pkg/signals"
)

const controllerAgentName = "fledged"

const (
	// SuccessSynced is used as part of the Event 'reason' when a Foo is synced
	SuccessSynced = "Synced"
	// ErrResourceExists is used as part of the Event 'reason' when a Foo fails
	// to sync due to a Deployment of the same name already existing.
	ErrResourceExists = "ErrResourceExists"

	// MessageResourceExists is the message used for Events when a resource
	// fails to sync due to a Deployment already existing
	MessageResourceExists = "Resource %q already exists and is not managed by Foo"
	// MessageResourceSynced is the message used for an Event fired when a Foo
	// is synced successfully
	MessageResourceSynced = "Foo synced successfully"
)

type EventType string

const (
	EventAdd    EventType = "add"
	EventUpdate EventType = "update"
	EventDelete EventType = "delete"
)

type WorkQueueKey struct {
	eventType EventType
	objKey    string
}

// Controller is the controller implementation for ImageCache resources
type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// fledgedclientset is a clientset for fledged.k8s.io API group
	fledgedclientset clientset.Interface

	nodesLister       corelisters.NodeLister
	nodesSynced       cache.InformerSynced
	imageCachesLister listers.ImageCacheLister
	imageCachesSynced cache.InformerSynced

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue   workqueue.RateLimitingInterface
	statusqueue workqueue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
}

// NewController returns a new fledged controller
func NewController(
	kubeclientset kubernetes.Interface,
	fledgedclientset clientset.Interface,
	nodeInformer coreinformers.NodeInformer,
	imageCacheInformer informers.ImageCacheInformer) *Controller {

	// Create event broadcaster
	// Add fledged types to the default Kubernetes Scheme so Events can be
	// logged for fledged types.
	utilruntime.Must(fledgedscheme.AddToScheme(scheme.Scheme))
	glog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		kubeclientset:     kubeclientset,
		fledgedclientset:  fledgedclientset,
		nodesLister:       nodeInformer.Lister(),
		nodesSynced:       nodeInformer.Informer().HasSynced,
		imageCachesLister: imageCacheInformer.Lister(),
		imageCachesSynced: imageCacheInformer.Informer().HasSynced,
		workqueue:         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ImageCaches"),
		statusqueue:       workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ImagePullerStatus"),
		recorder:          recorder,
	}

	glog.Info("Setting up event handlers")
	// Set up an event handler for when ImageCache resources change
	imageCacheInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			controller.enqueueImageCache(EventAdd, nil, obj)
		},
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueImageCache(EventUpdate, old, new)
		},
		DeleteFunc: func(obj interface{}) {
			controller.enqueueImageCache(EventDelete, obj, nil)
		},
	})
	// Set up an event handler for when Node resources change. This
	// handler will lookup the owner of the given Node, and if it is
	// owned by a Foo resource will enqueue that Foo resource for
	// processing. This way, we don't need to implement custom logic for
	// handling Node resources. More info on this pattern:
	// https://github.com/kubernetes/community/blob/8cafef897a22026d42f5e5bb3f104febe7e29830/contributors/devel/controllers.md
	nodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,
		UpdateFunc: func(old, new interface{}) {
			newNode := new.(*corev1.Node)
			oldNode := old.(*corev1.Node)
			if newNode.ResourceVersion == oldNode.ResourceVersion {
				// Periodic resync will send update events for all known Nodes.
				// Two different versions of the same Nodes will always have different RVs.
				return
			}
			controller.handleObject(new)
		},
		DeleteFunc: controller.handleObject,
	})

	return controller
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	defer c.workqueue.ShutDown()
	defer c.statusqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	glog.Info("Starting fledged controller")

	// Wait for the caches to be synced before starting workers
	glog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.nodesSynced, c.imageCachesSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	glog.Info("Starting workers")
	// Launch two workers to process ImageCache resources
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	glog.Info("Started workers")
	<-stopCh
	glog.Info("Shutting down workers")

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

func (c *Controller) runStatusUpdateWorker() {
	for c.processNextStatusUpdateWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem() bool {
	//glog.Info("processNextWorkItem::Beginning...")
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.workqueue.Done(obj)
		var key WorkQueueKey
		var ok bool
		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workqueue.
		if key, ok = obj.(WorkQueueKey); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workqueue.Forget(obj)
			runtime.HandleError(fmt.Errorf("Unexpected type in workqueue: %#v", obj))
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the
		// ImageCache resource to be synced.
		if err := c.syncHandler(key); err != nil {
			return fmt.Errorf("error syncing '%s': %s", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		glog.Infof("Successfully synced '%s' for event '%s'", key.objKey, key.eventType)
		return nil
	}(obj)

	if err != nil {
		runtime.HandleError(err)
		return true
	}

	return true
}

func (c *Controller) processNextStatusUpdateWorkItem() bool {
	//glog.Info("processNextWorkItem::Beginning...")
	obj, shutdown := c.statusqueue.Get()

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.statusqueue.Done(obj)
		var key string
		var ok bool
		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workqueue.
		if key, ok = obj.(string); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.statusqueue.Forget(obj)
			runtime.HandleError(fmt.Errorf("Unexpected type in statusqueue: %#v", obj))
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the
		// ImageCache resource to be synced.
		if err := c.statusUpdateSyncHandler(key); err != nil {
			return fmt.Errorf("error syncing '%s': %s", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.statusqueue.Forget(obj)
		glog.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		runtime.HandleError(err)
		return true
	}

	return true
}

func (c *Controller) statusUpdateSyncHandler(key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	job, err := c.kubeclientset.BatchV1().Jobs(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		glog.Errorf("Error getting job %s: %v", name, err)
		return err
	}

	if job.Status.CompletionTime == nil {
		// Add this job to the end of the queue
		c.statusqueue.AddRateLimited(key)
		return nil
	}

	// Job has terminated. Read the Job completion status. If it is successful, delete the job
	// from etcd. If not do not delete the job.
	return nil
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the ImageCache resource
// with the current status of the resource.
func (c *Controller) syncHandler(wqKey WorkQueueKey) error {

	var status *fledgedv1alpha1.ImageCacheStatus

	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(wqKey.objKey)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", wqKey.objKey))
		return nil
	}

	switch wqKey.eventType {
	case EventAdd:

		// Get the ImageCache resource with this namespace/name
		imageCache, err := c.imageCachesLister.ImageCaches(namespace).Get(name)
		if err != nil {
			// The ImageCache resource may no longer exist, in which case we stop
			// processing.
			if errors.IsNotFound(err) {
				runtime.HandleError(fmt.Errorf("ImageCache '%s' in work queue no longer exists", wqKey.objKey))
				return nil
			}
			return err
		}

		cacheSpec := imageCache.Spec.CacheSpec
		glog.Infof("cacheSpec: %+v", cacheSpec)
		var nodes []*corev1.Node

		status = &fledgedv1alpha1.ImageCacheStatus{
			Status:  fledgedv1alpha1.ImageCacheActionStatusProcessing,
			Reason:  fledgedv1alpha1.ImageCacheReasonPullingImages,
			Message: fledgedv1alpha1.ImageCacheMessagePullingImages,
		}

		if err = c.updateImageCacheStatus(imageCache, status); err != nil {
			glog.Errorf("Error updating imagecache status to %s: %v", status.Status, err)
			return err
		}

		for _, i := range cacheSpec {
			if len(i.NodeSelector) > 0 {
				if nodes, err = c.nodesLister.List(labels.Set(i.NodeSelector).AsSelector()); err != nil {
					glog.Errorf("Error listing nodes using nodeselector %+v: %v", i.NodeSelector, err)
					return err
				}
			} else {
				if nodes, err = c.nodesLister.List(labels.Everything()); err != nil {
					glog.Errorf("Error listing nodes using nodeselector labels.Everything(): %v", err)
					return err
				}
			}
			//glog.Infof("No. of nodes in %+v is %d", i.NodeSelector, len(nodes))
			if len(nodes) == 0 {
				glog.Errorf("NodeSelector %+v did not match any nodes.", i.NodeSelector)
				return fmt.Errorf("NodeSelector %+v did not match any nodes", i.NodeSelector)
			}

			for _, n := range nodes {
				for m := range i.Images {
					// Define new Job manifest
					job := newJob(imageCache, i.Images[m], n.Labels["kubernetes.io/hostname"])
					// Create a Job to pull the image into the node
					if _, err = c.kubeclientset.BatchV1().Jobs(imageCache.Namespace).Create(job); err != nil {
						glog.Errorf("Error creating job in node %s: %v", n.Name, err)
						return err
					}

					key, err := cache.MetaNamespaceKeyFunc(*job)
					if err != nil {
						runtime.HandleError(err)
						return err
					}
					c.statusqueue.AddRateLimited(key)
				}
			}
		}
		// Start and stop the statusqueue worker in an exponential backoff manner
		// subject to a max deadlineseconds.
		// Launch one worker to process the imagepuller queue
		stopCh := signals.SetupSignalHandler()
		go wait.Until(c.runStatusUpdateWorker, time.Second, stopCh)
		// Now check the length of the statusqueue. If there are still items in the queue,
		// it means those Jobs are stuck because of some error. fetch the pods, get the error
		// reasons, update the imagecache status. Do not delete the failing jobs, so user can
		// check the reason and delete those jobs manually

		// Finally, we update the status block of the ImageCache resource to reflect the
		// current state of the world
		err = c.updateImageCacheStatus(imageCache, status)
		if err != nil {
			return err
		}

		c.recorder.Event(imageCache, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
		return nil

	}

	return nil

}

func (c *Controller) updateImageCacheStatus(imageCache *fledgedv1alpha1.ImageCache, status *fledgedv1alpha1.ImageCacheStatus) error {
	// NEVER modify objects from the store. It's a read-only, local cache.
	// You can use DeepCopy() to make a deep copy of original object and modify this copy
	// Or create a copy manually for better performance
	imageCacheCopy := imageCache.DeepCopy()
	imageCacheCopy.Status = *status
	// If the CustomResourceSubresources feature gate is not enabled,
	// we must use Update instead of UpdateStatus to update the Status block of the ImageCache resource.
	// UpdateStatus will not allow changes to the Spec of the resource,
	// which is ideal for ensuring nothing other than resource status has been updated.
	_, err := c.fledgedclientset.FledgedV1alpha1().ImageCaches(imageCache.Namespace).Update(imageCacheCopy)
	return err
}

// enqueueImageCache takes a ImageCache resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than ImageCache.
func (c *Controller) enqueueImageCache(eventType EventType, old, new interface{}) {
	var key string
	var err error
	var obj interface{}
	var wqKey WorkQueueKey

	//glog.Info("enqueueImageCache::Processing ImageCache creation/modification...")

	switch eventType {
	case EventAdd:
		obj = new
	case EventUpdate:
		obj = new
		return
	case EventDelete:
		obj = old
	}

	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	}
	wqKey.eventType = eventType
	wqKey.objKey = key

	c.workqueue.AddRateLimited(wqKey)

	//glog.Info("enqueueImageCache::ImageCache resource queued...")
}

// handleObject will take any resource implementing metav1.Object and attempt
// to find the Foo resource that 'owns' it. It does this by looking at the
// objects metadata.ownerReferences field for an appropriate OwnerReference.
// It then enqueues that Foo resource to be processed. If the object does not
// have an appropriate OwnerReference, it will simply be skipped.
func (c *Controller) handleObject(obj interface{}) {
	/*
		var object metav1.Object
		var ok bool
		if object, ok = obj.(metav1.Object); !ok {
			tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
			if !ok {
				runtime.HandleError(fmt.Errorf("error decoding object, invalid type"))
				return
			}
			object, ok = tombstone.Obj.(metav1.Object)
			if !ok {
				runtime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
				return
			}
			glog.V(4).Infof("Recovered deleted object '%s' from tombstone", object.GetName())
		}
		glog.V(4).Infof("Processing object: %s", object.GetName())
		if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
			// If this object is not owned by a Foo, we should not do anything more
			// with it.
			if ownerRef.Kind != "Foo" {
				return
			}

			foo, err := c.foosLister.Foos(object.GetNamespace()).Get(ownerRef.Name)
			if err != nil {
				glog.V(4).Infof("ignoring orphaned object '%s' of foo '%s'", object.GetSelfLink(), ownerRef.Name)
				return
			}

			c.enqueueFoo(foo)
			return
		}
	*/
}

func newJob(imagecache *fledgedv1alpha1.ImageCache, image string, hostname string) *batchv1.Job {
	labels := map[string]string{
		"app":        "imagecache",
		"imagecache": imagecache.Name,
		"controller": controllerAgentName,
	}

	backoffLimit := int32(0)
	//activeDeadlineSeconds := int64(120)

	/*
		container := corev1.Container{
			Command: []string{"/tmp/bin/echo", "Image pulled successfully!"},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "tmp-bin",
					MountPath: "/tmp/bin",
				},
			},
			ImagePullPolicy: corev1.PullIfNotPresent,
		}

		var containers []corev1.Container

		for i := range images {
			container.Image = images[i]
			container.Name = "imagepuller-" + strconv.Itoa(i+1)
			containers = append(containers, container)
		}
	*/

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: imagecache.Name + "-",
			Namespace:    imagecache.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(imagecache, schema.GroupVersionKind{
					Group:   fledgedv1alpha1.SchemeGroupVersion.Group,
					Version: fledgedv1alpha1.SchemeGroupVersion.Version,
					Kind:    "ImageCache",
				}),
			},
			Labels: labels,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			//ActiveDeadlineSeconds: &activeDeadlineSeconds,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					//GenerateName: imagecache.Name,
					Namespace: imagecache.Namespace,
					Labels:    labels,
					//Finalizers:   []string{"imagecache"},
				},
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"kubernetes.io/hostname": hostname,
					},
					InitContainers: []corev1.Container{
						{
							Name:    "busybox",
							Image:   "busybox:1.29.2",
							Command: []string{"cp", "/bin/echo", "/tmp/bin"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "tmp-bin",
									MountPath: "/tmp/bin",
								},
							},
							ImagePullPolicy: corev1.PullIfNotPresent,
						},
					},
					Containers: []corev1.Container{
						{
							Name:    "imagepuller",
							Image:   image,
							Command: []string{"/tmp/bin/echo", "Image pulled successfully!"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "tmp-bin",
									MountPath: "/tmp/bin",
								},
							},
							ImagePullPolicy: corev1.PullIfNotPresent,
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "tmp-bin",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					//ActiveDeadlineSeconds: &activeDeadlineSeconds,
				},
			},
		},
	}

	return job
}
