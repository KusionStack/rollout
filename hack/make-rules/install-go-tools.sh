#!/usr/bin/env bash

# Copyright 2025 KusionStack Authors.
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

BASE_SOURCE_ROOT="$(cd "$(dirname "${BASH_SOURCE}")/../.." && pwd -P)"
ROOT_DIR="${BASE_SOURCE_ROOT}"

# shellcheck source=/dev/null
source "${ROOT_DIR}/hack/lib/init.sh"

## Tool Versions
KUSTOMIZE_VERSION=${KUSTOMIZE_VERSION:-"v5.3.0"}
CONTROLLER_TOOLS_VERSION=${CONTROLLER_TOOLS_VERSION:-"v0.15.0"}
HELM_VERSION=${HELM_VERSION:-"v3.18.0"}
GOLANGCI_VERSION=${GOLANGCI_VERSION:-"v2.0.2"}

LOCALBIN="${ROOT_DIR}/bin"
LOCALBIN_OS_ARCH="${ROOT_DIR}/bin/$(golang::get_host_os_arch)"

install::kustomize() {
    if test -s "${LOCALBIN_OS_ARCH}"/kustomize && "${LOCALBIN_OS_ARCH}"/kustomize version | grep "${KUSTOMIZE_VERSION}"; then
        return
    fi
    log::status "Installing kustomize ${KUSTOMIZE_VERSION}"
    GOBIN="${LOCALBIN_OS_ARCH}" golang::install sigs.k8s.io/kustomize/kustomize/v5@"${KUSTOMIZE_VERSION}"
}

install::controller-gen() {
    if test -e "${LOCALBIN_OS_ARCH}"/controller-gen && "${LOCALBIN_OS_ARCH}"/controller-gen --version | grep -q "${CONTROLLER_TOOLS_VERSION}"; then
        return
    fi
    log::status "Installing controller-gen ${CONTROLLER_TOOLS_VERSION}"
    GOBIN="${LOCALBIN_OS_ARCH}" golang::install sigs.k8s.io/controller-tools/cmd/controller-gen "${CONTROLLER_TOOLS_VERSION}"
}

install::setup-envtest() {
    if test -s "${LOCALBIN_OS_ARCH}"/setup-envtest; then
        return
    fi
    log::status "Installing setup-envtest latest"
    GOBIN="${LOCALBIN_OS_ARCH}" golang::install sigs.k8s.io/controller-runtime/tools/setup-envtest latest
}

install::golangci-lint() {
    if test -s "${LOCALBIN_OS_ARCH}"/golangci-lint && "${LOCALBIN_OS_ARCH}"/golangci-lint version | grep -q ${GOLANGCI_VERSION}; then
        return
    fi
    log::status "Installing golangci-lint ${GOLANGCI_VERSION}"
    GOBIN="${LOCALBIN_OS_ARCH}" golang::install github.com/golangci/golangci-lint/v2/cmd/golangci-lint "${GOLANGCI_VERSION}"
}

install::helm() {
    if test -e "${LOCALBIN_OS_ARCH}"/helm && "${LOCALBIN_OS_ARCH}"/helm version | grep -q ${HELM_VERSION}; then
        return
    fi
    log::status "Installing helm ${HELM_VERSION}"
    GOBIN="${LOCALBIN_OS_ARCH}" golang::install helm.sh/helm/v3/cmd/helm "${HELM_VERSION}"
}

install::kube-codegen() {
    if test -e "${LOCALBIN_OS_ARCH}"/kube-codegen; then
        return
    fi
    log::status "Installing zoumo/kube-codegen from source"
    GOBIN="${LOCALBIN_OS_ARCH}" golang::install_from_src github.com/zoumo/kube-codegen main cmd/kube-codegen
}

case $1 in
*)
    install::"$1"
    ln -sf "${LOCALBIN_OS_ARCH}/$1" "${LOCALBIN}/"
    ;;
esac
