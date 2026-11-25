package engine

import (
	"os"
	"path/filepath"
	"regexp"
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
		"-backend proxmox", "-region pve-test-1 -state-dir", "pveApiUrls", "caPem",
		"-backend regions -iam grpc", "/api/v1/admin/regions",
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
	for _, forbidden := range []string{
		"etc/pve", "-helper-path", "-helper-root", "-registration-file", "-pve-endpoint", "-pve-ca-file",
		"region-kek", "VWS_HELPER_TEST_ROOT", "\"pveApiUrl\"",
	} {
		if strings.Contains(string(script), forbidden) {
			t.Fatalf("local-pve script still uses removed %q", forbidden)
		}
	}
	if strings.Contains(job, "regionhelper") || strings.Contains(job, "region-helper") {
		t.Fatal("local-pve job still builds the region helper")
	}
}

// Both jobs build console-api, so they must read one pin; a second definition lets them drift apart.
func TestCIJobsShareOneConsoleAPIPin(t *testing.T) {
	body := workflow(t)
	head, jobs, ok := strings.Cut(body, "\njobs:\n")
	if !ok {
		t.Fatal("workflow has no jobs")
	}
	_, env, ok := strings.Cut(head, "\nenv:\n")
	if !ok {
		t.Fatal("workflow has no workflow-scope env")
	}
	if !regexp.MustCompile(`(?m)^  CONSOLE_API_PIN: [0-9a-f]{40}$`).MatchString(env) {
		t.Fatal("workflow env does not pin CONSOLE_API_PIN to a full commit")
	}
	if strings.Contains(jobs, "CONSOLE_API_PIN:") || strings.Contains(jobs, "CONSOLE_API_PIN=") {
		t.Fatal("a job redefines CONSOLE_API_PIN instead of reading the workflow pin")
	}
	for _, name := range []string{"local-fake", "local-pve"} {
		job := workflowJob(t, jobs, name)
		for _, required := range []string{
			`fetch --depth 1 origin "$CONSOLE_API_PIN"`, `rev-parse HEAD`, `"$CONSOLE_API_PIN"`,
		} {
			if !strings.Contains(job, required) {
				t.Fatalf("%s does not check out console-api at the workflow pin: missing %q", name, required)
			}
		}
		if regexp.MustCompile(`vws-console-api[^\n]*fetch[^\n]*origin "?[0-9a-f]{40}`).MatchString(job) {
			t.Fatalf("%s fetches console-api at a literal commit", name)
		}
	}
}

// The README builds the local-pve stack by hand, so its pins must be the ones CI tests.
func TestREADMEBuildsLocalPVEAtWorkflowPins(t *testing.T) {
	body := workflow(t)
	_, file, _, _ := goruntime.Caller(0)
	readme, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"CONSOLE_API_PIN", "IAM_PIN"} {
		pin := regexp.MustCompile(`(?m)^ +` + name + `: ([0-9a-f]{40})$`).FindStringSubmatch(body)
		if pin == nil {
			t.Fatalf("workflow does not pin %s", name)
		}
		if !strings.Contains(string(readme), name+"="+pin[1]+"\n") {
			t.Fatalf("README does not build at %s=%s", name, pin[1])
		}
	}
	for _, required := range []string{
		"for cmd in pvefake vws-region vws-console-api", `"$STACK_BIN/vws-iam" ./cmd/vws-iam`,
	} {
		if !strings.Contains(string(readme), required) {
			t.Fatalf("README does not build %q", required)
		}
	}
}

// workflowJob returns the body of one job under jobs:, up to the next job.
func workflowJob(t *testing.T, jobs, name string) string {
	t.Helper()
	_, rest, ok := strings.Cut("\n"+jobs, "\n  "+name+":\n")
	if !ok {
		t.Fatalf("workflow has no %s job", name)
	}
	if loc := regexp.MustCompile(`\n  [A-Za-z0-9_-]+:\n`).FindStringIndex(rest); loc != nil {
		rest = rest[:loc[0]]
	}
	return rest
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
