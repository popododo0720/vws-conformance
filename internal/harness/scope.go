package harness

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// caseScope owns what one case created. Every gated mutation holds mu from observation through
// the SDK call to the record update, and close holds it while cleaning, so cleanup never
// overlaps a call in flight and no mutation starts after close.
type caseScope struct {
	caseID  string
	mu      sync.Mutex
	closed  bool
	err     error
	cleanup Cleaner
	ledger  []*Record
}

type scopeRegistry struct {
	mu       sync.Mutex
	stopping bool
	live     map[*Runtime]struct{}
}

// BeginCase returns a view of rt that shares its config and clients and owns a fresh case scope.
func (rt *Runtime) BeginCase(caseID string) (*Runtime, error) {
	if rt.scopes == nil {
		return nil, fmt.Errorf("begin case %s: runtime has no scope registry", caseID)
	}
	view := &Runtime{
		Cfg: rt.Cfg, Clients: rt.Clients, secret: rt.secret, scopes: rt.scopes,
		scope: &caseScope{caseID: caseID},
	}
	r := rt.scopes
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopping {
		return nil, fmt.Errorf("begin case %s: shutting down", caseID)
	}
	r.live[view] = struct{}{}
	return view, nil
}

// EndCase runs the case's cleanup once. A later or overlapping call waits and gets the same error.
func (rt *Runtime) EndCase(ctx context.Context) error {
	s := rt.scope
	if s == nil {
		return errors.New("end case: runtime has no case scope")
	}
	err := s.close(context.WithoutCancel(ctx))
	rt.scopes.mu.Lock()
	delete(rt.scopes.live, rt)
	rt.scopes.mu.Unlock()
	return err
}

func (s *caseScope) close(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		if err := s.cleanup.Run(ctx); err != nil {
			s.err = fmt.Errorf("case %s: %w", s.caseID, err)
		}
	}
	return s.err
}

// Shutdown refuses new cases and mutations, then closes every live case scope.
func (rt *Runtime) Shutdown(ctx context.Context) error {
	r := rt.scopes
	if r == nil {
		return nil
	}
	r.mu.Lock()
	r.stopping = true
	views := make([]*Runtime, 0, len(r.live))
	for v := range r.live {
		views = append(views, v)
	}
	r.mu.Unlock()
	errs := make([]error, len(views))
	var wg sync.WaitGroup
	for i, v := range views {
		wg.Go(func() { errs[i] = v.EndCase(ctx) })
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}

// find returns this case's latest record of ref, or nil.
func (s *caseScope) find(ref Ref) *Record {
	for i := len(s.ledger) - 1; i >= 0; i-- {
		if s.ledger[i].Ref == ref {
			return s.ledger[i]
		}
	}
	return nil
}

// admit returns the case scope locked for one gated mutation; the caller unlocks it.
func (rt *Runtime) admit() (*caseScope, error) {
	s := rt.scope
	if s == nil {
		return nil, errors.New("mutation outside a case scope")
	}
	s.mu.Lock()
	rt.scopes.mu.Lock()
	stopping := rt.scopes.stopping
	rt.scopes.mu.Unlock()
	if s.closed || stopping {
		s.mu.Unlock()
		return nil, fmt.Errorf("case %s: mutation after the case scope closed", s.caseID)
	}
	return s, nil
}
