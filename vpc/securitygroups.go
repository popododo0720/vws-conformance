package vpc

import (
	"context"
	"fmt"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func SecurityGroups() bind.Group {
	return bind.Group{Service: "vpc", Name: "securitygroups", Run: runSG, Cases: []cases.Case{
		{
			ID: "VPC-SG-104", Service: "vpc", Group: "securitygroups", Op: "DescribeSecurityGroups",
			Kind: cases.KindError, Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors,
			Expect: cases.Expect{HTTP: 400, Code: "InvalidGroup.NotFound"}, Impl: "yes",
		},
	}}
}

func runSG(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	api, err := client(rt)
	if err != nil {
		return cases.Outcome{}, err
	}
	switch c.ID {
	case "VPC-SG-104":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeSecurityGroups(ctx, &awsec2.DescribeSecurityGroupsInput{
				GroupIds: []string{"sg-0123456789abcdef0"},
			})
			return wrapErr("runSG", err)
		})
		return o, wrapErr("runSG", err)
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}
