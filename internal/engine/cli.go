package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/popododo0720/vws-conformance/internal/harness"
)

func RunCLI(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "cli args are empty")
		return 1
	}
	for _, a := range args {
		if a == "--endpoint-url" || strings.HasPrefix(a, "--endpoint-url=") {
			fmt.Fprintln(os.Stderr, "endpoint URL is set by conf-run")
			return 1
		}
	}
	if os.Getenv("AWS_PROFILE") != "" || os.Getenv("AWS_DEFAULT_PROFILE") != "" {
		fmt.Fprintln(os.Stderr, "AWS_PROFILE is refused")
		return 1
	}
	if _, err := exec.LookPath("aws"); err != nil {
		if CheckCLI() != 0 {
			return 1
		}
		fmt.Println("skip: aws cli is not installed")
		return 0
	}
	cfg, err := harness.FromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if cfg.S3Endpoint == "" {
		fmt.Fprintln(os.Stderr, "VWS_CONF_S3_ENDPOINT is required")
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pin, err := harness.PinEndpoint(ctx, cfg.S3Endpoint, cfg.AllowedHosts, cfg.AllowedCIDRs)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer pin.Close()
	if !strings.HasPrefix(pin.URL, "http://127.0.0.1:") {
		fmt.Fprintln(os.Stderr, "pinned endpoint is not loopback")
		return 1
	}
	cmd := exec.CommandContext(ctx, "aws", append(args, "--endpoint-url", pin.URL)...)
	cmd.Env = append(harness.ChildEnv(os.Environ()),
		"AWS_ACCESS_KEY_ID="+cfg.AccessKey,
		"AWS_SECRET_ACCESS_KEY="+cfg.Secret,
		"AWS_DEFAULT_REGION="+cfg.Region,
	)
	if cfg.Session != "" {
		cmd.Env = append(cmd.Env, "AWS_SESSION_TOKEN="+cfg.Session)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
