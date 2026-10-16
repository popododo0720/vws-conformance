package main

import (
	"fmt"
	"os"

	"github.com/popododo0720/vws-conformance/internal/inventory"
)

func main() {
	src := os.Getenv("VWS_CONF_CONSOLE_API_SRC")
	path, err := inventory.Find()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	file, err := inventory.Load(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if src == "" {
		for _, op := range file {
			fmt.Printf("%s %s %s\n", op.Service, op.Name, op.Impl)
		}
		return
	}
	live, err := inventory.FromCheckout(src)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := inventory.SameImpl(file, live); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("inventory matches %s (%d implemented operations)\n", src, len(live))
}
