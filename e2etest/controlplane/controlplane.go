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

package controlplane

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/senthilrch/kube-fledged/e2etest/utils"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/release"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"sigs.k8s.io/e2e-framework/klient/decoder"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

const (
	retryCount               int           = 100
	pollingInterval          time.Duration = 3 * time.Second
	controllerPodSelector                  = "app=kubefledged,kubefledged=kubefledged-controller"
	webhookServerPodSelector               = "app=kubefledged,kubefledged=kubefledged-webhook-server"
)

//HandlerConfig is a config for the kube-fledged control plane
type HandlerConfig struct {
	Strategy            DeployStrategy
	YamlDir             string
	HelmChartDir        string
	OperatorDir         string
	TestEnvConfig       *envconf.Config
	Namespace           string
	OperatorNamespace   string
	EnableWebhookServer bool
	// these retain helm chart's
	// state after installation
	helmConfig  *action.Configuration
	helmRelease *release.Release
	// these retain the CR info
	// for operator-created control
	// plane
	crFilepath string
}

//Handler is an intermittent object during control plane creation/deletion. This abstracts away the installation and deletion complexities
type Handler struct {
	CreateFn  func(context.Context) error
	IsCreated func(context.Context) bool
	DeleteFn  func(context.Context) error
	IsDeleted func(context.Context) bool
}

