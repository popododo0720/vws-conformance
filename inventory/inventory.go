// Package inventory lists the operations vws-console-api dispatches at a pinned commit.
package inventory

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
)

// API is a console-api surface.
type API string

const (
	EC2 API = "ec2"
	S3  API = "s3"
)

// apis is every API, sorted.
var apis = []API{EC2, S3}

// Op is one operation. It prints in the AWS IAM action notation, "ec2:RunInstances", which keeps
// the same name on two APIs apart.
type Op struct {
	API  API
	Name string
}

func (o Op) String() string { return string(o.API) + ":" + o.Name }

var opNameRE = regexp.MustCompile(`^[A-Za-z0-9]+$`)

// Inventory is what the console-api checkout at Commit dispatches.
type Inventory struct {
	Commit string
	Ops    map[Op]bool
}

const (
	schemaVersion = "1"
	consoleRepo   = "popododo0720/vws-console-api"
)

var commitRE = regexp.MustCompile(`^[0-9a-f]{40}$`)

// wire is the file form. Encode is its only user.
type wire struct {
	SchemaVersion string           `json:"schema_version"`
	ConsoleAPI    wireSource       `json:"console_api"`
	Operations    map[API][]string `json:"operations"`
}

type wireSource struct {
	Repo   string `json:"repo"`
	Commit string `json:"commit"`
}

// Encode is the canonical file form: sorted operations, two-space indent, trailing newline.
func Encode(inv Inventory) ([]byte, error) {
	if !commitRE.MatchString(inv.Commit) {
		return nil, fmt.Errorf("commit %q is not 40 lowercase hex digits", inv.Commit)
	}
	w := wire{schemaVersion, wireSource{consoleRepo, inv.Commit}, map[API][]string{}}
	for op := range inv.Ops {
		if !slices.Contains(apis, op.API) || !opNameRE.MatchString(op.Name) {
			return nil, fmt.Errorf("invalid operation %q", op)
		}
		w.Operations[op.API] = append(w.Operations[op.API], op.Name)
	}
	for _, api := range apis {
		if len(w.Operations[api]) == 0 {
			return nil, fmt.Errorf("api %s has no operations", api)
		}
		slices.Sort(w.Operations[api])
	}
	b, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode inventory: %w", err)
	}
	return append(b, '\n'), nil
}
