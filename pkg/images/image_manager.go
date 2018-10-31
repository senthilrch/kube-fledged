/*
Copyright 2016 The Kubernetes Authors.

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

package images

import (
	"fmt"
	"time"

	"github.com/golang/glog"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	fledgedv1alpha1 "k8s.io/kube-fledged/pkg/apis/fledged/v1alpha1"
)

const controllerAgentName = "fledged"
const fledgedNameSpace = "kube-fledged"

const (
	ImagePullResultStatusSucceeded  = "succeeded"
	ImagePullResultStatusFailed     = "failed"
	ImagePullResultStatusJobCreated = "jobcreated"

	ImagePullResultReasonImagePullFailed = "imagepullfailed"

	ImagePullResultMessageImagePullFailed = "failed to pull image to node. for details, please check events of pod"
)

// ImageManager provides the functionalities for pulling and deleting images
type ImageManager struct {
	workqueue                 workqueue.RateLimitingInterface
	imagepullqueue            workqueue.RateLimitingInterface
	kubeclientset             kubernetes.Interface
	imagepullstatus           map[string]ImagePullResult
	kubeInformerFactory       kubeinformers.SharedInformerFactory
	podsLister                corelisters.PodLister
	podsSynced                cache.InformerSynced
	imagePullDeadlineDuration time.Duration
}

// ImagePullRequest has image name and node name
type ImagePullRequest struct {
	Image      string
	Node       string
	Imagecache *fledgedv1alpha1.ImageCache
}

// ImagePullResult stores the result of pulling image
type ImagePullResult struct {
	ImagePullRequest ImagePullRequest
	Status           string
	Reason           string
	Message          string
}

// WorkType refers to type of work to be done by sync handler
type WorkType string

// Work types
const (
	ImageCacheCreate       WorkType = "create"
	ImageCacheUpdate       WorkType = "update"
	ImageCacheDelete       WorkType = "delete"
	ImageCacheStatusUpdate WorkType = "statusupdate"
	ImageCacheRefresh      WorkType = "refresh"
)

// WorkQueueKey is an item in the sync handler's work queue
type WorkQueueKey struct {
	WorkType WorkType
	ObjKey   string
	Status   *map[string]ImagePullResult
}

// NewImageManager returns a new image manager object
func NewImageManager(
	workqueue workqueue.RateLimitingInterface,
	imagepullqueue workqueue.RateLimitingInterface,
	kubeclientset kubernetes.Interface,
	namespace string,
	imagePullDeadlineDuration time.Duration) *ImageManager {

	kubeInformerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(
		kubeclientset,
		time.Second*30,
		kubeinformers.WithNamespace(namespace))
	podInformer := kubeInformerFactory.Core().V1().Pods()

	imagemanager := &ImageManager{
		workqueue:                 workqueue,
		imagepullqueue:            imagepullqueue,
		kubeclientset:             kubeclientset,
		imagepullstatus:           map[string]ImagePullResult{},
		kubeInformerFactory:       kubeInformerFactory,
		podsLister:                podInformer.Lister(),
		podsSynced:                podInformer.Informer().HasSynced,
		imagePullDeadlineDuration: imagePullDeadlineDuration,
	}
	podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		//AddFunc: ,
		UpdateFunc: func(old, new interface{}) {
			newPod := new.(*corev1.Pod)
			oldPod := old.(*corev1.Pod)
			if newPod.ResourceVersion == oldPod.ResourceVersion {
				// Periodic resync will send update events for all known Pods.
				// Two different versions of the same Pod will always have different RVs.
				return
			}
			glog.V(4).Infof("Pod %s changed status to %s", newPod.Name, newPod.Status.Phase)
			if (newPod.Status.Phase == corev1.PodSucceeded || newPod.Status.Phase == corev1.PodFailed) &&
				(oldPod.Status.Phase != corev1.PodSucceeded && oldPod.Status.Phase != corev1.PodFailed) {
				imagemanager.handlePodStatusChange(newPod)
			}
		},
		//DeleteFunc: ,
	})
	return imagemanager
}

func (m *ImageManager) handlePodStatusChange(pod *corev1.Pod) {
	glog.V(4).Infof("Pod %s changed status to %s", pod.Name, pod.Status.Phase)
	ipres, ok := m.imagepullstatus[pod.Labels["job-name"]]

	// Corresponding job might have expired and got deleted.
	// ignore pod status change for such jobs
	if !ok {
		return
	}

	if pod.Status.Phase == corev1.PodSucceeded {
		ipres.Status = ImagePullResultStatusSucceeded
		glog.Infof("Job %s succeeded (%s --> %s)", pod.Labels["job-name"], ipres.ImagePullRequest.Image, ipres.ImagePullRequest.Node)
	}
	if pod.Status.Phase == corev1.PodFailed {
		ipres.Status = ImagePullResultStatusFailed
		ipres.Reason = ImagePullResultReasonImagePullFailed
		ipres.Message = ImagePullResultMessageImagePullFailed + " " + pod.Name
		glog.Infof("Job %s failed (%s --> %s)", pod.Labels["job-name"], ipres.ImagePullRequest.Image, ipres.ImagePullRequest.Node)
	}
	m.imagepullstatus[pod.Labels["job-name"]] = ipres
	return
}

func (m *ImageManager) updatePendingImagePullResults() error {
	for job, ipres := range m.imagepullstatus {
		if ipres.Status == ImagePullResultStatusJobCreated {
			/*
				if ipres.ImagePullRequest.Imagecache == nil {
					glog.Errorf("Error accessing image cache for job %s", job)
					return fmt.Errorf("Error accessing image cache for job %s", job)
				}
			*/
			pods, err := m.podsLister.Pods(fledgedNameSpace).
				List(labels.Set(map[string]string{"job-name": job}).AsSelector())
			if err != nil {
				glog.Errorf("Error listing Pods: %v", err)
				return err
			}
			if len(pods) == 0 {
				glog.Errorf("No pods matched job %s", job)
				return fmt.Errorf("No pods matched job %s", job)
			}
			if len(pods) > 1 {
				glog.Errorf("More than one pod matched job %s", job)
				return fmt.Errorf("More than one pod matched job %s", job)
			}
			ipres.Status = ImagePullResultStatusFailed
			if pods[0].Status.Phase == corev1.PodPending {
				if pods[0].Status.ContainerStatuses[0].State.Waiting != nil {
					ipres.Reason = pods[0].Status.ContainerStatuses[0].State.Waiting.Reason
				}
				if pods[0].Status.ContainerStatuses[0].State.Terminated != nil {
					ipres.Reason = pods[0].Status.ContainerStatuses[0].State.Terminated.Reason
				}
			}
			fieldSelector := fields.Set{
				"involvedObject.kind":      "Pod",
				"involvedObject.name":      pods[0].Name,
				"involvedObject.namespace": fledgedNameSpace,
				"reason":                   "Failed",
			}.AsSelector().String()

			eventlist, err := m.kubeclientset.CoreV1().Events(fledgedNameSpace).
				List(metav1.ListOptions{FieldSelector: fieldSelector})
			if err != nil {
				glog.Errorf("Error listing events for pod (%s): %v", pods[0].Name, err)
				return err
			}

			for _, v := range eventlist.Items {
				ipres.Message = ipres.Message + ":" + v.Message
			}

			m.imagepullstatus[job] = ipres
		}
	}
	glog.V(4).Infof("imagepullstatus map: %+v", m.imagepullstatus)
	return nil
}

