package inventory

import (
	_ "embed"
	"fmt"
	"slices"

	"github.com/popododo0720/vws-conformance/internal/cases"
)

//go:embed inventory.json
var inventoryFile []byte

// Load reads the embedded inventory.json and waivers.tsv.
func Load() (Inventory, []Waiver, error) {
	inv, err := Decode(inventoryFile)
	if err != nil {
		return Inventory{}, nil, err
	}
	waivers, err := ParseWaivers(waiversFile)
	if err != nil {
		return Inventory{}, nil, err
	}
	return inv, waivers, nil
}

// Audit compares what the pinned console-api dispatches with the cases and waivers. It returns
// one sorted line per problem, and none when they agree. A case counts whether or not it is
// skipped: passing coverage is a separate measure. Every case must target a dispatched
// operation, whatever its Impl.
func Audit(inv Inventory, waivers []Waiver, cs []cases.Case) []string {
	pin := inv.Commit[:8]
	var findings []string
	cased := map[Op]string{}
	for _, c := range cs {
		op, err := opOf(c, inv)
		if err != nil {
			findings = append(findings, fmt.Sprintf("%s: %v", c.ID, err))
			continue
		}
		if !inv.Ops[op] {
			findings = append(findings, fmt.Sprintf("%s: case targets %s, which console-api does not dispatch at %s",
				c.ID, op, pin))
			continue
		}
		if first, ok := cased[op]; !ok || c.ID < first {
			cased[op] = c.ID
		}
	}
	waived := map[Op]bool{}
	for _, w := range waivers {
		waived[w.Op] = true
		switch id, hasCase := cased[w.Op]; {
		case !inv.Ops[w.Op]:
			findings = append(findings, fmt.Sprintf(
				"%s: waiver on line %d excuses an operation console-api does not dispatch at %s", w.Op, w.Line, pin))
		case hasCase:
			findings = append(findings, fmt.Sprintf("%s: waiver on line %d is stale, case %s covers it", w.Op, w.Line, id))
		}
	}
	for op := range inv.Ops {
		if _, ok := cased[op]; !ok && !waived[op] {
			findings = append(findings, fmt.Sprintf("%s: implemented at %s but has no case and no waiver", op, pin))
		}
	}
	slices.Sort(findings)
	return findings
}

// opOf maps a case to its operation. The case Service is coarser than the API (vpc and ebs are EC2
// Query) and auth cases span APIs, so an auth case resolves to the one API that dispatches its Op.
func opOf(c cases.Case, inv Inventory) (Op, error) {
	switch c.Service {
	case "ec2", "vpc", "ebs":
		return Op{EC2, c.Op}, nil
	case "s3", "iam", "sts":
		return Op{API(c.Service), c.Op}, nil
	case "auth":
		var hits []Op
		for _, api := range apis {
			if inv.Ops[Op{api, c.Op}] {
				hits = append(hits, Op{api, c.Op})
			}
		}
		switch len(hits) {
		case 1:
			return hits[0], nil
		case 0:
			return Op{}, fmt.Errorf("auth case targets %q, which no console-api API dispatches at %s", c.Op, inv.Commit[:8])
		}
		return Op{}, fmt.Errorf("auth case op %q is dispatched by %v, so its API is ambiguous", c.Op, hits)
	}
	return Op{}, fmt.Errorf("case service %q is not one of ec2, vpc, ebs, s3, iam, sts, auth", c.Service)
}
