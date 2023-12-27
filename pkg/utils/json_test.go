package utils

import (
	"testing"

	"kusionstack.io/rollout/apis/rollout"
)

func TestEscape(t *testing.T) {
	val := Escape("")
	if len(val) != 0 {
		t.Errorf("blank string expect len 0 but got %d", len(val))
	}

	val = Escape(rollout.AnnoManualCommandKey)
	escapedAnnoManualCommandKey := "rollout.kusionstack.io~1manual-command"
	if val != escapedAnnoManualCommandKey {
		t.Errorf("%s expect %s but got %s", rollout.AnnoManualCommandKey, escapedAnnoManualCommandKey, val)
	}
}
