#!/usr/bin/env bash

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

export GOPATH=${HOME}/go
go get -d k8s.io/code-generator@v0.21.1

SCRIPT_ROOT=$(dirname ${BASH_SOURCE})/..
CODEGEN_PKG=${CODEGEN_PKG:-$(cd ${SCRIPT_ROOT}; ls -d -1 $GOPATH/pkg/mod/k8s.io/code-generator@v0.21.1 2>/dev/null || echo ../code-generator)}

# generate the code with:
# --output-base    because this script should also be able to run inside the vendor dir of
#                  k8s.io/kubernetes. The output-base is needed for the generators to output into the vendor dir
#                  instead of the $GOPATH directly. For normal projects this can be dropped.
chmod u+x ${CODEGEN_PKG}/generate-groups.sh
${CODEGEN_PKG}/generate-groups.sh "deepcopy,client,informer,lister" \
  github.com/senthilrch/kube-fledged/pkg/client github.com/senthilrch/kube-fledged/pkg/apis \
  kubefledged:v1alpha2 \
  --output-base "$(dirname ${BASH_SOURCE})/../../../.." \
  --go-header-file ${SCRIPT_ROOT}/hack/boilerplate/boilerplate.generatego.txt

# To use your own boilerplate text use:
#   --go-header-file ${SCRIPT_ROOT}/hack/custom-boilerplate.go.txt
