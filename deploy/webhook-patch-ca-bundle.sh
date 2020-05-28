#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

export CA_BUNDLE=$(kubectl config view --raw --flatten -o json | jq -r '.clusters[] | select(.name == "'$(kubectl config current-context)'") | .cluster."certificate-authority-data"')

sed -i "s|\${CA_BUNDLE}|${CA_BUNDLE}|g" deploy/kubefledged-validatingwebhook.yaml
