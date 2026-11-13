package ec2

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/expect"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Instances() bind.Group {
	return bind.Group{
		Service: "ec2", Name: "instances", Run: runInstances,
		Cases: []cases.Case{
			{
				ID: "EC2-RUN-001", Service: "ec2", Group: "instances", Op: "RunInstances", Kind: cases.KindHappy,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2("RunInstances"),
				Expect: cases.Expect{HTTP: 200, Patterns: map[string]string{"id": expect.InstanceID}}, Impl: "yes",
			},
			{
				ID: "EC2-RUN-101", Service: "ec2", Group: "instances", Op: "RunInstances", Kind: cases.KindError,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors,
				Expect: cases.Expect{HTTP: 400, Code: "MissingParameter"}, Impl: "yes",
			},
			{
				ID: "EC2-RUN-110", Service: "ec2", Group: "instances", Op: "RunInstances", Kind: cases.KindHappy,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2("RunInstances"),
				Expect: cases.Expect{HTTP: 200}, Impl: "yes",
			},
			{
				ID: "EC2-RUN-201", Service: "ec2", Group: "instances", Op: "RunInstances", Kind: cases.KindIdempotency,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2("RunInstances"),
				Expect: cases.Expect{HTTP: 200, Patterns: map[string]string{"same": "^yes$"}}, Impl: "yes",
			},
			{
				ID: "EC2-RUN-202", Service: "ec2", Group: "instances", Op: "RunInstances", Kind: cases.KindIdempotency,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors,
				Expect: cases.Expect{
					HTTP: 400, Code: "IdempotentParameterMismatch", Patterns: map[string]string{"setup": "^ok$"},
				}, Impl: "yes",
			},
			{
				ID: "EC2-DI-001", Service: "ec2", Group: "instances", Op: "DescribeInstances", Kind: cases.KindHappy,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DescribeInstances"),
				Expect: cases.Expect{HTTP: 200, Patterns: map[string]string{"id": expect.InstanceID}}, Impl: "yes",
			},
			{
				ID: "EC2-DI-201", Service: "ec2", Group: "instances", Op: "DescribeInstances", Kind: cases.KindFilter,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DescribeInstances"),
				Expect: cases.Expect{HTTP: 200}, Impl: "yes",
			},
			{
				ID: "EC2-DIA-001", Service: "ec2", Group: "instances", Op: "DescribeInstanceAttribute", Kind: cases.KindHappy,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DescribeInstanceAttribute"),
				Expect: cases.Expect{HTTP: 200}, Impl: "yes",
			},
			{
				ID: "EC2-STOP-001", Service: "ec2", Group: "instances", Op: "StopInstances", Kind: cases.KindHappy,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2("StopInstances"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
			},
			{
				ID: "EC2-START-001", Service: "ec2", Group: "instances", Op: "StartInstances", Kind: cases.KindHappy,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2("StartInstances"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
			},
			{
				ID: "EC2-TERM-001", Service: "ec2", Group: "instances", Op: "TerminateInstances", Kind: cases.KindHappy,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2("TerminateInstances"),
				Expect: cases.Expect{HTTP: 200, Patterns: map[string]string{"state": "^terminated$"}}, Impl: "yes",
			},
			{
				ID: "EC2-DI-101", Service: "ec2", Group: "instances", Op: "DescribeInstances", Kind: cases.KindError,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{
					HTTP: 400,
					Code: "InvalidInstanceID.NotFound",
				}, Impl: "yes",
			},
			{
				ID: "EC2-DI-102", Service: "ec2", Group: "instances", Op: "DescribeInstances", Kind: cases.KindError,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{
					HTTP: 400,
					Code: "InvalidInstanceID.Malformed",
				}, Impl: "yes",
			},
			{
				ID: "EC2-DIA-101", Service: "ec2", Group: "instances", Op: "DescribeInstanceAttribute",
				Kind: cases.KindError, Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{
					HTTP: 400,
					Code: "InvalidInstanceID.NotFound",
				}, Impl: "yes",
			},
		},
	}
}

func runInstances(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	api, err := client(rt)
	if err != nil {
		return cases.Outcome{}, err
	}
	switch c.ID {
	case "EC2-RUN-001", "EC2-RUN-101", "EC2-RUN-110", "EC2-RUN-201", "EC2-RUN-202",
		"EC2-DI-001", "EC2-DI-201", "EC2-DIA-001", "EC2-STOP-001", "EC2-START-001", "EC2-TERM-001":
		return runOwnedInstance(ctx, rt, api, c)
	case "EC2-DI-101":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeInstances(ctx, &awsec2.DescribeInstancesInput{
				InstanceIds: []string{"i-0123456789abcdef0"},
			})
			return wrapErr("runInstances", err)
		})
		return o, wrapErr("runInstances", err)
	case "EC2-DI-102":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeInstances(ctx, &awsec2.DescribeInstancesInput{InstanceIds: []string{"i-xyz"}})
			return wrapErr("runInstances", err)
		})
		return o, wrapErr("runInstances", err)
	case "EC2-DIA-101":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeInstanceAttribute(ctx, &awsec2.DescribeInstanceAttributeInput{
				InstanceId: aws.String("i-0123456789abcdef0"),
				Attribute:  types.InstanceAttributeNameUserData,
			})
			return wrapErr("runInstances", err)
		})
		return o, wrapErr("runInstances", err)
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}

