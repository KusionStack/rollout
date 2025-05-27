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

# golang::get_module_dir use go list to get the module dir
#
# Parameters:
#  - $1: the module name
golang::get_module_dir() {
    local pkg="$1"
    go list -mod=readonly -m -f '{{.Dir}}' "$pkg"
}

# golang::get_go_module_from_path get the go module from given path.
# It will try to read model from go.mod, if not found, use go list -m
#
# Parameters:
#  - $1: the path of repo which contains go.mod
golang::get_go_module_from_path() {
    local repo_root=$1
    local model=""
    # try to read model from go.mod
    if [[ -f "${repo_root}/go.mod" ]]; then
        # read model from go.mod
        model=$(grep '^module ' "${repo_root}/go.mod" | awk '{print $2}')
    fi
    if [[ -z "${model}" ]]; then
        model=$(go list -m | head -1)
    fi
    echo "${model}"
}

# golang:create_gopath_tree create the temporary GOPATH tree on local path
#
# Parameters:
#  - $1: the root path of repo
#  - $2: temporary go path
#  - $3: go module
golang:create_gopath_tree() {
    local repo_root=$1
    local go_path=$2
    local go_module=$3

    local go_pkg_dir="${go_path}/${go_module}"
    go_pkg_dir=$(dirname "${go_pkg_dir}")

    mkdir -p "${go_pkg_dir}"

    if [[ ! -e "${go_pkg_dir}" || "$(readlink "${go_pkg_dir}")" != "${repo_root}" ]]; then
        ln -snf "${repo_root}" "${go_pkg_dir}"
    fi
}

# golang::install install the package to local path
# Parameters:
#  - $1: the package with command
#  - $2: the version of package
golang::install() {
    local pkg="$1"
    local version="$2"
    go install -mod=readonly "$pkg@$version"
}

# golang::install_from_src install command from module source code
# which contains directives like replace or exclude
# ref https://go.dev/doc/go-get-install-deprecation
# Parameters:
#  - $1: the module
#  - $2: the version of package
#  - $3: the relitive path of cmd
golang::install_from_src() {
    local pkg="$1"
    local version="$2"
    local relitive_cmd_path="$3"

    # create a temporary dir
    tmp_dir=$(mktemp -d -t golang-install-XXXXXX)
    # init temp go module
    cd "${tmp_dir}" || return
    go mod init tmp
    # download target go module
    go get "${pkg}@${version}"
    # get target model dir
    mod_dir=$(golang::get_module_dir "$pkg")
    # install from source code
    cd "$mod_dir" || return
    go install "${mod_dir}/${relitive_cmd_path}"
}

# golang::get_host_os_arch get the host os and arch, like linux_amd64 darwin_arm64
golang::get_host_os_arch() {
    echo "$(go env GOHOSTOS)_$(go env GOHOSTARCH)"
}
