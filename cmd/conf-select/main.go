package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/popododo0720/vws-conformance/internal/touch"
)

func main() {
	var files []string
	if len(os.Args) > 1 {
		files = os.Args[1:]
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
