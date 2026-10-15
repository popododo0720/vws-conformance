package ec2

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Gaps() bind.Group {
	return bind.Group{
		Service: "ec2", Name: "gaps", Run: runEC2Gaps,
		Cases: []cases.Case{
			ec2Gap("EC2-GAP-001", "RebootInstances"),
			ec2Gap("EC2-GAP-002", "ModifyInstanceAttribute"),
			ec2Gap("EC2-GAP-003", "DescribeInstanceStatus"),
			ec2Gap("EC2-GAP-004", "DescribeInstanceTypes"),
			ec2Gap("EC2-GAP-005", "CreateImage"),
			ec2Gap("EC2-GAP-006", "DeregisterImage"),
			ec2Gap("EC2-GAP-007", "DeleteTags"),
			ec2Gap("EC2-GAP-008", "DescribeTags"),
		},
	}
}

func ec2Gap(id, op string) cases.Case {
	exp := cases.Expect{HTTP: 400, Code: "InvalidInstanceID.NotFound"}
	if op == "DescribeInstanceTypes" || op == "DescribeTags" || op == "DescribeInstanceStatus" {
		exp = cases.Expect{HTTP: 200}
	}
	if op == "DeregisterImage" {
		exp = cases.Expect{HTTP: 400, Code: "InvalidAMIID.NotFound"}
	}
	return cases.Case{ID: id, Service: "ec2", Group: "gaps", Op: op, Kind: cases.KindGap, Suite: cases.SuiteSDK, DocURL: cases.DocEC2(op), Expect: exp, Impl: "gap"}
}

func runEC2Gaps(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	api, err := client(rt)
	if err != nil {
		return cases.Outcome{}, err
	}
	id := "i-0123456789abcdef0"
	switch c.ID {
	case "EC2-GAP-001":
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.RebootInstances(ctx, &awsec2.RebootInstancesInput{InstanceIds: []string{id}})
			return err
		})
	case "EC2-GAP-002":
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.ModifyInstanceAttribute(ctx, &awsec2.ModifyInstanceAttributeInput{InstanceId: aws.String(id), InstanceType: &types.AttributeValue{Value: aws.String("t2.nano")}})
			return err
		})
	case "EC2-GAP-003":
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeInstanceStatus(ctx, &awsec2.DescribeInstanceStatusInput{})
			return err
		})
	case "EC2-GAP-004":
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeInstanceTypes(ctx, &awsec2.DescribeInstanceTypesInput{})
			return err
		})
	case "EC2-GAP-005":
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.CreateImage(ctx, &awsec2.CreateImageInput{InstanceId: aws.String(id), Name: aws.String(rt.Name("img"))})
			return err
		})
	case "EC2-GAP-006":
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DeregisterImage(ctx, &awsec2.DeregisterImageInput{ImageId: aws.String("ami-0123456789abcdef1")})
			return err
		})
	case "EC2-GAP-007":
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DeleteTags(ctx, &awsec2.DeleteTagsInput{Resources: []string{id}, Tags: []types.Tag{{Key: aws.String("Name")}}})
			return err
		})
	case "EC2-GAP-008":
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeTags(ctx, &awsec2.DescribeTagsInput{})
			return err
		})
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}
