package ebs

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

func Volumes() bind.Group {
	return bind.Group{Service: "ebs", Name: "volumes", Run: runVol, Cases: []cases.Case{
		{ID: "EBS-VOL-001", Service: "ebs", Group: "volumes", Op: "CreateVolume", Kind: cases.KindHappy, Suite: cases.SuiteSDK, DocURL: cases.DocEC2("CreateVolume"), Expect: cases.Expect{HTTP: 200, Patterns: map[string]string{"id": expect.VolumeID, "size": "1"}}, Impl: "yes"},
		{ID: "EBS-VOL-002", Service: "ebs", Group: "volumes", Op: "DescribeVolumes", Kind: cases.KindHappy, Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DescribeVolumes"), Expect: cases.Expect{HTTP: 200}, Impl: "yes"},
		{ID: "EBS-VOL-003", Service: "ebs", Group: "volumes", Op: "AttachVolume", Kind: cases.KindHappy, Suite: cases.SuiteSDK, DocURL: cases.DocEC2("AttachVolume"), Expect: cases.Expect{HTTP: 200}, Impl: "yes", Slow: true},
		{ID: "EBS-VOL-004", Service: "ebs", Group: "volumes", Op: "ModifyVolume", Kind: cases.KindHappy, Suite: cases.SuiteSDK, DocURL: cases.DocEC2("ModifyVolume"), Expect: cases.Expect{HTTP: 200}, Impl: "yes"},
		{ID: "EBS-VOL-005", Service: "ebs", Group: "volumes", Op: "DeleteVolume", Kind: cases.KindHappy, Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DeleteVolume"), Expect: cases.Expect{HTTP: 200}, Impl: "yes"},
		{ID: "EBS-VOL-101", Service: "ebs", Group: "volumes", Op: "DeleteVolume", Kind: cases.KindError, Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{HTTP: 400, Code: "InvalidVolume.NotFound"}, Impl: "yes"},
		{ID: "EBS-VOL-102", Service: "ebs", Group: "volumes", Op: "DeleteVolume", Kind: cases.KindError, Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{HTTP: 400, Code: "VolumeInUse"}, Impl: "yes", Slow: true},
		{ID: "EBS-VOL-103", Service: "ebs", Group: "volumes", Op: "CreateVolume", Kind: cases.KindError, Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{HTTP: 400, Code: "InvalidParameterValue"}, Impl: "yes"},
		{ID: "EBS-VOL-104", Service: "ebs", Group: "volumes", Op: "DescribeVolumes", Kind: cases.KindError, Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{HTTP: 400, Code: "InvalidVolume.NotFound"}, Impl: "yes"},
		{ID: "EBS-VOL-105", Service: "ebs", Group: "volumes", Op: "AttachVolume", Kind: cases.KindError, Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{HTTP: 400, Code: "InvalidVolume.NotFound"}, Impl: "yes"},
		{ID: "EBS-VOL-106", Service: "ebs", Group: "volumes", Op: "ModifyVolume", Kind: cases.KindError, Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{HTTP: 400, Code: "InvalidVolume.NotFound"}, Impl: "yes"},
	}}
}

func createVolume(ctx context.Context, api *awsec2.Client, in *awsec2.CreateVolumeInput) (
	*awsec2.CreateVolumeOutput, error,
) {
	out, err := api.CreateVolume(ctx, in)
	if err == nil || len(in.TagSpecifications) == 0 || !harness.UnsupportedParameter(err) {
		return out, err
	}
	bare := *in
	bare.TagSpecifications = nil
	return api.CreateVolume(ctx, &bare)
}

func VolumeRequest(zone string, size int32) *awsec2.CreateVolumeInput {
	return &awsec2.CreateVolumeInput{
		AvailabilityZone: aws.String(zone),
		Size:             aws.Int32(size),
		VolumeType:       types.VolumeTypeGp2,
	}
}

func runVol(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	api, err := client(rt)
	if err != nil {
		return cases.Outcome{}, err
	}
	switch c.ID {
	case "EBS-VOL-001":
		in := VolumeRequest("vws-1a", harness.VolumeGiB)
		in.ClientToken = aws.String(rt.Token("vol1"))
		in.TagSpecifications = rt.TagSpec(types.ResourceTypeVolume)
		var id, size string
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			out, err := createVolume(ctx, api, in)
			if err == nil {
				id = aws.ToString(out.VolumeId)
				size = fmt.Sprint(aws.ToInt32(out.Size))
			}
			return err
		})
		o.Fields["id"] = id
		o.Fields["size"] = size
		trackVol(rt, api, id)
		return o, err
	case "EBS-VOL-002":
		id, err := makeVol(ctx, rt, api, "vol2")
		if err != nil {
			return cases.Outcome{}, err
		}
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeVolumes(ctx, &awsec2.DescribeVolumesInput{VolumeIds: []string{id}})
			return err
		})
	case "EBS-VOL-004":
		id, err := makeVol(ctx, rt, api, "vol4")
		if err != nil {
			return cases.Outcome{}, err
		}
		_, _ = api.DescribeVolumes(ctx, &awsec2.DescribeVolumesInput{VolumeIds: []string{id}})
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.ModifyVolume(ctx, &awsec2.ModifyVolumeInput{VolumeId: aws.String(id), Size: aws.Int32(2)})
			return err
		})
	case "EBS-VOL-005":
		id, err := makeVol(ctx, rt, api, "vol5")
		if err != nil {
			return cases.Outcome{}, err
		}
		_, _ = api.DescribeVolumes(ctx, &awsec2.DescribeVolumesInput{VolumeIds: []string{id}})
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DeleteVolume(ctx, &awsec2.DeleteVolumeInput{VolumeId: aws.String(id)})
			return err
		})
	case "EBS-VOL-101", "EBS-VOL-104", "EBS-VOL-105", "EBS-VOL-106":
		id := "vol-0123456789abcdef0"
		return rt.Call(ctx, func(ctx context.Context) error {
			var err error
			switch c.ID {
			case "EBS-VOL-104":
				_, err = api.DescribeVolumes(ctx, &awsec2.DescribeVolumesInput{VolumeIds: []string{id}})
			case "EBS-VOL-105":
				_, err = api.AttachVolume(ctx, &awsec2.AttachVolumeInput{VolumeId: aws.String(id), InstanceId: aws.String("i-0123456789abcdef0"), Device: aws.String("/dev/sdf")})
			case "EBS-VOL-106":
				_, err = api.ModifyVolume(ctx, &awsec2.ModifyVolumeInput{VolumeId: aws.String(id), Size: aws.Int32(2)})
			default:
				_, err = api.DeleteVolume(ctx, &awsec2.DeleteVolumeInput{VolumeId: aws.String(id)})
			}
			return err
		})
	case "EBS-VOL-103":
		in := VolumeRequest("vws-1a", 0)
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.CreateVolume(ctx, in)
			return err
		})
	case "EBS-VOL-003", "EBS-VOL-102":
		_, subnet, err := rt.EnsureNet(ctx)
		if err != nil {
			return cases.Outcome{}, err
		}
		run, err := api.RunInstances(ctx, &awsec2.RunInstancesInput{
			ImageId: aws.String("ami-0123456789abcdef0"), InstanceType: types.InstanceType(harness.InstanceType),
			MinCount: aws.Int32(1), MaxCount: aws.Int32(1), SubnetId: aws.String(subnet),
			ClientToken: aws.String(rt.Token("volinst")), TagSpecifications: rt.TagSpec(types.ResourceTypeInstance),
		})
		if err != nil {
			return cases.Outcome{}, fmt.Errorf("run: %w", err)
		}
		inst := aws.ToString(run.Instances[0].InstanceId)
		_ = rt.Track("instance", inst, rt.Name("volinst"))
		rt.Cleanup(func(ctx context.Context) error {
			_, err := api.TerminateInstances(ctx, &awsec2.TerminateInstancesInput{InstanceIds: []string{inst}})
			return harness.IgnoreCleanup(err)
		})
		_, _ = api.DescribeInstances(ctx, &awsec2.DescribeInstancesInput{InstanceIds: []string{inst}})
		vol, err := makeVol(ctx, rt, api, "attach")
		if err != nil {
			return cases.Outcome{}, err
		}
		_, _ = api.DescribeVolumes(ctx, &awsec2.DescribeVolumesInput{VolumeIds: []string{vol}})
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.AttachVolume(ctx, &awsec2.AttachVolumeInput{VolumeId: aws.String(vol), InstanceId: aws.String(inst), Device: aws.String("/dev/sdf")})
			return err
		})
		if c.ID == "EBS-VOL-003" || err != nil {
			return o, err
		}
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DeleteVolume(ctx, &awsec2.DeleteVolumeInput{VolumeId: aws.String(vol)})
			return err
		})
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}

