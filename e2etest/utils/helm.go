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
	"testing"

	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/kube"
	apiextensionsv1client "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

const helmDriver = "memory"

// NewHelmDefaultConfig generates a default action Configuration (for use with helm commands)
func NewHelmDefaultConfig(t *testing.T, releaseName, releaseNamespace, kubeconfigPath string) *action.Configuration {
	t.Helper()

	actionConfig := new(action.Configuration)
	err := actionConfig.Init(
		kube.GetConfig(kubeconfigPath, "", releaseNamespace),
		releaseNamespace,
		helmDriver,
		func(format string, v ...interface{}) {
			t.Helper()
			t.Logf(format, v...)
		},
	)
	if err != nil {
		t.Fatalf("failed to generate helm configuration: %s", err.Error())
	}

	return actionConfig
}

// DeleteCRDs is there to delete CRDs especially when helm wouldn't after a 'helm uninstall'.
// Ref: https://helm.sh/docs/topics/charts/#limitations-on-crds
func DeleteCRDs(ctx context.Context, t *testing.T, cfg *rest.Config, crdNames []string) error {
	t.Helper()

	client, err := apiextensionsv1client.NewForConfig(cfg)
	if err != nil {
		return errors.Wrap(err, "failed to delete CRDs")
	}

	for _, crdName := range crdNames {
		err = client.CustomResourceDefinitions().Delete(ctx, crdName, metav1.DeleteOptions{})
		if err != nil && !k8serrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to delete CRD %s", crdName)
		}
	}

	t.Logf("Successfully deleted CRDs %v", crdNames)
	return nil
}
