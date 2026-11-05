package harness

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/smithy-go"
)

const (
	testRun     = "conf-20261105T000000Z-abc123"
	testAbsence = "InvalidVolume.NotFound"
)

type step struct {
	items []item
	next  string
	err   error
}

// script answers one step per call and records the token each call received.
type script struct {
	steps  []step
	tokens []string
}

func (s *script) fetch(ctx context.Context, token *string) (page, error) {
	if _, ok := ctx.Deadline(); !ok {
		return page{}, errors.New("fetch without a deadline")
	}
	n := len(s.tokens)
	if token == nil {
		s.tokens = append(s.tokens, "")
	} else {
		s.tokens = append(s.tokens, *token)
	}
	if n >= len(s.steps) {
		return page{}, fmt.Errorf("unexpected call %d", n+1)
	}
	st := s.steps[n]
	if st.err != nil {
		return page{}, st.err
	}
	p := page{items: st.items}
	if st.next != "" {
		p.next = &st.next
	}
	return p, nil
}

func apiErr(code, msg string) error {
	return fmt.Errorf("describe volumes: %w", &smithy.GenericAPIError{Code: code, Message: msg})
}

func tagged(id string, tags ...string) item { return item{ID: id, RunTags: tags} }

func TestObserve(t *testing.T) {
	tests := []struct {
		name    string
		steps   []step
		status  Status
		matches int
		owner   Owner
		errText string
	}{
		{"absence code on page 1", []step{{err: apiErr(testAbsence, "gone")}}, StatusAbsent, 0, OwnerNone, ""},
		{
			"NotFound in the message under another code",
			[]step{{err: apiErr("InvalidParameterValue", "InvalidVolume.NotFound: vol-a")}},
			StatusUnknown, 0, OwnerNone, "InvalidParameterValue",
		},
		{
			"absence code on page 2 after a match",
			[]step{{items: []item{tagged("vol-a", testRun)}, next: "t1"}, {err: apiErr(testAbsence, "gone")}},
			StatusUnknown, 1, OwnerNone, "page 2",
		},
		{
			"absence code on page 2 after an empty page",
			[]step{{next: "t1"}, {err: apiErr(testAbsence, "gone")}},
			StatusUnknown, 0, OwnerNone, "page 2",
		},
		{"other API error", []step{{err: apiErr("InvalidAction", "no")}}, StatusUnknown, 0, OwnerNone, "InvalidAction"},
		{
			"transport error",
			[]step{{err: errors.New("connection reset by peer")}},
			StatusUnknown, 0, OwnerNone, "connection reset",
		},
		{"empty answer", []step{{}}, StatusUnknown, 0, OwnerNone, "empty answer is not absence"},
		{
			"id mismatch",
			[]step{{items: []item{tagged("vol-a", testRun), tagged("vol-b", testRun)}}},
			StatusUnknown, 1, OwnerNone, "id mismatch",
		},
		{
			"pagination loop",
			[]step{{next: "t1"}, {next: "t2"}, {next: "t1"}},
			StatusUnknown, 0, OwnerNone, "pagination loop",
		},
		{
			"match then empty last page",
			[]step{{items: []item{tagged("vol-a", testRun)}, next: "t1"}, {}},
			StatusPresent, 1, OwnerThisRun, "",
		},
		{
			"two matches",
			[]step{{items: []item{tagged("vol-a"), tagged("vol-a")}}},
			StatusPresent, 2, OwnerNone, "",
		},
		{"untagged", []step{{items: []item{tagged("vol-a")}}}, StatusPresent, 1, OwnerNone, ""},
		{
			"foreign run tag",
			[]step{{items: []item{tagged("vol-a", "conf-20261104T000000Z-def456")}}},
			StatusPresent, 1, OwnerForeign, "",
		},
		{"malformed run tag", []step{{items: []item{tagged("vol-a", "prod")}}}, StatusPresent, 1, OwnerForeign, ""},
		{"empty run tag", []step{{items: []item{tagged("vol-a", "")}}}, StatusPresent, 1, OwnerForeign, ""},
		{
			"two run tags",
			[]step{{items: []item{tagged("vol-a", testRun, testRun)}}},
			StatusPresent, 1, OwnerForeign, "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &script{steps: tt.steps}
			obs := observe(context.Background(), testRun, refA, testAbsence, s.fetch)
			if obs.Ref != refA || obs.Status != tt.status || obs.Matches != tt.matches || obs.Owner != tt.owner {
				t.Fatalf("got %+v", obs)
			}
			if (obs.Status == StatusUnknown) != (obs.Err != nil) {
				t.Fatalf("status %d with err %v", obs.Status, obs.Err)
			}
			if tt.errText != "" && !strings.Contains(obs.Err.Error(), tt.errText) {
				t.Fatalf("err %v, want %q", obs.Err, tt.errText)
			}
			if len(s.tokens) != len(tt.steps) {
				t.Fatalf("calls %d, want %d", len(s.tokens), len(tt.steps))
			}
		})
	}
}

func TestObserveFollowsTokens(t *testing.T) {
	s := &script{steps: []step{{next: "t1"}, {next: "t2"}, {items: []item{tagged("vol-a")}}}}
	obs := observe(context.Background(), testRun, refA, testAbsence, s.fetch)
	if obs.Status != StatusPresent || strings.Join(s.tokens, ",") != ",t1,t2" {
		t.Fatalf("%+v tokens %q", obs, s.tokens)
	}
}

func TestObserveStopsAtPageBound(t *testing.T) {
	calls := 0
	fetch := func(context.Context, *string) (page, error) {
		calls++
		next := fmt.Sprintf("t%d", calls)
		return page{next: &next}, nil
	}
	obs := observe(context.Background(), testRun, refA, testAbsence, fetch)
	if obs.Status != StatusUnknown || calls != maxPages {
		t.Fatalf("%+v after %d calls", obs, calls)
	}
}

func TestObserveTimeoutIsUnknown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	fetch := func(ctx context.Context, _ *string) (page, error) {
		<-ctx.Done()
		return page{}, fmt.Errorf("describe volumes: %w", ctx.Err())
	}
	obs := observe(ctx, testRun, refA, testAbsence, fetch)
	if obs.Status != StatusUnknown || !errors.Is(obs.Err, context.DeadlineExceeded) {
		t.Fatalf("%+v", obs)
	}
}

func TestObserveRefusesBadInput(t *testing.T) {
	for _, tt := range []struct {
		name    string
		runID   string
		ref     Ref
		absence string
	}{
		{"invalid run id", "prod", refA, testAbsence},
		{"empty id", testRun, Ref{Kind: testKind}, testAbsence},
		{"no absence code", testRun, refA, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := &script{steps: []step{{err: apiErr(testAbsence, "gone")}}}
			obs := observe(context.Background(), tt.runID, tt.ref, tt.absence, s.fetch)
			if obs.Status != StatusUnknown || obs.Err == nil || len(s.tokens) != 0 {
				t.Fatalf("%+v after %d calls", obs, len(s.tokens))
			}
		})
	}
}
