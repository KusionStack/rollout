package api

import "os"

const (
	EnvTestMode = "ENV_TEST_MODE"
)

var (
	IsTestMode = os.Getenv(EnvTestMode) == "true"
)
