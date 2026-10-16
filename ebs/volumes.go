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
		{
			ID: "EBS-VOL-001", Service: "ebs", Group: "volumes", Op: "CreateVolume", Kind: cases.KindHappy,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2("CreateVolume"), Expect: cases.Expect{
				HTTP:     200,
				Patterns: map[string]string{"id": expect.VolumeID, "size": "1"},
			}, Impl: "yes",
		},
		{
			ID: "EBS-VOL-002", Service: "ebs", Group: "volumes", Op: "DescribeVolumes", Kind: cases.KindHappy,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DescribeVolumes"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
		},
		{
			ID: "EBS-VOL-003", Service: "ebs", Group: "volumes", Op: "AttachVolume", Kind: cases.KindHappy,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2("AttachVolume"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
			Slow: true,
		},
		{
			ID: "EBS-VOL-004", Service: "ebs", Group: "volumes", Op: "ModifyVolume", Kind: cases.KindHappy,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2("ModifyVolume"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
		},
		{
			ID: "EBS-VOL-005", Service: "ebs", Group: "volumes", Op: "DeleteVolume", Kind: cases.KindHappy,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DeleteVolume"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
		},
		{
			ID: "EBS-VOL-006", Service: "ebs", Group: "volumes", Op: "DetachVolume", Kind: cases.KindHappy,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DetachVolume"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
			Slow: true,
		},
		{
			ID: "EBS-VOL-101", Service: "ebs", Group: "volumes", Op: "DeleteVolume", Kind: cases.KindError,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{HTTP: 400, Code: "InvalidVolume.NotFound"},
			Impl: "yes",
		},
		{
			ID: "EBS-VOL-102", Service: "ebs", Group: "volumes", Op: "DeleteVolume", Kind: cases.KindError,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{HTTP: 400, Code: "VolumeInUse"},
			Impl: "yes", Slow: true,
		},
		{
			ID: "EBS-VOL-103", Service: "ebs", Group: "volumes", Op: "CreateVolume", Kind: cases.KindError,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{HTTP: 400, Code: "InvalidParameterValue"},
			Impl: "yes",
		},
		{
			ID: "EBS-VOL-104", Service: "ebs", Group: "volumes", Op: "DescribeVolumes", Kind: cases.KindError,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{HTTP: 400, Code: "InvalidVolume.NotFound"},
			Impl: "yes",
		},
		{
			ID: "EBS-VOL-105", Service: "ebs", Group: "volumes", Op: "AttachVolume", Kind: cases.KindError,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{HTTP: 400, Code: "InvalidVolume.NotFound"},
			Impl: "yes",
		},
		{
			ID: "EBS-VOL-106", Service: "ebs", Group: "volumes", Op: "ModifyVolume", Kind: cases.KindError,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{HTTP: 400, Code: "InvalidVolume.NotFound"},
			Impl: "yes",
		},
		{
			ID: "EBS-VOL-107", Service: "ebs", Group: "volumes", Op: "DetachVolume", Kind: cases.KindError,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{HTTP: 400, Code: "InvalidVolume.NotFound"},
			Impl: "yes",
		},
	}}
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
			out, err := api.CreateVolume(ctx, in)
			if err == nil {
				id = aws.ToString(out.VolumeId)
				size = fmt.Sprint(aws.ToInt32(out.Size))
			}
			return wrapErr("runVol", err)
		})
		o.Fields["id"] = id
		o.Fields["size"] = size
		trackErr := trackVol(rt, api, id)
		if err != nil {
			return o, wrapErr("runVol", err)
		}
		if trackErr != nil {
			return o, trackErr
		}
		return o, nil
	case "EBS-VOL-002":
		id, err := makeVol(ctx, rt, api, "vol2")
		if err != nil {
			return cases.Outcome{}, err
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DescribeVolumes(ctx, &awsec2.DescribeVolumesInput{VolumeIds: []string{id}})
				return wrapErr("runVol", err)
			})
			return callOut, wrapErr("runVol", callErr)
		}
	case "EBS-VOL-004":
		id, err := makeVol(ctx, rt, api, "vol4")
		if err != nil {
			return cases.Outcome{}, err
		}
		if _, err := api.DescribeVolumes(ctx, &awsec2.DescribeVolumesInput{VolumeIds: []string{id}}); err != nil {
			return cases.Outcome{}, fmt.Errorf("describe volume: %w", err)
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.ModifyVolume(ctx, &awsec2.ModifyVolumeInput{VolumeId: aws.String(id), Size: aws.Int32(2)})
				return wrapErr("runVol", err)
			})
			return callOut, wrapErr("runVol", callErr)
		}
	case "EBS-VOL-005":
		id, err := makeVol(ctx, rt, api, "vol5")
		if err != nil {
			return cases.Outcome{}, err
		}
		if _, err := api.DescribeVolumes(ctx, &awsec2.DescribeVolumesInput{VolumeIds: []string{id}}); err != nil {
			return cases.Outcome{}, fmt.Errorf("describe volume: %w", err)
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				err := rt.EC2DeleteVolume(ctx, id)
				return wrapErr("runVol", err)
			})
			return callOut, wrapErr("runVol", callErr)
		}
	case "EBS-VOL-101", "EBS-VOL-104", "EBS-VOL-105", "EBS-VOL-106", "EBS-VOL-107":
		id, err := goneVol(ctx, rt, api, c.ID)
		if err != nil {
			return cases.Outcome{}, err
		}
		inst := ""
		if c.ID == "EBS-VOL-105" {
			inst, err = ownedInstance(ctx, rt, api, "vol105")
			if err != nil {
				return cases.Outcome{}, err
			}
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				var err error
				switch c.ID {
				case "EBS-VOL-104":
					_, err = api.DescribeVolumes(ctx, &awsec2.DescribeVolumesInput{VolumeIds: []string{id}})
				case "EBS-VOL-105":
					_, err = rt.EC2AttachVolume(ctx, id, inst, "/dev/sdf")
				case "EBS-VOL-106":
					_, err = api.ModifyVolume(ctx, &awsec2.ModifyVolumeInput{VolumeId: aws.String(id), Size: aws.Int32(2)})
				case "EBS-VOL-107":
					err = rt.EC2DetachVolume(ctx, id, "", "")
				default:
					err = rt.EC2DeleteVolume(ctx, id)
				}
				return wrapErr("runVol", err)
			})
			return callOut, wrapErr("runVol", callErr)
		}
	case "EBS-VOL-103":
		in := VolumeRequest("vws-1a", 0)
		var id string
		callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
			out, err := api.CreateVolume(ctx, in)
			if err == nil && out != nil {
				id = aws.ToString(out.VolumeId)
			}
			return wrapErr("runVol", err)
		})
		if id != "" {
			if err := trackVol(rt, api, id); err != nil {
				return callOut, wrapErr("runVol", err)
			}
		}
		if harness.SetupReady(callOut, callErr) && id == "" {
			return callOut, fmt.Errorf("create volume: success did not identify a volume")
		}
		return callOut, wrapErr("runVol", callErr)
	case "EBS-VOL-003", "EBS-VOL-102", "EBS-VOL-006":
		_, subnet, err := rt.EnsureNet(ctx)
		if err != nil {
			return cases.Outcome{}, wrapErr("runVol", err)
		}
		instToken, volToken := "volinst", "attach"
		if c.ID == "EBS-VOL-006" {
			instToken, volToken = "detach", "detach"
		}
		run, err := api.RunInstances(ctx, &awsec2.RunInstancesInput{
			ImageId: aws.String("ami-0123456789abcdef0"), InstanceType: types.InstanceType(harness.InstanceType),
			MinCount: aws.Int32(1), MaxCount: aws.Int32(1), SubnetId: aws.String(subnet),
			ClientToken: aws.String(rt.Token(instToken)), TagSpecifications: rt.TagSpec(types.ResourceTypeInstance),
		})
		if err != nil {
			return cases.Outcome{}, fmt.Errorf("run: %w", err)
		}
		inst := aws.ToString(run.Instances[0].InstanceId)
		if err := rt.Track("instance", inst, rt.Name("volinst")); err != nil {
			return cases.Outcome{}, wrapErr("attachVol", err)
		}
		if err := noteBlocks(rt, run.Instances[0]); err != nil {
			return cases.Outcome{}, wrapErr("attachVol", err)
		}
		rt.HoldEC2(api)
		rt.Cleanup(func(ctx context.Context) error {
			return rt.TerminateOwned(ctx, inst)
		})
		if _, err := api.DescribeInstances(ctx, &awsec2.DescribeInstancesInput{InstanceIds: []string{inst}}); err != nil {
			return cases.Outcome{}, fmt.Errorf("describe instance: %w", err)
		}
		vol, err := makeVol(ctx, rt, api, volToken)
		if err != nil {
			return cases.Outcome{}, err
		}
		if _, err := api.DescribeVolumes(ctx, &awsec2.DescribeVolumesInput{VolumeIds: []string{vol}}); err != nil {
			return cases.Outcome{}, fmt.Errorf("describe volume: %w", err)
		}
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := rt.EC2AttachVolume(ctx, vol, inst, "/dev/sdf")
			return wrapErr("runVol", err)
		})
		if c.ID == "EBS-VOL-003" {
			return o, wrapErr("runVol", err)
		}
		if !harness.SetupReady(o, err) {
			if err != nil || (o.HTTP == c.Expect.HTTP && o.Code == c.Expect.Code) {
				return o, fmt.Errorf("attach setup: http %d code %s", o.HTTP, o.Code)
			}
			return o, nil
		}
		if c.ID == "EBS-VOL-006" {
			{
				callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
					err := rt.EC2DetachVolume(ctx, vol, inst, "/dev/sdf")
					return wrapErr("runVol", err)
				})
				return callOut, wrapErr("runVol", callErr)
			}
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				err := rt.EC2DeleteVolume(ctx, vol)
				return wrapErr("runVol", err)
			})
			return callOut, wrapErr("runVol", callErr)
		}
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
	out, err := api.CreateVolume(ctx, in)
	if harness.UnsupportedParameter(err) {
		bare := *in
		bare.TagSpecifications = nil
		out, err = api.CreateVolume(ctx, &bare)
	}
	if err != nil {
		return "", fmt.Errorf("create volume: %w", err)
	}
	id := aws.ToString(out.VolumeId)
	if err := trackVol(rt, api, id); err != nil {
		return "", err
	}
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

