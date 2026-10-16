package ec2

import (
	"context"
	"errors"
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
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2("RunInstances"), Expect: cases.Expect{
					HTTP:     200,
					Patterns: map[string]string{"id": expect.InstanceID},
				}, Impl: "yes", Slow: true,
			},
			{
				ID: "EC2-DI-001", Service: "ec2", Group: "instances", Op: "DescribeInstances", Kind: cases.KindHappy,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DescribeInstances"), Expect: cases.Expect{
					HTTP:     200,
					Patterns: map[string]string{"id": expect.InstanceID},
				}, Impl: "yes", Slow: true,
			},
			{
				ID: "EC2-DIA-001", Service: "ec2", Group: "instances", Op: "DescribeInstanceAttribute", Kind: cases.KindHappy,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DescribeInstanceAttribute"), Expect: cases.Expect{HTTP: 200},
				Impl: "yes", Slow: true,
			},
			{
				ID: "EC2-STOP-001", Service: "ec2", Group: "instances", Op: "StopInstances", Kind: cases.KindHappy,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2("StopInstances"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
				Slow: true,
			},
			{
				ID: "EC2-START-001", Service: "ec2", Group: "instances", Op: "StartInstances", Kind: cases.KindHappy,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2("StartInstances"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
				Slow: true,
			},
			{
				ID: "EC2-TERM-001", Service: "ec2", Group: "instances", Op: "TerminateInstances", Kind: cases.KindHappy,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2("TerminateInstances"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
				Slow: true,
			},
			{
				ID: "EC2-RUN-101", Service: "ec2", Group: "instances", Op: "RunInstances", Kind: cases.KindError,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{HTTP: 400, Code: "MissingParameter"},
				Impl: "yes",
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
				ID: "EC2-RUN-110", Service: "ec2", Group: "instances", Op: "RunInstances", Kind: cases.KindHappy,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2("RunInstances"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
				Slow: true,
			},
			{
				ID: "EC2-RUN-201", Service: "ec2", Group: "instances", Op: "RunInstances", Kind: cases.KindIdempotency,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2("RunInstances"), Expect: cases.Expect{
					HTTP:     200,
					Patterns: map[string]string{"same": "yes"},
				}, Impl: "yes", Slow: true,
			},
			{
				ID: "EC2-RUN-202", Service: "ec2", Group: "instances", Op: "RunInstances", Kind: cases.KindIdempotency,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{
					HTTP: 400,
					Code: "IdempotentParameterMismatch",
				}, Impl: "yes", Slow: true,
			},
			{
				ID: "EC2-DI-201", Service: "ec2", Group: "instances", Op: "DescribeInstances", Kind: cases.KindFilter,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DescribeInstances"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
				Slow: true,
			},
			{
				ID: "EC2-STOP-101", Service: "ec2", Group: "instances", Op: "StopInstances", Kind: cases.KindError,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{
					HTTP: 400,
					Code: "InvalidInstanceID.NotFound",
				}, Skip: cases.SkipUnsafeFixedID, Impl: "yes",
			},
			{
				ID: "EC2-START-101", Service: "ec2", Group: "instances", Op: "StartInstances", Kind: cases.KindError,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{
					HTTP: 400,
					Code: "InvalidInstanceID.NotFound",
				}, Skip: cases.SkipUnsafeFixedID, Impl: "yes",
			},
			{
				ID: "EC2-TERM-101", Service: "ec2", Group: "instances", Op: "TerminateInstances", Kind: cases.KindError,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{
					HTTP: 400,
					Code: "InvalidInstanceID.NotFound",
				}, Skip: cases.SkipUnsafeFixedID, Impl: "yes",
			},
			{
				ID: "EC2-DIA-101", Service: "ec2", Group: "instances", Op: "DescribeInstanceAttribute", Kind: cases.KindError,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{
					HTTP: 400,
					Code: "InvalidInstanceID.NotFound",
				}, Impl: "yes",
			},
		},
	}
}

func LaunchRequest(rt *harness.Runtime, subnet, token string) *awsec2.RunInstancesInput {
	in := &awsec2.RunInstancesInput{
		ImageId:           aws.String("ami-0123456789abcdef0"),
		InstanceType:      types.InstanceType(harness.InstanceType),
		MinCount:          aws.Int32(1),
		MaxCount:          aws.Int32(1),
		SubnetId:          aws.String(subnet),
		TagSpecifications: rt.TagSpec(types.ResourceTypeInstance),
	}
	if token != "" {
		in.ClientToken = aws.String(token)
	}
	return in
}

func runInstances(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	if rt.Clients == nil || rt.Clients.EC2 == nil {
		return cases.Outcome{}, fmt.Errorf("ec2 client is unset")
	}
	api := rt.Clients.EC2
	switch c.ID {
	case "EC2-RUN-101":
		var launched *awsec2.RunInstancesOutput
		callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
			var err error
			launched, err = api.RunInstances(ctx, &awsec2.RunInstancesInput{MinCount: aws.Int32(1), MaxCount: aws.Int32(1)})
			return wrapErr("runInstances", err)
		})
		if err := keepLaunched(rt, api, launched, "run101"); err != nil {
			return callOut, wrapErr("runInstances", err)
		}
		if harness.SetupReady(callOut, callErr) && (launched == nil || len(launched.Instances) == 0) {
			return callOut, fmt.Errorf("run: success did not identify an instance")
		}
		return callOut, wrapErr("runInstances", callErr)
	case "EC2-DI-101":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DescribeInstances(ctx, &awsec2.DescribeInstancesInput{InstanceIds: []string{"i-0123456789abcdef0"}})
				return wrapErr("runInstances", err)
			})
			return callOut, wrapErr("runInstances", callErr)
		}
	case "EC2-DI-102":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DescribeInstances(ctx, &awsec2.DescribeInstancesInput{InstanceIds: []string{"i-xyz"}})
				return wrapErr("runInstances", err)
			})
			return callOut, wrapErr("runInstances", callErr)
		}
	case "EC2-STOP-101", "EC2-START-101", "EC2-TERM-101":
		return cases.Outcome{}, errors.New(cases.SkipUnsafeFixedID)
	case "EC2-DIA-101":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DescribeInstanceAttribute(ctx, &awsec2.DescribeInstanceAttributeInput{
					InstanceId: aws.String("i-0123456789abcdef0"),
					Attribute:  types.InstanceAttributeNameUserData,
				})
				return wrapErr("runInstances", err)
			})
			return callOut, wrapErr("runInstances", callErr)
		}
	}
	_, subnet, err := rt.EnsureNet(ctx)
	if err != nil {
		return cases.Outcome{}, wrapErr("runInstances", err)
	}
	switch c.ID {
	case "EC2-RUN-001":
		return runOne(ctx, rt, api, subnet, rt.Token("run1"))
	case "EC2-DI-001":
		id, err := launch(ctx, rt, api, subnet, rt.Token("di"))
		if err != nil {
			return cases.Outcome{}, err
		}
		var out *awsec2.DescribeInstancesOutput
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			var callErr error
			out, callErr = api.DescribeInstances(ctx, &awsec2.DescribeInstancesInput{InstanceIds: []string{id}})
			return wrapErr("runInstances", callErr)
		})
		if err != nil {
			return o, wrapErr("runInstances", err)
		}
		got, err := describedInstance(out, id, o.HTTP)
		if err != nil {
			return o, err
		}
		if got != "" {
			o.Fields["id"] = got
		}
		return o, nil
	case "EC2-DIA-001":
		id, err := launch(ctx, rt, api, subnet, rt.Token("dia"))
		if err != nil {
			return cases.Outcome{}, err
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DescribeInstanceAttribute(ctx, &awsec2.DescribeInstanceAttributeInput{
					InstanceId: aws.String(id),
					Attribute:  types.InstanceAttributeNameUserData,
				})
				return wrapErr("runInstances", err)
			})
			return callOut, wrapErr("runInstances", callErr)
		}
	case "EC2-STOP-001":
		id, err := launch(ctx, rt, api, subnet, rt.Token("stop"))
		if err != nil {
			return cases.Outcome{}, err
		}
		if _, err := api.DescribeInstances(ctx, &awsec2.DescribeInstancesInput{InstanceIds: []string{id}}); err != nil {
			return cases.Outcome{}, fmt.Errorf("describe instance: %w", err)
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.StopInstances(ctx, &awsec2.StopInstancesInput{InstanceIds: []string{id}})
				return wrapErr("runInstances", err)
			})
			return callOut, wrapErr("runInstances", callErr)
		}
	case "EC2-START-001":
		id, err := launch(ctx, rt, api, subnet, rt.Token("start"))
		if err != nil {
			return cases.Outcome{}, err
		}
		if _, err := api.DescribeInstances(ctx, &awsec2.DescribeInstancesInput{InstanceIds: []string{id}}); err != nil {
			return cases.Outcome{}, fmt.Errorf("describe instance: %w", err)
		}
		if _, err := api.StopInstances(ctx, &awsec2.StopInstancesInput{InstanceIds: []string{id}}); err != nil {
			return cases.Outcome{}, fmt.Errorf("stop: %w", err)
		}
		if err := waitStopped(ctx, api, id); err != nil {
			return cases.Outcome{}, err
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.StartInstances(ctx, &awsec2.StartInstancesInput{InstanceIds: []string{id}})
				return wrapErr("runInstances", err)
			})
			return callOut, wrapErr("runInstances", callErr)
		}
	case "EC2-TERM-001":
		id, err := launch(ctx, rt, api, subnet, rt.Token("term"))
		if err != nil {
			return cases.Outcome{}, err
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.TerminateInstances(ctx, &awsec2.TerminateInstancesInput{InstanceIds: []string{id}})
				return wrapErr("runInstances", err)
			})
			return callOut, wrapErr("runInstances", callErr)
		}
	case "EC2-RUN-110":
		var launched *awsec2.RunInstancesOutput
		callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
			var err error
			launched, err = api.RunInstances(ctx, LaunchRequest(rt, subnet, ""))
			return wrapErr("runInstances", err)
		})
		if err := keepLaunched(rt, api, launched, "run110"); err != nil {
			return callOut, wrapErr("runInstances", err)
		}
		return callOut, wrapErr("runInstances", callErr)
	case "EC2-RUN-201":
		token := rt.Token("idem")
		first, err := api.RunInstances(ctx, LaunchRequest(rt, subnet, token))
		if err != nil {
			return cases.Outcome{}, fmt.Errorf("first run: %w", err)
		}
		id1 := aws.ToString(first.Instances[0].InstanceId)
		if err := rememberInstance(rt, api, first.Instances[0], "idem"); err != nil {
			return cases.Outcome{}, err
		}
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.RunInstances(ctx, LaunchRequest(rt, subnet, token))
			return wrapErr("runInstances", err)
		})
		if err != nil {
			return o, wrapErr("runInstances", err)
		}
		second, err := api.RunInstances(ctx, LaunchRequest(rt, subnet, token))
		if err != nil {
			return o, wrapErr("runInstances", err)
		}
		id2 := aws.ToString(second.Instances[0].InstanceId)
		o.Fields["id"] = id2
		if id1 == id2 {
			o.Fields["same"] = "yes"
		} else {
			o.Fields["same"] = "no"
		}
		return o, nil
	case "EC2-RUN-202":
		token := rt.Token("mismatch")
		in := LaunchRequest(rt, subnet, token)
		first, err := api.RunInstances(ctx, in)
		if err != nil {
			return cases.Outcome{}, fmt.Errorf("first run: %w", err)
		}
		if err := rememberInstance(rt, api, first.Instances[0], "i"); err != nil {
			return cases.Outcome{}, err
		}
		in.ImageId = aws.String("ami-0000000000000001")
		var mismatched *awsec2.RunInstancesOutput
		callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
			var err error
			mismatched, err = api.RunInstances(ctx, in)
			return wrapErr("runInstances", err)
		})
		if err := keepLaunched(rt, api, mismatched, "mismatch"); err != nil {
			return callOut, wrapErr("runInstances", err)
		}
		return callOut, wrapErr("runInstances", callErr)
	case "EC2-DI-201":
		if _, err := launch(ctx, rt, api, subnet, rt.Token("filt")); err != nil {
			return cases.Outcome{}, err
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DescribeInstances(ctx, &awsec2.DescribeInstancesInput{Filters: []types.Filter{{
					Name: aws.String("instance-state-name"), Values: []string{"pending", "running"},
				}}})
				return wrapErr("runInstances", err)
			})
			return callOut, wrapErr("runInstances", callErr)
		}
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}

