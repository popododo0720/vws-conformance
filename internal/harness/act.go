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
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/popododo0720/vws-conformance/internal/cases"
)

// This file is the only place allowed to select a mutating method on an SDK client
// (census_test.go). Every such call goes through create or mutate, and cleanup through remove.

const (
	KindVolume   Kind = "volume"
	KindKeyPair  Kind = "key-pair"
	KindInstance Kind = "instance"
)

const KindVPC Kind = "vpc"

type kindSpec struct {
	absence string // the error code AWS documents for a missing resource, cited in doc
	doc     string
	minted  bool
	fetch   func(ctx context.Context, cl *Clients, id string, token *string) (page, error)
	remove  func(ctx context.Context, cl *Clients, id string) error
}

var kinds = map[Kind]kindSpec{
	KindKeyPair: {
		absence: "InvalidKeyPair.NotFound", doc: cases.DocEC2Errors, minted: true,
		fetch: fetchKeyPairs,
		remove: func(ctx context.Context, cl *Clients, id string) error {
			_, err := deleteKeyPair(ctx, cl, id)
			return err
		},
	},
	KindInstance: {
		absence: "InvalidInstanceID.NotFound", doc: cases.DocEC2Errors, minted: true,
		fetch: fetchInstances,
		remove: func(ctx context.Context, cl *Clients, id string) error {
			_, err := terminateInstance(ctx, cl, id)
			return err
		},
	},
	KindVPC: {
		absence: "InvalidVpcID.NotFound", doc: cases.DocEC2Errors, minted: true,
		fetch: fetchVpcs,
		remove: func(ctx context.Context, cl *Clients, id string) error {
			_, err := deleteVpc(ctx, cl, id)
			return err
		},
	},
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
		out, err := rt.Clients.EC2.CreateVolume(ctx, &tagged, oneAttempt)
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
	out, err := cl.EC2.DeleteVolume(ctx, &ec2.DeleteVolumeInput{VolumeId: aws.String(id)}, oneAttempt)
	return out, wrapErr("DeleteVolume", err)
}

