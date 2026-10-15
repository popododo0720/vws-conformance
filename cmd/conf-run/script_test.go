package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestScriptRefusesAWSBeforeCLI(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	called := filepath.Join(dir, "called")
	script := "#!/bin/sh\ntouch " + called + "\n"
	if err := os.WriteFile(filepath.Join(dir, "aws"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", filepath.Join(root, "scripts/cli/s3.sh"))
	cmd.Dir = root
	cmd.Env = []string{
		"PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"GOROOT=" + os.Getenv("GOROOT"),
		"GOPATH=" + os.Getenv("GOPATH"),
		"GOCACHE=" + os.Getenv("GOCACHE"),
		"GOMODCACHE=" + os.Getenv("GOMODCACHE"),
		"GOTOOLCHAIN=local",
		"VWS_CONF_S3_ENDPOINT=https://s3.us-east-1.amazonaws.com",
		"VWS_CONF_ACCESS_KEY_ID=AKIDEXAMPLE",
		"VWS_CONF_SECRET_ACCESS_KEY=secret",
		"VWS_CONF_ALLOWED_HOSTS=s3.us-east-1.amazonaws.com",
		"VWS_CONF_ALLOWED_ACCOUNTS=123456789012",
		"VWS_CONF_TARGET=local-fake",
	}
	err := cmd.Run()
	if err == nil {
		t.Fatal("script exited 0")
	}
	if _, statErr := os.Stat(called); statErr == nil {
		t.Fatal("aws ran")
	}
}

func TestScriptRefusesAWSProfile(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	called := filepath.Join(dir, "called")
	if err := os.WriteFile(filepath.Join(dir, "aws"), []byte("#!/bin/sh\ntouch "+called+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", filepath.Join(root, "scripts/cli/s3.sh"))
	cmd.Dir = root
	cmd.Env = []string{
		"PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"GOTOOLCHAIN=local",
		"AWS_PROFILE=default",
		"VWS_CONF_S3_ENDPOINT=http://127.0.0.1:8082",
		"VWS_CONF_ACCESS_KEY_ID=AKIDEXAMPLE",
		"VWS_CONF_SECRET_ACCESS_KEY=secret",
		"VWS_CONF_ALLOWED_HOSTS=127.0.0.1",
		"VWS_CONF_ALLOWED_ACCOUNTS=123456789012",
	}
	if err := cmd.Run(); err == nil {
		t.Fatal("profile was accepted")
	}
	if _, statErr := os.Stat(called); statErr == nil {
		t.Fatal("aws ran")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("go.mod not found")
	return ""
}
