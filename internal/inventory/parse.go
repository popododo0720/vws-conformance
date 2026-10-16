package inventory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Op struct {
	Service string
	Name    string
	Impl    string
}

func Load(path string) ([]Op, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, wrapErr("Load", err)
	}
	return Parse(string(b))
}

func Find() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", wrapErr("Find", err)
	}
	for i := 0; i < 8; i++ {
		p := filepath.Join(dir, "inventory", "inventory.yaml")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		dir = filepath.Dir(dir)
	}
	return "", fmt.Errorf("inventory/inventory.yaml not found")
}

func Parse(text string) ([]Op, error) {
	var out []Op
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- {") || !strings.HasSuffix(line, "}") {
			continue
		}
		body := strings.TrimSuffix(strings.TrimPrefix(line, "- {"), "}")
		op := Op{}
		for _, part := range strings.Split(body, ",") {
			kv := strings.SplitN(strings.TrimSpace(part), ":", 2)
			if len(kv) != 2 {
				return nil, fmt.Errorf("bad inventory line %q", line)
			}
			val := strings.TrimSpace(kv[1])
			switch strings.TrimSpace(kv[0]) {
			case "service":
				op.Service = val
			case "op":
				op.Name = val
			case "impl":
				op.Impl = val
			default:
				return nil, fmt.Errorf("bad inventory key %q", kv[0])
			}
		}
		if op.Service == "" || op.Name == "" || (op.Impl != "yes" && op.Impl != "gap") {
			return nil, fmt.Errorf("bad inventory op %q", line)
		}
		out = append(out, op)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("inventory is empty")
	}
	return out, nil
}
