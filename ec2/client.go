package ec2

import (
	"fmt"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/popododo0720/vws-conformance/internal/harness"
)

func client(rt *harness.Runtime) (*awsec2.Client, error) {
	if rt == nil || rt.Clients == nil || rt.Clients.EC2 == nil {
		return nil, fmt.Errorf("ec2 client is unset")
	}
	return rt.Clients.EC2, nil
}
