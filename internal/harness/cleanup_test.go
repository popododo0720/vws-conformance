package harness

import (
	"context"
	"errors"
	"strings"
	"syscall"
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

func TestCleanupRunsEveryCallbackPastFailureAndPanic(t *testing.T) {
	var c Cleaner
	ran := 0
	c.Add(func(context.Context) error { ran++; return nil })
	c.Add(func(context.Context) error { panic("boom") })
	c.Add(func(context.Context) error { ran++; return errors.New("delete failed") })
	err := c.Run(context.Background())
	if ran != 2 || err == nil || !strings.Contains(err.Error(), "boom") ||
		!strings.Contains(err.Error(), "delete failed") {
		t.Fatalf("ran %d err %v", ran, err)
	}
	if err := c.Run(context.Background()); err != nil {
		t.Fatalf("second run %v", err)
	}
}

func TestCleanupOverlappingRunWaits(t *testing.T) {
	var c Cleaner
	started := make(chan struct{})
	release := make(chan struct{})
	c.Add(func(context.Context) error {
		close(started)
		<-release
		return nil
	})
	first := make(chan error, 1)
	go func() { first <- c.Run(context.Background()) }()
	<-started
	second := make(chan error, 1)
	go func() { second <- c.Run(context.Background()) }()
	select {
	case err := <-second:
		t.Fatalf("overlapping run returned %v while the first was still cleaning", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatal(err)
	}
}

func TestRunMainCleanup(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ran := false
		SetProcessCleanup(func() error { ran = true; return nil })
		code := RunMain(staticRun(0), blockedSignals(), func(int) {})
		if code != 0 || !ran {
			t.Fatalf("code %d ran %v", code, ran)
		}
	})
	t.Run("failure", func(t *testing.T) {
		ran := false
		SetProcessCleanup(func() error { ran = true; return nil })
		code := RunMain(staticRun(2), blockedSignals(), func(int) {})
		if code != 2 || !ran {
			t.Fatalf("code %d ran %v", code, ran)
		}
	})
	t.Run("SIGTERM", func(t *testing.T) {
		ran := make(chan struct{})
		SetProcessCleanup(func() error { close(ran); return nil })
		sig := make(chan osSignal, 1)
		exitCode := make(chan int, 1)
		entered := make(chan struct{})
		go RunMain(waitRun(entered), sig, func(code int) { exitCode <- code })
		<-entered
		sig <- syscall.SIGTERM
		select {
		case <-ran:
		case <-time.After(2 * time.Second):
			t.Fatal("cleanup did not run")
		}
		select {
		case code := <-exitCode:
			if code != 1 {
				t.Fatalf("exit %d", code)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("process did not exit")
		}
	})
	t.Run("overlap", func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})
		SetProcessCleanup(func() error {
			close(started)
			<-release
			return nil
		})
		sig := make(chan osSignal, 1)
		entered := make(chan struct{})
		runlet := &gateRun{entered: entered, release: make(chan struct{})}
		returned := make(chan int, 1)
		exited := make(chan int, 1)
		go func() {
			returned <- RunMain(runlet, sig, func(code int) { exited <- code })
		}()
		<-entered
		sig <- syscall.SIGTERM
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("cleanup did not start")
		}
		close(runlet.release)
		select {
		case code := <-returned:
			t.Fatalf("RunMain returned %d before cleanup finished", code)
		case code := <-exited:
			t.Fatalf("exit %d before cleanup finished", code)
		case <-time.After(200 * time.Millisecond):
		}
		close(release)
		select {
		case <-returned:
		case <-exited:
		case <-time.After(2 * time.Second):
			t.Fatal("shutdown did not finish")
		}
	})
	t.Run("cleanup error", func(t *testing.T) {
		SetProcessCleanup(func() error { return errors.New("sweep failed") })
		code := RunMain(staticRun(0), blockedSignals(), func(int) {})
		if code != 1 {
			t.Fatalf("code %d", code)
		}
	})
}

type staticRun int

func (s staticRun) Run() int { return int(s) }

func blockedSignals() <-chan osSignal {
	return make(chan osSignal)
}

type gateRun struct {
	entered chan struct{}
	release chan struct{}
}

func (g *gateRun) Run() int {
	close(g.entered)
	<-g.release
	return 0
}

type waitRun chan struct{}

func (w waitRun) Run() int {
	close(w)
	select {}
}
