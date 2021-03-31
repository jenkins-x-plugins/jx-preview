#!/usr/bin/env bash

# Copyright 2019 The Tekton Authors

set -o errexit
set -o nounset
set -o pipefail

# Conveniently set GOPATH if unset
if [[ -z "${GOPATH:-}" ]]; then
  export GOPATH="$(go env GOPATH)"
  if [[ -z "${GOPATH}" ]]; then
    echo "WARNING: GOPATH not set and go binary unable to provide it"
  fi
fi

GENERATOR_VERSION=v0.19.2
(
  # To support running this script from anywhere, we have to first cd into this directory
  # so we can install the tools.
  cd "$(dirname "${0}")"
  go get k8s.io/code-generator/cmd/{defaulter-gen,client-gen,lister-gen,informer-gen,deepcopy-gen}@$GENERATOR_VERSION
)

SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
rm -rf "${SCRIPT_ROOT}"/pkg/client
# generate the code with:
# --output-base    because this script should also be able to run inside the vendor dir of
#                  k8s.io/kubernetes. The output-base is needed for the generators to output into the vendor dir
#                  instead of the $GOPATH directly. For normal projects this can be dropped.
bash hack/generate-groups.sh all \
  github.com/jenkins-x-plugins/jx-preview/pkg/client github.com/jenkins-x-plugins/jx-preview/pkg/apis \
  preview:v1alpha1 \
  --output-base "$(dirname "${BASH_SOURCE[0]}")/../../../.." \
  --go-header-file "${SCRIPT_ROOT}"/hack/custom-boilerplate.go.txt


