package harness

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsarn "github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/smithy-go"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/popododo0720/vws-conformance/internal/cases"
)

// This file is the only place allowed to select a mutating method on an SDK client
// (census_test.go). Every such call goes through create or mutate, and cleanup through remove.

const (
	KindVolume   Kind = "volume"
	KindKeyPair  Kind = "key-pair"
	KindInstance Kind = "instance"
	KindPolicy   Kind = "iam-policy"
	KindRole     Kind = "iam-role"
)

const (
	KindVPC             Kind = "vpc"
	KindInternetGateway Kind = "internet-gateway"
)

// KindMFADevice is a virtual MFA device; its id is the serial number.
const KindMFADevice Kind = "mfa-device"

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
	KindInternetGateway: {
		absence: "InvalidInternetGatewayID.NotFound", doc: cases.DocEC2Errors, minted: true,
		fetch: fetchInternetGateways,
		remove: func(ctx context.Context, cl *Clients, id string) error {
			_, err := deleteInternetGateway(ctx, cl, id)
			return err
		},
	},
	// A serial number is made from the device name, so a deleted one can be made again by anyone:
	// only the run tag proves ownership.
	KindMFADevice: {
		absence: "NoSuchEntity", doc: cases.DocIAM("ListMFADeviceTags"), minted: false,
		fetch: fetchMFADeviceTags,
		remove: func(ctx context.Context, cl *Clients, id string) error {
			_, err := deleteVirtualMFADevice(ctx, cl, id)
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
	// A policy name carries the run id, so only this run mints that ARN and the ledger proves
	// ownership. VWS CreatePolicy does not take Tags, so a run tag cannot be stamped at creation;
	// a policy that carries another run's tag is still preserved.
	KindPolicy: {
		absence: "NoSuchEntity",
		doc:     cases.DocIAM("ListPolicyTags"),
		minted:  true,
		fetch:   fetchPolicy,
		remove:  deletePolicyAndVersions,
	},
	// A role is owned the way a policy is: its name carries the run id, because VWS CreateRole
	// does not take Tags. The id is the role name, the key GetRole and DeleteRole take.
	KindRole: {
		absence: "NoSuchEntity",
		doc:     cases.DocIAM("GetRole"),
		minted:  true,
		fetch:   fetchRole,
		remove: func(ctx context.Context, cl *Clients, id string) error {
			_, err := deleteRole(ctx, cl, id)
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

// IAMCreatePolicy creates a policy and registers its deletion before the request is sent. The
// policy name must carry this run's id.
func (rt *Runtime) IAMCreatePolicy(ctx context.Context, in *iam.CreatePolicyInput) (*iam.CreatePolicyOutput, error) {
	if in == nil {
		return nil, errors.New("CreatePolicy: input is nil")
	}
	if rt.Clients == nil || rt.Clients.IAM == nil {
		return nil, errors.New("CreatePolicy: iam client is unset")
	}
	if !strings.HasPrefix(aws.ToString(in.PolicyName), NamePrefix(rt.RunID())) {
		return nil, fmt.Errorf("CreatePolicy: policy name must start with %s", NamePrefix(rt.RunID()))
	}
	return create(ctx, rt, KindPolicy, func(ctx context.Context) (*iam.CreatePolicyOutput, error) {
		out, err := rt.Clients.IAM.CreatePolicy(ctx, in, iamOneAttempt)
		return out, wrapErr("CreatePolicy", err)
	}, func(out *iam.CreatePolicyOutput) string {
		if out == nil || out.Policy == nil {
			return ""
		}
		return aws.ToString(out.Policy.Arn)
	})
}

// IAMTagPolicy tags a policy only when a fresh observation proves this run owns it.
func (rt *Runtime) IAMTagPolicy(ctx context.Context, in *iam.TagPolicyInput) (*iam.TagPolicyOutput, error) {
	if in == nil {
		return nil, errors.New("TagPolicy: input is nil")
	}
	return mutate(ctx, rt, Ref{Kind: KindPolicy, ID: aws.ToString(in.PolicyArn)}, false,
		func(ctx context.Context) (*iam.TagPolicyOutput, error) {
			out, err := rt.Clients.IAM.TagPolicy(ctx, in, iamOneAttempt)
			return out, wrapErr("TagPolicy", err)
		})
}

// IAMUntagPolicy untags a policy only when a fresh observation proves this run owns it.
func (rt *Runtime) IAMUntagPolicy(ctx context.Context, in *iam.UntagPolicyInput) (*iam.UntagPolicyOutput, error) {
	if in == nil {
		return nil, errors.New("UntagPolicy: input is nil")
	}
	return mutate(ctx, rt, Ref{Kind: KindPolicy, ID: aws.ToString(in.PolicyArn)}, false,
		func(ctx context.Context) (*iam.UntagPolicyOutput, error) {
			return untagPolicy(ctx, rt.Clients, in)
		})
}

// IAMCreatePolicyVersion adds a version only when a fresh observation proves this run owns the
// policy. Cleanup deletes it with the policy.
func (rt *Runtime) IAMCreatePolicyVersion(ctx context.Context, in *iam.CreatePolicyVersionInput,
) (*iam.CreatePolicyVersionOutput, error) {
	if in == nil {
		return nil, errors.New("CreatePolicyVersion: input is nil")
	}
	return mutate(ctx, rt, Ref{Kind: KindPolicy, ID: aws.ToString(in.PolicyArn)}, false,
		func(ctx context.Context) (*iam.CreatePolicyVersionOutput, error) {
			out, err := rt.Clients.IAM.CreatePolicyVersion(ctx, in, iamOneAttempt)
			return out, wrapErr("CreatePolicyVersion", err)
		})
}

// IAMSetDefaultPolicyVersion sets the default version only when a fresh observation proves this
// run owns the policy.
func (rt *Runtime) IAMSetDefaultPolicyVersion(ctx context.Context, in *iam.SetDefaultPolicyVersionInput,
) (*iam.SetDefaultPolicyVersionOutput, error) {
	if in == nil {
		return nil, errors.New("SetDefaultPolicyVersion: input is nil")
	}
	return mutate(ctx, rt, Ref{Kind: KindPolicy, ID: aws.ToString(in.PolicyArn)}, false,
		func(ctx context.Context) (*iam.SetDefaultPolicyVersionOutput, error) {
			out, err := rt.Clients.IAM.SetDefaultPolicyVersion(ctx, in, iamOneAttempt)
			return out, wrapErr("SetDefaultPolicyVersion", err)
		})
}

// IAMDeletePolicyVersion deletes a version only when a fresh observation proves this run owns the
// policy.
func (rt *Runtime) IAMDeletePolicyVersion(ctx context.Context, in *iam.DeletePolicyVersionInput,
) (*iam.DeletePolicyVersionOutput, error) {
	if in == nil {
		return nil, errors.New("DeletePolicyVersion: input is nil")
	}
	return mutate(ctx, rt, Ref{Kind: KindPolicy, ID: aws.ToString(in.PolicyArn)}, false,
		func(ctx context.Context) (*iam.DeletePolicyVersionOutput, error) {
			return deletePolicyVersion(ctx, rt.Clients, in)
		})
}

// IAMProbeUntagPolicy sends an UntagPolicy the service is expected to reject (see probePolicy).
func (rt *Runtime) IAMProbeUntagPolicy(ctx context.Context, in *iam.UntagPolicyInput) (*iam.UntagPolicyOutput, error) {
	if in == nil {
		return nil, errors.New("UntagPolicy: input is nil")
	}
	return probePolicy(ctx, rt, aws.ToString(in.PolicyArn), func(ctx context.Context) (*iam.UntagPolicyOutput, error) {
		return untagPolicy(ctx, rt.Clients, in)
	})
}

// IAMProbeDeletePolicy sends a DeletePolicy the service is expected to reject (see probePolicy).
func (rt *Runtime) IAMProbeDeletePolicy(ctx context.Context, arn string) (*iam.DeletePolicyOutput, error) {
	return probePolicy(ctx, rt, arn, func(ctx context.Context) (*iam.DeletePolicyOutput, error) {
		return deletePolicy(ctx, rt.Clients, arn)
	})
}

// probePolicy sends a call the service is expected to reject, for a policy this run does not own:
// an absent one, one in another account, or none. The ARN must carry this run's id, so only this
// run could ever have made the policy, and an observation must not show it present. In this run's
// account it must show it absent: a policy that exists, or whose state is unknown, is touched only
// through the gated calls. A target in another verified-different account is sent even when its
// state is unknown, because a cross-account read is denied just like the call under test, and this
// run creates resources only in its own account, so the target cannot be its own.
func probePolicy[Out any](ctx context.Context, rt *Runtime, arn string, send func(context.Context) (Out, error),
) (Out, error) {
	var zero Out
	ref := Ref{Kind: KindPolicy, ID: arn}
	switch {
	case arn != "" && !strings.Contains(arn, NamePrefix(rt.RunID())):
		return zero, &PreserveError{Ref: ref, Reason: "probe target is not named for this run"}
	}
	s, err := rt.admit()
	if err != nil {
		return zero, fmt.Errorf("probe %s %s: %w", ref.Kind, arn, err)
	}
	defer s.mu.Unlock()
	if arn != "" {
		switch obs := rt.observeRef(ctx, kinds[KindPolicy], ref); obs.Status {
		case StatusPresent:
			return zero, &PreserveError{Ref: ref, Reason: "probe target exists"}
		case StatusAbsent:
		case StatusUnknown:
			if !inOtherAccount(arn, rt.Cfg.AccountID) {
				return zero, &PreserveError{Ref: ref, Reason: "probe target is not proven absent"}
			}
		default:
			return zero, &PreserveError{Ref: ref, Reason: "probe target is not proven absent"}
		}
	}
	out, err := withDeadline(ctx, send)
	if err != nil {
		return out, fmt.Errorf("probe %s %s: %w", ref.Kind, arn, err)
	}
	return out, nil
}

// inOtherAccount reports whether arn parses and names an account other than the verified account.
func inOtherAccount(arn, account string) bool {
	parsed, err := awsarn.Parse(arn)
	return err == nil && account != "" && parsed.AccountID != "" && parsed.AccountID != account
}

// IAMDeletePolicy deletes a policy only when a fresh observation proves this run owns it.
func (rt *Runtime) IAMDeletePolicy(ctx context.Context, arn string) (*iam.DeletePolicyOutput, error) {
	return mutate(ctx, rt, Ref{Kind: KindPolicy, ID: arn}, true,
		func(ctx context.Context) (*iam.DeletePolicyOutput, error) { return deletePolicy(ctx, rt.Clients, arn) })
}

// IAMCreateRole creates a role and registers its deletion before the request is sent. The role
// name must carry this run's id. A response naming another role leaves the record lost, so cleanup
// never deletes a role this run did not ask for.
func (rt *Runtime) IAMCreateRole(ctx context.Context, in *iam.CreateRoleInput) (*iam.CreateRoleOutput, error) {
	if in == nil {
		return nil, errors.New("CreateRole: input is nil")
	}
	if rt.Clients == nil || rt.Clients.IAM == nil {
		return nil, errors.New("CreateRole: iam client is unset")
	}
	name := aws.ToString(in.RoleName)
	if !strings.HasPrefix(name, NamePrefix(rt.RunID())) {
		return nil, fmt.Errorf("CreateRole: role name must start with %s", NamePrefix(rt.RunID()))
	}
	return create(ctx, rt, KindRole, func(ctx context.Context) (*iam.CreateRoleOutput, error) {
		out, err := rt.Clients.IAM.CreateRole(ctx, in, iamOneAttempt)
		return out, wrapErr("CreateRole", err)
	}, func(out *iam.CreateRoleOutput) string {
		if out == nil || out.Role == nil || aws.ToString(out.Role.RoleName) != name {
			return ""
		}
		return name
	})
}

// IAMDeleteRole deletes a role only when a fresh observation proves this run owns it.
func (rt *Runtime) IAMDeleteRole(ctx context.Context, name string) (*iam.DeleteRoleOutput, error) {
	return mutate(ctx, rt, Ref{Kind: KindRole, ID: name}, true,
		func(ctx context.Context) (*iam.DeleteRoleOutput, error) { return deleteRole(ctx, rt.Clients, name) })
}

// IAMProbeDeleteRole sends a DeleteRole the service is expected to reject: the role is named for
// this run, so only this run could have made it, and a fresh observation must show it absent. A
// role that exists, or whose state is unknown, is touched only through IAMDeleteRole.
func (rt *Runtime) IAMProbeDeleteRole(ctx context.Context, name string) (*iam.DeleteRoleOutput, error) {
	ref := Ref{Kind: KindRole, ID: name}
	if !strings.HasPrefix(name, NamePrefix(rt.RunID())) {
		return nil, &PreserveError{Ref: ref, Reason: "probe target is not named for this run"}
	}
	s, err := rt.admit()
	if err != nil {
		return nil, fmt.Errorf("probe %s %s: %w", ref.Kind, name, err)
	}
	defer s.mu.Unlock()
	if obs := rt.observeRef(ctx, kinds[KindRole], ref); obs.Status != StatusAbsent {
		return nil, &PreserveError{Ref: ref, Reason: "probe target is not proven absent"}
	}
	out, err := withDeadline(ctx, func(ctx context.Context) (*iam.DeleteRoleOutput, error) {
		return deleteRole(ctx, rt.Clients, name)
	})
	if err != nil {
		return out, fmt.Errorf("probe %s %s: %w", ref.Kind, name, err)
	}
	return out, nil
}

func fetchRole(ctx context.Context, cl *Clients, id string, token *string) (page, error) {
	if cl == nil || cl.IAM == nil {
		return page{}, errors.New("GetRole: iam client is unset")
	}
	if token != nil && *token != "" {
		return page{}, errors.New("GetRole: unexpected pagination token")
	}
	out, err := cl.IAM.GetRole(ctx, &iam.GetRoleInput{RoleName: aws.String(id)})
	if err != nil {
		return page{}, fmt.Errorf("GetRole: %w", err)
	}
	if out == nil || out.Role == nil {
		return page{}, errors.New("GetRole: no role in output")
	}
	it := item{ID: aws.ToString(out.Role.RoleName)}
	for _, t := range out.Role.Tags {
		if aws.ToString(t.Key) == RunTagKey {
			it.RunTags = append(it.RunTags, aws.ToString(t.Value))
		}
	}
	return page{items: []item{it}}, nil
}

func deleteRole(ctx context.Context, cl *Clients, name string) (*iam.DeleteRoleOutput, error) {
	if cl == nil || cl.IAM == nil {
		return nil, errors.New("DeleteRole: iam client is unset")
	}
	out, err := cl.IAM.DeleteRole(ctx, &iam.DeleteRoleInput{RoleName: aws.String(name)}, iamOneAttempt)
	return out, wrapErr("DeleteRole", err)
}

func fetchPolicy(ctx context.Context, cl *Clients, id string, token *string) (page, error) {
	if cl == nil || cl.IAM == nil {
		return page{}, errors.New("ListPolicyTags: iam client is unset")
	}
	out, err := cl.IAM.ListPolicyTags(ctx, &iam.ListPolicyTagsInput{PolicyArn: aws.String(id), Marker: token})
	if err != nil {
		return page{}, fmt.Errorf("ListPolicyTags: %w", err)
	}
	if out == nil {
		return page{}, errors.New("ListPolicyTags: nil output")
	}
	it := item{ID: id}
	for _, t := range out.Tags {
		if aws.ToString(t.Key) == RunTagKey {
			it.RunTags = append(it.RunTags, aws.ToString(t.Value))
		}
	}
	p := page{items: []item{it}}
	if out.IsTruncated {
		p.next = out.Marker
	}
	return p, nil
}

func untagPolicy(ctx context.Context, cl *Clients, in *iam.UntagPolicyInput) (*iam.UntagPolicyOutput, error) {
	if cl == nil || cl.IAM == nil {
		return nil, errors.New("UntagPolicy: iam client is unset")
	}
	out, err := cl.IAM.UntagPolicy(ctx, in, withoutInputValidation, iamOneAttempt)
	return out, wrapErr("UntagPolicy", err)
}

// deletePolicyAndVersions deletes every non-default version, then the policy: DeletePolicy answers
// DeleteConflict while one is left. All pages are read before the first delete so a delete cannot
// shift a page marker.
func deletePolicyAndVersions(ctx context.Context, cl *Clients, arn string) error {
	if cl == nil || cl.IAM == nil {
		return errors.New("ListPolicyVersions: iam client is unset")
	}
	var stale []string
	in := &iam.ListPolicyVersionsInput{PolicyArn: aws.String(arn)}
	for {
		out, err := cl.IAM.ListPolicyVersions(ctx, in)
		if err != nil {
			return wrapErr("ListPolicyVersions", err)
		}
		for _, v := range out.Versions {
			if !v.IsDefaultVersion {
				stale = append(stale, aws.ToString(v.VersionId))
			}
		}
		if !out.IsTruncated {
			break
		}
		if aws.ToString(out.Marker) == "" {
			return errors.New("ListPolicyVersions: truncated page has no marker")
		}
		in.Marker = out.Marker
	}
	for _, id := range stale {
		if _, err := deletePolicyVersion(ctx, cl, &iam.DeletePolicyVersionInput{
			PolicyArn: aws.String(arn), VersionId: aws.String(id),
		}); err != nil {
			return err
		}
	}
	_, err := deletePolicy(ctx, cl, arn)
	return err
}

func deletePolicyVersion(ctx context.Context, cl *Clients, in *iam.DeletePolicyVersionInput,
) (*iam.DeletePolicyVersionOutput, error) {
	if cl == nil || cl.IAM == nil {
		return nil, errors.New("DeletePolicyVersion: iam client is unset")
	}
	out, err := cl.IAM.DeletePolicyVersion(ctx, in, iamOneAttempt)
	return out, wrapErr("DeletePolicyVersion", err)
}

func deletePolicy(ctx context.Context, cl *Clients, arn string) (*iam.DeletePolicyOutput, error) {
	if cl == nil || cl.IAM == nil {
		return nil, errors.New("DeletePolicy: iam client is unset")
	}
	out, err := cl.IAM.DeletePolicy(ctx, &iam.DeletePolicyInput{PolicyArn: aws.String(arn)}, iamOneAttempt)
	return out, wrapErr("DeletePolicy", err)
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

func iamOneAttempt(o *iam.Options) { o.RetryMaxAttempts = 1 }

// withoutInputValidation drops the SDK's required-member check so a request with a missing member
// reaches the service, which must answer the documented error itself.
func withoutInputValidation(o *iam.Options) {
	o.APIOptions = append(o.APIOptions, func(s *middleware.Stack) error {
		_, err := s.Initialize.Remove("OperationInputValidation")
		return wrapErr("withoutInputValidation", err)
	})
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

func (rt *Runtime) EC2CreateInternetGateway(ctx context.Context, in *ec2.CreateInternetGatewayInput,
) (*ec2.CreateInternetGatewayOutput, error) {
	if in == nil {
		return nil, errors.New("CreateInternetGateway: input is nil")
	}
	if rt.Clients == nil || rt.Clients.EC2 == nil {
		return nil, errors.New("CreateInternetGateway: ec2 client is unset")
	}
	specs, err := withRunTag(in.TagSpecifications, types.ResourceTypeInternetGateway, rt.RunID())
	if err != nil {
		return nil, fmt.Errorf("CreateInternetGateway: %w", err)
	}
	tagged := *in
	tagged.TagSpecifications = specs
	return create(ctx, rt, KindInternetGateway, func(ctx context.Context) (*ec2.CreateInternetGatewayOutput, error) {
		out, err := rt.Clients.EC2.CreateInternetGateway(ctx, &tagged, oneAttempt)
		return out, wrapErr("CreateInternetGateway", err)
	}, func(out *ec2.CreateInternetGatewayOutput) string {
		if out == nil || out.InternetGateway == nil {
			return ""
		}
		return aws.ToString(out.InternetGateway.InternetGatewayId)
	})
}

func (rt *Runtime) EC2DeleteInternetGateway(ctx context.Context, id string) (*ec2.DeleteInternetGatewayOutput, error) {
	return mutate(ctx, rt, Ref{Kind: KindInternetGateway, ID: id}, true,
		func(ctx context.Context) (*ec2.DeleteInternetGatewayOutput, error) {
			return deleteInternetGateway(ctx, rt.Clients, id)
		})
}

func fetchInternetGateways(ctx context.Context, cl *Clients, id string, token *string) (page, error) {
	if cl == nil || cl.EC2 == nil {
		return page{}, errors.New("DescribeInternetGateways: ec2 client is unset")
	}
	out, err := cl.EC2.DescribeInternetGateways(ctx, &ec2.DescribeInternetGatewaysInput{
		InternetGatewayIds: []string{id}, NextToken: token,
	})
	if err != nil {
		return page{}, fmt.Errorf("DescribeInternetGateways: %w", err)
	}
	if out == nil {
		return page{}, errors.New("DescribeInternetGateways: nil output")
	}
	p := page{next: out.NextToken}
	for _, v := range out.InternetGateways {
		p.items = append(p.items, item{ID: aws.ToString(v.InternetGatewayId), RunTags: ec2RunTags(v.Tags)})
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

func writableTags(tags []types.Tag) error {
	for _, tag := range tags {
		if aws.ToString(tag.Key) == "" {
			return errors.New("tag key is required by the cleanup gate")
		}
		if aws.ToString(tag.Key) == RunTagKey {
			return fmt.Errorf("tag %s is reserved for the cleanup gate", RunTagKey)
		}
	}
	return nil
}

func (rt *Runtime) EC2CreateTags(ctx context.Context, id string, tags []types.Tag) (*ec2.CreateTagsOutput, error) {
	if err := writableTags(tags); err != nil {
		return nil, err
	}
	return mutate(ctx, rt, Ref{Kind: KindVolume, ID: id}, false, func(ctx context.Context) (*ec2.CreateTagsOutput, error) {
		if rt.Clients == nil || rt.Clients.EC2 == nil {
			return nil, errors.New("CreateTags: ec2 client is unset")
		}
		out, err := rt.Clients.EC2.CreateTags(ctx, &ec2.CreateTagsInput{Resources: []string{id}, Tags: tags}, oneAttempt)
		return out, wrapErr("CreateTags", err)
	})
}

func (rt *Runtime) EC2DeleteTags(ctx context.Context, id string, tags []types.Tag) (*ec2.DeleteTagsOutput, error) {
	if len(tags) == 0 {
		return nil, errors.New("DeleteTags: deleting all tags would remove cleanup ownership")
	}
	if err := writableTags(tags); err != nil {
		return nil, err
	}
	return mutate(ctx, rt, Ref{Kind: KindVolume, ID: id}, false, func(ctx context.Context) (*ec2.DeleteTagsOutput, error) {
		if rt.Clients == nil || rt.Clients.EC2 == nil {
			return nil, errors.New("DeleteTags: ec2 client is unset")
		}
		out, err := rt.Clients.EC2.DeleteTags(ctx, &ec2.DeleteTagsInput{Resources: []string{id}, Tags: tags}, oneAttempt)
		return out, wrapErr("DeleteTags", err)
	})
}

func deleteInternetGateway(ctx context.Context, cl *Clients, id string) (*ec2.DeleteInternetGatewayOutput, error) {
	if cl == nil || cl.EC2 == nil {
		return nil, errors.New("DeleteInternetGateway: ec2 client is unset")
	}
	out, err := cl.EC2.DeleteInternetGateway(ctx,
		&ec2.DeleteInternetGatewayInput{InternetGatewayId: aws.String(id)}, oneAttempt)
	return out, wrapErr("DeleteInternetGateway", err)
}

// IAM tag keys are case-insensitive, so any spelling of the run tag key is refused.
func reservedIAMKey(key string) error {
	if strings.EqualFold(key, RunTagKey) {
		return fmt.Errorf("tag %s is reserved for the cleanup gate", RunTagKey)
	}
	return nil
}

// IAMCreateVirtualMFADevice creates a virtual MFA device tagged with this run and registers its
// deletion before the request is sent. The caller's input is not modified.
func (rt *Runtime) IAMCreateVirtualMFADevice(ctx context.Context, in *iam.CreateVirtualMFADeviceInput,
) (*iam.CreateVirtualMFADeviceOutput, error) {
	if in == nil {
		return nil, errors.New("CreateVirtualMFADevice: input is nil")
	}
	if rt.Clients == nil || rt.Clients.IAM == nil {
		return nil, errors.New("CreateVirtualMFADevice: iam client is unset")
	}
	for _, t := range in.Tags {
		if err := reservedIAMKey(aws.ToString(t.Key)); err != nil {
			return nil, fmt.Errorf("CreateVirtualMFADevice: %w", err)
		}
	}
	tagged := *in
	tagged.Tags = append(slices.Clone(in.Tags), iamtypes.Tag{Key: aws.String(RunTagKey), Value: aws.String(rt.RunID())})
	return create(ctx, rt, KindMFADevice, func(ctx context.Context) (*iam.CreateVirtualMFADeviceOutput, error) {
		out, err := rt.Clients.IAM.CreateVirtualMFADevice(ctx, &tagged, iamOneAttempt)
		return out, wrapErr("CreateVirtualMFADevice", err)
	}, func(out *iam.CreateVirtualMFADeviceOutput) string {
		if out == nil || out.VirtualMFADevice == nil {
			return ""
		}
		return aws.ToString(out.VirtualMFADevice.SerialNumber)
	})
}

func (rt *Runtime) IAMDeleteVirtualMFADevice(ctx context.Context, serial string,
) (*iam.DeleteVirtualMFADeviceOutput, error) {
	return mutate(ctx, rt, Ref{Kind: KindMFADevice, ID: serial}, true,
		func(ctx context.Context) (*iam.DeleteVirtualMFADeviceOutput, error) {
			return deleteVirtualMFADevice(ctx, rt.Clients, serial)
		})
}

func (rt *Runtime) IAMTagMFADevice(ctx context.Context, serial string, tags []iamtypes.Tag,
) (*iam.TagMFADeviceOutput, error) {
	for _, t := range tags {
		if err := reservedIAMKey(aws.ToString(t.Key)); err != nil {
			return nil, fmt.Errorf("TagMFADevice: %w", err)
		}
	}
	return mutate(ctx, rt, Ref{Kind: KindMFADevice, ID: serial}, false,
		func(ctx context.Context) (*iam.TagMFADeviceOutput, error) {
			if rt.Clients == nil || rt.Clients.IAM == nil {
				return nil, errors.New("TagMFADevice: iam client is unset")
			}
			out, err := rt.Clients.IAM.TagMFADevice(ctx,
				&iam.TagMFADeviceInput{SerialNumber: aws.String(serial), Tags: tags}, iamOneAttempt)
			return out, wrapErr("TagMFADevice", err)
		})
}

func (rt *Runtime) IAMUntagMFADevice(ctx context.Context, serial string, keys []string,
) (*iam.UntagMFADeviceOutput, error) {
	for _, key := range keys {
		if err := reservedIAMKey(key); err != nil {
			return nil, fmt.Errorf("UntagMFADevice: %w", err)
		}
	}
	return mutate(ctx, rt, Ref{Kind: KindMFADevice, ID: serial}, false,
		func(ctx context.Context) (*iam.UntagMFADeviceOutput, error) {
			if rt.Clients == nil || rt.Clients.IAM == nil {
				return nil, errors.New("UntagMFADevice: iam client is unset")
			}
			out, err := rt.Clients.IAM.UntagMFADevice(ctx,
				&iam.UntagMFADeviceInput{SerialNumber: aws.String(serial), TagKeys: keys}, iamOneAttempt)
			return out, wrapErr("UntagMFADevice", err)
		})
}

func deleteVirtualMFADevice(ctx context.Context, cl *Clients, serial string,
) (*iam.DeleteVirtualMFADeviceOutput, error) {
	if cl == nil || cl.IAM == nil {
		return nil, errors.New("DeleteVirtualMFADevice: iam client is unset")
	}
	out, err := cl.IAM.DeleteVirtualMFADevice(ctx,
		&iam.DeleteVirtualMFADeviceInput{SerialNumber: aws.String(serial)}, iamOneAttempt)
	return out, wrapErr("DeleteVirtualMFADevice", err)
}

// fetchMFADeviceTags observes a device through its tags. A device holds at most 50 tags, so one
// page of the maximum size is the whole answer; a truncated one is not trusted.
func fetchMFADeviceTags(ctx context.Context, cl *Clients, serial string, token *string) (page, error) {
	if cl == nil || cl.IAM == nil {
		return page{}, errors.New("ListMFADeviceTags: iam client is unset")
	}
	if token != nil && *token != "" {
		return page{}, errors.New("ListMFADeviceTags: unexpected pagination token")
	}
	out, err := cl.IAM.ListMFADeviceTags(ctx,
		&iam.ListMFADeviceTagsInput{SerialNumber: aws.String(serial), MaxItems: aws.Int32(1000)})
	if err != nil {
		return page{}, fmt.Errorf("ListMFADeviceTags: %w", err)
	}
	if out == nil || out.IsTruncated {
		return page{}, errors.New("ListMFADeviceTags: nil or truncated output")
	}
	it := item{ID: serial}
	for _, t := range out.Tags {
		if strings.EqualFold(aws.ToString(t.Key), RunTagKey) {
			it.RunTags = append(it.RunTags, aws.ToString(t.Value))
		}
	}
	return page{items: []item{it}}, nil
}