func (m *ImageManager) updateImageCacheStatus() error {
	//time.Sleep(m.imagePullDeadlineDuration)
	wait.Poll(time.Second, m.imagePullDeadlineDuration,
		func() (done bool, err error) {
			done, err = true, nil
			for _, ipres := range m.imagepullstatus {
				if ipres.Status == ImagePullResultStatusJobCreated {
					done, err = false, nil
					return
				}
			}
			return
		})
	err := m.updatePendingImagePullResults()
	if err != nil {
		return err
	}

	ipstatus := map[string]ImagePullResult{}

	deletePropagation := metav1.DeletePropagationBackground
	for job, ipres := range m.imagepullstatus {
		ipstatus[job] = ipres
		// delete jobs
		if err := m.kubeclientset.BatchV1().Jobs(fledgedNameSpace).
			Delete(job, &metav1.DeleteOptions{PropagationPolicy: &deletePropagation}); err != nil {
			glog.Errorf("Error deleting job %s: %v", job, err)
			return err
		}
	}

	imagecache := &fledgedv1alpha1.ImageCache{}
	for _, ipres := range m.imagepullstatus {
		if ipres.ImagePullRequest.Imagecache != nil {
			imagecache = ipres.ImagePullRequest.Imagecache
			break
		}
	}

	if imagecache == nil {
		glog.Errorf("Unable to obtain reference to image cache")
		return fmt.Errorf("Unable to obtain reference to image cache")
	}
	objKey, err := cache.MetaNamespaceKeyFunc(imagecache)
	if err != nil {
		glog.Errorf("Error from cache.MetaNamespaceKeyFunc(imagecache): %v", err)
		return err
	}
	m.workqueue.AddRateLimited(WorkQueueKey{
		WorkType: ImageCacheStatusUpdate,
		Status:   &ipstatus,
		ObjKey:   objKey,
	})
	m.imagepullstatus = map[string]ImagePullResult{}
	return nil
}

