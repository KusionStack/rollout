#!/bin/bash

# Copyright 2024 The KusionStack Authors
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

KIND_NODE_IMAGE="kindest/node:v1.22.17"

kind::ensure_cluster() {
    local kind_cluster_name=${1}
    if ! kind get clusters | grep "${kind_cluster_name}"; then
        log::status "create kind cluster ${kind_cluster_name}"
        kind create cluster --name="${kind_cluster_name}" --image="${KIND_NODE_IMAGE}"
    fi

    kubeconfig="${HOME}/.kube/kind-${kind_cluster_name}.kubeconfig"
    if [[ ! -f "${kubeconfig}" ]]; then
        log::status "write kubeconfig to ${kubeconfig}"
        mkdir -p "$(dirname $kubeconfig)"
        kind get kubeconfig --name "${kind_cluster_name}" >"${kubeconfig}"
    fi

    export KUBECONFIG="${kubeconfig}"
}

kind::setup_rollout_cluster() {
    local _kind_cluster_name=${1}
    # ensure cluster
    kind::ensure_cluster "${_kind_cluster_name}"
    # apply crds
    log::status "applying crds..."
    kubectl apply -k "${ROLLOUT_CONFIG_CRD}"
    # create namespace clusterrolebinding
    log::status "apply namespace clusterrolebinding"
    kubectl apply -k "${ROLLOUT_CONFIG_PREREQUISITE}"

    # deploy kuperator by helm
    if [[ -z $(helm list --deployed --filter "kuperator" --no-headers) ]]; then
        helm repo add kusionstack https://kusionstack.github.io/charts
        helm repo update kusionstack
        helm install kuperator kusionstack/kuperator 
    fi
}

kind::setup_rollout_webhook() {
    local _kind_cluster_name=${1}
    local _overlay=${2}
    # ensure cluster
    kind::ensure_cluster "${_kind_cluster_name}"
    log::status "apply rollout webhook configuration"
    kubectl apply -k "${ROLLOUT_CONFIG_WEBHOOK}/${_overlay}"
}

kind::setup_rollout_workloads() {
    local _kind_cluster_name=${1}
    # ensure cluster
    kind::ensure_cluster "${_kind_cluster_name}"
    # create rollout and workloads
    log::status "apply rollout and workloads v1"
    kubectl apply -k "${ROLLOUT_CONFIG_WORKLOADS_V1}"
}
