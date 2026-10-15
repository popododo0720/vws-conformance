package harness

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWithDeadline(t *testing.T) {
	_, err := withDeadline(context.Background(), func(ctx context.Context) (struct{}, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			return struct{}{}, errors.New("missing deadline")
		}
		remain := time.Until(deadline)
		if remain <= 29*time.Second || remain > OpTimeout {
			return struct{}{}, errors.New("deadline " + remain.String())
		}
		return struct{}{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWithDeadlineKeepsShorterParent(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := withDeadline(parent, func(ctx context.Context) (struct{}, error) {
		<-ctx.Done()
		return struct{}{}, ctx.Err()
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("parent deadline was ignored")
	}
}

func TestHTTPClientUsesOpTimeout(t *testing.T) {
	c := NewHTTPClient([]string{"127.0.0.1"})
	if c.Timeout != OpTimeout {
		t.Fatalf("client timeout %s", c.Timeout)
	}
}