func client(rt *harness.Runtime) (*awsec2.Client, error) {
	if rt.Clients == nil || rt.Clients.EC2 == nil {
		return nil, fmt.Errorf("ec2 client is unset")
	}
	return rt.Clients.EC2, nil
}

func makeVol(ctx context.Context, rt *harness.Runtime, api *awsec2.Client, token string) (string, error) {
	in := VolumeRequest("vws-1a", harness.VolumeGiB)
	in.ClientToken = aws.String(rt.Token(token))
	in.TagSpecifications = rt.TagSpec(types.ResourceTypeVolume)
	out, err := createVolume(ctx, api, in)
	if err != nil {
		return "", fmt.Errorf("create volume: %w", err)
	}
	id := aws.ToString(out.VolumeId)
	trackVol(rt, api, id)
	if err := waitAvailable(ctx, api, id); err != nil {
		return "", err
	}
	return id, nil
}

func waitAvailable(ctx context.Context, api *awsec2.Client, id string) error {
	ctx, cancel := context.WithTimeout(ctx, harness.OpTimeout)
	defer cancel()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	var status string
	for {
		out, err := api.DescribeVolumes(ctx, &awsec2.DescribeVolumesInput{VolumeIds: []string{id}})
		if err != nil {
			return fmt.Errorf("describe %s: %w", id, err)
		}
		if len(out.Volumes) > 0 {
			status = string(out.Volumes[0].State)
		}
		switch types.VolumeState(status) {
		case types.VolumeStateAvailable, types.VolumeStateInUse:
			return nil
		case types.VolumeStateError, types.VolumeStateDeleted, types.VolumeStateDeleting:
			return fmt.Errorf("volume %s is %s", id, status)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("volume %s stayed %s: %w", id, status, ctx.Err())
		case <-tick.C:
		}
	}
}

func trackVol(rt *harness.Runtime, api *awsec2.Client, id string) {
	if id == "" {
		return
	}
	rt.Cleanup(func(ctx context.Context) error {
		_, err := api.DeleteVolume(ctx, &awsec2.DeleteVolumeInput{VolumeId: aws.String(id)})
		return harness.IgnoreCleanup(err)
	})
	_ = rt.Track("volume", id, rt.Name("vol"))
}
