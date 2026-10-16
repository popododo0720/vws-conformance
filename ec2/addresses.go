package ec2

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/expect"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Addresses() bind.Group {
	return bind.Group{
		Service: "ec2", Name: "addresses", Run: runAddr,
		Cases: []cases.Case{
			{
				ID: "EC2-EIP-001", Service: "ec2", Group: "addresses", Op: "AllocateAddress", Kind: cases.KindHappy,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2("AllocateAddress"), Expect: cases.Expect{
					HTTP:     200,
					Patterns: map[string]string{"id": expect.AllocID},
				}, Impl: "yes",
			},
			{
				ID: "EC2-EIP-002", Service: "ec2", Group: "addresses", Op: "AssociateAddress", Kind: cases.KindHappy,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2("AssociateAddress"), Expect: cases.Expect{
					HTTP:     200,
					Patterns: map[string]string{"id": expect.AssocID},
				}, Impl: "yes", Slow: true,
			},
			{
				ID: "EC2-EIP-003", Service: "ec2", Group: "addresses", Op: "DescribeAddresses", Kind: cases.KindHappy,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DescribeAddresses"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
			},
			{
				ID: "EC2-EIP-004", Service: "ec2", Group: "addresses", Op: "DisassociateAddress", Kind: cases.KindHappy,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DisassociateAddress"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
				Slow: true,
			},
			{
				ID: "EC2-EIP-005", Service: "ec2", Group: "addresses", Op: "ReleaseAddress", Kind: cases.KindHappy,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2("ReleaseAddress"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
			},
			{
				ID: "EC2-EIP-101", Service: "ec2", Group: "addresses", Op: "ReleaseAddress", Kind: cases.KindError,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{
					HTTP: 400,
					Code: "InvalidAllocationID.NotFound",
				}, Impl: "yes",
			},
			{
				ID: "EC2-EIP-102", Service: "ec2", Group: "addresses", Op: "DisassociateAddress", Kind: cases.KindError,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{
					HTTP: 400,
					Code: "InvalidAssociationID.NotFound",
				}, Skip: cases.SkipUnsafeFixedID, Impl: "yes",
			},
			{
				ID: "EC2-EIP-103", Service: "ec2", Group: "addresses", Op: "DescribeAddresses", Kind: cases.KindError,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{
					HTTP: 400,
					Code: "InvalidAllocationID.NotFound",
				}, Impl: "yes",
			},
			{
				ID: "EC2-EIP-104", Service: "ec2", Group: "addresses", Op: "AssociateAddress", Kind: cases.KindError,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{
					HTTP: 400,
					Code: "InvalidAllocationID.NotFound",
				}, Impl: "yes",
			},
			{
				ID: "EC2-EIP-105", Service: "ec2", Group: "addresses", Op: "AllocateAddress", Kind: cases.KindError,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{HTTP: 400, Code: "InvalidParameterValue"},
				Impl: "yes",
			},
		},
	}
}

