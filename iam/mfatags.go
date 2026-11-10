package iam

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsiam "github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func mfaTagCase(id, op, kind string, exp cases.Expect) cases.Case {
	return cases.Case{
		ID: id, Service: "iam", Group: "mfatags", Op: op, Kind: kind, Suite: cases.SuiteSDK,
		DocURL: cases.DocIAM(op), Expect: exp, Impl: "yes",
	}
}

func setupOK(fields ...string) map[string]string {
	p := map[string]string{"setup": "^ok$"}
	for i := 0; i+1 < len(fields); i += 2 {
		p[fields[i]] = fields[i+1]
	}
	return p
}

// MFATags exercises the tag actions on a virtual MFA device each case creates through the
// cleanup gate. A device with a refused setup fails its case: setup=ok is always required.
func MFATags() bind.Group {
	return bind.Group{Service: "iam", Name: "mfatags", Run: runMFATags, Cases: []cases.Case{
		mfaTagCase("IAM-MTAG-001", "TagMFADevice", cases.KindTags,
			cases.Expect{HTTP: 200, Patterns: setupOK("tag_state", "^present$")}),
		mfaTagCase("IAM-MTAG-002", "UntagMFADevice", cases.KindTags,
			cases.Expect{HTTP: 200, Patterns: setupOK("tag_state", "^absent$")}),
		mfaTagCase("IAM-MTAG-003", "ListMFADeviceTags", cases.KindHappy,
			cases.Expect{HTTP: 200, Patterns: setupOK("tag_state", "^present$")}),
		mfaTagCase("IAM-MTAG-101", "TagMFADevice", cases.KindError,
			cases.Expect{HTTP: 404, Code: "NoSuchEntity", Patterns: setupOK()}),
		mfaTagCase("IAM-MTAG-102", "UntagMFADevice", cases.KindError,
			cases.Expect{HTTP: 404, Code: "NoSuchEntity", Patterns: setupOK()}),
		mfaTagCase("IAM-MTAG-103", "ListMFADeviceTags", cases.KindError,
			cases.Expect{HTTP: 404, Code: "NoSuchEntity"}),
		mfaTagCase("IAM-MTAG-104", "TagMFADevice", cases.KindError,
			cases.Expect{HTTP: 400, Code: "InvalidInput", Patterns: setupOK()}),
		mfaTagCase("IAM-MTAG-105", "TagMFADevice", cases.KindError,
			cases.Expect{HTTP: 400, Code: "InvalidInput", Patterns: setupOK()}),
		mfaTagCase("IAM-MTAG-106", "TagMFADevice", cases.KindError,
			cases.Expect{HTTP: 409, Code: "LimitExceeded", Patterns: setupOK()}),
		mfaTagCase("IAM-MTAG-107", "TagMFADevice", cases.KindError,
			cases.Expect{HTTP: 400, Code: "InvalidInput", Patterns: setupOK()}),
	}}
}

func runMFATags(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	if rt.Clients == nil || rt.Clients.IAM == nil {
		return cases.Outcome{}, fmt.Errorf("iam client is unset")
	}
	if c.ID == "IAM-MTAG-103" {
		if rt.Cfg.AccountID == "" {
			return cases.Outcome{}, fmt.Errorf("account ID is unset")
		}
		serial := fmt.Sprintf("arn:aws:iam::%s:mfa/%s", rt.Cfg.AccountID, rt.Name("mtag"))
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := rt.Clients.IAM.ListMFADeviceTags(ctx, &awsiam.ListMFADeviceTagsInput{SerialNumber: aws.String(serial)})
			return wrapErr("list tags", err)
		})
		return o, wrapErr("runMFATags", err)
	}
	var device *awsiam.CreateVirtualMFADeviceOutput
	setup, err := rt.Call(ctx, func(ctx context.Context) error {
		var createErr error
		device, createErr = rt.IAMCreateVirtualMFADevice(ctx,
			&awsiam.CreateVirtualMFADeviceInput{VirtualMFADeviceName: aws.String(rt.Name("mtag"))})
		return wrapErr("create device", createErr)
	})
	if err != nil {
		return setup, fmt.Errorf("mfa tag device: %w", err)
	}
	if setup.Code != "" {
		setup.Fields["setup"] = "refused"
		return setup, nil
	}
	serial := aws.ToString(device.VirtualMFADevice.SerialNumber)
	o, err := exerciseMFATags(ctx, rt, c.ID, serial)
	if err != nil {
		return o, wrapErr("runMFATags", err)
	}
	o.Fields["setup"] = "ok"
	return o, nil
}

