#!/usr/bin/env bash

# Copyright 2025 The jim.zoumo@gmail.com Authors.
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

set -o errexit
set -o nounset
set -o pipefail

BASE_SOURCE_ROOT="$(cd "$(dirname "${BASH_SOURCE}")/../.." && pwd -P)"
ROOT_DIR="${BASE_SOURCE_ROOT}"

bash "${ROOT_DIR}/hack/make-rules/install-go-tools.sh" controller-gen

"${ROOT_DIR}/bin/controller-gen" rbac:roleName=manager-role \
    crd:generateEmbeddedObjectMeta=true \
    webhook \
    paths="./..." \
    output:crd:artifacts:config=config/crd/bases
