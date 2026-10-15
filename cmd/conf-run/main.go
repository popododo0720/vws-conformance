package main

import (
	"flag"
	"os"

	"github.com/popododo0720/vws-conformance/internal/engine"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func main() {
	flag.Parse()
	os.Exit(execute())
}

func execute() int {
	if flag.Arg(0) == "check-endpoint" {
		return engine.CheckCLI()
	}
	if flag.Arg(0) == "cli" {
		return engine.RunCLI(flag.Args()[1:])
	}
	return harness.Main(engineMain{})
}

type engineMain struct{}

func (engineMain) Run() int { return engine.Main() }
