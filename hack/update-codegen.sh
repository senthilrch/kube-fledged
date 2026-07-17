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

SCRIPT_ROOT=$(dirname "${BASH_SOURCE}")/..

export GOPATH=${HOME}/go
go mod download k8s.io/code-generator@v0.36.2

CODEGEN_PKG=$(cd "${SCRIPT_ROOT}"; ls -d -1 ${GOPATH}/pkg/mod/k8s.io/code-generator@v0.36.2 2>/dev/null || echo ../code-generator)

source "${CODEGEN_PKG}/kube_codegen.sh"

# Generate deepcopy functions
kube::codegen::gen_helpers \
  --boilerplate "${SCRIPT_ROOT}/hack/boilerplate/boilerplate.generatego.txt" \
  "${SCRIPT_ROOT}/pkg/apis"

# Generate clientset, listers, informers
kube::codegen::gen_client \
  --with-watch \
  --output-dir "${SCRIPT_ROOT}/pkg/client" \
  --output-pkg "github.com/senthilrch/kube-fledged/pkg/client" \
  --boilerplate "${SCRIPT_ROOT}/hack/boilerplate/boilerplate.generatego.txt" \
  "${SCRIPT_ROOT}/pkg/apis"
