package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/popododo0720/vws-conformance/internal/harness"
)

func main() {
	dry := flag.Bool("dry-run", false, "list leftover suite resources and do not delete them")
	flag.Parse()
	cfg, err := harness.FromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if cfg.RunID == "" {
		id, err := harness.NewRunID(time.Now())
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		cfg.RunID = id
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := cfg.CheckEndpoints(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cl, err := harness.Load(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	rt := harness.NewRuntime(cfg, cl)
	rt.Dry = *dry
	account, err := rt.Identify(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := harness.CheckAccount(account, cfg.AllowedAccounts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := rt.Sweep(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
