package inventory

import (
	"fmt"
	"strings"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
)

type hit struct {
	happy bool
	bad   bool
	gap   bool
}

func Cover(ops []Op, groups []bind.Group) error {
	seen := map[string]*hit{}
	for _, g := range groups {
		for _, c := range g.Cases {
			key := c.Service + " " + c.Op
			h := seen[key]
			if h == nil {
				h = &hit{}
				seen[key] = h
			}
			switch c.Kind {
			case cases.KindError:
				h.bad = true
			case cases.KindGap:
				h.gap = true
			default:
				h.happy = true
			}
		}
	}
	var missing []string
	for _, op := range ops {
		h := seen[op.Service+" "+op.Name]
		if op.Impl == "gap" {
			if h == nil || !h.gap {
				missing = append(missing, op.Service+" "+op.Name+" missing gap probe")
			}
			continue
		}
		if h == nil || !h.happy || !h.bad {
			missing = append(missing, op.Service+" "+op.Name+" needs a happy case and an error case")
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s", strings.Join(missing, "; "))
	}
	return nil
}
