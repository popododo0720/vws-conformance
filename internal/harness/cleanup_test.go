package harness

import (
	"context"
	"testing"
	"time"
)

func TestCleanupRunsInReverse(t *testing.T) {
	var c Cleaner
	var order []string
	c.Add(func(context.Context) error { order = append(order, "a"); return nil })
	c.Add(func(context.Context) error { order = append(order, "b"); return nil })
	if err := c.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "b" || order[1] != "a" {
		t.Fatalf("%v", order)
	}
}

func TestCleanupRunsAfterPanic(t *testing.T) {
	ran := false
	panicked := RunProtected(func() { panic("boom") }, func() { ran = true })
	if panicked == nil {
		t.Fatal("expected panic")
	}
	if !ran {
		t.Fatal("cleanup did not run")
	}
}

func TestSignalRunsCleanup(t *testing.T) {
	sig := make(chan osSignal, 1)
	ran := make(chan struct{})
	done := OnSignal(sig, func() { close(ran) })
	sig <- osInterrupt
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
	select {
	case <-ran:
	default:
		t.Fatal("cleanup did not run")
	}
}
