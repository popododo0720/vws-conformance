package bind

import (
	"context"

	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

type Group struct {
	Service string
	Name    string
	Cases   []cases.Case
	Run     func(context.Context, *harness.Runtime, cases.Case) (cases.Outcome, error)
}
