// Command conf-inventory lists the operations a vws-console-api checkout dispatches and writes
// them to inventory/inventory.json, or with -check fails when that file is out of date.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/popododo0720/vws-conformance/inventory"
)

func main() {
	code, msg := run(os.Args[1:])
	if code == 0 {
		fmt.Print(msg)
	} else {
		fmt.Fprint(os.Stderr, msg)
	}
	os.Exit(code)
}

// run returns the exit code and the text for stdout (code 0) or stderr (otherwise).
func run(args []string) (int, string) {
	var msg strings.Builder
	fs := flag.NewFlagSet("conf-inventory", flag.ContinueOnError)
	fs.SetOutput(&msg)
	src := fs.String("src", "", "detached vws-console-api checkout at the pinned commit")
	out := fs.String("out", "inventory/inventory.json", "inventory file")
	check := fs.Bool("check", false, "fail when the file differs from the checkout; write nothing")
	if err := fs.Parse(args); err != nil {
		return 2, msg.String()
	}
	if *src == "" || fs.NArg() != 0 {
		return 2, "usage: conf-inventory -src DIR [-out FILE] [-check]\n"
	}
	want, err := generate(*src)
	if err != nil {
		return 1, fmt.Sprintf("conf-inventory: %v\n", err)
	}
	if !*check {
		if err := writeAtomic(*out, want); err != nil {
			return 1, fmt.Sprintf("conf-inventory: %v\n", err)
		}
		return 0, fmt.Sprintf("wrote %s\n", *out)
	}
	have, err := os.ReadFile(*out)
	if err != nil {
		return 1, fmt.Sprintf("conf-inventory: %v\n", err)
	}
	if bytes.Equal(have, want) {
		return 0, ""
	}
	fmt.Fprintf(&msg, "conf-inventory: %s differs from %s; regenerate with: go run ./cmd/conf-inventory -src %s\n",
		*out, *src, *src)
	for _, l := range changedLines(string(have), string(want)) {
		fmt.Fprintln(&msg, l)
	}
	return 1, msg.String()
}

func generate(src string) ([]byte, error) {
	inv, err := inventory.Extract(src)
	if err != nil {
		return nil, fmt.Errorf("extract %s: %w", src, err)
	}
	b, err := inventory.Encode(inv)
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	return b, nil
}

// changedLines lists lines only in want ("+") or only in have ("-"). The file is one name per
// line, so this is the operations added or removed.
func changedLines(have, want string) []string {
	hl, wl := strings.Split(have, "\n"), strings.Split(want, "\n")
	var out []string
	for _, l := range hl {
		if !slices.Contains(wl, l) {
			out = append(out, "- "+strings.TrimSpace(l))
		}
	}
	for _, l := range wl {
		if !slices.Contains(hl, l) {
			out = append(out, "+ "+strings.TrimSpace(l))
		}
	}
	return out
}

// writeAtomic replaces path in one rename, so a run killed mid-write leaves the old file intact.
func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".inventory-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	// CreateTemp makes the file 0600; the committed file is 0644.
	merr := os.Chmod(tmp.Name(), 0o644)
	if err := errors.Join(werr, cerr, merr); err != nil {
		return errors.Join(fmt.Errorf("write %s: %w", tmp.Name(), err), os.Remove(tmp.Name()))
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return errors.Join(fmt.Errorf("replace %s: %w", path, err), os.Remove(tmp.Name()))
	}
	return nil
}
