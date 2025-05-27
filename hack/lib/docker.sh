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

DOCKER_RUN_IMAGE=${DOCKER_RUN_IMAGE:-"golang:1.23.7-bookworm"}

# author: @zoumo
# note: Use -- to distinguish between docker parameters and in-container commands.
#
# Basic usage (no additional Flags):
#   docker::run -- ls -l
#
# With complex parameters
#   docker::run \
#      -e "CONFIG={\"key\":\"value\"}" \
#      --volume "${HOME}/My Docs:/docs" \
#      -- \
#      python /docs/script.py
#
# Change docker run image with Env DOCKER_RUN_IMAGE
#   DOCKER_RUN_IMAGE="golang:1.24.2-bookworm" docker::run -- go version
docker::run() {
    local -a flags=()

    # Parsing command line Flags (supports temporary addition of parameters)
    while [[ $# -gt 0 && $1 != "--" ]]; do
        flags+=("$1")
        shift
    done
    [[ $1 == "--" ]] && shift # Remove Separator

    docker run -it --rm \
        -v "${PROJECT_ROOT_DIR}:/workspace" \
        -w "/workspace" \
        "${flags[@]}" \
        "${DOCKER_RUN_IMAGE}" \
        $@
}