func describedInstance(out *awsec2.DescribeInstancesOutput, launched string, status int) (string, error) {
	if status < 200 || status >= 300 {
		return "", nil
	}
	got := ""
	if out != nil {
		for _, res := range out.Reservations {
			for _, inst := range res.Instances {
				if id := aws.ToString(inst.InstanceId); id != "" {
					got = id
					break
				}
			}
			if got != "" {
				break
			}
		}
	}
	if got == "" || got != launched {
		return "", fmt.Errorf("DescribeInstances returned %q, launched %s", got, launched)
	}
	return got, nil
}

func runOne(ctx context.Context, rt *harness.Runtime, api *awsec2.Client, subnet, token string) (cases.Outcome, error) {
	var id string
	var launched types.Instance
	o, err := rt.Call(ctx, func(ctx context.Context) error {
		out, err := api.RunInstances(ctx, LaunchRequest(rt, subnet, token))
		if err == nil && len(out.Instances) > 0 {
			launched = out.Instances[0]
			id = aws.ToString(launched.InstanceId)
		}
		return wrapErr("runOne", err)
	})
	o.Fields["id"] = id
	if id != "" {
		if err := noteInstanceDevices(rt, launched); err != nil {
			return o, wrapErr("runOne", err)
		}
		if err := rt.Track("instance", id, rt.Name("i")); err != nil {
			return o, wrapErr("runOne", err)
		}
		rt.HoldEC2(api)
		rt.Cleanup(func(ctx context.Context) error {
			return rt.TerminateOwned(ctx, id)
		})
	}
	return o, wrapErr("runOne", err)
}

