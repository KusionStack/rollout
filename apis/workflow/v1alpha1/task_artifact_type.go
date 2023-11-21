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

package v1alpha1

// Artifact indicates an artifact
type Artifact struct {
	// Name is the name of the artifact
	// +optional
	Name string `json:"name,omitempty"`

	// Git defines the git artifact details
	// +optional
	Git *GitArtifact `json:"git,omitempty"`

	// HTTP defines the http artifact details
	// +optional
	HTTP *HTTPArtifact `json:"http,omitempty"`
}

// GitArtifact defines the git artifact details
type GitArtifact struct {
}

// HTTPArtifact defines the http artifact details
type HTTPArtifact struct {
	// URL is the url of the artifact
	URL string `json:"url"`

	// Headers are the headers of the http request
	// +optional
	Headers map[string]string `json:"headers,omitempty"`

	// Body is the body of the http request
	// +optional
	Body string `json:"body,omitempty"`

	// Method is the method of the http request
	// +optional
	Method string `json:"method,omitempty"`
}
