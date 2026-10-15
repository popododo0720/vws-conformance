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
			{ID: "EC2-DIMG-001", Service: "ec2", Group: "images", Op: "DescribeImages", Kind: cases.KindHappy, Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DescribeImages"), Expect: cases.Expect{HTTP: 200, Patterns: map[string]string{"id": expect.ImageID}}, Impl: "yes"},
			{ID: "EC2-DIMG-101", Service: "ec2", Group: "images", Op: "DescribeImages", Kind: cases.KindError, Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{HTTP: 400, Code: "InvalidAMIID.NotFound"}, Impl: "yes"},
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
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeImages(ctx, &awsec2.DescribeImagesInput{})
			return err
		})
		if err != nil {
			return o, err
		}
		out, err := api.DescribeImages(ctx, &awsec2.DescribeImagesInput{ImageIds: []string{"ami-0123456789abcdef0"}})
		if err != nil || len(out.Images) == 0 {
			o.Fields["id"] = ""
			return o, err
		}
		o.Fields["id"] = aws.ToString(out.Images[0].ImageId)
		return o, nil
	case "EC2-DIMG-101":
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeImages(ctx, &awsec2.DescribeImagesInput{ImageIds: []string{"ami-0123456789abcdef1"}})
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
