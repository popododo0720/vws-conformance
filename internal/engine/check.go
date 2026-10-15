package engine

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/popododo0720/vws-conformance/internal/harness"
)

func CheckCLI() int {
	if os.Getenv("AWS_PROFILE") != "" || os.Getenv("AWS_DEFAULT_PROFILE") != "" {
		fmt.Fprintln(os.Stderr, "AWS_PROFILE is refused; set VWS_CONF_ACCESS_KEY_ID and VWS_CONF_SECRET_ACCESS_KEY")
		return 1
	}
	if os.Getenv("VWS_CONF_ACCESS_KEY_ID") == "" || os.Getenv("VWS_CONF_SECRET_ACCESS_KEY") == "" {
		fmt.Fprintln(os.Stderr, "VWS_CONF_ACCESS_KEY_ID and VWS_CONF_SECRET_ACCESS_KEY are required")
		return 1
	}
	cfg, err := harness.FromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	n := 0
	for _, ep := range []string{cfg.EC2Endpoint, cfg.S3Endpoint, cfg.IAMEndpoint} {
		if ep == "" {
			continue
		}
		n++
		if err := harness.ResolveEndpointCIDR(ctx, ep, cfg.AllowedHosts, cfg.AllowedCIDRs); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	if n == 0 {
		fmt.Fprintln(os.Stderr, "no endpoint configured")
		return 1
	}
	return 0
}
