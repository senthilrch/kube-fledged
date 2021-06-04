#!/bin/bash

# Copyright 2018 The kube-fledged authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

CLUSTER=$(kubectl config view --raw --flatten -o json | jq -r '.contexts[] | select(.name == "'$(kubectl config current-context)'") | .context.cluster')
export CA_BUNDLE=$(kubectl config view --raw --flatten -o json | jq -r '.clusters[] | select(.name == "'${CLUSTER}'") | .cluster."certificate-authority-data"')
#CA_DECODED=$(echo ${CA_BUNDLE} | base64 -d -)
sed -i "s|{{CA_BUNDLE}}|${CA_BUNDLE}|g" deploy/kubefledged-validatingwebhook.yaml
#sed -i "s|{{CA_BUNDLE}}|${CA_DECODED}|g" deploy/kubefledged-operator/deploy/crds/charts.helm.k8s.io_v1alpha1_kubefledged_cr.yaml
sed -i "s|{{CA_BUNDLE}}|${CA_BUNDLE}|g" deploy/kubefledged-operator/deploy/crds/charts.helm.k8s.io_v1alpha1_kubefledged_cr.yaml