//NewHandler returns a Create-able and Delete-able object
func NewHandler(t *testing.T, config HandlerConfig) *Handler {
	t.Helper()

	switch config.Strategy {
	case DeployUsingYaml:
		return &Handler{
			CreateFn: func(ctx context.Context) error {
				filepaths := make([]string, 0)

				filepaths = append(filepaths,
					"kubefledged-crd.yaml",
					"kubefledged-serviceaccount-controller.yaml",
					"kubefledged-clusterrole-controller.yaml",
					"kubefledged-clusterrolebinding-controller.yaml",
					"kubefledged-deployment-controller.yaml",
				)

				if config.EnableWebhookServer {
					filepaths = append(filepaths,
						"kubefledged-validatingwebhook.yaml",
						"kubefledged-serviceaccount-webhook-server.yaml",
						"kubefledged-clusterrole-webhook-server.yaml",
						"kubefledged-clusterrolebinding-webhook-server.yaml",
						"kubefledged-service-webhook-server.yaml",
						"kubefledged-deployment-webhook-server.yaml",
					)
				}

				//Decode all of the required YAML files
				err := decodeEachFile(
					ctx,
					os.DirFS(config.YamlDir),
					filepaths,
					//Try to create the objects in the YAML files
					decoder.CreateIgnoreAlreadyExists(config.TestEnvConfig.Client().Resources()),
					decoder.MutateNamespace(config.Namespace),
				)
				if err != nil {
					return errors.Wrapf(err,
						"failed to decode and create the YAML files: %v",
						filepaths,
					)
				}

				return nil
			},
			IsCreated: func(ctx context.Context) bool {
				return controlPlaneIsCreated(ctx, config)
			},
			DeleteFn: func(ctx context.Context) error {
				filepaths := make([]string, 0)

				filepaths = append(filepaths,
					"kubefledged-deployment-controller.yaml",
					"kubefledged-clusterrolebinding-controller.yaml",
					"kubefledged-clusterrole-controller.yaml",
					"kubefledged-serviceaccount-controller.yaml",
					"kubefledged-crd.yaml",
				)

				if config.EnableWebhookServer {
					filepaths = append(filepaths,
						"kubefledged-deployment-webhook-server.yaml",
						"kubefledged-service-webhook-server.yaml",
						"kubefledged-clusterrolebinding-webhook-server.yaml",
						"kubefledged-clusterrole-webhook-server.yaml",
						"kubefledged-serviceaccount-webhook-server.yaml",
						"kubefledged-validatingwebhook.yaml",
					)
				}

				//Decode all of the required YAML files
				err := decodeEachFile(
					ctx,
					os.DirFS(config.YamlDir),
					filepaths,
					//Try to delete the objects in the YAML files
					decoder.DeleteHandler(config.TestEnvConfig.Client().Resources()),
					decoder.MutateNamespace(config.Namespace),
				)
				if err != nil {
					return errors.Wrapf(err,
						"failed to decode and delete the objects from YAML files: %v",
						filepaths,
					)
				}

				return nil
			},
			IsDeleted: func(ctx context.Context) bool {
				return controlPlaneIsDeleted(ctx, config)
			},
		}
	case DeployUsingHelmChart:
		return &Handler{
			CreateFn: func(ctx context.Context) error {
				chart, err := loader.LoadDir(config.HelmChartDir)
				if err != nil {
					return err
				}

				releaseName := envconf.RandomName("kubefledged", 17)
				config.helmConfig = utils.NewHelmDefaultConfig(t, releaseName, config.Namespace, config.TestEnvConfig.KubeconfigFile())

				install := action.NewInstall(config.helmConfig)
				// Set defaults for install values
				install.Timeout = 300 * time.Second
				install.Namespace = config.Namespace
				install.ReleaseName = releaseName

				helmOpts := new(values.Options)
				if !config.EnableWebhookServer {
					helmOpts.Values = append(helmOpts.Values, "webhookServer.enable=true")
				}

				//Merging all values from values.yaml and above
				// manual options
				vals, err := helmOpts.MergeValues(getter.All(cli.New()))
				if err != nil {
					return err
				}

				//Finally helm install the chart
				config.helmRelease, err = install.RunWithContext(ctx, chart, vals)
				if err != nil {
					return errors.Wrapf(err, "failed to install helm chart")
				}

				return nil
			},
			IsCreated: func(ctx context.Context) bool {
				return controlPlaneIsCreated(ctx, config)
			},
			DeleteFn: func(ctx context.Context) error {
				if config.helmRelease == nil || config.helmConfig == nil {
					return errors.New("Invalid state of helm parameters, failed to delete")
				}

				//NOTE: This does not remove CRDs.
				//      Ref: https://helm.sh/docs/topics/charts/#limitations-on-crds
				_, err := action.NewUninstall(config.helmConfig).Run(config.helmRelease.Name)
				if err != nil {
					return err
				}

				//Deleting CRDs manually
				err = utils.DeleteCRDs(
					ctx,
					t,
					config.TestEnvConfig.Client().RESTConfig(),
					[]string{"imagecaches.kubefledged.io"},
				)
				return err
			},
			IsDeleted: func(ctx context.Context) bool {
				return controlPlaneIsDeleted(ctx, config)
			},
		}
	case DeployUsingOperator:
		return &Handler{
			CreateFn: func(ctx context.Context) error {
				const (
					namespacePlaceholder = "{{KUBEFLEDGED_NAMESPACE}}"
				)

				clusterScopedFilepaths := []string{
					"crds/charts.helm.kubefledged.io_kubefledgeds_crd.yaml",
					"clusterrole.yaml",
				}

				err := decodeEachFile(
					ctx,
					os.DirFS(config.OperatorDir),
					clusterScopedFilepaths,
					//Try to create the objects in the Operator's YAML files
					decoder.CreateIgnoreAlreadyExists(config.TestEnvConfig.Client().Resources()),
				)
				if err != nil {
					return errors.Errorf("failed to decode and create Operator files %v", clusterScopedFilepaths)
				}

				// String replace the namespace value
				// on these files
				namespaceScopedFilepaths := []string{
					filepath.Join(config.OperatorDir, "service_account.yaml"),
					filepath.Join(config.OperatorDir, "clusterrole_binding.yaml"),
					filepath.Join(config.OperatorDir, "operator.yaml"),
				}

				//Create the string-replaced files
				for _, file := range namespaceScopedFilepaths {
					err = decoder.DecodeEach(
						ctx,
						strings.NewReader(utils.Sed(t, namespacePlaceholder, config.OperatorNamespace, file)),
						decoder.CreateIgnoreAlreadyExists(config.TestEnvConfig.Client().Resources()),
					)
					if err != nil {
						errors.Errorf("failed to decode and create Operator file %s", file)
					}
				}

				config.crFilepath = filepath.Join(config.OperatorDir, "crds/charts.helm.kubefledged.io_v1alpha2_kubefledged_cr.yaml")

				// Create Control Plane via CR
				crData := utils.Sed(t, "namespace: kube-fledged", "namespace: "+config.Namespace, config.crFilepath)

				if config.EnableWebhookServer {
					crData = strings.ReplaceAll(crData, "enable: false", "enable: true")
				}

				err = decoder.DecodeEach(
					ctx,
					strings.NewReader(crData),
					decoder.CreateIgnoreAlreadyExists(config.TestEnvConfig.Client().Resources()),
				)
				if err != nil {
					errors.New("failed to decode and create Kube-Fledged CR file")
				}

				return nil
			},
			IsCreated: func(ctx context.Context) bool {
				return controlPlaneIsCreated(ctx, config)
			},
			DeleteFn: func(ctx context.Context) error {
				err := decodeEachFile(
					ctx,
					os.DirFS("config.OperatorDir"),
					[]string{config.crFilepath},
					decoder.DeleteHandler(config.TestEnvConfig.Client().Resources()),
					decoder.MutateNamespace(config.Namespace),
				)
				if err != nil {
					errors.Errorf("failed to delete control plane CR from namespace %s", config.Namespace)
				}

				//Waiting for the Operator to clean up the control plane
				t.Logf("Waiting for 5 seconds while the Operator cleans up the control plane in %s namespace...", config.Namespace)
				time.Sleep(5 * time.Second)

				//Deleting CRD manually, because helm does not delete CRD
				// Ref: https://helm.sh/docs/topics/charts/#limitations-on-crds
				err = utils.DeleteCRDs(
					ctx,
					t,
					config.TestEnvConfig.Client().RESTConfig(),
					[]string{"imagecaches.kubefledged.io"},
				)
				if err != nil {
					return err
				}

				const (
					namespacePlaceholder = "{{KUBEFLEDGED_NAMESPACE}}"
				)

				//Delete the Operator components
				namespaceScopedFilepaths := []string{
					filepath.Join(config.OperatorDir, "operator.yaml"),
					filepath.Join(config.OperatorDir, "clusterrole_binding.yaml"),
					filepath.Join(config.OperatorDir, "service_account.yaml"),
				}

				//Create the string-replaced files
				for _, file := range namespaceScopedFilepaths {
					err = decoder.DecodeEach(
						ctx,
						strings.NewReader(utils.Sed(t, namespacePlaceholder, config.OperatorNamespace, file)),
						decoder.DeleteHandler(config.TestEnvConfig.Client().Resources()),
					)
					if err != nil {
						errors.Errorf("failed to decode and delete Operator file %s", file)
					}
				}

				clusterScopedFilepaths := []string{
					"crds/charts.helm.kubefledged.io_kubefledgeds_crd.yaml",
					"clusterrole.yaml",
				}

				err = decodeEachFile(
					ctx,
					os.DirFS(config.OperatorDir),
					clusterScopedFilepaths,
					//Try to create the objects in the YAML files
					decoder.DeleteHandler(config.TestEnvConfig.Client().Resources()),
				)
				if err != nil {
					return errors.Errorf("failed to decode and delete Operator files %v", clusterScopedFilepaths)
				}

				return nil
			},
			IsDeleted: func(ctx context.Context) bool {
				coreClient, err := corev1client.NewForConfig(config.TestEnvConfig.Client().RESTConfig())
				if err != nil {
					return false
				}

				return controlPlaneIsDeleted(ctx, config) && utils.ArePodsWithLabelDeletedEventually(ctx, config.OperatorNamespace, coreClient, "name=kubefledged-operator", retryCount, pollingInterval)
			},
		}
	}
	return nil
}

