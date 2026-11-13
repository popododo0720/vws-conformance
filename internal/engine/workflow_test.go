package engine

import (
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
)

func TestCIUploadsReportsAfterFailure(t *testing.T) {
	body := workflow(t)
	if !strings.Contains(body, "actions/upload-artifact@v4") {
		t.Fatal("workflow does not upload artifacts")
	}
	if !strings.Contains(body, "if: always()") {
		t.Fatal("artifact upload is skipped when the job fails")
	}
	if !strings.Contains(body, "out/") {
		t.Fatal("artifact upload does not keep out/")
	}
}

func TestCIRunsCommandPackageTests(t *testing.T) {
	body := workflow(t)
	if !strings.Contains(body, "./cmd/...") {
		t.Fatal("unit tests skip ./cmd/...")
	}
}

func TestCIRunsLocalPVE(t *testing.T) {
	body := workflow(t)
	_, job, ok := strings.Cut(body, "\n  local-pve:\n")
	if !ok {
		t.Fatal("workflow has no local-pve job")
	}
	for _, required := range []string{
		"repository_dispatch:\n    types: [console-api]", "workflow_dispatch:", "pull_request:",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("missing trigger %q", required)
		}
	}
	for _, required := range []string{
		"05840905e958f6ad5e57e200931d542798b22d73",
		"./cmd/pvefake", "./cmd/vws-region", "./cmd/vws-console-api",
		"IAM_PIN: d01a814a91afba7d884d9f5b0bf4ad7a23110a91",
		"\"$(git -C \"$src\" rev-parse HEAD)\" = \"$IAM_PIN\"",
		"./cmd/vws-iam",
		"./internal/harness ./cmd/conf-run", "bash scripts/local-pve.sh",
		"if: always()", "name: conformance-local-pve",
	} {
		if !strings.Contains(job, required) {
			t.Fatalf("local-pve is missing %q", required)
		}
	}
	_, file, _, _ := goruntime.Caller(0)
	script, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "scripts", "local-pve.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(script), "curl ") {
		t.Fatal("local-pve requests bypass the guarded probe")
	}
	for _, required := range []string{
		"-backend proxmox", "-backend regions -iam grpc", "/api/v1/admin/regions",
		"bootstrap -store etcd", "\"$STACK_BIN/vws-iam\" -store etcd",
		"-iam-endpoint 127.0.0.1:9095", "-listen 127.0.0.1:9095 -health-listen 127.0.0.1:9096",
		"-iam-tls-ca-file", "-iam-tls-cert-file", "-iam-tls-key-file",
		"--client-cert-auth --trusted-ca-file", "ss -Hltnp",
		"VWS_IAM_ROOT_PROJECT=123456789012", "export VWS_IAM_ROOT_ACCESS_KEY_ID",
		"go build -o \"$probe\" ./cmd/conf-probe", "\"$probe\" -method POST",
		"VWS_CONF_ALLOWED_HOSTS=127.0.0.1", "VWS_CONF_REGION=pve-test-1",
		"./ec2 ./ebs ./vpc -args -full -read-only",
		"./ec2 -args -service ec2 -group instances || status=1",
		"-ec2-launch-burst 100 -ec2-launch-rate 100",
		"./iam ./sts ./auth -args -full || status=1", "trap cleanup EXIT",
	} {
		if !strings.Contains(string(script), required) {
			t.Fatalf("local-pve script is missing %q", required)
		}
	}
}

func workflow(t *testing.T) string {
	t.Helper()
	_, file, _, ok := goruntime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", ".github", "workflows", "conformance.yml")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
