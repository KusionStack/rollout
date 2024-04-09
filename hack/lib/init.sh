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

# Exit on error. Append "|| true" if you expect an error.
set -o errexit
# Do not allow use of undefined vars. Use ${VAR:-} to use an undefined VAR
set -o nounset
# Catch the error in pipeline.
set -o pipefail

BASE_SOURCE_ROOT="$(cd "$(dirname "${BASH_SOURCE}")/../.." && pwd -P)"

PROJECT_ROOT_DIR="${BASE_SOURCE_ROOT}"

COLOR_LOG=true

export ROLLOUT_CONFIG_CRD="${PROJECT_ROOT_DIR}/config/crd"
export ROLLOUT_CONFIG_PREREQUISITE="${PROJECT_ROOT_DIR}/config/kind/prerequisite"
export ROLLOUT_CONFIG_CONTROLLER="${PROJECT_ROOT_DIR}/config/kind/controller"
export ROLLOUT_CONFIG_WEBHOOK="${PROJECT_ROOT_DIR}/config/kind/webhook"
export ROLLOUT_CONFIG_WORKLOADS_V1="${PROJECT_ROOT_DIR}/config/kind/workload/overlays/v1"
export ROLLOUT_CONFIG_WORKLOADS_V2="${PROJECT_ROOT_DIR}/config/kind/workload/overlays/v2"

# shellcheck source=/dev/null
source "${PROJECT_ROOT_DIR}/hack/lib/logging.sh"
# shellcheck source=/dev/null
source "${PROJECT_ROOT_DIR}/hack/lib/kind.sh"
