#!/usr/bin/env bash

# Copyright 2025 The KusionStack Authors
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

KIND_NODE_IMAGE=${KIND_NODE_IMAGE:-"kindest/node:v1.22.17"}

kind::ensure_cluster() {
    local kind_cluster_name=${1}
    if ! kind get clusters | grep "${kind_cluster_name}"; then
        log::status "create kind cluster ${kind_cluster_name}"
        kind create cluster --name="${kind_cluster_name}" --image="${KIND_NODE_IMAGE}"
    fi

    kubeconfig="${HOME}/.kube/kind-${kind_cluster_name}.kubeconfig"
    log::status "write kubeconfig to ${kubeconfig}"
    mkdir -p "$(dirname $kubeconfig)"
    kind get kubeconfig --name "${kind_cluster_name}" >"${kubeconfig}"

    export KUBECONFIG="${kubeconfig}"
}

kind::kustomize_apply() {
    local _kind_cluster_name=${1}
    local _kustomize_path=${2}
    # ensure cluster
    kind::ensure_cluster "${_kind_cluster_name}"
    log::status "apply kustomize configuration on ${_kustomize_path}"
    kubectl apply -k "${_kustomize_path}"
}

kind::kustomize_delete() {
    local _kind_cluster_name=${1}
    local _kustomize_path=${2}
    # ensure cluster
    kind::ensure_cluster "${_kind_cluster_name}"
    log::status "delete kustomize configuration on ${_kustomize_path}"
    kubectl delete --ignore-not-found -k "${_kustomize_path}"
}

kind::apply_yamls_in_dir() {
    local _kind_cluster_name=${1}
    local _dir=${2}
    # ensure cluster
    kind::ensure_cluster "${_kind_cluster_name}"
    # apply crds
    log::status "apply all yamls in dir ${_dir}"
    for file in ${_dir}/*.yaml; do
        kubectl apply -f "${file}"
    done
}