func exerciseMFATags(ctx context.Context, rt *harness.Runtime, id, serial string) (cases.Outcome, error) {
	name := types.Tag{Key: aws.String("Name"), Value: aws.String(rt.Name("tag"))}
	tagCall := func(tags ...types.Tag) (cases.Outcome, error) {
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := rt.IAMTagMFADevice(ctx, serial, tags)
			return wrapErr("tag device", err)
		})
	}
	untagCall := func() (cases.Outcome, error) {
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := rt.IAMUntagMFADevice(ctx, serial, []string{"Name"})
			return wrapErr("untag device", err)
		})
	}
	switch id {
	case "IAM-MTAG-001":
		o, err := tagCall(name)
		return settle(ctx, rt, serial, "present", o, err)
	case "IAM-MTAG-002", "IAM-MTAG-003":
		if _, err := rt.IAMTagMFADevice(ctx, serial, []types.Tag{name}); err != nil {
			return cases.Outcome{}, fmt.Errorf("seed tag: %w", err)
		}
		if id == "IAM-MTAG-002" {
			o, err := untagCall()
			return settle(ctx, rt, serial, "absent", o, err)
		}
		if err := waitTag(ctx, rt, serial, "present", func(ctx context.Context) (string, error) {
			return tagState(ctx, rt, serial)
		}); err != nil {
			return cases.Outcome{}, err
		}
		var state string
		listed, err := rt.Call(ctx, func(ctx context.Context) error {
			var listErr error
			state, listErr = tagState(ctx, rt, serial)
			return listErr
		})
		if err != nil {
			return listed, wrapErr("list tags", err)
		}
		if listed.Code == "" {
			listed.Fields["tag_state"] = state
		}
		return listed, nil
	case "IAM-MTAG-101", "IAM-MTAG-102":
		if _, err := rt.IAMDeleteVirtualMFADevice(ctx, serial); err != nil {
			return cases.Outcome{}, fmt.Errorf("delete device: %w", err)
		}
		if id == "IAM-MTAG-101" {
			return tagCall(name)
		}
		return untagCall()
	case "IAM-MTAG-104":
		return tagCall(types.Tag{Key: aws.String("bad#key"), Value: aws.String("v")})
	case "IAM-MTAG-105":
		return tagCall(types.Tag{Key: aws.String("Name"), Value: aws.String(strings.Repeat("x", 257))})
	case "IAM-MTAG-106":
		// The device already holds the run tag, so 50 more is one past the limit of 50.
		tags := make([]types.Tag, 50)
		for i := range tags {
			tags[i] = types.Tag{Key: aws.String("k" + strconv.Itoa(i)), Value: aws.String("v")}
		}
		return tagCall(tags...)
	case "IAM-MTAG-107":
		return tagCall(types.Tag{Key: aws.String("aws:conformance"), Value: aws.String("v")})
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", id)
	}
}

// settle records the tag state a successful write leaves, read back once IAM shows it.
func settle(ctx context.Context, rt *harness.Runtime, serial, wanted string, o cases.Outcome, err error,
) (cases.Outcome, error) {
	if err != nil || o.Code != "" {
		return o, err
	}
	if err := waitTag(ctx, rt, serial, wanted, func(ctx context.Context) (string, error) {
		return tagState(ctx, rt, serial)
	}); err != nil {
		return o, err
	}
	o.Fields["tag_state"] = wanted
	return o, nil
}

// waitTag polls read until it answers wanted: IAM is eventually consistent, so a read right
// after a write can still show the old tags.
func waitTag(ctx context.Context, rt *harness.Runtime, serial, wanted string,
	read func(context.Context) (string, error),
) error {
	ctx, cancel := context.WithTimeout(ctx, harness.OpTimeout)
	defer cancel()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		state, err := read(ctx)
		if err != nil {
			return fmt.Errorf("read tags of %s: %w", serial, err)
		}
		if state == wanted {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for tag state %s, last %s: %w", wanted, state, ctx.Err())
		case <-tick.C:
		}
	}
}

// tagState reads the device's tags: "present" when Name has this run's value, "absent" when
// there is no Name tag, and an error when the run tag no longer proves ownership.
func tagState(ctx context.Context, rt *harness.Runtime, serial string) (string, error) {
	out, err := rt.Clients.IAM.ListMFADeviceTags(ctx, &awsiam.ListMFADeviceTagsInput{SerialNumber: aws.String(serial)})
	if err != nil {
		return "", wrapErr("list tags", err)
	}
	state, owner := "absent", ""
	for _, t := range out.Tags {
		switch aws.ToString(t.Key) {
		case harness.RunTagKey:
			owner = aws.ToString(t.Value)
		case "Name":
			state = "wrong-value"
			if aws.ToString(t.Value) == rt.Name("tag") {
				state = "present"
			}
		}
	}
	if owner != rt.RunID() {
		return "", fmt.Errorf("tag mutation changed cleanup ownership")
	}
	return state, nil
}