func runOwnedInstance(ctx context.Context, rt *harness.Runtime, api *awsec2.Client, c cases.Case,
) (cases.Outcome, error) {
	input := &awsec2.RunInstancesInput{
		ImageId: aws.String("ami-0123456789abcdef0"), InstanceType: types.InstanceType(harness.InstanceType),
		MinCount: aws.Int32(1), MaxCount: aws.Int32(1),
	}
	if c.ID == "EC2-RUN-101" {
		input.ImageId = nil
	} else {
		var subnets *awsec2.DescribeSubnetsOutput
		setup, err := rt.Call(ctx, func(ctx context.Context) error {
			var err error
			subnets, err = api.DescribeSubnets(ctx, &awsec2.DescribeSubnetsInput{})
			return wrapErr("select launch subnet", err)
		})
		if err != nil || setup.Code != "" {
			return setup, wrapErr("select launch subnet", err)
		}
		for _, subnet := range subnets.Subnets {
			if subnet.State == types.SubnetStateAvailable && aws.ToString(subnet.SubnetId) != "" {
				input.SubnetId = subnet.SubnetId
				break
			}
		}
		if input.SubnetId == nil {
			return setup, fmt.Errorf("instance setup: no available subnet")
		}
	}
	if c.ID == "EC2-RUN-110" {
		input.BlockDeviceMappings = []types.BlockDeviceMapping{{
			DeviceName: aws.String("/dev/sdf"),
			Ebs:        &types.EbsBlockDevice{VolumeSize: aws.Int32(harness.VolumeGiB), DeleteOnTermination: aws.Bool(true)},
		}}
	}
	if c.ID == "EC2-RUN-201" || c.ID == "EC2-RUN-202" {
		input.ClientToken = aws.String(rt.Name(c.ID))
	}
	var launched *awsec2.RunInstancesOutput
	setup, err := rt.Call(ctx, func(ctx context.Context) error {
		var err error
		launched, err = rt.EC2RunInstances(ctx, input)
		return wrapErr("launch instance", err)
	})
	if err != nil || setup.Code != "" {
		return setup, wrapErr("launch instance", err)
	}
	id := aws.ToString(launched.Instances[0].InstanceId)
	setup.Fields["id"] = id
	switch c.ID {
	case "EC2-RUN-001", "EC2-RUN-101", "EC2-RUN-110":
		return setup, nil
	case "EC2-RUN-201", "EC2-RUN-202":
		if c.ID == "EC2-RUN-202" {
			input.ImageId = aws.String("ami-0000000000000001")
		}
		var second *awsec2.RunInstancesOutput
		out, err := rt.Call(ctx, func(ctx context.Context) error {
			var err error
			second, err = rt.EC2RunInstances(ctx, input)
			return wrapErr("repeat launch", err)
		})
		if err == nil && out.Code == "" {
			out.Fields["same"] = "no"
			if aws.ToString(second.Instances[0].InstanceId) == id {
				out.Fields["same"] = "yes"
			}
		}
		out.Fields["setup"] = "ok"
		return out, wrapErr("repeat launch", err)
	}
	if err := awsec2.NewInstanceRunningWaiter(api).Wait(ctx,
		&awsec2.DescribeInstancesInput{InstanceIds: []string{id}}, harness.OpTimeout); err != nil {
		return setup, fmt.Errorf("wait for running instance: %w", err)
	}
	if c.ID == "EC2-START-001" {
		if _, err := rt.EC2StopInstances(ctx, id); err != nil {
			return cases.Outcome{}, fmt.Errorf("stop before start: %w", err)
		}
		if err := awsec2.NewInstanceStoppedWaiter(api).Wait(ctx,
			&awsec2.DescribeInstancesInput{InstanceIds: []string{id}}, harness.OpTimeout); err != nil {
			return cases.Outcome{}, fmt.Errorf("wait before start: %w", err)
		}
	}
	out, err := rt.Call(ctx, func(ctx context.Context) error {
		switch c.ID {
		case "EC2-DI-001", "EC2-DI-201":
			query := &awsec2.DescribeInstancesInput{InstanceIds: []string{id}}
			if c.ID == "EC2-DI-201" {
				query.Filters = []types.Filter{{Name: aws.String("instance-state-name"), Values: []string{"running"}}}
			}
			described, err := api.DescribeInstances(ctx, query)
			if err != nil {
				return wrapErr("describe launched instance", err)
			}
			if len(described.Reservations) != 1 || len(described.Reservations[0].Instances) != 1 ||
				aws.ToString(described.Reservations[0].Instances[0].InstanceId) != id {
				return fmt.Errorf("DescribeInstances did not return the exact launched instance")
			}
			if c.ID == "EC2-DI-201" && (described.Reservations[0].Instances[0].State == nil ||
				described.Reservations[0].Instances[0].State.Name != types.InstanceStateNameRunning) {
				return fmt.Errorf("DescribeInstances returned an instance outside the state filter")
			}
		case "EC2-DIA-001":
			attr, err := api.DescribeInstanceAttribute(ctx, &awsec2.DescribeInstanceAttributeInput{
				InstanceId: aws.String(id), Attribute: types.InstanceAttributeNameUserData,
			})
			if err != nil {
				return wrapErr("describe instance attribute", err)
			}
			if aws.ToString(attr.InstanceId) != id {
				return fmt.Errorf("DescribeInstanceAttribute returned a different instance")
			}
		case "EC2-STOP-001":
			_, err := rt.EC2StopInstances(ctx, id)
			return wrapErr("stop instance", err)
		case "EC2-START-001":
			_, err := rt.EC2StartInstances(ctx, id)
			return wrapErr("start instance", err)
		case "EC2-TERM-001":
			_, err := rt.EC2TerminateInstances(ctx, id)
			return wrapErr("terminate instance", err)
		}
		return nil
	})
	out.Fields["id"] = id
	if err == nil && out.Code == "" && c.ID == "EC2-TERM-001" {
		// Recently terminated instances remain visible for a short interval.
		// https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeInstances.html
		verifyCtx, cancel := context.WithTimeout(ctx, harness.OpTimeout)
		defer cancel()
		tick := time.NewTicker(50 * time.Millisecond)
		defer tick.Stop()
		for {
			var state *awsec2.DescribeInstancesOutput
			observed, describeErr := rt.Call(verifyCtx, func(ctx context.Context) error {
				var err error
				state, err = api.DescribeInstances(ctx, &awsec2.DescribeInstancesInput{InstanceIds: []string{id}})
				return wrapErr("describe terminated instance", err)
			})
			if describeErr != nil || observed.Code != "" {
				return observed, wrapErr("verify termination", describeErr)
			}
			if len(state.Reservations) == 1 && len(state.Reservations[0].Instances) == 1 {
				inst := state.Reservations[0].Instances[0]
				if aws.ToString(inst.InstanceId) == id && inst.State != nil &&
					inst.State.Name == types.InstanceStateNameTerminated {
					out.Fields["state"] = string(inst.State.Name)
					break
				}
			}
			select {
			case <-verifyCtx.Done():
				return out, nil
			case <-tick.C:
			}
		}
	}
	return out, wrapErr("instance lifecycle", err)
}
