package utils

import (
	"context"
	"time"
)

func RunWithTimeout(parentCtx context.Context, duration time.Duration, task func()) error {
	ctx, cancel := context.WithTimeout(parentCtx, duration)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		task()
		done <- struct{}{}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}