func keepLaunched(rt *harness.Runtime, api *awsec2.Client, out *awsec2.RunInstancesOutput, suffix string) error {
	if out == nil || len(out.Instances) == 0 {
		return nil
	}
	return rememberInstance(rt, api, out.Instances[0], suffix)
}

func rememberInstance(rt *harness.Runtime, api *awsec2.Client, inst types.Instance, suffix string) error {
	id := aws.ToString(inst.InstanceId)
	if id == "" {
		return fmt.Errorf("run: empty id")
	}
	if err := noteInstanceDevices(rt, inst); err != nil {
		return err
	}
	if err := rt.Track("instance", id, rt.Name(suffix)); err != nil {
		return wrapErr("rememberInstance", err)
	}
	rt.HoldEC2(api)
	rt.Cleanup(func(ctx context.Context) error {
		return rt.TerminateOwned(ctx, id)
	})
	return nil
}

func launch(ctx context.Context, rt *harness.Runtime, api *awsec2.Client, subnet, token string) (string, error) {
	out, err := api.RunInstances(ctx, LaunchRequest(rt, subnet, token))
	if err != nil {
		return "", fmt.Errorf("run: %w", err)
	}
	if err := noteInstanceDevices(rt, out.Instances[0]); err != nil {
		return "", wrapErr("launch", err)
	}
	id := aws.ToString(out.Instances[0].InstanceId)
	if err := rt.Track("instance", id, rt.Name("i")); err != nil {
		return "", wrapErr("launch", err)
	}
	rt.HoldEC2(api)
	rt.Cleanup(func(ctx context.Context) error {
		return rt.TerminateOwned(ctx, id)
	})
	if err := waitRunning(ctx, api, id); err != nil {
		return "", err
	}
	return id, nil
}

