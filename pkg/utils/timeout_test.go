package utils

import (
	"context"
	"testing"
	"time"
)

func TestRunWithTimeout(t *testing.T) {
	err := RunWithTimeout(context.Background(), time.Duration(50)*time.Microsecond, func() { time.Sleep(time.Duration(100) * time.Microsecond) })
	if err == nil {
		t.Errorf("expect timeout error")
	}

	err = RunWithTimeout(context.Background(), time.Duration(100)*time.Microsecond, func() { time.Sleep(time.Duration(50) * time.Microsecond) })
	if err != nil {
		t.Errorf("unexpect timeout error")
	}
}
