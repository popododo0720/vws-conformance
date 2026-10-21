package ec2

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/expect"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Images() bind.Group {
	return bind.Group{
		Service: "ec2", Name: "images", Run: runImages,
		Cases: []cases.Case{
			{
				ID: "EC2-DIMG-001", Service: "ec2", Group: "images", Op: "DescribeImages", Kind: cases.KindHappy,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DescribeImages"), Expect: cases.Expect{
					HTTP:     200,
					Patterns: map[string]string{"id": expect.ImageID},
				}, Impl: "yes",
			},
			{
				ID: "EC2-DIMG-101", Service: "ec2", Group: "images", Op: "DescribeImages", Kind: cases.KindError,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors,
				Expect: cases.Expect{HTTP: 400, Code: "InvalidAMIID.NotFound"}, Impl: "yes",
			},
		},
	}
}

func runImages(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	api, err := client(rt)
	if err != nil {
		return cases.Outcome{}, err
	}
	switch c.ID {
	case "EC2-DIMG-001":
		var id string
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			out, err := api.DescribeImages(ctx, &awsec2.DescribeImagesInput{})
			if err == nil && len(out.Images) > 0 {
				id = aws.ToString(out.Images[0].ImageId)
			}
			return wrapErr("runImages", err)
		})
		o.Fields["id"] = id
		return o, wrapErr("runImages", err)
	case "EC2-DIMG-101":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeImages(ctx, &awsec2.DescribeImagesInput{ImageIds: []string{"ami-0123456789abcdef1"}})
			return wrapErr("runImages", err)
		})
		return o, wrapErr("runImages", err)
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}
