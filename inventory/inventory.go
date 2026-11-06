// Package inventory lists the operations vws-console-api dispatches at a pinned commit and audits
// the conformance cases against that list.
package inventory

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// API is a console-api surface. The case Service field differs: vpc and ebs are EC2 Query, auth is
// any of them.
type API string

const (
	EC2 API = "ec2"
	IAM API = "iam"
	S3  API = "s3"
	STS API = "sts"
)

// apis is every API, sorted.
var apis = []API{EC2, IAM, S3, STS}

// Op is one operation. It prints in the AWS IAM action notation, "ec2:RunInstances", which keeps
// the same name on two APIs apart.
type Op struct {
	API  API
	Name string
}

func (o Op) String() string { return string(o.API) + ":" + o.Name }

var opNameRE = regexp.MustCompile(`^[A-Za-z0-9]+$`)

// ParseOp reads "api:Name".
func ParseOp(s string) (Op, error) {
	api, name, ok := strings.Cut(s, ":")
	if !ok || !slices.Contains(apis, API(api)) || !opNameRE.MatchString(name) {
		return Op{}, fmt.Errorf("operation %q: want api:Name with api one of %v", s, apis)
	}
	return Op{API(api), name}, nil
}

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

// wire is the file form. Encode and Decode are its only users.
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

// Decode is Encode's inverse. It rejects unknown fields, an unknown or empty API, an unsorted or
// repeated operation and a malformed commit, so a hand edit cannot drift from Encode's output.
func Decode(b []byte) (Inventory, error) {
	var w wire
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&w); err != nil {
		return Inventory{}, fmt.Errorf("decode inventory: %w", err)
	}
	if dec.More() {
		return Inventory{}, errors.New("decode inventory: trailing data")
	}
	switch {
	case w.SchemaVersion != schemaVersion:
		return Inventory{}, fmt.Errorf("decode inventory: schema_version %q, want %q", w.SchemaVersion, schemaVersion)
	case w.ConsoleAPI.Repo != consoleRepo:
		return Inventory{}, fmt.Errorf("decode inventory: console_api.repo %q, want %q", w.ConsoleAPI.Repo, consoleRepo)
	case !commitRE.MatchString(w.ConsoleAPI.Commit):
		return Inventory{}, fmt.Errorf("decode inventory: console_api.commit %q is not 40 lowercase hex digits",
			w.ConsoleAPI.Commit)
	}
	inv := Inventory{Commit: w.ConsoleAPI.Commit, Ops: map[Op]bool{}}
	for api, names := range w.Operations {
		if !slices.Contains(apis, api) {
			return Inventory{}, fmt.Errorf("decode inventory: unknown api %q", api)
		}
		for i, name := range names {
			if !opNameRE.MatchString(name) {
				return Inventory{}, fmt.Errorf("decode inventory: invalid %s operation %q", api, name)
			}
			if i > 0 && names[i-1] >= name {
				return Inventory{}, fmt.Errorf("decode inventory: %s operations are not sorted and unique at %q", api, name)
			}
			inv.Ops[Op{api, name}] = true
		}
	}
	for _, api := range apis {
		if len(w.Operations[api]) == 0 {
			return Inventory{}, fmt.Errorf("decode inventory: api %s has no operations", api)
		}
	}
	return inv, nil
}
