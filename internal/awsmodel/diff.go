package awsmodel

import (
	"fmt"
	"slices"
	"strconv"
)

// UnknownOperationError is an operation that the baseline model does not define. The baseline is
// the pin, so an implemented operation missing from it means the operation list or the pin is wrong.
type UnknownOperationError struct {
	Operation string
}

func (e *UnknownOperationError) Error() string {
	return fmt.Sprintf("api model: baseline has no operation %q", e.Operation)
}

// Change is one operation whose facts differ between two model versions. Added and Removed are
// sorted fact lines such as "input.Filters[].Name: string", "input.Name required",
// "input.State enum stopped" or "error NoSuchEntityException code=NoSuchEntity http=404".
type Change struct {
	Operation string
	Added     []string
	Removed   []string
}

// Diff compares ops between the baseline and the current model and returns one Change per
// operation that differs, in the order of ops. An operation the current model dropped is a Change
// that removes all of its facts.
func Diff(baseline, current *Model, ops []string) ([]Change, error) {
	var changes []Change
	for _, op := range ops {
		if _, ok := baseline.operations[op]; !ok {
			return nil, &UnknownOperationError{Operation: op}
		}
		added, removed := subtract(current.facts(op), baseline.facts(op))
		if len(added) > 0 || len(removed) > 0 {
			changes = append(changes, Change{Operation: op, Added: added, Removed: removed})
		}
	}
	return changes, nil
}

// subtract returns the lines only in a and the lines only in b; both inputs are sorted.
func subtract(a, b []string) (onlyA, onlyB []string) {
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case j == len(b) || (i < len(a) && a[i] < b[j]):
			onlyA = append(onlyA, a[i])
			i++
		case i == len(a) || b[j] < a[i]:
			onlyB = append(onlyB, b[j])
			j++
		default:
			i++
			j++
		}
	}
	return onlyA, onlyB
}

// facts flattens one operation into sorted lines, so two versions differ exactly where their lines
// differ. It returns nil for an operation the model does not define.
func (m *Model) facts(op string) []string {
	o, ok := m.operations[op]
	if !ok {
		return nil
	}
	w := walker{m: m, out: []string{"operation " + op}, open: map[string]bool{}}
	if o.Input != nil {
		w.walk("input", o.Input.Shape)
	}
	if o.Output != nil {
		w.walk("output", o.Output.Shape)
	}
	for _, e := range o.Errors {
		line := "error " + e.Shape
		if t := m.shapes[e.Shape].Error; t != nil {
			if t.Code != "" {
				line += " code=" + t.Code
			}
			if t.Status != 0 {
				line += " http=" + strconv.Itoa(t.Status)
			}
		}
		w.out = append(w.out, line)
	}
	slices.Sort(w.out)
	return slices.Compact(w.out)
}

type walker struct {
	m    *Model
	open map[string]bool
	out  []string
}

// walk records the shape at path and descends into it. A shape already open on the current path
// is recorded as recursive instead of being walked again, so recursive models terminate.
func (w *walker) walk(path, name string) {
	if w.open[name] {
		w.out = append(w.out, path+": recursive "+name)
		return
	}
	s := w.m.shapes[name]
	w.out = append(w.out, path+": "+s.Type)
	for _, v := range s.Enum {
		w.out = append(w.out, path+" enum "+v)
	}
	w.open[name] = true
	defer delete(w.open, name)
	switch s.Type {
	case "structure":
		for member, r := range s.Members {
			if slices.Contains(s.Required, member) {
				w.out = append(w.out, path+"."+member+" required")
			}
			w.walk(path+"."+member, r.Shape)
		}
	case "list":
		w.walk(path+"[]", s.Member.Shape)
	case "map":
		w.walk(path+"{key}", s.Key.Shape)
		w.walk(path+"{value}", s.Value.Shape)
	}
}
