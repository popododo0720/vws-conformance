package harness

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/smithy-go"
)

// RunTagKey is the tag every gated create stamps with the run id.
const RunTagKey = "vws-conformance:run"

// maxPages bounds a walk whose server keeps minting fresh continuation tokens.
const maxPages = 100

type item struct {
	Gone    bool
	ID      string
	RunTags []string // values of every tag whose key is RunTagKey
}

type page struct {
	items []item
	next  *string
}

// observe walks every page of an exact-id query. Absence is only the kind's documented code on
// the first page; anything short of a complete, consistent answer is StatusUnknown.
func observe(ctx context.Context, runID string, ref Ref, absence string,
	fetch func(context.Context, *string) (page, error),
) Observation {
	obs := Observation{Ref: ref}
	fail := func(err error) Observation {
		obs.Status = StatusUnknown
		obs.Err = fmt.Errorf("observe %s %s: %w", ref.Kind, ref.ID, err)
		return obs
	}
	if err := ValidateRunID(runID); err != nil {
		return fail(err)
	}
	if ref.Kind == "" || ref.ID == "" || absence == "" {
		return fail(errors.New("kind, id and absence code are required"))
	}
	var token *string
	seen := map[string]bool{}
	var tags []string
	var gone int
	for n := 1; ; n++ {
		if n > maxPages {
			return fail(fmt.Errorf("more than %d pages", maxPages))
		}
		p, err := withDeadline(ctx, func(ctx context.Context) (page, error) { return fetch(ctx, token) })
		if err != nil {
			var ae smithy.APIError
			if n == 1 && errors.As(err, &ae) && ae.ErrorCode() == absence {
				obs.Status = StatusAbsent
				return obs
			}
			return fail(fmt.Errorf("page %d: %w", n, err))
		}
		for _, it := range p.items {
			if it.ID != ref.ID {
				return fail(fmt.Errorf("page %d: id mismatch: got %q", n, it.ID))
			}
			obs.Matches++
			if it.Gone {
				gone++
			}
			tags = append(tags, it.RunTags...)
		}
		if p.next == nil || *p.next == "" {
			break
		}
		if seen[*p.next] {
			return fail(fmt.Errorf("page %d: pagination loop on token %q", n, *p.next))
		}
		seen[*p.next] = true
		token = p.next
	}
	if obs.Matches == 0 {
		return fail(errors.New("empty answer is not absence"))
	}
	if gone != 0 {
		if gone != 1 || obs.Matches != 1 {
			return fail(errors.New("conflicting terminated observations"))
		}
		// A terminated instance is gone: report the same shape as a NotFound answer.
		obs.Status = StatusAbsent
		obs.Matches = 0
		return obs
	}
	obs.Status = StatusPresent
	obs.Owner = owner(runID, tags)
	return obs
}

func owner(runID string, tags []string) Owner {
	switch {
	case len(tags) == 0:
		return OwnerNone
	case len(tags) == 1 && tags[0] == runID:
		return OwnerThisRun
	default:
		return OwnerForeign
	}
}
