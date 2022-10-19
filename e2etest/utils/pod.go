/*
Copyright 2022 The kube-fledged authors.

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

package utils

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

// ArePodsWithLabelRunningEventually asserts Pod successfully reaching Running state
func ArePodsWithLabelRunningEventually(
	ctx context.Context,
	namespace string,
	coreClient *corev1client.CoreV1Client,
	labelSelector string,
	retryCount int,
	interval time.Duration) bool {
	for i := 0; i < retryCount; {
		podList, err := coreClient.Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
		if err != nil {
			return false
		}
		for _, pod := range podList.Items {
			if pod.Status.Phase != corev1.PodRunning {
				i++
				time.Sleep(interval)
				continue
			}
		}
		// Iterated through all listed Pods
		// and all were in running state
		return true
	}
	// Tried for (pollingInterval * retryCount) times,
	// and all Pods failed to come to running state
	return false
}

// ArePodsWithLabelDeletedEventually assert Pod deletion
func ArePodsWithLabelDeletedEventually(
	ctx context.Context,
	namespace string,
	coreClient *corev1client.CoreV1Client,
	labelSelector string,
	retryCount int,
	interval time.Duration) bool {
	for i := 0; i < retryCount; {
		podList, err := coreClient.Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
		if err != nil {
			return false
		}

		if len(podList.Items) == 0 {
			return true
		}

		// List is not empty
		time.Sleep(interval)
	}
	// Tried for (pollingInterval * retryCount) times,
	// and list was not empty
	return false
}
