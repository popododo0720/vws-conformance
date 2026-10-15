package harness

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

type Cleaner struct {
	mu  sync.Mutex
	fns []func(context.Context) error
}

func (c *Cleaner) Add(fn func(context.Context) error) {
	c.mu.Lock()
	c.fns = append(c.fns, fn)
	c.mu.Unlock()
}

func (c *Cleaner) Run(ctx context.Context) error {
	c.mu.Lock()
	fns := append([]func(context.Context) error{}, c.fns...)
	c.fns = nil
	c.mu.Unlock()
	var errs []error
	for i := len(fns) - 1; i >= 0; i-- {
		if err := fns[i](ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("cleanup: %v", errs)
}

func RunProtected(fn func(), cleanup func()) (panicked any) {
	defer func() {
		panicked = recover()
		cleanup()
	}()
	fn()
	return nil
}

func OnSignal(sig <-chan os.Signal, cleanup func()) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		<-sig
		cleanup()
		close(done)
	}()
	return done
}

var process struct {
	mu sync.Mutex
	fn func()
}

func SetProcessCleanup(fn func()) {
	process.mu.Lock()
	process.fn = fn
	process.mu.Unlock()
}

func runProcessCleanup() {
	process.mu.Lock()
	fn := process.fn
	process.mu.Unlock()
	if fn != nil {
		fn()
	}
}

func Main(m interface{ Run() int }) int {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		runProcessCleanup()
		os.Exit(1)
	}()
	defer func() {
		if recover() != nil {
			runProcessCleanup()
			panic("recovered")
		}
	}()
	code := m.Run()
	runProcessCleanup()
	return code
}
