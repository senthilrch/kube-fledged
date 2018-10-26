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
	"reflect"
	"time"

	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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
	"k8s.io/kube-fledged/pkg/images"
)

const controllerAgentName = "fledged"
const fledgedNameSpace = "kube-fledged"

const (
	// SuccessSynced is used as part of the Event 'reason' when a ImageCache is synced
	SuccessSynced = "Synced"
	// MessageResourceSynced is the message used for an Event fired when a ImageCache
	// is synced successfully
	MessageResourceSynced = "Foo synced successfully"
)

// Controller is the controller for ImageCache resources
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
	workqueue      workqueue.RateLimitingInterface
	imagepullqueue workqueue.RateLimitingInterface
	imageManager   *images.ImageManager
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder                   record.EventRecorder
	imageCacheRefreshFrequency time.Duration
}

// NewController returns a new fledged controller
func NewController(
	kubeclientset kubernetes.Interface,
	fledgedclientset clientset.Interface,
	nodeInformer coreinformers.NodeInformer,
	imageCacheInformer informers.ImageCacheInformer,
	imageCacheRefreshFrequency time.Duration,
	imagePullDeadlineDuration time.Duration) *Controller {

	utilruntime.Must(fledgedscheme.AddToScheme(scheme.Scheme))
	glog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		kubeclientset:              kubeclientset,
		fledgedclientset:           fledgedclientset,
		nodesLister:                nodeInformer.Lister(),
		nodesSynced:                nodeInformer.Informer().HasSynced,
		imageCachesLister:          imageCacheInformer.Lister(),
		imageCachesSynced:          imageCacheInformer.Informer().HasSynced,
		workqueue:                  workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ImageCaches"),
		imagepullqueue:             workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ImagePullerStatus"),
		recorder:                   recorder,
		imageCacheRefreshFrequency: imageCacheRefreshFrequency,
	}

	imageManager := images.NewImageManager(controller.workqueue, controller.imagepullqueue, controller.kubeclientset, fledgedNameSpace, imagePullDeadlineDuration)
	controller.imageManager = imageManager

	glog.Info("Setting up event handlers")
	// Set up an event handler for when ImageCache resources change
	imageCacheInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			controller.enqueueImageCache(images.ImageCacheCreate, nil, obj)
		},
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueImageCache(images.ImageCacheUpdate, old, new)
		},
		DeleteFunc: func(obj interface{}) {
			controller.enqueueImageCache(images.ImageCacheDelete, obj, nil)
		},
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
	defer c.imagepullqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	glog.Info("Starting fledged controller")

	// Wait for the caches to be synced before starting workers
	glog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.nodesSynced, c.imageCachesSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	glog.Info("Starting image cache worker")
	// Launch workers to process ImageCache resources
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	if c.imageCacheRefreshFrequency.Nanoseconds() != int64(0) {
		glog.Info("Starting cache refresh worker")
		go wait.Until(c.runRefreshWorker, c.imageCacheRefreshFrequency, stopCh)
	}

	glog.Info("Started workers")
	c.imageManager.Run(stopCh)

	<-stopCh
	glog.Info("Shutting down workers")

	return nil
}

// enqueueImageCache takes a ImageCache resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than ImageCache.
func (c *Controller) enqueueImageCache(workType images.WorkType, old, new interface{}) {
	var key string
	var err error
	var obj interface{}
	var wqKey images.WorkQueueKey

	switch workType {
	case images.ImageCacheCreate:
		obj = new
	case images.ImageCacheUpdate:
		obj = new
		oldImageCache := old.(*fledgedv1alpha1.ImageCache)
		newImageCache := new.(*fledgedv1alpha1.ImageCache)
		if reflect.DeepEqual(newImageCache.Spec, oldImageCache.Spec) {
			return
		}
	case images.ImageCacheDelete:
		obj = old
	case images.ImageCacheRefresh:
		obj = old
	}

	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	}
	wqKey.WorkType = workType
	wqKey.ObjKey = key

	c.workqueue.AddRateLimited(wqKey)

	glog.V(4).Infof("enqueueImageCache::ImageCache resource queued for work type %s", workType)
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
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
		var key images.WorkQueueKey
		var ok bool
		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workqueue.
		if key, ok = obj.(images.WorkQueueKey); !ok {
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
			return fmt.Errorf("error syncing imagecache: %v", err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		//glog.Infof("Successfully synced '%s' for event '%s'", key.ObjKey, key.WorkType)
		return nil
	}(obj)

	if err != nil {
		runtime.HandleError(err)
		return true
	}

	return true
}

