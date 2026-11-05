package harness

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

const testKind Kind = "volume"

var (
	refA = Ref{Kind: testKind, ID: "vol-a"}
	refB = Ref{Kind: testKind, ID: "vol-b"}
)

func present(owner Owner) Observation {
	return Observation{Ref: refA, Status: StatusPresent, Matches: 1, Owner: owner}
}

func absent() Observation { return Observation{Ref: refA, Status: StatusAbsent} }

func TestDecideTable(t *testing.T) {
	created := Record{Ref: refA, State: StateCreated}
	deleted := Record{Ref: refA, State: StateDeleted}
	never := Record{Ref: refA, State: StateNone}
	tests := []struct {
		row    int
		name   string
		intent Intent
		minted bool
		rec    Record
		obs    Observation
		want   Verdict
		reason string
	}{
		{
			1, "lost create", IntentCleanup, true,
			Record{Ref: refA, State: StateLost},
			present(OwnerThisRun),
			VerdictPreserve, "create outcome unknown",
		},
		{
			1, "pending create", IntentCleanup, true,
			Record{Ref: refA, State: StatePending},
			absent(),
			VerdictPreserve, "create outcome unknown",
		},
		{
			2, "other id observed", IntentExercise, true, created,
			Observation{Ref: refB, Status: StatusPresent, Matches: 1, Owner: OwnerThisRun},
			VerdictPreserve, "target mismatch",
		},
		{
			2, "other kind observed", IntentExercise, true, created,
			Observation{Ref: Ref{Kind: "bucket", ID: "vol-a"}, Status: StatusAbsent},
			VerdictPreserve, "target mismatch",
		},
		{
			2, "empty id", IntentExercise, true,
			Record{Ref: Ref{Kind: testKind}, State: StateCreated},
			Observation{Ref: Ref{Kind: testKind}, Status: StatusAbsent},
			VerdictPreserve, "target mismatch",
		},
		{
			3, "unknown status", IntentCleanup, true, created,
			Observation{Ref: refA, Err: errors.New("page 2: throttled")},
			VerdictPreserve, "page 2: throttled",
		},
		{
			3, "out of range status", IntentCleanup, true, created,
			Observation{Ref: refA, Status: 9, Matches: 1},
			VerdictPreserve, "invalid decision input",
		},
		{
			3, "out of range owner", IntentCleanup, true, created,
			Observation{Ref: refA, Status: StatusPresent, Matches: 1, Owner: 9},
			VerdictPreserve, "invalid",
		},
		{
			4, "two matches", IntentCleanup, true, created,
			Observation{Ref: refA, Status: StatusPresent, Matches: 2, Owner: OwnerThisRun},
			VerdictPreserve, "ambiguous",
		},
		{
			5, "absent with a match", IntentCleanup, true, created,
			Observation{Ref: refA, Status: StatusAbsent, Matches: 1},
			VerdictPreserve, "inconsistent",
		},
		{
			6, "foreign tag beats the ledger", IntentCleanup, true, created, present(OwnerForeign),
			VerdictPreserve, "foreign owner",
		},
		{7, "run tag, empty ledger", IntentExercise, false, never, present(OwnerThisRun), VerdictProceed, ""},
		{8, "untagged id this case created", IntentCleanup, true, created, present(OwnerNone), VerdictProceed, ""},
		{
			8, "untagged id this case deleted", IntentExercise, true, deleted, present(OwnerNone),
			VerdictProceed, "",
		},
		{
			9, "untagged reusable name", IntentCleanup, false, created, present(OwnerNone),
			VerdictPreserve, "no ownership proof",
		},
		{
			9, "untagged, empty ledger", IntentCleanup, true, never, present(OwnerNone),
			VerdictPreserve, "no ownership proof",
		},
		{10, "cleanup of a deleted id", IntentCleanup, true, deleted, absent(), VerdictAlreadyGone, ""},
		{10, "cleanup of a vanished id", IntentCleanup, false, created, absent(), VerdictAlreadyGone, ""},
		{11, "negative delete of our deleted id", IntentExercise, true, deleted, absent(), VerdictProceed, ""},
		{12, "absent id never ours", IntentExercise, true, never, absent(), VerdictPreserve, "never ours"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("row %d %s", tt.row, tt.name), func(t *testing.T) {
			got := Decide(tt.intent, tt.minted, tt.rec, tt.obs)
			if got.Verdict != tt.want {
				t.Fatalf("verdict %d (%s), want %d", got.Verdict, got.Reason, tt.want)
			}
			if !strings.Contains(got.Reason, tt.reason) {
				t.Fatalf("reason %q, want it to contain %q", got.Reason, tt.reason)
			}
		})
	}
}

func TestDecideCrossProduct(t *testing.T) {
	intents := []Intent{IntentExercise, IntentCleanup, 7}
	states := []State{StateNone, StatePending, StateCreated, StateDeleted, StateLost, 9}
	statuses := []Status{StatusUnknown, StatusPresent, StatusAbsent, 9}
	owners := []Owner{OwnerNone, OwnerThisRun, OwnerForeign, 9}
	seen := map[Verdict]int{}
	for _, intent := range intents {
		for _, minted := range []bool{false, true} {
			for _, state := range states {
				for _, status := range statuses {
					for _, matches := range []int{0, 1, 2} {
						for _, owner := range owners {
							for _, refMatch := range []bool{false, true} {
								obs := Observation{Ref: refB, Status: status, Matches: matches, Owner: owner}
								if refMatch {
									obs.Ref = refA
								}
								got := Decide(intent, minted, Record{Ref: refA, State: state}, obs)
								want := allowed(intent, minted, state, obs)
								if got.Verdict != want {
									t.Errorf("intent %d minted %v state %d status %d matches %d owner %d "+
										"refMatch %v: verdict %d (%s), want %d",
										intent, minted, state, status, matches, owner, refMatch,
										got.Verdict, got.Reason, want)
								}
								if got.Reason == "" {
									t.Errorf("empty reason for %+v", obs)
								}
								seen[got.Verdict]++
							}
						}
					}
				}
			}
		}
	}
	if seen[VerdictProceed] == 0 || seen[VerdictAlreadyGone] == 0 || seen[VerdictPreserve] == 0 {
		t.Fatalf("verdicts seen %v", seen)
	}
}

// allowed lists, positively, the only inputs rows 7, 8, 10 and 11 let through.
func allowed(intent Intent, minted bool, state State, obs Observation) Verdict {
	valid := intent <= IntentCleanup && obs.Ref == refA
	ours := state == StateCreated || state == StateDeleted
	switch {
	case !valid:
		return VerdictPreserve
	case obs.Status == StatusPresent && obs.Matches == 1 && obs.Owner == OwnerThisRun &&
		state != StateLost && state != StatePending && state <= StateLost:
		return VerdictProceed
	case obs.Status == StatusPresent && obs.Matches == 1 && obs.Owner == OwnerNone && ours && minted:
		return VerdictProceed
	case obs.Status == StatusAbsent && obs.Matches == 0 && obs.Owner == OwnerNone && ours:
		if intent == IntentCleanup {
			return VerdictAlreadyGone
		}
		return VerdictProceed
	default:
		return VerdictPreserve
	}
}

func TestPreserveErrorNamesTheResource(t *testing.T) {
	err := error(&PreserveError{Ref: refA, Reason: "foreign owner"})
	if err.Error() != "preserve volume vol-a: foreign owner" {
		t.Fatal(err)
	}
	var pe *PreserveError
	if !errors.As(fmt.Errorf("delete: %w", err), &pe) || pe.Ref != refA {
		t.Fatalf("%v", err)
	}
}
