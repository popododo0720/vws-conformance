package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/popododo0720/vws-conformance/internal/report"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: conf-report <report.json>...")
		os.Exit(2)
	}
	var parts []*report.Report
	for _, path := range os.Args[1:] {
		b, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		var rep report.Report
		if err := json.Unmarshal(b, &rep); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		parts = append(parts, &rep)
	}
	out, err := report.Merge(parts)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	b, err := out.JSON()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	dir := filepath.Dir(os.Args[1])
	if err := os.WriteFile(filepath.Join(dir, "report.json"), b, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(dir, "report.md"), []byte(report.Markdown(out)), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("pass %d fail %d skip %d error %d\n", out.Summary.Pass, out.Summary.Fail, out.Summary.Skip,
		out.Summary.Error)
}
