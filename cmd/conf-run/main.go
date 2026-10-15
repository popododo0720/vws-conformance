package main

import (
	"flag"
	"os"

	"github.com/popododo0720/vws-conformance/internal/engine"
)

func main() {
	flag.Parse()
	if flag.Arg(0) == "check-endpoint" {
		os.Exit(engine.CheckCLI())
	}
	os.Exit(engine.Main())
}
