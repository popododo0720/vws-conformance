package harness

import (
	"context"
	"time"
)

const OpTimeout = 30 * time.Second

func withDeadline[T any](ctx context.Context, fn func(context.Context) (T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(ctx, OpTimeout)
	defer cancel()
	return fn(ctx)
}
