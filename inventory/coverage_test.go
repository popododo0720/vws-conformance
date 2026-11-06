package inventory_test

import (
	"testing"

	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/engine"
	"github.com/popododo0720/vws-conformance/inventory"
)

// TestCasesCoverInventory is the gate: every operation console-api dispatches at the pin has a
// case or a waiver, and every case targets a dispatched operation.
func TestCasesCoverInventory(t *testing.T) {
	inv, waivers, err := inventory.Load()
	if err != nil {
		t.Fatal(err)
	}
	var cs []cases.Case
	for _, g := range engine.Groups() {
		cs = append(cs, g.Cases...)
	}
	for _, f := range inventory.Audit(inv, waivers, cs) {
		t.Error(f)
	}
}