func noteInstanceDevices(rt *harness.Runtime, inst types.Instance) error {
	var err error
	for _, block := range inst.BlockDeviceMappings {
		if block.Ebs == nil {
			continue
		}
		if trackErr := rt.NoteDevice(aws.ToString(block.Ebs.VolumeId)); trackErr != nil && err == nil {
			err = trackErr
		}
	}
	return err
}

func waitRunning(ctx context.Context, api *awsec2.Client, id string) error {
	return waitInstance(ctx, api, id, types.InstanceStateNameRunning, types.InstanceStateNameStopped)
}

func waitStopped(ctx context.Context, api *awsec2.Client, id string) error {
	return waitInstance(ctx, api, id, types.InstanceStateNameStopped)
}

func waitInstance(ctx context.Context, api *awsec2.Client, id string, ready ...types.InstanceStateName) error {
	ctx, cancel := context.WithTimeout(ctx, harness.OpTimeout)
	defer cancel()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	var state string
	for {
		out, err := api.DescribeInstances(ctx, &awsec2.DescribeInstancesInput{InstanceIds: []string{id}})
		if err != nil {
			return fmt.Errorf("describe %s: %w", id, err)
		}
		state = instanceState(out)
		named := types.InstanceStateName(state)
		for _, want := range ready {
			if named == want {
				return nil
			}
		}
		switch named {
		case types.InstanceStateNameTerminated, types.InstanceStateNameShuttingDown:
			return fmt.Errorf("instance %s is %s", id, state)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("instance %s stayed %s: %w", id, state, ctx.Err())
		case <-tick.C:
		}
	}
}

func instanceState(out *awsec2.DescribeInstancesOutput) string {
	if out == nil {
		return ""
	}
	for _, res := range out.Reservations {
		for _, in := range res.Instances {
			if in.State != nil {
				return string(in.State.Name)
			}
		}
	}
	return ""
}
