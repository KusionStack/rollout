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

log::status "compiling rollout binary"
go build -o bin/manager kusionstack.io/rollout/cmd/rollout

kind_cluster_name="rollout-dev"
kind::setup_rollout_cluster "${kind_cluster_name}"

rm -rf bin/logs && mkdir -p bin/logs

bin/manager --federated-mode=false \
    --health-probe-bind-address=:18081 \
    --webhook-cert-dir=bin/certs \
    --log_dir=bin/logs \
    --logtostderr=false \
    --alsologtostderr=true \
    --controllers=swarm \
    --webhooks= \
    --leader-elect=false \
    --v=3 \
    "$*"
