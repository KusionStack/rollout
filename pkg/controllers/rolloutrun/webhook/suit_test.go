package webhook

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestWebhookExecutorTestSuite(t *testing.T) {
	suite.Run(t, new(workerTestSuite))
}