func goneVol(ctx context.Context, rt *harness.Runtime, api *awsec2.Client, token string) (string, error) {
	id, err := makeVol(ctx, rt, api, token)
	if err != nil {
		return "", err
	}
	if err := rt.EC2DeleteVolume(ctx, id); err != nil {
		return "", fmt.Errorf("delete volume %s: %w", id, err)
	}
	return id, nil
}

func ownedInstance(ctx context.Context, rt *harness.Runtime, api *awsec2.Client, token string) (string, error) {
	_, subnet, err := rt.EnsureNet(ctx)
	if err != nil {
		return "", wrapErr("ownedInstance", err)
	}
	run, err := api.RunInstances(ctx, &awsec2.RunInstancesInput{
		ImageId: aws.String("ami-0123456789abcdef0"), InstanceType: types.InstanceType(harness.InstanceType),
		MinCount: aws.Int32(1), MaxCount: aws.Int32(1), SubnetId: aws.String(subnet),
		ClientToken: aws.String(rt.Token(token)), TagSpecifications: rt.TagSpec(types.ResourceTypeInstance),
	})
	if err != nil {
		return "", fmt.Errorf("run: %w", err)
	}
	inst := aws.ToString(run.Instances[0].InstanceId)
	rt.HoldEC2(api)
	if err := rt.Track("instance", inst, rt.Name(token)); err != nil {
		return "", wrapErr("ownedInstance", err)
	}
	if err := noteBlocks(rt, run.Instances[0]); err != nil {
		return "", wrapErr("ownedInstance", err)
	}
	rt.Cleanup(func(ctx context.Context) error {
		return rt.TerminateOwned(ctx, inst)
	})
	if _, err := api.DescribeInstances(ctx, &awsec2.DescribeInstancesInput{InstanceIds: []string{inst}}); err != nil {
		return "", fmt.Errorf("describe instance: %w", err)
	}
	return inst, nil
}

func noteBlocks(rt *harness.Runtime, inst types.Instance) error {
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

func trackVol(rt *harness.Runtime, api *awsec2.Client, id string) error {
	if id == "" {
		return nil
	}
	rt.HoldEC2(api)
	rt.Cleanup(func(ctx context.Context) error {
		return rt.DeleteOwnedVolume(ctx, id)
	})
	if err := rt.Track("volume", id, rt.Name("vol")); err != nil {
		return wrapErr("trackVol", err)
	}
	return nil
}
