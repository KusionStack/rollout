package v1alpha1

// EchoTask is a task that echos a message, mainly used for testing or demo purpose
type EchoTask struct {
	// Message is the message to echo
	Message string `json:"message"`
}
