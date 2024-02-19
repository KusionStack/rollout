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

BASE_SOURCE_ROOT="$(cd "$(dirname "${BASH_SOURCE}")/.." && pwd -P)" # nolint
ROOT_DIR="${BASE_SOURCE_ROOT}"

# shellcheck source=/dev/null
source "${ROOT_DIR}/hack/lib/init.sh"

kind_cluster_name="kusionstack-rollout"
context_name="kind-${kind_cluster_name}"

kind::setup_rollout_cluster "${kind_cluster_name}"

# build binary and container
log::status "building binary and container"
image_tag=$(date +%s%N | md5sum | cut -c 1-10)
docker build --build-arg VERSION="${image_tag}" -f build/rollout/Dockerfile . -t rollout:"${image_tag}"
kind load docker-image --name="${kind_cluster_name}" rollout:"${image_tag}"

log::status "starting rollout controller"
# change image
cd "${ROLLOUT_CONFIG_CONTROLLER}" || exit
kustomize edit set image rollout:"${image_tag}"
kubectl --context "${context_name}" apply -k "${ROLLOUT_CONFIG_CONTROLLER}"
# reset image
kustomize edit set image rollout:local-up
cd - || exit

echo "
Congratulations !!

KusionStack Rollout is running in rollout-system now, You can check it by:

    kubectl --context ${context_name} -n rollout-system get deployments

The statefulset workloads are created at default namespace:

    kubectl --context ${context_name} -n default get statefulsets

And the rollout demo is created at default namespace:

    kubectl --context ${context_name} -n default get rollouts

Then you can update workloads by kustomize:

    kubectl --context ${context_name} apply -k ${ROLLOUT_CONFIG_WORKLOADS_V2}

"