// Run starts the Image Manager go routine
func (m *ImageManager) Run(stopCh <-chan struct{}) {
	defer runtime.HandleCrash()
	glog.Info("Starting image manager")
	go m.kubeInformerFactory.Start(stopCh)
	go wait.Until(m.runWorker, time.Second, stopCh)
	glog.Info("Started image manager")
	<-stopCh
	glog.Info("Shutting down image manager")

}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (m *ImageManager) runWorker() {
	for m.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (m *ImageManager) processNextWorkItem() bool {
	//glog.Info("processNextWorkItem::Beginning...")
	obj, shutdown := m.imagepullqueue.Get()

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
		defer m.imagepullqueue.Done(obj)
		var ipr ImagePullRequest
		var ok bool
		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workqueue.
		if ipr, ok = obj.(ImagePullRequest); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			m.imagepullqueue.Forget(obj)
			runtime.HandleError(fmt.Errorf("Unexpected type in workqueue: %#v", obj))
			return nil
		}
		//glog.Infof("ipr=%+v", ipr)

		if (ipr == ImagePullRequest{}) {
			m.imagepullqueue.Forget(obj)
			if err := m.updateImageCacheStatus(); err != nil {
				return err
			}
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the
		// ImageCache resource to be synced.
		job, err := m.pullImage(ipr)
		if err != nil {
			return fmt.Errorf("error pulling image '%s' to node '%s': %s", ipr.Image, ipr.Node, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		m.imagepullstatus[job.Name] = ImagePullResult{ImagePullRequest: ipr, Status: ImagePullResultStatusJobCreated}
		m.imagepullqueue.Forget(obj)
		glog.Infof("Job %s created (%s --> %s)", job.Name, ipr.Image, ipr.Node)
		return nil
	}(obj)

	if err != nil {
		runtime.HandleError(err)
		return true
	}

	return true
}

// pullImage pulls the image to the node
func (m *ImageManager) pullImage(ipr ImagePullRequest) (*batchv1.Job, error) {
	// Construct the Job manifest
	newjob, err := newJob(ipr.Imagecache, ipr.Image, ipr.Node)
	if err != nil {
		glog.Errorf("Error when constructing job manifest: %v", err)
		return nil, err
	}
	// Create a Job to pull the image into the node
	job, err := m.kubeclientset.BatchV1().Jobs(fledgedNameSpace).Create(newjob)
	if err != nil {
		glog.Errorf("Error creating job in node %s: %v", ipr.Node, err)
		return nil, err
	}
	return job, nil
}

// newJob constructs a job manifest for pulling an image to a node
func newJob(imagecache *fledgedv1alpha1.ImageCache, image string, hostname string) (*batchv1.Job, error) {
	if imagecache == nil {
		glog.Error("imagecache pointer is nil")
		return nil, fmt.Errorf("imagecache pointer is nil")
	}

	labels := map[string]string{
		"app":        "imagecache",
		"imagecache": imagecache.Name,
		"controller": controllerAgentName,
	}

	backoffLimit := int32(0)
	activeDeadlineSeconds := int64((time.Hour).Seconds())

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
			BackoffLimit:          &backoffLimit,
			ActiveDeadlineSeconds: &activeDeadlineSeconds,
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
					ImagePullSecrets: imagecache.Spec.ImagePullSecrets,
				},
			},
		},
	}
	return job, nil
}
