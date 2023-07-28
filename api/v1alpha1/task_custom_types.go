package v1alpha1

// CustomTask is defined by the user which will be executed by user's executor server
type CustomTask struct {
	// Name is the name of the custom task
	Name string `json:"name"`

	// Executor is the executor that will execute the custom task
	Executor Executor `json:"executor"`
}

// Executor is the executor that will execute the custom task
type Executor struct {
	// Endpoint is the endpoint of the executor server
	Endpoint string `json:"endpoint"`
}