// runRefreshWorker is resposible of refreshing the image cache
func (c *Controller) runRefreshWorker() {
	// List the ImageCache resources
	imageCaches, err := c.imageCachesLister.ImageCaches(fledgedNameSpace).List(labels.Everything())
	if err != nil {
		glog.Errorf("Error in listing image caches: %v", err)
		return
	}
	for i := range imageCaches {
		// Do not refresh if image cache is already under processing
		if imageCaches[i].Status.Status != fledgedv1alpha1.ImageCacheActionStatusProcessing {
			c.enqueueImageCache(images.ImageCacheRefresh, imageCaches[i], nil)
		}
	}
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the ImageCache resource
// with the current status of the resource.
func (c *Controller) syncHandler(wqKey images.WorkQueueKey) error {
	status := &fledgedv1alpha1.ImageCacheStatus{
		Failures: map[string][]fledgedv1alpha1.NodeReasonMessage{},
	}

	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(wqKey.ObjKey)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", wqKey.ObjKey))
		return nil
	}

	glog.Infof("Starting to sync image cache %s(%s)", name, wqKey.WorkType)

	switch wqKey.WorkType {
	case images.ImageCacheCreate, images.ImageCacheUpdate, images.ImageCacheRefresh:

		// Get the ImageCache resource with this namespace/name
		imageCache, err := c.imageCachesLister.ImageCaches(namespace).Get(name)
		if err != nil {
			// The ImageCache resource may no longer exist, in which case we stop
			// processing.
			if errors.IsNotFound(err) {
				runtime.HandleError(fmt.Errorf("ImageCache '%s' in work queue no longer exists", wqKey.ObjKey))
				return nil
			}
			return err
		}

		cacheSpec := imageCache.Spec.CacheSpec
		glog.V(4).Infof("cacheSpec: %+v", cacheSpec)
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
			glog.V(4).Infof("No. of nodes in %+v is %d", i.NodeSelector, len(nodes))
			if len(nodes) == 0 {
				glog.Errorf("NodeSelector %+v did not match any nodes.", i.NodeSelector)
				return fmt.Errorf("NodeSelector %+v did not match any nodes", i.NodeSelector)
			}

			for _, n := range nodes {
				for m := range i.Images {
					ipr := images.ImagePullRequest{
						Image:      i.Images[m],
						Node:       n.Labels["kubernetes.io/hostname"],
						Imagecache: imageCache,
					}
					c.imagepullqueue.AddRateLimited(ipr)
				}
			}
		}
		// We add an empty image pull request to signal the image manager that all
		// requests for this sync action have been placed in the imagepullqueue
		c.imagepullqueue.AddRateLimited(images.ImagePullRequest{})

		//c.recorder.Event(imageCache, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
		//return nil

	case images.ImageCacheStatusUpdate:
		glog.Infof("wqKey.Status = %+v", wqKey.Status)
		// Finally, we update the status block of the ImageCache resource to reflect the
		// current state of the world
		// Get the ImageCache resource with this namespace/name
		imageCache, err := c.fledgedclientset.FledgedV1alpha1().ImageCaches(namespace).Get(name, metav1.GetOptions{})
		if err != nil {
			glog.Errorf("Error getting image cache %s: %v", name, err)
			return err
		}

		status.Status = fledgedv1alpha1.ImageCacheActionStatusSucceeded
		status.Reason = fledgedv1alpha1.ImageCacheReasonImagesPulledSuccessfully
		status.Message = fledgedv1alpha1.ImageCacheMessageImagesPulledSuccessfully

		for _, v := range *wqKey.Status {
			if v.Status == images.ImagePullResultStatusFailed {
				status.Failures[v.ImagePullRequest.Image] = append(
					status.Failures[v.ImagePullRequest.Image], fledgedv1alpha1.NodeReasonMessage{
						Node:    v.ImagePullRequest.Node,
						Reason:  v.Reason,
						Message: v.Message,
					})
				status.Status = fledgedv1alpha1.ImageCacheActionStatusFailed
				status.Reason = fledgedv1alpha1.ImageCacheReasonImagePullFailedForSomeImages
				status.Message = fledgedv1alpha1.ImageCacheMessageImagePullFailedForSomeImages
			}
		}

		err = c.updateImageCacheStatus(imageCache, status)
		if err != nil {
			glog.Errorf("Error updating ImageCache status: %v", err)
			return err
		}
	case images.ImageCacheDelete:
		//TODO: Use finalizer. delete all images in the nodes. Once deleted, remove the finalizer
		//For now we leave the images in the nodes.
	}
	glog.Infof("Completed sync actions for image cache %s(%s)", name, wqKey.WorkType)
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
