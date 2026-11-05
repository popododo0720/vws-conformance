package harness

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"

	"github.com/popododo0720/vws-conformance/internal/cases"
)

// This file is the only place allowed to select a mutating method on an SDK client
// (census_test.go). Every such call goes through create or mutate, and cleanup through remove.

const KindVolume Kind = "volume"

type kindSpec struct {
	absence string // the error code AWS documents for a missing resource, cited in doc
	doc     string
	minted  bool
	fetch   func(ctx context.Context, cl *Clients, id string, token *string) (page, error)
	remove  func(ctx context.Context, cl *Clients, id string) error
}

var kinds = map[Kind]kindSpec{
	KindVolume: {
		absence: "InvalidVolume.NotFound",
		doc:     cases.DocEC2Errors,
		minted:  true,
		fetch:   fetchVolumes,
		remove: func(ctx context.Context, cl *Clients, id string) error {
			_, err := deleteVolume(ctx, cl, id)
			return err
		},
	},
}

// EC2CreateVolume creates a volume tagged with this run and registers its deletion before the
// request is sent. The caller's input is not modified.
func (rt *Runtime) EC2CreateVolume(ctx context.Context, in *ec2.CreateVolumeInput) (*ec2.CreateVolumeOutput, error) {
	if in == nil {
		return nil, errors.New("CreateVolume: input is nil")
	}
	if rt.Clients == nil || rt.Clients.EC2 == nil {
		return nil, errors.New("CreateVolume: ec2 client is unset")
	}
	specs, err := withRunTag(in.TagSpecifications, types.ResourceTypeVolume, rt.RunID())
	if err != nil {
		return nil, fmt.Errorf("CreateVolume: %w", err)
	}
	tagged := *in
	tagged.TagSpecifications = specs
	return create(ctx, rt, KindVolume, func(ctx context.Context) (*ec2.CreateVolumeOutput, error) {
		// One attempt: a retry after a lost response can answer with an API error for a volume the
		// first attempt made, and create records an API error as "nothing created".
		out, err := rt.Clients.EC2.CreateVolume(ctx, &tagged, func(o *ec2.Options) { o.RetryMaxAttempts = 1 })
		return out, wrapErr("CreateVolume", err)
	}, func(out *ec2.CreateVolumeOutput) string {
		if out == nil {
			return ""
		}
		return aws.ToString(out.VolumeId)
	})
}

// EC2DeleteVolume deletes a volume only when a fresh exact-id observation proves this run owns it.
func (rt *Runtime) EC2DeleteVolume(ctx context.Context, id string) (*ec2.DeleteVolumeOutput, error) {
	return mutate(ctx, rt, Ref{Kind: KindVolume, ID: id}, true,
		func(ctx context.Context) (*ec2.DeleteVolumeOutput, error) { return deleteVolume(ctx, rt.Clients, id) })
}

func fetchVolumes(ctx context.Context, cl *Clients, id string, token *string) (page, error) {
	if cl == nil || cl.EC2 == nil {
		return page{}, errors.New("DescribeVolumes: ec2 client is unset")
	}
	out, err := cl.EC2.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{VolumeIds: []string{id}, NextToken: token})
	if err != nil {
		return page{}, fmt.Errorf("DescribeVolumes: %w", err)
	}
	if out == nil {
		return page{}, errors.New("DescribeVolumes: nil output")
	}
	p := page{next: out.NextToken}
	for _, v := range out.Volumes {
		p.items = append(p.items, item{ID: aws.ToString(v.VolumeId), RunTags: ec2RunTags(v.Tags)})
	}
	return p, nil
}

func deleteVolume(ctx context.Context, cl *Clients, id string) (*ec2.DeleteVolumeOutput, error) {
	if cl == nil || cl.EC2 == nil {
		return nil, errors.New("DeleteVolume: ec2 client is unset")
	}
	out, err := cl.EC2.DeleteVolume(ctx, &ec2.DeleteVolumeInput{VolumeId: aws.String(id)})
	return out, wrapErr("DeleteVolume", err)
}

func ec2RunTags(tags []types.Tag) []string {
	var values []string
	for _, t := range tags {
		if aws.ToString(t.Key) == RunTagKey {
			values = append(values, aws.ToString(t.Value))
		}
	}
	return values
}

