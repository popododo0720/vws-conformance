package main

import (
	"flag"
	"os"

	"github.com/popododo0720/vws-conformance/internal/engine"
)

func main() {
	flag.Parse()
	os.Exit(engine.Main())
}