//Create kube-fledged control plane and verify deployment
func (h *Handler) Create(ctx context.Context, t *testing.T) error {
	t.Helper()
	err := h.CreateFn(ctx)
	if err != nil {
		return err
	}

	if !h.IsCreated(ctx) {
		return errors.New("failed to create Kube-Fledged control plane, all Pod did not reach Running state")
	}

	return nil
}

//Delete kube-fledged control plane and verify deletion
func (h *Handler) Delete(ctx context.Context, t *testing.T) error {
	t.Helper()
	err := h.DeleteFn(ctx)
	if err != nil {
		return err
	}

	if !h.IsDeleted(ctx) {
		return errors.New("failed to remove Kube-Fledged control plane, all Pod were not Deleted")
	}

	return nil
}

func decodeEachFile(ctx context.Context, fsys fs.FS, files []string, handlerFn decoder.HandlerFunc, options ...decoder.DecodeOption) error {
	for _, file := range files {
		f, err := fsys.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()
		if err := decoder.DecodeEach(ctx, f, handlerFn, options...); err != nil {
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return nil
}

/*
// This can be used with the install with YAML decoder options to
// replace image values
func setImagesWhileDecoding(controllerImage, criClientImage, webhookServerImage string) decoder.DecodeOption {
	return decoder.MutateOption(
		func(obj k8s.Object) error {
			if obj.GetObjectKind().GroupVersionKind().Kind == "Deployment" {
				deployObj, ok := obj.(*appsv1.Deployment)
				if !ok {
					return errors.New("failed to assert type of %v" +
						" runtime.Object to Deployemnt")
				}

				componentName, ok := deployObj.ObjectMeta.Labels["component"]
				if !ok {
					return errors.Errorf("failed to get 'component' label"+
						" key in Deployment object %s", deployObj.Name)
				}

				const (
					controllerComponentValue    = "kubefledged-controller"
					webhookServerComponentValue = "kubefledged-webhook-server"
				)

				switch componentName {
				case controllerComponentValue:
					// Setting controller image in the Decoded deployment
					deployObj.Spec.Template.Spec.Containers[0].Image = controllerImage
					// Setting CriClient image in the Decoded deployment
					for _, env := range deployObj.Spec.Template.Spec.Containers[0].Env {
						if env.Name == "KUBEFLEDGED_CRI_CLIENT_IMAGE" {
							env.Value = criClientImage
						}
					}
					obj = deployObj.DeepCopyObject().(k8s.Object)
					return nil
				case webhookServerComponentValue:
					// Setting webhook server image in the deployment
					deployObj.Spec.Template.Spec.Containers[0].Image = webhookServerImage
					obj = deployObj.DeepCopyObject().(k8s.Object)
					return nil
				}
			}
			return nil
		})
}
*/

func controlPlaneIsCreated(ctx context.Context, config HandlerConfig) bool {
	coreClient, err := corev1client.NewForConfig(config.TestEnvConfig.Client().RESTConfig())
	if err != nil {
		return false
	}

	if config.EnableWebhookServer {
		return utils.ArePodsWithLabelRunningEventually(
			ctx,
			config.Namespace,
			coreClient,
			controllerPodSelector,
			retryCount,
			pollingInterval,
		) && utils.ArePodsWithLabelRunningEventually(
			ctx,
			config.Namespace,
			coreClient,
			webhookServerPodSelector,
			retryCount,
			pollingInterval,
		)
	}

	return utils.ArePodsWithLabelRunningEventually(
		ctx,
		config.Namespace,
		coreClient,
		controllerPodSelector,
		retryCount,
		pollingInterval,
	)
}

func controlPlaneIsDeleted(ctx context.Context, config HandlerConfig) bool {
	coreClient, err := corev1client.NewForConfig(config.TestEnvConfig.Client().RESTConfig())
	if err != nil {
		return false
	}

	if config.EnableWebhookServer {
		return utils.ArePodsWithLabelDeletedEventually(
			ctx,
			config.Namespace,
			coreClient,
			controllerPodSelector,
			retryCount,
			pollingInterval,
		) && utils.ArePodsWithLabelDeletedEventually(
			ctx,
			config.Namespace,
			coreClient,
			webhookServerPodSelector,
			retryCount,
			pollingInterval,
		)
	}

	return utils.ArePodsWithLabelDeletedEventually(
		ctx,
		config.Namespace,
		coreClient,
		controllerPodSelector,
		retryCount,
		pollingInterval,
	)
}
