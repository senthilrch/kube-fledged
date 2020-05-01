/*
Copyright 2018 The kube-fledged authors.

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
	fledgedv1alpha1 "github.com/senthilrch/kube-fledged/pkg/apis/fledged/v1alpha1"
	clientset "github.com/senthilrch/kube-fledged/pkg/client/clientset/versioned"
	fledgedscheme "github.com/senthilrch/kube-fledged/pkg/client/clientset/versioned/scheme"
	informers "github.com/senthilrch/kube-fledged/pkg/client/informers/externalversions/fledged/v1alpha1"
	listers "github.com/senthilrch/kube-fledged/pkg/client/listers/fledged/v1alpha1"
	"github.com/senthilrch/kube-fledged/pkg/images"
	corev1 "k8s.io/api/core/v1"
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
)

const controllerAgentName = "fledged"
const fledgedCacheSpecValidationKey = "fledged.k8s.io/cachespecvalidation"
const imageCachePurgeAnnotationKey = "fledged.k8s.io/purge-imagecache"
const imageCacheRefreshAnnotationKey = "fledged.k8s.io/refresh-imagecache"

const (
	// SuccessSynced is used as part of the Event 'reason' when a ImageCache is synced
	SuccessSynced = "Synced"
	// MessageResourceSynced is the message used for an Event fired when a ImageCache
	// is synced successfully
	MessageResourceSynced = "ImageCache synced successfully"
)

// Controller is the controller for ImageCache resources
type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// fledgedclientset is a clientset for fledged.k8s.io API group
	fledgedclientset clientset.Interface

	fledgedNameSpace  string
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
	imageworkqueue workqueue.RateLimitingInterface
	imageManager   *images.ImageManager
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder                   record.EventRecorder
	imageCacheRefreshFrequency time.Duration
	imagePullPolicy            string
}

// NewController returns a new fledged controller
func NewController(
	kubeclientset kubernetes.Interface,
	fledgedclientset clientset.Interface,
	namespace string,
	nodeInformer coreinformers.NodeInformer,
	imageCacheInformer informers.ImageCacheInformer,
	imageCacheRefreshFrequency time.Duration,
	imagePullDeadlineDuration time.Duration,
	dockerClientImage string,
	imagePullPolicy string) *Controller {

	utilruntime.Must(fledgedscheme.AddToScheme(scheme.Scheme))
	glog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		kubeclientset:              kubeclientset,
		fledgedclientset:           fledgedclientset,
		fledgedNameSpace:           namespace,
		nodesLister:                nodeInformer.Lister(),
		nodesSynced:                nodeInformer.Informer().HasSynced,
		imageCachesLister:          imageCacheInformer.Lister(),
		imageCachesSynced:          imageCacheInformer.Informer().HasSynced,
		workqueue:                  workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ImageCaches"),
		imageworkqueue:             workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ImagePullerStatus"),
		recorder:                   recorder,
		imageCacheRefreshFrequency: imageCacheRefreshFrequency,
		imagePullPolicy:            imagePullPolicy,
	}

	imageManager, _ := images.NewImageManager(controller.workqueue, controller.imageworkqueue, controller.kubeclientset, controller.fledgedNameSpace, imagePullDeadlineDuration, dockerClientImage, imagePullPolicy)
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

// PreFlightChecks performs pre-flight checks and actions before the controller is started
func (c *Controller) PreFlightChecks() error {
	if err := c.danglingJobs(); err != nil {
		return err
	}
	if err := c.danglingImageCaches(); err != nil {
		return err
	}
	return nil
}

// danglingJobs finds and removes dangling or stuck jobs
func (c *Controller) danglingJobs() error {
	joblist, err := c.kubeclientset.BatchV1().Jobs(c.fledgedNameSpace).List(metav1.ListOptions{})
	if err != nil {
		glog.Errorf("Error listing jobs: %v", err)
		return err
	}

	if joblist == nil || len(joblist.Items) == 0 {
		glog.Info("No dangling or stuck jobs found...")
		return nil
	}
	deletePropagation := metav1.DeletePropagationBackground
	for _, job := range joblist.Items {
		err := c.kubeclientset.BatchV1().Jobs(c.fledgedNameSpace).
			Delete(job.Name, &metav1.DeleteOptions{PropagationPolicy: &deletePropagation})
		if err != nil {
			glog.Errorf("Error deleting job(%s): %v", job.Name, err)
			return err
		}
		glog.Infof("Dangling Job(%s) deleted", job.Name)
	}
	return nil
}

// danglingImageCaches finds dangling or stuck image cache and marks them as abhorted. Such
// image caches will get refreshed in the next cycle
func (c *Controller) danglingImageCaches() error {
	dangling := false
	imagecachelist, err := c.fledgedclientset.FledgedV1alpha1().ImageCaches(c.fledgedNameSpace).List(metav1.ListOptions{})
	if err != nil {
		glog.Errorf("Error listing imagecaches: %v", err)
		return err
	}

	if imagecachelist == nil || len(imagecachelist.Items) == 0 {
		glog.Info("No dangling or stuck imagecaches found...")
		return nil
	}
	status := &fledgedv1alpha1.ImageCacheStatus{
		Failures: map[string]fledgedv1alpha1.NodeReasonMessageList{},
		Status:   fledgedv1alpha1.ImageCacheActionStatusAborted,
		Reason:   fledgedv1alpha1.ImageCacheReasonImagePullAborted,
		Message:  fledgedv1alpha1.ImageCacheMessageImagePullAborted,
	}
	for _, imagecache := range imagecachelist.Items {
		if imagecache.Status.Status == fledgedv1alpha1.ImageCacheActionStatusProcessing {
			status.StartTime = imagecache.Status.StartTime
			err := c.updateImageCacheStatus(&imagecache, status)
			if err != nil {
				glog.Errorf("Error updating ImageCache(%s) status to '%s': %v", imagecache.Name, fledgedv1alpha1.ImageCacheActionStatusAborted, err)
				return err
			}
			dangling = true
			glog.Infof("Dangling Image cache(%s) status changed to '%s'", imagecache.Name, fledgedv1alpha1.ImageCacheActionStatusAborted)
		}
	}

	if !dangling {
		glog.Info("No dangling or stuck imagecaches found...")
	}
	return nil
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	defer c.workqueue.ShutDown()
	defer c.imageworkqueue.ShutDown()

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
	if err := c.imageManager.Run(stopCh); err != nil {
		glog.Fatalf("Error running image manager: %s", err.Error())
	}

	<-stopCh
	glog.Info("Shutting down workers")

	return nil
}

// enqueueImageCache takes a ImageCache resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than ImageCache.
func (c *Controller) enqueueImageCache(workType images.WorkType, old, new interface{}) bool {
	var key string
	var err error
	var obj interface{}
	wqKey := images.WorkQueueKey{}

	switch workType {
	case images.ImageCacheCreate:
		obj = new
		newImageCache := new.(*fledgedv1alpha1.ImageCache)
		// If the ImageCache resource already has a status field, it means it's already
		// synced, so do not queue it for processing
		if !reflect.DeepEqual(newImageCache.Status, fledgedv1alpha1.ImageCacheStatus{}) {
			return false
		}
	case images.ImageCacheUpdate:
		obj = new
		oldImageCache := old.(*fledgedv1alpha1.ImageCache)
		newImageCache := new.(*fledgedv1alpha1.ImageCache)

		if oldImageCache.Status.Status == fledgedv1alpha1.ImageCacheActionStatusProcessing {
			if !reflect.DeepEqual(newImageCache.Spec, oldImageCache.Spec) {
				glog.Warningf("Received image cache update/purge/delete for '%s' while it is under processing, so ignoring.", oldImageCache.Name)
				return false
			}
		}
		if _, exists := newImageCache.Annotations[imageCachePurgeAnnotationKey]; exists {
			if _, exists := oldImageCache.Annotations[imageCachePurgeAnnotationKey]; !exists {
				workType = images.ImageCachePurge
				break
			}
		}
		if _, exists := newImageCache.Annotations[imageCacheRefreshAnnotationKey]; exists {
			if _, exists := oldImageCache.Annotations[imageCacheRefreshAnnotationKey]; !exists {
				workType = images.ImageCacheRefresh
				break
			}
		}
		if !reflect.DeepEqual(newImageCache.Spec, oldImageCache.Spec) {
			if validation, ok := newImageCache.Annotations[fledgedCacheSpecValidationKey]; ok {
				if validation == "failed" {
					if err := c.removeAnnotation(newImageCache, fledgedCacheSpecValidationKey); err != nil {
						glog.Errorf("Error removing Annotation %s from imagecache(%s): %v", fledgedCacheSpecValidationKey, newImageCache.Name, err)
					}
					return false
				}
			}
		} else {
			return false
		}
	case images.ImageCacheDelete:
		return false

	case images.ImageCacheRefresh:
		obj = old
	}

	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return false
	}
	wqKey.WorkType = workType
	wqKey.ObjKey = key
	if workType == images.ImageCacheUpdate {
		oldImageCache := old.(*fledgedv1alpha1.ImageCache)
		wqKey.OldImageCache = oldImageCache
	}

	c.workqueue.AddRateLimited(wqKey)
	glog.V(4).Infof("enqueueImageCache::ImageCache resource queued for work type %s", workType)
	return true
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
			glog.Errorf("error syncing imagecache: %v", err.Error())
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
	imageCaches, err := c.imageCachesLister.ImageCaches(c.fledgedNameSpace).List(labels.Everything())
	if err != nil {
		glog.Errorf("Error in listing image caches: %v", err)
		return
	}
	for i := range imageCaches {
		// Do not refresh if status is not yet updated
		if reflect.DeepEqual(imageCaches[i].Status, fledgedv1alpha1.ImageCacheStatus{}) {
			continue
		}
		// Do not refresh if image cache is already under processing
		if imageCaches[i].Status.Status == fledgedv1alpha1.ImageCacheActionStatusProcessing {
			continue
		}
		// Do not refresh image cache if cache spec validation failed
		if imageCaches[i].Status.Status == fledgedv1alpha1.ImageCacheActionStatusFailed &&
			imageCaches[i].Status.Reason == fledgedv1alpha1.ImageCacheReasonCacheSpecValidationFailed {
			continue
		}
		// Do not refresh if image cache has been purged
		if imageCaches[i].Status.Reason == fledgedv1alpha1.ImageCacheReasonImageCachePurge {
			continue
		}
		c.enqueueImageCache(images.ImageCacheRefresh, imageCaches[i], nil)
	}
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the ImageCache resource
// with the current status of the resource.
func (c *Controller) syncHandler(wqKey images.WorkQueueKey) error {
	status := &fledgedv1alpha1.ImageCacheStatus{
		Failures: map[string]fledgedv1alpha1.NodeReasonMessageList{},
	}

	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(wqKey.ObjKey)
	if err != nil {
		glog.Errorf("Error from cache.SplitMetaNamespaceKey(): %v", err)
		return err
	}

	glog.Infof("Starting to sync image cache %s(%s)", name, wqKey.WorkType)

	switch wqKey.WorkType {
	case images.ImageCacheCreate, images.ImageCacheUpdate, images.ImageCacheRefresh, images.ImageCachePurge:

		startTime := metav1.Now()
		status.StartTime = &startTime
		// Get the ImageCache resource with this namespace/name
		imageCache, err := c.imageCachesLister.ImageCaches(namespace).Get(name)
		if err != nil {
			// The ImageCache resource may no longer exist, in which case we stop
			// processing.
			glog.Errorf("Error getting imagecache(%s): %v", name, err)
			return err
		}

		err = validateCacheSpec(c, imageCache)
		if err != nil {
			status.Status = fledgedv1alpha1.ImageCacheActionStatusFailed
			status.Reason = fledgedv1alpha1.ImageCacheReasonCacheSpecValidationFailed
			status.Message = err.Error()

			if err := c.updateImageCacheStatus(imageCache, status); err != nil {
				glog.Errorf("Error updating imagecache status to %s: %v", status.Status, err)
				return err
			}

			return err
		}

		if wqKey.WorkType == images.ImageCacheUpdate && wqKey.OldImageCache == nil {
			status.Status = fledgedv1alpha1.ImageCacheActionStatusFailed
			status.Reason = fledgedv1alpha1.ImageCacheReasonOldImageCacheNotFound
			status.Message = fledgedv1alpha1.ImageCacheMessageOldImageCacheNotFound

			if err := c.updateImageCacheStatus(imageCache, status); err != nil {
				glog.Errorf("Error updating imagecache status to %s: %v", status.Status, err)
				return err
			}
			glog.Errorf("%s: %s", fledgedv1alpha1.ImageCacheReasonOldImageCacheNotFound, fledgedv1alpha1.ImageCacheMessageOldImageCacheNotFound)
			return fmt.Errorf("%s: %s", fledgedv1alpha1.ImageCacheReasonOldImageCacheNotFound, fledgedv1alpha1.ImageCacheMessageOldImageCacheNotFound)
		}

		if wqKey.WorkType == images.ImageCacheUpdate {
			if len(wqKey.OldImageCache.Spec.CacheSpec) != len(imageCache.Spec.CacheSpec) {
				status.Status = fledgedv1alpha1.ImageCacheActionStatusFailed
				status.Reason = fledgedv1alpha1.ImageCacheReasonCacheSpecValidationFailed
				status.Message = fledgedv1alpha1.ImageCacheMessageNotSupportedUpdates

				if err = c.updateImageCacheSpecAndStatus(imageCache, wqKey.OldImageCache.Spec, status); err != nil {
					glog.Errorf("Error updating imagecache spec and status to %s: %v", status.Status, err)
					return err
				}
				glog.Errorf("%s: %s", fledgedv1alpha1.ImageCacheReasonCacheSpecValidationFailed, "Mismatch in no. of image lists")
				return fmt.Errorf("%s: %s", fledgedv1alpha1.ImageCacheReasonCacheSpecValidationFailed, "Mismatch in no. of image lists")
			}

			for i := range wqKey.OldImageCache.Spec.CacheSpec {
				if !reflect.DeepEqual(wqKey.OldImageCache.Spec.CacheSpec[i].NodeSelector, imageCache.Spec.CacheSpec[i].NodeSelector) {
					status.Status = fledgedv1alpha1.ImageCacheActionStatusFailed
					status.Reason = fledgedv1alpha1.ImageCacheReasonCacheSpecValidationFailed
					status.Message = fledgedv1alpha1.ImageCacheMessageNotSupportedUpdates

					if err = c.updateImageCacheSpecAndStatus(imageCache, wqKey.OldImageCache.Spec, status); err != nil {
						glog.Errorf("Error updating imagecache spec and status to %s: %v", status.Status, err)
						return err
					}
					glog.Errorf("%s: %s", fledgedv1alpha1.ImageCacheReasonCacheSpecValidationFailed, "Mismatch in node selector")
					return fmt.Errorf("%s: %s", fledgedv1alpha1.ImageCacheReasonCacheSpecValidationFailed, "Mismatch in node selector")
				}
			}
		}

		cacheSpec := imageCache.Spec.CacheSpec
		glog.V(4).Infof("cacheSpec: %+v", cacheSpec)
		var nodes []*corev1.Node

		status.Status = fledgedv1alpha1.ImageCacheActionStatusProcessing

		if wqKey.WorkType == images.ImageCacheCreate {
			status.Reason = fledgedv1alpha1.ImageCacheReasonImageCacheCreate
			status.Message = fledgedv1alpha1.ImageCacheMessagePullingImages
		}

		if wqKey.WorkType == images.ImageCacheUpdate {
			status.Reason = fledgedv1alpha1.ImageCacheReasonImageCacheUpdate
			status.Message = fledgedv1alpha1.ImageCacheMessageUpdatingCache
		}

		if wqKey.WorkType == images.ImageCacheRefresh {
			status.Reason = fledgedv1alpha1.ImageCacheReasonImageCacheRefresh
			status.Message = fledgedv1alpha1.ImageCacheMessageRefreshingCache
		}

		if wqKey.WorkType == images.ImageCachePurge {
			status.Reason = fledgedv1alpha1.ImageCacheReasonImageCachePurge
			status.Message = fledgedv1alpha1.ImageCacheMessagePurgeCache
		}

		imageCache, err = c.fledgedclientset.FledgedV1alpha1().ImageCaches(namespace).Get(name, metav1.GetOptions{})
		if err != nil {
			glog.Errorf("Error getting imagecache(%s) from api server: %v", name, err)
			return err
		}

		if err = c.updateImageCacheStatus(imageCache, status); err != nil {
			glog.Errorf("Error updating imagecache status to %s: %v", status.Status, err)
			return err
		}

		for k, i := range cacheSpec {
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
					pullImage, err := checkIfImageNeedsToBePulled(c.imagePullPolicy, i.Images[m], n)
					if err != nil {
						glog.Errorf("Error from checkIfImageNeedsToBePulled(): %+v", err)
						return fmt.Errorf("Error from checkIfImageNeedsToBePulled(): %+v", err)
					}
					if pullImage {
						ipr := images.ImageWorkRequest{
							Image:                   i.Images[m],
							Node:                    n.Labels["kubernetes.io/hostname"],
							ContainerRuntimeVersion: n.Status.NodeInfo.ContainerRuntimeVersion,
							WorkType:                wqKey.WorkType,
							Imagecache:              imageCache,
						}
						c.imageworkqueue.AddRateLimited(ipr)
					} else {
						glog.Infof("Image %s already present in node %s: image not re-pulled", i.Images[m], n.Labels["kubernetes.io/hostname"])
					}
				}
				if wqKey.WorkType == images.ImageCacheUpdate {
					for _, oldimage := range wqKey.OldImageCache.Spec.CacheSpec[k].Images {
						matched := false
						for _, newimage := range i.Images {
							if oldimage == newimage {
								matched = true
								break
							}
						}
						if !matched {
							pullImage, err := checkIfImageNeedsToBePulled(c.imagePullPolicy, oldimage, n)
							if err != nil {
								glog.Errorf("Error from checkIfImageNeedsToBePulled(): %+v", err)
								return fmt.Errorf("Error from checkIfImageNeedsToBePulled(): %+v", err)
							}
							if pullImage {
								ipr := images.ImageWorkRequest{
									Image:                   oldimage,
									Node:                    n.Labels["kubernetes.io/hostname"],
									ContainerRuntimeVersion: n.Status.NodeInfo.ContainerRuntimeVersion,
									WorkType:                images.ImageCachePurge,
									Imagecache:              imageCache,
								}
								c.imageworkqueue.AddRateLimited(ipr)
							} else {
								glog.Infof("Image %s already present in node %s: image not re-pulled", oldimage, n.Labels["kubernetes.io/hostname"])
							}
						}
					}
				}
			}
		}

		// We add an empty image pull request to signal the image manager that all
		// requests for this sync action have been placed in the imageworkqueue
		c.imageworkqueue.AddRateLimited(images.ImageWorkRequest{WorkType: wqKey.WorkType, Imagecache: imageCache})

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

		if imageCache.Status.StartTime != nil {
			status.StartTime = imageCache.Status.StartTime
		}

		status.Reason = imageCache.Status.Reason

		failures := false
		for _, v := range *wqKey.Status {
			if v.Status == images.ImageWorkResultStatusSucceeded && !failures {
				status.Status = fledgedv1alpha1.ImageCacheActionStatusSucceeded
				if v.ImageWorkRequest.WorkType == images.ImageCachePurge {
					status.Message = fledgedv1alpha1.ImageCacheMessageImagesDeletedSuccessfully
				} else {
					status.Message = fledgedv1alpha1.ImageCacheMessageImagesPulledSuccessfully
				}
			}
			if v.Status == images.ImageWorkResultStatusFailed && !failures {
				failures = true
				status.Status = fledgedv1alpha1.ImageCacheActionStatusFailed
				if v.ImageWorkRequest.WorkType == images.ImageCachePurge {
					status.Message = fledgedv1alpha1.ImageCacheMessageImageDeleteFailedForSomeImages
				} else {
					status.Message = fledgedv1alpha1.ImageCacheMessageImagePullFailedForSomeImages
				}
			}
			if v.Status == images.ImageWorkResultStatusFailed {
				status.Failures[v.ImageWorkRequest.Image] = append(
					status.Failures[v.ImageWorkRequest.Image], fledgedv1alpha1.NodeReasonMessage{
						Node:    v.ImageWorkRequest.Node,
						Reason:  v.Reason,
						Message: v.Message,
					})
			}
		}

		err = c.updateImageCacheStatus(imageCache, status)
		if err != nil {
			glog.Errorf("Error updating ImageCache status: %v", err)
			return err
		}

		if imageCache.Status.Reason == fledgedv1alpha1.ImageCacheReasonImageCachePurge || imageCache.Status.Reason == fledgedv1alpha1.ImageCacheReasonImageCacheRefresh {
			imageCache, err := c.fledgedclientset.FledgedV1alpha1().ImageCaches(namespace).Get(name, metav1.GetOptions{})
			if err != nil {
				glog.Errorf("Error getting image cache %s: %v", name, err)
				return err
			}
			if imageCache.Status.Reason == fledgedv1alpha1.ImageCacheReasonImageCachePurge {
				if err := c.removeAnnotation(imageCache, imageCachePurgeAnnotationKey); err != nil {
					glog.Errorf("Error removing Annotation %s from imagecache(%s): %v", imageCachePurgeAnnotationKey, imageCache.Name, err)
					return err
				}
			}
			if imageCache.Status.Reason == fledgedv1alpha1.ImageCacheReasonImageCacheRefresh {
				if err := c.removeAnnotation(imageCache, imageCacheRefreshAnnotationKey); err != nil {
					glog.Errorf("Error removing Annotation %s from imagecache(%s): %v", imageCacheRefreshAnnotationKey, imageCache.Name, err)
					return err
				}
			}
		}

		if status.Status == fledgedv1alpha1.ImageCacheActionStatusSucceeded {
			c.recorder.Event(imageCache, corev1.EventTypeNormal, status.Reason, status.Message)
		}

		if status.Status == fledgedv1alpha1.ImageCacheActionStatusFailed {
			c.recorder.Event(imageCache, corev1.EventTypeWarning, status.Reason, status.Message)
		}
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
	if imageCacheCopy.Status.Status != fledgedv1alpha1.ImageCacheActionStatusProcessing {
		completionTime := metav1.Now()
		imageCacheCopy.Status.CompletionTime = &completionTime
	}
	// If the CustomResourceSubresources feature gate is not enabled,
	// we must use Update instead of UpdateStatus to update the Status block of the ImageCache resource.
	// UpdateStatus will not allow changes to the Spec of the resource,
	// which is ideal for ensuring nothing other than resource status has been updated.
	_, err := c.fledgedclientset.FledgedV1alpha1().ImageCaches(imageCache.Namespace).Update(imageCacheCopy)
	return err
}

func (c *Controller) updateImageCacheSpecAndStatus(imageCache *fledgedv1alpha1.ImageCache, spec fledgedv1alpha1.ImageCacheSpec, status *fledgedv1alpha1.ImageCacheStatus) error {
	// NEVER modify objects from the store. It's a read-only, local cache.
	// You can use DeepCopy() to make a deep copy of original object and modify this copy
	// Or create a copy manually for better performance
	imageCacheCopy := imageCache.DeepCopy()
	imageCacheCopy.Spec = spec
	imageCacheCopy.Status = *status

	if status.Status == fledgedv1alpha1.ImageCacheActionStatusFailed &&
		status.Reason == fledgedv1alpha1.ImageCacheReasonCacheSpecValidationFailed {
		imageCacheCopy.Annotations = make(map[string]string)
		imageCacheCopy.Annotations[fledgedCacheSpecValidationKey] = "failed"
	}

	if imageCacheCopy.Status.Status != fledgedv1alpha1.ImageCacheActionStatusProcessing {
		completionTime := metav1.Now()
		imageCacheCopy.Status.CompletionTime = &completionTime
	}
	// If the CustomResourceSubresources feature gate is not enabled,
	// we must use Update instead of UpdateStatus to update the Status block of the ImageCache resource.
	// UpdateStatus will not allow changes to the Spec of the resource,
	// which is ideal for ensuring nothing other than resource status has been updated.
	_, err := c.fledgedclientset.FledgedV1alpha1().ImageCaches(imageCache.Namespace).Update(imageCacheCopy)
	return err
}

func (c *Controller) removeAnnotation(imageCache *fledgedv1alpha1.ImageCache, annotationKey string) error {
	imageCacheCopy := imageCache.DeepCopy()
	delete(imageCacheCopy.Annotations, annotationKey)
	_, err := c.fledgedclientset.FledgedV1alpha1().ImageCaches(imageCache.Namespace).Update(imageCacheCopy)
	if err == nil {
		glog.Infof("Annotation %s removed from imagecache(%s)", fledgedCacheSpecValidationKey, imageCache.Name)
	}
	return err
}
