package harness

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestRuntime() *Runtime {
	return NewRuntime(Config{RunID: testRun, Region: "vws-1"}, nil)
}

func begin(t *testing.T, rt *Runtime, id string) *Runtime {
	t.Helper()
	view, err := rt.BeginCase(id)
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func TestEndCaseRunsCleanupOnceLIFO(t *testing.T) {
	view := begin(t, newTestRuntime(), "CASE-1")
	var order []string
	view.scope.cleanup.Add(func(context.Context) error { order = append(order, "first"); return nil })
	view.scope.cleanup.Add(func(context.Context) error {
		order = append(order, "second")
		return errors.New("delete failed")
	})
	err := view.EndCase(context.Background())
	if err == nil || !strings.Contains(err.Error(), "CASE-1") || !strings.Contains(err.Error(), "delete failed") {
		t.Fatalf("err %v", err)
	}
	again := view.EndCase(context.Background())
	if again == nil || again.Error() != err.Error() {
		t.Fatalf("second EndCase %v, want %v", again, err)
	}
	if strings.Join(order, ",") != "second,first" {
		t.Fatalf("order %v", order)
	}
}

func TestCleanupContextIgnoresCaseCancel(t *testing.T) {
	view := begin(t, newTestRuntime(), "CASE-1")
	view.scope.cleanup.Add(func(ctx context.Context) error { return ctx.Err() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := view.EndCase(ctx); err != nil {
		t.Fatalf("cleanup saw the canceled case context: %v", err)
	}
}

func TestEndCaseWaitsForMutationInFlight(t *testing.T) {
	view := begin(t, newTestRuntime(), "CASE-1")
	s, err := view.admit()
	if err != nil {
		t.Fatal(err)
	}
	var cleaned atomic.Bool
	s.cleanup.Add(func(context.Context) error { cleaned.Store(true); return nil })
	ended := make(chan error, 1)
	go func() { ended <- view.EndCase(context.Background()) }()
	select {
	case err := <-ended:
		t.Fatalf("EndCase returned %v during a mutation", err)
	case <-time.After(100 * time.Millisecond):
	}
	s.mu.Unlock()
	if err := <-ended; err != nil || !cleaned.Load() {
		t.Fatalf("err %v cleaned %v", err, cleaned.Load())
	}
	if _, err := view.admit(); err == nil {
		t.Fatal("mutation admitted after EndCase")
	}
}

func TestShutdownClosesLiveScopesAndRefusesNewWork(t *testing.T) {
	rt := newTestRuntime()
	a := begin(t, rt, "CASE-A")
	b := begin(t, rt, "CASE-B")
	var runs atomic.Int32
	a.scope.cleanup.Add(func(context.Context) error { runs.Add(1); return nil })
	b.scope.cleanup.Add(func(context.Context) error { runs.Add(1); return errors.New("preserve volume vol-b") })
	err := rt.Shutdown(context.Background())
	if err == nil || !strings.Contains(err.Error(), "CASE-B") {
		t.Fatalf("shutdown %v", err)
	}
	if runs.Load() != 2 {
		t.Fatalf("cleanup ran %d times", runs.Load())
	}
	if _, err := rt.BeginCase("CASE-C"); err == nil {
		t.Fatal("BeginCase accepted after Shutdown")
	}
	if _, err := a.admit(); err == nil {
		t.Fatal("mutation admitted after Shutdown")
	}
	if err := a.EndCase(context.Background()); err != nil {
		t.Fatalf("EndCase after Shutdown reran cleanup: %v", err)
	}
	if runs.Load() != 2 {
		t.Fatalf("cleanup ran %d times", runs.Load())
	}
}

func TestShutdownStopsMutationsBeforeScopesClose(t *testing.T) {
	rt := newTestRuntime()
	view := begin(t, rt, "CASE-1")
	held, err := view.admit()
	if err != nil {
		t.Fatal(err)
	}
	shut := make(chan error, 1)
	go func() { shut <- rt.Shutdown(context.Background()) }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		rt.scopes.mu.Lock()
		stopping := rt.scopes.stopping
		rt.scopes.mu.Unlock()
		if stopping {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("shutdown did not start")
		}
		time.Sleep(time.Millisecond)
	}
	held.mu.Unlock()
	if err := <-shut; err != nil {
		t.Fatal(err)
	}
	other := &Runtime{scopes: rt.scopes, scope: &caseScope{caseID: "LATE"}}
	if _, err := other.admit(); err == nil {
		t.Fatal("mutation admitted while stopping")
	}
}

func TestUnscopedRuntimeRefusesMutation(t *testing.T) {
	rt := newTestRuntime()
	if _, err := rt.admit(); err == nil {
		t.Fatal("singleton runtime admitted a mutation")
	}
	if err := rt.EndCase(context.Background()); err == nil {
		t.Fatal("EndCase on the singleton succeeded")
	}
}
