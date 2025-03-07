package rolloutclasspredicate

import (
	"os"
)

const (
	RolloutClassDefault string = "default"
)

func GetRolloutClassFromEnv() string {
	rolloutClassEnv := os.Getenv("ROLLOUT_CLASS")
	if rolloutClassEnv == "" {
		rolloutClassEnv = RolloutClassDefault
	}

	return rolloutClassEnv
}
