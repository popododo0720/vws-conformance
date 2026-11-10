// Command awsmodel-diff compares the botocore API models at the pinned release (pin.json) with a
// newer release, for the operations the case catalog marks as implemented, and prints one JSON
// line {title, body, marker} per changed operation that no filed issue records yet
// (vws-conformance#121). It reads model files fetched by scripts/awsmodel-fetch.sh and does no
// network I/O itself.
//
//	awsmodel-diff -base DIR -current DIR -tag TAG [-filed FILE]
//	awsmodel-diff -repin -current DIR -tag TAG > cmd/awsmodel-diff/pin.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/popododo0720/vws-conformance/internal/awsmodel"
	"github.com/popododo0720/vws-conformance/internal/engine"
)

type config struct {
	Pin        *pin
	Ops        map[string][]string
	BaseDir    string
	CurrentDir string
	Newest     string
	// Filed is the text of the issues already filed (their bodies); a finding whose marker
	// appears in it is not printed again.
	Filed string
}

func main() {
	pinPath := flag.String("pin", "cmd/awsmodel-diff/pin.json", "pin file")
	base := flag.String("base", "", "directory with <service>.json at the pinned release")
	current := flag.String("current", "", "directory with <service>.json at -tag")
	tag := flag.String("tag", "", "botocore release tag of -current")
	filed := flag.String("filed", "", "file with the bodies of issues already filed")
	repinMode := flag.Bool("repin", false, "print pin.json moved to -tag instead of diffing")
	flag.Parse()
	cfg, err := load(*pinPath, *filed)
	if err == nil {
		cfg.BaseDir, cfg.CurrentDir, cfg.Newest = *base, *current, *tag
		if *repinMode {
			err = repin(cfg, os.Stdout)
		} else {
			err = run(cfg, os.Stdout)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func load(pinPath, filedPath string) (config, error) {
	data, err := os.ReadFile(pinPath)
	if err != nil {
		return config{}, fmt.Errorf("read pin: %w", err)
	}
	p, err := parsePin(data)
	if err != nil {
		return config{}, err
	}
	ops, err := implementedOps(engine.Groups())
	if err != nil {
		return config{}, err
	}
	cfg := config{Pin: p, Ops: ops}
	if filedPath != "" {
		filed, err := os.ReadFile(filedPath)
		if err != nil {
			return config{}, fmt.Errorf("read filed issues: %w", err)
		}
		cfg.Filed = string(filed)
	}
	return cfg, nil
}

func readModel(dir, service string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(dir, service+".json"))
	if err != nil {
		return nil, fmt.Errorf("read %s model: %w", service, err)
	}
	return data, nil
}

// diffService returns the changes of one service's implemented operations; the baseline file
// must be the pinned copy.
func diffService(cfg config, m pinModel) ([]awsmodel.Change, error) {
	baseData, err := readModel(cfg.BaseDir, m.Service)
	if err != nil {
		return nil, err
	}
	if err := m.check(baseData); err != nil {
		return nil, err
	}
	curData, err := readModel(cfg.CurrentDir, m.Service)
	if err != nil {
		return nil, err
	}
	baseline, err := awsmodel.Parse(baseData)
	if err != nil {
		return nil, fmt.Errorf("%s pinned model: %w", m.Service, err)
	}
	current, err := awsmodel.Parse(curData)
	if err != nil {
		return nil, fmt.Errorf("%s %s model: %w", m.Service, cfg.Newest, err)
	}
	changes, err := awsmodel.Diff(baseline, current, cfg.Ops[m.Service])
	if err != nil {
		return nil, fmt.Errorf("%s: %w", m.Service, err)
	}
	return changes, nil
}

func run(cfg config, out io.Writer) error {
	if !releaseTag.MatchString(cfg.Newest) {
		return &TagError{Tag: cfg.Newest}
	}
	pinned := make(map[string]bool, len(cfg.Pin.Models))
	for _, m := range cfg.Pin.Models {
		pinned[m.Service] = true
	}
	for svc := range cfg.Ops {
		if !pinned[svc] {
			return fmt.Errorf("awsmodel: implemented service %q has no model in pin.json", svc)
		}
	}
	enc := json.NewEncoder(out)
	// The marker is an HTML comment; escaping it would make the issue body differ from the marker.
	enc.SetEscapeHTML(false)
	rel := release{Pinned: cfg.Pin.Botocore, Newest: cfg.Newest}
	for _, m := range cfg.Pin.Models {
		changes, err := diffService(cfg, m)
		if err != nil {
			return err
		}
		for _, ch := range changes {
			f := newFinding(m.Service, rel, ch)
			if strings.Contains(cfg.Filed, f.Marker) {
				continue
			}
			if err := enc.Encode(f); err != nil {
				return fmt.Errorf("write finding: %w", err)
			}
		}
	}
	return nil
}

// repin prints pin.json moved to cfg.Newest, with the sha256 of each model in cfg.CurrentDir.
func repin(cfg config, out io.Writer) error {
	if !releaseTag.MatchString(cfg.Newest) {
		return &TagError{Tag: cfg.Newest}
	}
	next := pin{Botocore: cfg.Newest, Models: make([]pinModel, 0, len(cfg.Pin.Models))}
	for _, m := range cfg.Pin.Models {
		data, err := readModel(cfg.CurrentDir, m.Service)
		if err != nil {
			return err
		}
		if _, err := awsmodel.Parse(data); err != nil {
			return fmt.Errorf("%s %s model: %w", m.Service, cfg.Newest, err)
		}
		next.Models = append(next.Models, pinModel{Service: m.Service, Path: m.Path, SHA256: digest(data)})
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(next); err != nil {
		return fmt.Errorf("write pin: %w", err)
	}
	return nil
}
