package main

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/popododo0720/vws-conformance/internal/awsmodel"
)

// finding is one changed operation, ready to become an issue. Marker is a stable HTML comment in
// the body: the same service, operation and fact lines always give the same marker, so a run
// never opens a second issue for a change an issue already records.
type finding struct {
	Title  string `json:"title"`
	Body   string `json:"body"`
	Marker string `json:"marker"`
}

// release names the two botocore versions a run compares.
type release struct {
	Pinned string
	Newest string
}

func marker(service string, ch awsmodel.Change) string {
	var key strings.Builder
	fmt.Fprintf(&key, "%s\n%s\n", service, ch.Operation)
	for _, line := range ch.Added {
		fmt.Fprintf(&key, "+%s\n", line)
	}
	for _, line := range ch.Removed {
		fmt.Fprintf(&key, "-%s\n", line)
	}
	sum := sha256.Sum256([]byte(key.String()))
	return fmt.Sprintf("<!-- awsmodel-diff %s %s %x -->", service, ch.Operation, sum[:8])
}

func newFinding(service string, rel release, ch awsmodel.Change) finding {
	m := marker(service, ch)
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", m)
	fmt.Fprintf(&b, "The botocore `%s` model changed operation `%s` between the pinned release %s "+
		"and %s. VWS implements this operation (it has an implemented case in the catalog).\n\n",
		service, ch.Operation, rel.Pinned, rel.Newest)
	b.WriteString("```diff\n")
	for _, line := range ch.Added {
		fmt.Fprintf(&b, "+ %s\n", line)
	}
	for _, line := range ch.Removed {
		fmt.Fprintf(&b, "- %s\n", line)
	}
	b.WriteString("```\n\n")
	fmt.Fprintf(&b, "Triage: decide whether VWS must follow this change and file the parity work in the "+
		"repo that serves `%s`. Moving the pin past %s is a pull request that updates "+
		"cmd/awsmodel-diff/pin.json (vws-conformance#121).\n", service, rel.Newest)
	return finding{
		Title:  fmt.Sprintf("parity(awsmodel): %s %s changed in botocore %s", service, ch.Operation, rel.Newest),
		Body:   b.String(),
		Marker: m,
	}
}
