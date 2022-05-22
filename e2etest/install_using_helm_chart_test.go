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

package e2etest

import (
	"context"
	"testing"

	controlplane "github.com/senthilrch/kube-fledged/e2etest/controlplane"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestInstallUsingHelmChart(t *testing.T) {
	ctrlPlaneHandler := &controlplane.Handler{}

	installControlPlaneUsingHelmChart := features.New("install kube-fledged using helm chart").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				ctrlPlaneHandler = controlplane.NewHandler(t, controlplane.HandlerConfig{
					Strategy:            controlplane.DeployUsingHelmChart,
					HelmChartDir:        helmChartDir,
					TestEnvConfig:       cfg,
					Namespace:           controlPlaneNamespace,
					EnableWebhookServer: true,
				})
				err := ctrlPlaneHandler.CreateFn(ctx)
				if err != nil {
					t.Fatalf("failed to setup control plane using helm chart: %s", err.Error())
				}
				return ctx
			},
		).
		Assess("controller and validating-webhook-server successfully deployed",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				ok := ctrlPlaneHandler.IsCreated(ctx)
				if !ok {
					t.Fatalf("Control Plane install using helm chart did not succeed, all pods are not Running")
				}
				return ctx
			},
		).
		Teardown(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				err := ctrlPlaneHandler.Delete(ctx, t)
				if err != nil {
					t.Fatalf("Control Plane delete using helm chart did not succeed, all pods were not deleted: %s", err.Error())
				}
				return ctx
			},
		).Feature()

	testenv.Test(t, installControlPlaneUsingHelmChart)
}
