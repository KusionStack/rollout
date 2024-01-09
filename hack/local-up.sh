#!/bin/bash

# Copyright 2022 ByteDance and its affiliates.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

readonly BASE_SOURCE_ROOT="$(cd "$(dirname "${BASH_SOURCE}")/.." && pwd -P)"

readonly ROOT_DIR="${BASE_SOURCE_ROOT}"

readonly kind_cluster_name="kusionstack-rollout"
readonly kind_node_image="kindest/node:v1.22.17"
readonly context_name="kind-${kind_cluster_name}"

readonly rollout_asserts="${ROOT_DIR}/hack/testdata/local-up/asserts.yaml"
readonly rollout_workloads="${ROOT_DIR}/hack/testdata/local-up/workloads.yaml"

ensure_kind_cluster() {
    echo ">> ensure kind cluster ${kind_cluster_name}"
    if ! kind get clusters | grep "${kind_cluster_name}"; then
        kind create cluster --name="${kind_cluster_name}" --image="${kind_node_image}"
    fi
}

install_crds() {
    echo ">> install crds"
    for variable in "${ROOT_DIR}"/config/crd/bases/*; do
        kubectl apply -f "${variable}"
    done
}

# start kind cluster
ensure_kind_cluster

# apply crds
install_crds

# build binary and container
docker build --build-arg VERSION=local-up -f build/rollout/Dockerfile . -t rollout:local-up
kind load docker-image --name="${kind_cluster_name}" rollout:local-up

kubectl --context "${context_name}" apply -f "${rollout_asserts}"
kubectl --context "${context_name}" apply -f "${rollout_workloads}"

echo "
Congratulations !!

KusionStack Rollout is running in rollout-system now, You can check it by:

    kubectl --context ${context_name} -n rollout-system get deployments

The statefulset workloads are created at default namespace:

    kubectl --context ${context_name} -n default get statefulsets

And the rollout demo is created at default namespace:

    kubectl --context ${context_name} -n default get rollouts

Then you can update workloads by:

    kubectl --context ${context_name} apply -f hack/testdata/local-up/update-workloads.yaml

"
