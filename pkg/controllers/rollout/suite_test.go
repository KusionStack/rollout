package rollout

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestRolloutInitializationTestSuite(t *testing.T) {
	suite.Run(t, new(RolloutInitializationTestSuite))
}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestRolloutControllerTestSuite(t *testing.T) {
	suite.Run(t, new(RolloutControllerTestSuite))
}
