package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/popododo0720/vws-conformance/internal/report"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: conf-issues <report.json>")
		os.Exit(2)
	}
	b, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var rep report.Report
	if err := json.Unmarshal(b, &rep); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	n := 0
	for _, res := range rep.Results {
		if res.Status != "fail" && res.Status != "error" {
			continue
		}
		fmt.Println(report.IssueText(&rep, res))
		fmt.Println("---")
		n++
	}
	fmt.Fprintf(os.Stderr, "dry-run %d issues\n", n)
}
