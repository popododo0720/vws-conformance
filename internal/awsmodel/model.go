// Package awsmodel reads AWS botocore API models (data/<service>/<version>/service-2.json) and
// reports, per operation, what changed between two versions of a model: input and output members,
// required flags, enum values and error shapes. Documentation and shape names are ignored, so a
// rename or a doc edit is not a change.
package awsmodel

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrNoOperations means the file parsed as JSON but defines no operation, so it is not an API model.
var ErrNoOperations = errors.New("api model has no operations")

// RefError is a reference to a shape the model does not define. Shape is empty when a list or map
// shape lacks its member, key or value reference.
type RefError struct {
	From  string
	Shape string
}

func (e *RefError) Error() string {
	return fmt.Sprintf("api model: %s refers to unknown shape %q", e.From, e.Shape)
}

type ref struct {
	Shape string `json:"shape"`
}

type operation struct {
	Input  *ref  `json:"input"`
	Output *ref  `json:"output"`
	Errors []ref `json:"errors"`
}

type errorTrait struct {
	Code   string `json:"code"`
	Status int    `json:"httpStatusCode"`
}

type shape struct {
	Members  map[string]ref `json:"members"`
	Member   *ref           `json:"member"`
	Key      *ref           `json:"key"`
	Value    *ref           `json:"value"`
	Error    *errorTrait    `json:"error"`
	Type     string         `json:"type"`
	Required []string       `json:"required"`
	Enum     []string       `json:"enum"`
}

// Model is a parsed API model in which every shape reference resolves.
type Model struct {
	operations map[string]operation
	shapes     map[string]shape
}

// Parse reads a botocore service-2.json file and checks that every reference resolves, so the
// walk over an operation never meets a missing shape.
func Parse(data []byte) (*Model, error) {
	var raw struct {
		Operations map[string]operation `json:"operations"`
		Shapes     map[string]shape     `json:"shapes"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse api model: %w", err)
	}
	if len(raw.Operations) == 0 {
		return nil, ErrNoOperations
	}
	m := &Model{operations: raw.Operations, shapes: raw.Shapes}
	if err := m.resolve(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Model) resolve() error {
	for name, op := range m.operations {
		if op.Input != nil {
			if err := m.need("operation "+name+" input", op.Input); err != nil {
				return err
			}
		}
		if op.Output != nil {
			if err := m.need("operation "+name+" output", op.Output); err != nil {
				return err
			}
		}
		for i := range op.Errors {
			if err := m.need("operation "+name+" error", &op.Errors[i]); err != nil {
				return err
			}
		}
	}
	for name, s := range m.shapes {
		var refs map[string]*ref
		switch s.Type {
		case "list":
			refs = map[string]*ref{"member": s.Member}
		case "map":
			refs = map[string]*ref{"key": s.Key, "value": s.Value}
		case "structure":
			refs = make(map[string]*ref, len(s.Members))
			for member, r := range s.Members {
				refs["member "+member] = &r
			}
		}
		for role, r := range refs {
			if err := m.need("shape "+name+" "+role, r); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Model) need(from string, r *ref) error {
	if r == nil {
		return &RefError{From: from}
	}
	if _, ok := m.shapes[r.Shape]; !ok {
		return &RefError{From: from, Shape: r.Shape}
	}
	return nil
}
