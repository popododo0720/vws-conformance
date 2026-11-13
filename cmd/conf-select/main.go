package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"

	"github.com/popododo0720/vws-conformance/internal/engine"
	"github.com/popododo0720/vws-conformance/internal/touch"
)

func main() {
	target := flag.String("target", "", "list the read-only groups for pve-test")
	flag.Parse()
	if *target == "pve-test" {
		for _, g := range engine.ReadOnlyGroups() {
			fmt.Printf("%s %s\n", g.Service, g.Name)
		}
		return
	}
	if *target != "" {
		fmt.Fprintf(os.Stderr, "unsupported selection target %q\n", *target)
		os.Exit(2)
	}
	var files []string
	if flag.NArg() > 0 {
		files = flag.Args()
	} else {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			if line := sc.Text(); line != "" {
				files = append(files, line)
			}
		}
	}
	sel := touch.Select(files)
	if sel.Full {
		fmt.Println("FULL")
		return
	}
	for _, g := range sel.Groups {
		fmt.Printf("%s %s\n", g.Service, g.Name)
	}
}
