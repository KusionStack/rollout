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

BASE_SOURCE_ROOT="$(cd "$(dirname "${BASH_SOURCE}")/.." && pwd -P)"
ROOT_DIR="${BASE_SOURCE_ROOT}"

# shellcheck source=/dev/null
source "${ROOT_DIR}/hack/lib/init.sh"

kind_cluster_name="rollout-dev"

kind::setup_rollout_cluster "${kind_cluster_name}"

log::status "building binary"
# build binary and container
make build

log::status "starting manager"
bin/manager --federated-mode=false \
    --health-probe-bind-address=:18081 \
    --feature-gates=OneTimeStrategy=true \
    --webhooks=""
