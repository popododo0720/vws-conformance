package engine

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	goruntime "runtime"
	"slices"
	"strings"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func checkPVEReadSelection(groups []bind.Group) error {
	// Describe operations with creating fixtures are deliberately absent.
	const safe = `EC2-DI-101 EC2-DI-102 EC2-DIA-101 EC2-DIMG-001 EC2-DIMG-101
EC2-KP-103 EC2-EIP-103 EC2-GAP-003 EC2-GAP-004 EC2-GAP-008
EBS-VOL-104 EBS-SNAP-102 EBS-GAP-002
VPC-VPC-104 VPC-SUB-103 VPC-SG-104 VPC-GAP-004 VPC-GAP-005 VPC-GAP-007
AUTH-001 AUTH-002 AUTH-003 AUTH-004 AUTH-005 AUTH-006 AUTH-007 AUTH-008 AUTH-011 AUTH-013`
	for _, g := range groups {
		for _, c := range g.Cases {
			if !slices.Contains(strings.Fields(safe), c.ID) {
				return fmt.Errorf("pve-test selected creating or unknown case %s", c.ID)
			}
		}
	}
	return nil
}

func TestPVETestSelectionNeverCreates(t *testing.T) {
	previous := *flagReadOnly
	t.Cleanup(func() { *flagReadOnly = previous })
	*flagReadOnly = false
	for range 2 {
		groups := selectGroups(harness.Config{Full: true, Target: "pve-test"}, "", "")
		if err := checkPVEReadSelection(groups); err != nil {
			t.Fatal(err)
		}
		count := 0
		for _, g := range groups {
			count += len(g.Cases)
		}
		if count != 29 {
			t.Fatalf("selected %d cases, want 29", count)
		}
		preview := ReadOnlyGroups()
		if len(preview) != len(groups) {
			t.Fatalf("conf-select lists %d groups, execution selects %d", len(preview), len(groups))
		}
		for i, g := range preview {
			if g.Service != groups[i].Service || g.Name != groups[i].Name ||
				!reflect.DeepEqual(g.Cases, groups[i].Cases) {
				t.Fatalf("conf-select group %s/%s differs from execution", g.Service, g.Name)
			}
		}
	}
	normal := selectGroups(harness.Config{Full: true, Target: "local-fake"}, "ebs", "volumes")
	if len(normal) != 1 || !slices.ContainsFunc(normal[0].Cases, func(c cases.Case) bool {
		return c.ID == "EBS-VOL-001"
	}) {
		t.Fatal("normal target lost creating cases")
	}
}

func TestPVETestSelectionRejectsPlantedCreatingCase(t *testing.T) {
	for _, c := range []cases.Case{
		{ID: "EBS-VOL-001", Op: "CreateVolume"},
		{ID: "EBS-VOL-002", Op: "DescribeVolumes"},
	} {
		groups := append(ReadOnlyGroups(), bind.Group{Cases: []cases.Case{c}})
		if err := checkPVEReadSelection(groups); err == nil || !strings.Contains(err.Error(), c.ID) {
			t.Fatalf("planted creating fixture %s: %v", c.ID, err)
		}
	}
}

func TestPVETestEntrypoint(t *testing.T) {
	_, file, _, ok := goruntime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	script := filepath.Join(filepath.Dir(file), "..", "..", "scripts", "pve-test.sh")
	bin := t.TempDir()
	// The boundary double records the command, never an endpoint or credential.
	const fakeGo = "#!/bin/sh\n" +
		"printf '%s\\n' \"$VWS_CONF_TARGET\" \"$VWS_CONF_ALLOWED_ACCOUNTS\" \"$@\"\n" +
		"exit \"${FAKE_EXIT:-0}\"\n"
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte(fakeGo), 0o700); err != nil {
		t.Fatal(err)
	}
	env := []string{
		"PATH=" + bin + ":/usr/bin:/bin",
		"VWS_CONF_TARGET=local-fake",
		"VWS_CONF_ALLOWED_HOSTS=127.0.0.1",
		"VWS_CONF_EC2_ENDPOINT=http://127.0.0.1:8081",
		"VWS_CONF_IAM_ENDPOINT=http://127.0.0.1:8084",
		"VWS_CONF_REGION=pve-test-1", "VWS_CONF_ACCOUNT_ID=123456789012",
		"VWS_CONF_ALLOWED_ACCOUNTS=foreign",
		"VWS_CONF_ACCESS_KEY_ID=test", "VWS_CONF_SECRET_ACCESS_KEY=test",
		"VWS_CONF_OUT=" + t.TempDir(),
	}
	const want = "pve-test\n123456789012\nrun\n./cmd/conf-run\n-full\n-read-only\n"
	for range 2 {
		cmd := exec.CommandContext(t.Context(), "sh", script)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil || string(out) != want {
			t.Fatalf("entrypoint: %v, output %q", err, out)
		}
	}
	t.Run("missing allowlist fails before command", func(t *testing.T) {
		cmd := exec.CommandContext(t.Context(), "sh", script)
		cmd.Env = append(slices.Clone(env), "VWS_CONF_ALLOWED_HOSTS=")
		out, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(out), "VWS_CONF_ALLOWED_HOSTS") ||
			strings.Contains(string(out), "./cmd/conf-run") {
			t.Fatalf("missing host: %v, output %q", err, out)
		}
	})
	t.Run("preserves run failure", func(t *testing.T) {
		cmd := exec.CommandContext(t.Context(), "sh", script)
		cmd.Env = append(slices.Clone(env), "FAKE_EXIT=23")
		out, err := cmd.CombinedOutput()
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 23 || string(out) != want {
			t.Fatalf("failed run: %v, output %q", err, out)
		}
	})
}
