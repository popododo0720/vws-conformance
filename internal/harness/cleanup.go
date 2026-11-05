package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// Cleaner is a LIFO stack of cleanup callbacks. Run executes every callback once, even after
// one fails or panics, and an overlapping Run waits for the first to finish.
type Cleaner struct {
	mu    sync.Mutex
	runMu sync.Mutex
	fns   []func(context.Context) error
}

func (c *Cleaner) Add(fn func(context.Context) error) {
	c.mu.Lock()
	c.fns = append(c.fns, fn)
	c.mu.Unlock()
}

func (c *Cleaner) Run(ctx context.Context) error {
	c.runMu.Lock()
	defer c.runMu.Unlock()
	c.mu.Lock()
	fns := c.fns
	c.fns = nil
	c.mu.Unlock()
	var errs []error
	for i := len(fns) - 1; i >= 0; i-- {
		errs = append(errs, runRecovered(ctx, fns[i]))
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("cleanup: %w", err)
	}
	return nil
}

func runRecovered(ctx context.Context, fn func(context.Context) error) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("cleanup panic: %v", rec)
		}
	}()
	return fn(ctx)
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
		if err := runProcessCleanup(); err != nil {
			fmt.Fprintf(os.Stderr, "cleanup: %v\n", err)
		}
		exit(1)
	}()
	defer func() {
		if recover() != nil {
			if err := runProcessCleanup(); err != nil {
				fmt.Fprintf(os.Stderr, "cleanup: %v\n", err)
			}
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