func runAddr(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	api, err := client(rt)
	if err != nil {
		return cases.Outcome{}, err
	}
	switch c.ID {
	case "EC2-EIP-001":
		return alloc(ctx, rt, api)
	case "EC2-EIP-003":
		id, err := allocID(ctx, rt, api)
		if err != nil {
			return cases.Outcome{}, err
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DescribeAddresses(ctx, &awsec2.DescribeAddressesInput{AllocationIds: []string{id}})
				return wrapErr("runAddr", err)
			})
			return callOut, wrapErr("runAddr", callErr)
		}
	case "EC2-EIP-005":
		id, err := allocID(ctx, rt, api)
		if err != nil {
			return cases.Outcome{}, err
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.ReleaseAddress(ctx, &awsec2.ReleaseAddressInput{AllocationId: aws.String(id)})
				return wrapErr("runAddr", err)
			})
			return callOut, wrapErr("runAddr", callErr)
		}
	case "EC2-EIP-101":
		id, err := goneAlloc(ctx, rt, api)
		if err != nil {
			return cases.Outcome{}, err
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.ReleaseAddress(ctx, &awsec2.ReleaseAddressInput{AllocationId: aws.String(id)})
				return wrapErr("runAddr", err)
			})
			return callOut, wrapErr("runAddr", callErr)
		}
	case "EC2-EIP-102":
		return cases.Outcome{}, errors.New(cases.SkipUnsafeFixedID)
	case "EC2-EIP-103":
		id, err := goneAlloc(ctx, rt, api)
		if err != nil {
			return cases.Outcome{}, err
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DescribeAddresses(ctx, &awsec2.DescribeAddressesInput{AllocationIds: []string{id}})
				return wrapErr("runAddr", err)
			})
			return callOut, wrapErr("runAddr", callErr)
		}
	case "EC2-EIP-104":
		id, err := goneAlloc(ctx, rt, api)
		if err != nil {
			return cases.Outcome{}, err
		}
		_, subnet, err := rt.EnsureNet(ctx)
		if err != nil {
			return cases.Outcome{}, wrapErr("runAddr", err)
		}
		inst, err := launch(ctx, rt, api, subnet, rt.Token("eip104"))
		if err != nil {
			return cases.Outcome{}, err
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.AssociateAddress(ctx, &awsec2.AssociateAddressInput{
					AllocationId: aws.String(id),
					InstanceId:   aws.String(inst),
				})
				return wrapErr("runAddr", err)
			})
			return callOut, wrapErr("runAddr", callErr)
		}
	case "EC2-EIP-105":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.AllocateAddress(ctx, &awsec2.AllocateAddressInput{Domain: types.DomainType("nope")})
				return wrapErr("runAddr", err)
			})
			return callOut, wrapErr("runAddr", callErr)
		}
	case "EC2-EIP-002", "EC2-EIP-004":
		_, subnet, err := rt.EnsureNet(ctx)
		if err != nil {
			return cases.Outcome{}, wrapErr("runAddr", err)
		}
		inst, err := launch(ctx, rt, api, subnet, rt.Token("eip"))
		if err != nil {
			return cases.Outcome{}, err
		}
		id, err := allocID(ctx, rt, api)
		if err != nil {
			return cases.Outcome{}, err
		}
		var assoc string
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			out, err := api.AssociateAddress(ctx, &awsec2.AssociateAddressInput{
				AllocationId: aws.String(id),
				InstanceId:   aws.String(inst),
			})
			if err == nil {
				assoc = aws.ToString(out.AssociationId)
			}
			return wrapErr("runAddr", err)
		})
		o.Fields["id"] = assoc
		if c.ID == "EC2-EIP-002" || !harness.SetupReady(o, err) || assoc == "" {
			return o, wrapErr("runAddr", err)
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DisassociateAddress(ctx, &awsec2.DisassociateAddressInput{AssociationId: aws.String(assoc)})
				return wrapErr("runAddr", err)
			})
			return callOut, wrapErr("runAddr", callErr)
		}
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}

func alloc(ctx context.Context, rt *harness.Runtime, api *awsec2.Client) (cases.Outcome, error) {
	var id string
	o, err := rt.Call(ctx, func(ctx context.Context) error {
		out, err := api.AllocateAddress(ctx, addressInput(rt))
		if err == nil {
			id = aws.ToString(out.AllocationId)
		}
		return wrapErr("alloc", err)
	})
	o.Fields["id"] = id
	if id != "" {
		if err := rememberAddress(rt, api, id); err != nil {
			return o, wrapErr("alloc", err)
		}
	}
	return o, wrapErr("alloc", err)
}

func goneAlloc(ctx context.Context, rt *harness.Runtime, api *awsec2.Client) (string, error) {
	id, err := allocID(ctx, rt, api)
	if err != nil {
		return "", err
	}
	if _, err := api.ReleaseAddress(ctx, &awsec2.ReleaseAddressInput{AllocationId: aws.String(id)}); err != nil {
		return "", fmt.Errorf("release %s: %w", id, err)
	}
	return id, nil
}

func allocID(ctx context.Context, rt *harness.Runtime, api *awsec2.Client) (string, error) {
	in := addressInput(rt)
	out, err := api.AllocateAddress(ctx, in)
	if harness.UnsupportedParameter(err) {
		bare := *in
		bare.TagSpecifications = nil
		out, err = api.AllocateAddress(ctx, &bare)
	}
	if err != nil {
		return "", fmt.Errorf("allocate: %w", err)
	}
	id := aws.ToString(out.AllocationId)
	if err := rememberAddress(rt, api, id); err != nil {
		return "", err
	}
	return id, nil
}

func addressInput(rt *harness.Runtime) *awsec2.AllocateAddressInput {
	return &awsec2.AllocateAddressInput{
		Domain:            types.DomainTypeVpc,
		TagSpecifications: rt.TagSpec(types.ResourceTypeElasticIp),
	}
}

func rememberAddress(rt *harness.Runtime, api *awsec2.Client, id string) error {
	if id == "" {
		return fmt.Errorf("allocate: empty id")
	}
	rt.Cleanup(func(ctx context.Context) error {
		_, err := api.ReleaseAddress(ctx, &awsec2.ReleaseAddressInput{AllocationId: aws.String(id)})
		return harness.IgnoreCleanup(err)
	})
	return wrapErr("rememberAddress", rt.Track("address", id, rt.Name("eip")))
}
