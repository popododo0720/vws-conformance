package iam

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsiam "github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

// MFADevices creates a virtual MFA device through the cleanup gate. IAM has no get for a virtual
// device, so the case observes it with ListVirtualMFADevices filtered to its exact serial number.
func MFADevices() bind.Group {
	return bind.Group{Service: "iam", Name: "mfadevices", Run: runMFADevices, Cases: []cases.Case{{
		ID: "IAM-GAP-005", Service: "iam", Group: "mfadevices", Op: "CreateVirtualMFADevice", Kind: cases.KindGap,
		Suite: cases.SuiteSDK, DocURL: cases.DocIAM("CreateVirtualMFADevice"), Impl: "yes",
		Expect: cases.Expect{HTTP: 200, Patterns: map[string]string{
			"SerialNumber": `^arn:aws:iam::[0-9]{12}:mfa/conf-[0-9]{8}t[0-9]{6}z-[0-9a-f]{6}-mfa-device$`,
			// API_ListVirtualMFADevices: a device not yet enabled for a user is Unassigned.
			"listed": "^unassigned$",
		}},
	}}}
}

func runMFADevices(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	if rt.Clients == nil || rt.Clients.IAM == nil {
		return cases.Outcome{}, fmt.Errorf("iam client is unset")
	}
	if c.ID != "IAM-GAP-005" {
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
	var device *awsiam.CreateVirtualMFADeviceOutput
	o, err := rt.Call(ctx, func(ctx context.Context) error {
		var createErr error
		device, createErr = rt.IAMCreateVirtualMFADevice(ctx,
			&awsiam.CreateVirtualMFADeviceInput{VirtualMFADeviceName: aws.String(rt.Name("mfa-device"))})
		return wrapErr("create device", createErr)
	})
	if err != nil || o.Code != "" {
		return o, wrapErr("runMFADevices", err)
	}
	serial := aws.ToString(device.VirtualMFADevice.SerialNumber)
	o.Fields["SerialNumber"] = serial
	if err := waitListed(ctx, rt, serial); err != nil {
		return o, err
	}
	o.Fields["listed"] = "unassigned"
	return o, nil
}

// waitListed polls until ListVirtualMFADevices shows serial as unassigned: IAM is eventually
// consistent, so a list right after the create can still miss it.
func waitListed(ctx context.Context, rt *harness.Runtime, serial string) error {
	ctx, cancel := context.WithTimeout(ctx, harness.OpTimeout)
	defer cancel()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		listed, err := unassigned(ctx, rt, serial)
		if err != nil {
			return err
		}
		if listed {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for %s in ListVirtualMFADevices: %w", serial, ctx.Err())
		case <-tick.C:
		}
	}
}

func unassigned(ctx context.Context, rt *harness.Runtime, serial string) (bool, error) {
	pages := awsiam.NewListVirtualMFADevicesPaginator(rt.Clients.IAM,
		&awsiam.ListVirtualMFADevicesInput{AssignmentStatus: types.AssignmentStatusTypeUnassigned})
	for pages.HasMorePages() {
		out, err := pages.NextPage(ctx)
		if err != nil {
			return false, wrapErr("list virtual mfa devices", err)
		}
		for _, d := range out.VirtualMFADevices {
			if aws.ToString(d.SerialNumber) == serial {
				return true, nil
			}
		}
	}
	return false, nil
}