// withRunTag returns a copy of specs whose spec for rtype carries the run tag. The run tag key
// is reserved, so a caller cannot forge another run's ownership.
func withRunTag(specs []types.TagSpecification, rtype types.ResourceType, runID string,
) ([]types.TagSpecification, error) {
	runTag := types.Tag{Key: aws.String(RunTagKey), Value: aws.String(runID)}
	out := make([]types.TagSpecification, 0, len(specs)+1)
	merged := false
	for _, s := range specs {
		for _, t := range s.Tags {
			if aws.ToString(t.Key) == RunTagKey {
				return nil, fmt.Errorf("tag %s is reserved for the cleanup gate", RunTagKey)
			}
		}
		if s.ResourceType == rtype && !merged {
			s.Tags = append(slices.Clone(s.Tags), runTag)
			merged = true
		}
		out = append(out, s)
	}
	if !merged {
		out = append(out, types.TagSpecification{ResourceType: rtype, Tags: []types.Tag{runTag}})
	}
	return out, nil
}

// create registers the record and its cleanup before sending. The send ignores cancellation so a
// signal cannot turn a create already on the wire into an unknown outcome; OpTimeout bounds it.
func create[Out any](ctx context.Context, rt *Runtime, k Kind, send func(context.Context) (Out, error),
	id func(Out) string,
) (Out, error) {
	var zero Out
	s, err := rt.admit()
	if err != nil {
		return zero, fmt.Errorf("create %s: %w", k, err)
	}
	defer s.mu.Unlock()
	rec := &Record{Ref: Ref{Kind: k}, State: StatePending}
	s.ledger = append(s.ledger, rec)
	s.cleanup.Add(func(ctx context.Context) error { return rt.remove(ctx, rec) })
	out, err := withDeadline(context.WithoutCancel(ctx), send)
	var ae smithy.APIError
	switch {
	case errors.As(err, &ae):
		rec.State = StateNone
		return out, fmt.Errorf("create %s: %w", k, err)
	case err != nil:
		rec.State = StateLost
		return out, fmt.Errorf("create %s: %w", k, err)
	}
	rec.Ref.ID = id(out)
	if rec.Ref.ID == "" {
		rec.State = StateLost
		return out, fmt.Errorf("create %s: response has no id", k)
	}
	rec.State = StateCreated
	return out, nil
}

// mutate sends a call that touches an existing resource only when Decide says Proceed.
func mutate[Out any](ctx context.Context, rt *Runtime, ref Ref, deletes bool,
	send func(context.Context) (Out, error),
) (Out, error) {
	var zero Out
	s, err := rt.admit()
	if err != nil {
		return zero, fmt.Errorf("mutate %s %s: %w", ref.Kind, ref.ID, err)
	}
	defer s.mu.Unlock()
	spec, ok := kinds[ref.Kind]
	if !ok {
		return zero, &PreserveError{Ref: ref, Reason: "kind has no observer"}
	}
	rec := Record{Ref: ref}
	if r := s.find(ref); r != nil {
		rec = *r
	}
	d := Decide(IntentExercise, spec.minted, rec, rt.observeRef(ctx, spec, ref))
	if d.Verdict != VerdictProceed {
		return zero, &PreserveError{Ref: ref, Reason: d.Reason}
	}
	out, err := withDeadline(ctx, send)
	if err != nil {
		return out, fmt.Errorf("mutate %s %s: %w", ref.Kind, ref.ID, err)
	}
	if r := s.find(ref); r != nil && deletes {
		r.State = StateDeleted
	}
	return out, nil
}

// remove is the cleanup callback of one record. It runs from close, which holds the scope lock.
func (rt *Runtime) remove(ctx context.Context, rec *Record) error {
	if rec.State == StateNone {
		return nil
	}
	spec, ok := kinds[rec.Ref.Kind]
	if !ok {
		return &PreserveError{Ref: rec.Ref, Reason: "kind has no observer"}
	}
	var obs Observation
	if rec.State == StateCreated || rec.State == StateDeleted {
		obs = rt.observeRef(ctx, spec, rec.Ref)
	}
	d := Decide(IntentCleanup, spec.minted, *rec, obs)
	switch d.Verdict {
	case VerdictAlreadyGone:
		rec.State = StateDeleted
		return nil
	case VerdictProceed:
		_, err := withDeadline(ctx, func(ctx context.Context) (struct{}, error) {
			return struct{}{}, spec.remove(ctx, rt.Clients, rec.Ref.ID)
		})
		if err != nil {
			return fmt.Errorf("cleanup %s %s: %w", rec.Ref.Kind, rec.Ref.ID, err)
		}
		rec.State = StateDeleted
		return nil
	default:
		return &PreserveError{Ref: rec.Ref, Reason: d.Reason}
	}
}

// observeRef hides the observation from rt.Call's capture so the outcome records only the mutation.
func (rt *Runtime) observeRef(ctx context.Context, spec kindSpec, ref Ref) Observation {
	return observe(withCapture(ctx, nil), rt.RunID(), ref, spec.absence,
		func(ctx context.Context, token *string) (page, error) {
			return spec.fetch(ctx, rt.Clients, ref.ID, token)
		})
}