// oneAttempt turns off SDK retries on a gated send. A retry skips observe and Decide: it can
// delete a resource whose owner changed after the observation, or answer an API error for a
// volume the first attempt made. census_test.go requires it on every mutating call here.
func oneAttempt(o *ec2.Options) { o.RetryMaxAttempts = 1 }

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
	switch {
	case refused(err):
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

// refused reports a 4xx API error: the server rejected the request and created nothing. A 5xx
// can come after the resource was made, so its outcome is unknown.
func refused(err error) bool {
	var ae smithy.APIError
	var re *smithyhttp.ResponseError
	if !errors.As(err, &ae) || !errors.As(err, &re) || re.Response == nil {
		return false
	}
	return re.HTTPStatusCode() >= 400 && re.HTTPStatusCode() < 500
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
		var ae smithy.APIError
		if err != nil && (!errors.As(err, &ae) || ae.ErrorCode() != spec.absence) {
			return fmt.Errorf("cleanup %s %s: %w", rec.Ref.Kind, rec.Ref.ID, err)
		}
		if err != nil {
			// The delete answered absent. Eventual consistency can say that of a volume that still
			// exists, so a fresh observation must agree before the volume counts as gone.
			if again := rt.observeRef(ctx, spec, rec.Ref); again.Status != StatusAbsent {
				return &PreserveError{
					Ref:    rec.Ref,
					Reason: "delete answered absent but a fresh observation did not; deletion not confirmed",
				}
			}
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

func (rt *Runtime) EC2CreateKeyPair(ctx context.Context, in *ec2.CreateKeyPairInput) (*ec2.CreateKeyPairOutput, error) {
	if in == nil {
		return nil, errors.New("CreateKeyPair: input is nil")
	}
	if rt.Clients == nil || rt.Clients.EC2 == nil {
		return nil, errors.New("CreateKeyPair: ec2 client is unset")
	}
	specs, err := withRunTag(in.TagSpecifications, types.ResourceTypeKeyPair, rt.RunID())
	if err != nil {
		return nil, fmt.Errorf("CreateKeyPair: %w", err)
	}
	tagged := *in
	tagged.TagSpecifications = specs
	return create(ctx, rt, KindKeyPair, func(ctx context.Context) (*ec2.CreateKeyPairOutput, error) {
		out, err := rt.Clients.EC2.CreateKeyPair(ctx, &tagged, oneAttempt)
		return out, wrapErr("CreateKeyPair", err)
	}, func(out *ec2.CreateKeyPairOutput) string {
		if out == nil {
			return ""
		}
		return aws.ToString(out.KeyPairId)
	})
}

func (rt *Runtime) EC2ImportKeyPair(ctx context.Context, in *ec2.ImportKeyPairInput) (*ec2.ImportKeyPairOutput, error) {
	if in == nil {
		return nil, errors.New("ImportKeyPair: input is nil")
	}
	if rt.Clients == nil || rt.Clients.EC2 == nil {
		return nil, errors.New("ImportKeyPair: ec2 client is unset")
	}
	specs, err := withRunTag(in.TagSpecifications, types.ResourceTypeKeyPair, rt.RunID())
	if err != nil {
		return nil, fmt.Errorf("ImportKeyPair: %w", err)
	}
	tagged := *in
	tagged.TagSpecifications = specs
	return create(ctx, rt, KindKeyPair, func(ctx context.Context) (*ec2.ImportKeyPairOutput, error) {
		out, err := rt.Clients.EC2.ImportKeyPair(ctx, &tagged, oneAttempt)
		return out, wrapErr("ImportKeyPair", err)
	}, func(out *ec2.ImportKeyPairOutput) string {
		if out == nil {
			return ""
		}
		return aws.ToString(out.KeyPairId)
	})
}

func (rt *Runtime) EC2DeleteKeyPair(ctx context.Context, id string) (*ec2.DeleteKeyPairOutput, error) {
	return mutate(ctx, rt, Ref{Kind: KindKeyPair, ID: id}, true,
		func(ctx context.Context) (*ec2.DeleteKeyPairOutput, error) { return deleteKeyPair(ctx, rt.Clients, id) })
}

func deleteKeyPair(ctx context.Context, cl *Clients, id string) (*ec2.DeleteKeyPairOutput, error) {
	if cl == nil || cl.EC2 == nil {
		return nil, errors.New("DeleteKeyPair: ec2 client is unset")
	}
	out, err := cl.EC2.DeleteKeyPair(ctx, &ec2.DeleteKeyPairInput{KeyPairId: aws.String(id)}, oneAttempt)
	return out, wrapErr("DeleteKeyPair", err)
}

func fetchKeyPairs(ctx context.Context, cl *Clients, id string, token *string) (page, error) {
	if cl == nil || cl.EC2 == nil {
		return page{}, errors.New("DescribeKeyPairs: ec2 client is unset")
	}
	if token != nil && *token != "" {
		return page{}, errors.New("DescribeKeyPairs: unexpected pagination token")
	}
	out, err := cl.EC2.DescribeKeyPairs(ctx, &ec2.DescribeKeyPairsInput{KeyPairIds: []string{id}})
	if err != nil {
		return page{}, fmt.Errorf("DescribeKeyPairs: %w", err)
	}
	if out == nil {
		return page{}, errors.New("DescribeKeyPairs: nil output")
	}
	var p page
	for _, key := range out.KeyPairs {
		p.items = append(p.items, item{ID: aws.ToString(key.KeyPairId), RunTags: ec2RunTags(key.Tags)})
	}
	return p, nil
}

// EC2RunInstances gates one instance per call so its cleanup record has one exact ID.
func (rt *Runtime) EC2RunInstances(ctx context.Context, in *ec2.RunInstancesInput) (*ec2.RunInstancesOutput, error) {
	if in == nil {
		return nil, errors.New("RunInstances: input is nil")
	}
	if rt.Clients == nil || rt.Clients.EC2 == nil {
		return nil, errors.New("RunInstances: ec2 client is unset")
	}
	if aws.ToInt32(in.MinCount) != 1 || aws.ToInt32(in.MaxCount) != 1 {
		return nil, errors.New("RunInstances: the cleanup gate requires MinCount and MaxCount of one")
	}
	if len(in.NetworkInterfaces) != 0 {
		return nil, errors.New("RunInstances: network interfaces would create resources the cleanup gate does not track")
	}
	for _, m := range in.BlockDeviceMappings {
		if m.Ebs != nil && m.Ebs.DeleteOnTermination != nil && !*m.Ebs.DeleteOnTermination {
			return nil, errors.New("RunInstances: DeleteOnTermination=false would leave an untracked volume")
		}
	}
	specs, err := withRunTag(in.TagSpecifications, types.ResourceTypeInstance, rt.RunID())
	if err != nil {
		return nil, fmt.Errorf("RunInstances: %w", err)
	}
	tagged := *in
	tagged.TagSpecifications = specs
	return create(ctx, rt, KindInstance, func(ctx context.Context) (*ec2.RunInstancesOutput, error) {
		out, err := rt.Clients.EC2.RunInstances(ctx, &tagged, oneAttempt)
		return out, wrapErr("RunInstances", err)
	}, func(out *ec2.RunInstancesOutput) string {
		if out == nil || len(out.Instances) != 1 {
			return ""
		}
		return aws.ToString(out.Instances[0].InstanceId)
	})
}

func (rt *Runtime) EC2TerminateInstances(ctx context.Context, id string) (*ec2.TerminateInstancesOutput, error) {
	return mutate(ctx, rt, Ref{Kind: KindInstance, ID: id}, true,
		func(ctx context.Context) (*ec2.TerminateInstancesOutput, error) {
			return terminateInstance(ctx, rt.Clients, id)
		})
}

func terminateInstance(ctx context.Context, cl *Clients, id string) (*ec2.TerminateInstancesOutput, error) {
	if cl == nil || cl.EC2 == nil {
		return nil, errors.New("TerminateInstances: ec2 client is unset")
	}
	out, err := cl.EC2.TerminateInstances(ctx, &ec2.TerminateInstancesInput{InstanceIds: []string{id}}, oneAttempt)
	return out, wrapErr("TerminateInstances", err)
}

func fetchInstances(ctx context.Context, cl *Clients, id string, token *string) (page, error) {
	if cl == nil || cl.EC2 == nil {
		return page{}, errors.New("DescribeInstances: ec2 client is unset")
	}
	out, err := cl.EC2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{InstanceIds: []string{id}, NextToken: token})
	if err != nil {
		return page{}, fmt.Errorf("DescribeInstances: %w", err)
	}
	if out == nil {
		return page{}, errors.New("DescribeInstances: nil output")
	}
	p := page{next: out.NextToken}
	for _, res := range out.Reservations {
		for _, inst := range res.Instances {
			gone := inst.State != nil && inst.State.Name == types.InstanceStateNameTerminated
			p.items = append(p.items, item{ID: aws.ToString(inst.InstanceId), RunTags: ec2RunTags(inst.Tags), Gone: gone})
		}
	}
	return p, nil
}

func (rt *Runtime) EC2CreateVpc(ctx context.Context, in *ec2.CreateVpcInput) (*ec2.CreateVpcOutput, error) {
	if in == nil {
		return nil, errors.New("CreateVpc: input is nil")
	}
	if rt.Clients == nil || rt.Clients.EC2 == nil {
		return nil, errors.New("CreateVpc: ec2 client is unset")
	}
	specs, err := withRunTag(in.TagSpecifications, types.ResourceTypeVpc, rt.RunID())
	if err != nil {
		return nil, fmt.Errorf("CreateVpc: %w", err)
	}
	tagged := *in
	tagged.TagSpecifications = specs
	return create(ctx, rt, KindVPC, func(ctx context.Context) (*ec2.CreateVpcOutput, error) {
		out, err := rt.Clients.EC2.CreateVpc(ctx, &tagged, oneAttempt)
		return out, wrapErr("CreateVpc", err)
	}, func(out *ec2.CreateVpcOutput) string {
		if out == nil || out.Vpc == nil {
			return ""
		}
		return aws.ToString(out.Vpc.VpcId)
	})
}

func (rt *Runtime) EC2DeleteVpc(ctx context.Context, id string) (*ec2.DeleteVpcOutput, error) {
	return mutate(ctx, rt, Ref{Kind: KindVPC, ID: id}, true,
		func(ctx context.Context) (*ec2.DeleteVpcOutput, error) { return deleteVpc(ctx, rt.Clients, id) })
}

func fetchVpcs(ctx context.Context, cl *Clients, id string, token *string) (page, error) {
	if cl == nil || cl.EC2 == nil {
		return page{}, errors.New("DescribeVpcs: ec2 client is unset")
	}
	out, err := cl.EC2.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{VpcIds: []string{id}, NextToken: token})
	if err != nil {
		return page{}, fmt.Errorf("DescribeVpcs: %w", err)
	}
	if out == nil {
		return page{}, errors.New("DescribeVpcs: nil output")
	}
	p := page{next: out.NextToken}
	for _, v := range out.Vpcs {
		p.items = append(p.items, item{ID: aws.ToString(v.VpcId), RunTags: ec2RunTags(v.Tags)})
	}
	return p, nil
}

func deleteVpc(ctx context.Context, cl *Clients, id string) (*ec2.DeleteVpcOutput, error) {
	if cl == nil || cl.EC2 == nil {
		return nil, errors.New("DeleteVpc: ec2 client is unset")
	}
	out, err := cl.EC2.DeleteVpc(ctx, &ec2.DeleteVpcInput{VpcId: aws.String(id)}, oneAttempt)
	return out, wrapErr("DeleteVpc", err)
}
