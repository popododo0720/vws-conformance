package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// buildOnce builds the conf-run binary a single time for every test in this file. Each test then
// execs that binary under strace, so the build cost is paid once per test run.
var buildOnce struct {
	sync.Once
	dir string
	bin string
	err error
}

// TestMain removes the build directory once every test in the package has run.
func TestMain(m *testing.M) {
	code := m.Run()
	if buildOnce.dir != "" {
		if err := os.RemoveAll(buildOnce.dir); err != nil {
			fmt.Fprintln(os.Stderr, "remove build dir:", err)
		}
	}
	os.Exit(code)
}

func confRunBinary(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "conf-run-egress-*")
		if err != nil {
			buildOnce.err = err
			return
		}
		buildOnce.dir = dir
		bin := filepath.Join(dir, "conf-run")
		cmd := exec.CommandContext(t.Context(), "go", "build", "-o", bin, "./cmd/conf-run")
		cmd.Dir = root
		// CGO_ENABLED=0 keeps every lookup in the Go resolver, so a lookup cannot go through
		// glibc NSS over AF_UNIX and slip past the AF_INET trace.
		cmd.Env = scriptEnv(dir, []string{"CGO_ENABLED=0"})
		if out, buildErr := cmd.CombinedOutput(); buildErr != nil {
			buildOnce.err = wrapBuildErr(buildErr, out)
			return
		}
		buildOnce.bin = bin
	})
	if buildOnce.err != nil {
		t.Fatal(buildOnce.err)
	}
	return buildOnce.bin
}

func wrapBuildErr(err error, out []byte) error {
	return &buildError{err: err, out: out}
}

type buildError struct {
	err error
	out []byte
}

func (e *buildError) Error() string { return e.err.Error() + "\n" + string(e.out) }

// straceOrSkip requires strace on PATH. CI must have it installed; everywhere else a missing
// strace only skips the test.
func straceOrSkip(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("strace"); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("strace is required in CI: %v", err)
		}
		t.Skip("strace is not installed")
	}
}

// tracedRun runs bin under strace and fails closed: connect, sendto, sendmsg and sendmmsg are
// injected with ENETUNREACH and never executed, so no packet (DNS included) can leave even if the
// guard under test regresses. The attempt is still recorded in the trace. It returns the process
// exit code, its stderr and the raw strace output.
func tracedRun(t *testing.T, bin string, args, env []string) (code int, stderr, trace string) {
	t.Helper()
	traceFile := filepath.Join(t.TempDir(), "strace.log")
	full := append([]string{
		"-f", "-e", "trace=connect,sendto,sendmsg,sendmmsg",
		"-e", "inject=connect,sendto,sendmsg,sendmmsg:error=ENETUNREACH", "-o", traceFile, bin,
	}, args...)
	cmd := exec.CommandContext(t.Context(), "strace", full...)
	cmd.Env = env
	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("run strace: %v", err)
		}
		exitCode = exit.ExitCode()
	}
	traceBytes, readErr := os.ReadFile(traceFile)
	if readErr != nil {
		t.Fatalf("read strace log: %v", readErr)
	}
	return exitCode, errBuf.String(), string(traceBytes)
}

// assertNoNetworkEgress fails the test if the traced process opened an AF_INET or AF_INET6
// socket address at all: the guard must refuse the amazonaws.com endpoint before it ever builds a
// dialer, so no connect, sendto or sendmsg on an IP socket, loopback or not, and no DNS lookup
// (itself a connect or sendto to port 53) should appear in the trace.
func assertNoNetworkEgress(t *testing.T, trace string) {
	t.Helper()
	for _, line := range strings.Split(trace, "\n") {
		if strings.Contains(line, "AF_INET") {
			t.Fatalf("binary touched the network before refusing the endpoint:\n%s", line)
		}
	}
}

const amazonAccount = "123456789012"

func amazonEnv(dir string, extra []string) []string {
	return scriptEnv(dir, append([]string{
		"VWS_CONF_ACCESS_KEY_ID=AKIDEXAMPLE",
		"VWS_CONF_SECRET_ACCESS_KEY=secret",
		"VWS_CONF_ACCOUNT_ID=" + amazonAccount,
		"VWS_CONF_ALLOWED_ACCOUNTS=" + amazonAccount,
		"VWS_CONF_TARGET=local-fake",
	}, extra...))
}

func TestEgressBinaryRefusesAmazonDefaultPath(t *testing.T) {
	straceOrSkip(t)
	bin := confRunBinary(t)
	dir := t.TempDir()
	env := amazonEnv(dir, []string{
		"VWS_CONF_EC2_ENDPOINT=https://ec2.us-east-1.amazonaws.com",
		"VWS_CONF_S3_ENDPOINT=https://s3.us-east-1.amazonaws.com",
		"VWS_CONF_IAM_ENDPOINT=https://iam.amazonaws.com",
		"VWS_CONF_ALLOWED_HOSTS=ec2.us-east-1.amazonaws.com,s3.us-east-1.amazonaws.com,iam.amazonaws.com",
		"VWS_CONF_FULL=1",
	})
	for i := 0; i < 2; i++ {
		code, stderr, trace := tracedRun(t, bin, nil, env)
		if code == 0 {
			t.Fatalf("run %d: exited 0", i)
		}
		if !strings.Contains(stderr, "refused") {
			t.Fatalf("run %d: stderr %q does not say refused", i, stderr)
		}
		assertNoNetworkEgress(t, trace)
	}
}

func TestEgressBinaryRefusesAmazonCheckEndpoint(t *testing.T) {
	straceOrSkip(t)
	bin := confRunBinary(t)
	dir := t.TempDir()
	env := amazonEnv(dir, []string{
		"VWS_CONF_S3_ENDPOINT=https://s3.us-east-1.amazonaws.com",
		"VWS_CONF_ALLOWED_HOSTS=s3.us-east-1.amazonaws.com",
	})
	for i := 0; i < 2; i++ {
		code, stderr, trace := tracedRun(t, bin, []string{"check-endpoint"}, env)
		if code == 0 {
			t.Fatalf("run %d: exited 0", i)
		}
		if !strings.Contains(stderr, "refused") {
			t.Fatalf("run %d: stderr %q does not say refused", i, stderr)
		}
		assertNoNetworkEgress(t, trace)
	}
}

func TestEgressBinaryRefusesAmazonCLI(t *testing.T) {
	straceOrSkip(t)
	bin := confRunBinary(t)
	dir := t.TempDir()
	canary := filepath.Join(dir, "called")
	fake := "#!/bin/sh\ntouch " + canary + "\n"
	if err := os.WriteFile(filepath.Join(dir, "aws"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	env := amazonEnv(dir, []string{
		"VWS_CONF_S3_ENDPOINT=https://s3.us-east-1.amazonaws.com",
		"VWS_CONF_ALLOWED_HOSTS=s3.us-east-1.amazonaws.com",
	})
	for i := 0; i < 2; i++ {
		code, stderr, trace := tracedRun(t, bin, []string{"cli", "s3api", "list-buckets"}, env)
		if code == 0 {
			t.Fatalf("run %d: exited 0", i)
		}
		if !strings.Contains(stderr, "refused") {
			t.Fatalf("run %d: stderr %q does not say refused", i, stderr)
		}
		if _, statErr := os.Stat(canary); statErr == nil {
			t.Fatalf("run %d: aws ran", i)
		}
		assertNoNetworkEgress(t, trace)
	}
}
