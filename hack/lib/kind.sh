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
    log::status "ensure kind cluster ${kind_cluster_name}"
    if ! kind get clusters | grep "${kind_cluster_name}"; then
        kind create cluster --name="${kind_cluster_name}" --image="${KIND_NODE_IMAGE}"
    fi
}

kind::install_crds() {
    local context_name=${1}
    kubectl --context "${context_name}" apply -k "${PROJECT_ROOT_DIR}"/config/crd
}

kind::setup_rollout_cluster() {
    local _kind_cluster_name=${1}
    local _context_name="kind-${_kind_cluster_name}"
    # ensure cluster
    kind::ensure_cluster "${_kind_cluster_name}"
    # apply crds
    kubectl config use-context "${_context_name}"
    log::status "applying crds..."
    kubectl apply -k "${ROLLOUT_CONFIG_CRD}"
    # create namespace clusterrolebinding
    log::status "apply namespace clusterrolebinding"
    kubectl apply -k "${ROLLOUT_CONFIG_PREREQUISITE}"
    # create rollout and workloads
    log::status "apply rollout and workloads v1"
    kubectl apply -k "${ROLLOUT_CONFIG_WORKLOADS_V1}"
}

kind::setup_rollout_webhook() {
    local _kind_cluster_name=${1}
    local _context_name="kind-${_kind_cluster_name}"
    local _overlay=${2}
    log::status "apply rollout webhook configuration"
    kubectl apply -k "${ROLLOUT_CONFIG_WEBHOOK}/${_overlay}"
}
