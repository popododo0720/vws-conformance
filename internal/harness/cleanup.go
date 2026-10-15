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
	mu      sync.Mutex
	fn      func() error
	ctx     context.Context
	cancel  context.CancelFunc
	running bool
	done    chan struct{}
	err     error
}

func init() {
	process.ctx, process.cancel = context.WithCancel(context.Background())
}

func ShutdownContext() context.Context {
	process.mu.Lock()
	defer process.mu.Unlock()
	return process.ctx
}

func SetProcessCleanup(fn func() error) {
	process.mu.Lock()
	process.fn = fn
	process.mu.Unlock()
}

func runProcessCleanup() error {
	process.mu.Lock()
	if process.running {
		done := process.done
		process.mu.Unlock()
		<-done
		process.mu.Lock()
		err := process.err
		process.mu.Unlock()
		return err
	}
	fn := process.fn
	process.fn = nil
	if fn == nil {
		process.mu.Unlock()
		return nil
	}
	done := make(chan struct{})
	process.done = done
	process.running = true
	process.cancel()
	process.mu.Unlock()
	err := fn()
	process.mu.Lock()
	process.err = err
	process.running = false
	process.mu.Unlock()
	close(done)
	return err
}

func RunMain(m interface{ Run() int }, sig <-chan os.Signal, exit func(int)) int {
	go func() {
		<-sig
		_ = runProcessCleanup()
		exit(1)
	}()
	defer func() {
		if recover() != nil {
			_ = runProcessCleanup()
			panic("recovered")
		}
	}()
	code := m.Run()
	if err := runProcessCleanup(); err != nil {
		return 1
	}
	return code
}

func Main(m interface{ Run() int }) int {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sig)
	return RunMain(m, sig, os.Exit)
}
