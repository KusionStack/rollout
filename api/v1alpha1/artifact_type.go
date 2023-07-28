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
