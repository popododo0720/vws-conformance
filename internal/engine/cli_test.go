package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunCLIEmptyCredentialsDoesNotExec(t *testing.T) {
	called := fakeAWS(t)
	quietCLIEnv(t)
	t.Setenv("VWS_CONF_ACCESS_KEY_ID", "")
	t.Setenv("VWS_CONF_SECRET_ACCESS_KEY", "")
	code := RunCLI([]string{"s3api", "list-buckets"})
	if code == 0 || awsRan(called) {
		t.Fatalf("code %d aws ran %v", code, awsRan(called))
	}
}

func TestRunCLIRejectsURLArgument(t *testing.T) {
	called := fakeAWS(t)
	quietCLIEnv(t)
	args := []string{"s3api", "put-bucket-policy", "--policy", "https://evil.example/policy.json"}
	code := RunCLI(args)
	if code == 0 || awsRan(called) {
		t.Fatalf("code %d aws ran %v", code, awsRan(called))
	}
}

func TestRunCLIRejectsUnknownCommand(t *testing.T) {
	called := fakeAWS(t)
	quietCLIEnv(t)
	code := RunCLI([]string{"sts", "get-caller-identity"})
	if code == 0 || awsRan(called) {
		t.Fatalf("code %d aws ran %v", code, awsRan(called))
	}
}

func awsRan(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func fakeAWS(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	called := filepath.Join(dir, "called")
	body := "#!/bin/sh\nset -e\n: > " + called + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "aws"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return called
}

func quietCLIEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"AWS_PROFILE", "AWS_DEFAULT_PROFILE", "VWS_CONF_RUN_ID",
		"VWS_CONF_PARALLEL", "VWS_CONF_ALLOWED_CIDRS", "VWS_CONF_SESSION_TOKEN",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("VWS_CONF_TARGET", "local-fake")
	t.Setenv("VWS_CONF_S3_ENDPOINT", "http://127.0.0.1:8082")
	t.Setenv("VWS_CONF_ALLOWED_HOSTS", "127.0.0.1")
	t.Setenv("VWS_CONF_ACCESS_KEY_ID", "AKIDEXAMPLE")
	t.Setenv("VWS_CONF_SECRET_ACCESS_KEY", "secret")
}
