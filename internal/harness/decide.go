package harness

import "fmt"

type Kind string

type Ref struct {
	Kind Kind
	ID   string
}

type Status uint8

const (
	StatusUnknown Status = iota
	StatusPresent
	StatusAbsent
)

// Owner is the evidence read from the matched item's run tags. OwnerForeign covers a run tag
// with another run id, a malformed value, and more than one run tag.
type Owner uint8

const (
	OwnerNone Owner = iota
	OwnerThisRun
	OwnerForeign
)

type Observation struct {
	Ref     Ref
	Status  Status
	Matches int
	Owner   Owner
	Err     error
}

// State is what this case did to a Ref. It is never a cached presence fact, so every
// mutation observes the server again.
type State uint8

const (
	StateNone State = iota
	StatePending
	StateCreated
	StateDeleted
	StateLost
)

type Record struct {
	Ref   Ref
	State State
}

type Intent uint8

const (
	IntentExercise Intent = iota
	IntentCleanup
)

type Verdict uint8

const (
	VerdictPreserve Verdict = iota
	VerdictProceed
	VerdictAlreadyGone
)

type Decision struct {
	Verdict Verdict
	Reason  string
}

type PreserveError struct {
	Ref    Ref
	Reason string
}

func (e *PreserveError) Error() string {
	id := e.Ref.ID
	if id == "" {
		id = "(no id)"
	}
	return fmt.Sprintf("preserve %s %s: %s", e.Ref.Kind, id, e.Reason)
}

// Decide is the only ownership policy. minted reports that the kind's ids are minted by the
// server and never reused, so this case's ledger alone proves an id is ours.
func Decide(intent Intent, minted bool, rec Record, obs Observation) Decision {
	present := obs.Status == StatusPresent
	ours := rec.State == StateCreated || rec.State == StateDeleted
	switch {
	case rec.State == StateLost || rec.State == StatePending:
		return preserve("create outcome unknown")
	case obs.Ref != rec.Ref || rec.Ref.Kind == "" || rec.Ref.ID == "":
		return preserve("observation target mismatch")
	case intent > IntentCleanup || rec.State > StateLost || obs.Status > StatusAbsent || obs.Owner > OwnerForeign:
		return preserve("invalid decision input")
	case obs.Status == StatusUnknown:
		if obs.Err == nil {
			return preserve("observation unknown")
		}
		return preserve(obs.Err.Error())
	case present && obs.Matches != 1:
		return preserve(fmt.Sprintf("ambiguous: %d exact-id matches", obs.Matches))
	case !present && (obs.Matches != 0 || obs.Owner != OwnerNone):
		return preserve("inconsistent: absent with matches or owner evidence")
	case present && obs.Owner == OwnerForeign:
		return preserve("foreign owner")
	case present && obs.Owner == OwnerThisRun:
		return Decision{Verdict: VerdictProceed, Reason: "run tag matches this run"}
	case present && ours && minted:
		return Decision{Verdict: VerdictProceed, Reason: "created by this case"}
	case present:
		return preserve("no ownership proof")
	case ours && intent == IntentCleanup:
		return Decision{Verdict: VerdictAlreadyGone, Reason: "absent"}
	case ours:
		return Decision{Verdict: VerdictProceed, Reason: "absent id this case created"}
	default:
		return preserve("never ours")
	}
}

func preserve(reason string) Decision {
	return Decision{Verdict: VerdictPreserve, Reason: reason}
}
