#!/bin/bash

# Copyright 2025 KusionStack Authors.
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

BASE_SOURCE_ROOT="$(cd "$(dirname "${BASH_SOURCE}")/.." && pwd -P)"
ROOT_DIR="${BASE_SOURCE_ROOT}"

# shellcheck source=/dev/null
source "${ROOT_DIR}/hack/lib/init.sh"

KIND_CONFIG_DIR="${PROJECT_ROOT_DIR}/config/kind"

log::status "compiling rollout binary"
# build binary and container
make dev-build

kind_cluster_name="rollout-dev"

crd_path="${PROJECT_ROOT_DIR}/config/crd/bases"

# setup local dev cluster
kind::ensure_cluster "${kind_cluster_name}"

# deploy kuperator by helm
if [[ -z $(helm list --deployed --filter "kuperator" --no-headers) ]]; then
    helm repo add kusionstack https://kusionstack.github.io/charts
    helm repo update kusionstack
    helm install kuperator kusionstack/kuperator
fi

# apply namespace and cluster rolebinding
kind::kustomize_apply "${kind_cluster_name}" "${KIND_CONFIG_DIR}/prerequisite"
# apply rollout crd
kind::apply_yamls_in_dir "${kind_cluster_name}" "$crd_path"
# delete webhook firstly
kind::kustomize_delete "${kind_cluster_name}" "${KIND_CONFIG_DIR}/webhook/outcluster"
# apply workload
kind::kustomize_apply "${kind_cluster_name}" "${KIND_CONFIG_DIR}/workload/overlays/v1"
# apply webhook
kind::kustomize_apply "${kind_cluster_name}" "${KIND_CONFIG_DIR}/webhook/outcluster"

log::status "starting manager"

export WEBHOOK_ALTERNATE_HOSTS=host.docker.internal
export ENABLE_SYNC_WEBHOOK_CERTS=true

rm -rf bin/logs && mkdir -p bin/logs

bin/manager --federated-mode=false \
    --health-probe-bind-address=:18081 \
    --feature-gates=OneTimeStrategy=true \
    --webhook-cert-dir=bin/certs \
    --log_dir=bin/logs \
    --logtostderr=false \
    --alsologtostderr=true \
    --controllers=*,podcanarylabel \
    --v=2 \
    "$*"
