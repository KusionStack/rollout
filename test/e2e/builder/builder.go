// Copyright 2023 The KusionStack Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package builder

import "k8s.io/apiserver/pkg/storage/names"

const (
	DefaultNamespace = "rollout-e2e-test"
	DefaultName      = "test"
	DefaultAppName   = "app"
	DefaultCluster   = "cluster"
)

// builder is a base builder for resource
type builder struct {
	namespace  string
	name       string
	namePrefix string
	appName    string
	cluster    string
}

// complete sets default values
func (b *builder) complete() {
	b.buildName()

	if b.namespace == "" {
		b.namespace = DefaultNamespace
	}

	if b.appName == "" {
		b.appName = DefaultAppName
	}
	if b.cluster == "" {
		b.cluster = DefaultCluster
	}
}

// buildName generates the name of resource
func (b *builder) buildName() *builder {
	if b.name != "" {
		return b
	}
	if b.namePrefix != "" {
		b.name = randomName(b.namePrefix)
		return b
	}
	b.name = randomName(DefaultName)
	return b
}

// randomName generates a random name with prefix
func randomName(prefix string) string {
	return names.SimpleNameGenerator.GenerateName(prefix)
}
