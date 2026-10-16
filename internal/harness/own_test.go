package harness

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestOwnRegistersDeletion(t *testing.T) {
	rt := NewRuntime(Config{RunID: "conf-20261015T120000Z-abc123", OutDir: t.TempDir()}, nil)
	hit := false
	if err := rt.Own("subnet", "subnet-1", "conf-sub", func(context.Context) error {
		hit = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := rt.Clean.Run(context.Background()); err != nil || !hit {
		t.Fatalf("err %v hit %v", err, hit)
	}
}

func TestClaimRunRefusesALiveHolder(t *testing.T) {
	const id = "conf-20261015T120000Z-abc123"
	dir := t.TempDir()
	t.Setenv("VWS_CONF_ACTIVITY_DIR", dir)
	path := filepath.Join(dir, id)
	if err := os.WriteFile(path, []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ClaimRun(id); err == nil {
		t.Fatal("claimed a run pid 1 still holds")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "1\n" {
		t.Fatalf("marker %q", body)
	}
}

func TestReleaseRunLeavesALiveHolder(t *testing.T) {
	const id = "conf-20261015T120000Z-abc123"
	dir := t.TempDir()
	t.Setenv("VWS_CONF_ACTIVITY_DIR", dir)
	path := filepath.Join(dir, id)
	if err := os.WriteFile(path, []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ReleaseRun(id); err == nil {
		t.Fatal("released a run pid 1 still holds")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "1\n" {
		t.Fatalf("marker %q", body)
	}
}

func TestClaimRunReclaimsADeadHolder(t *testing.T) {
	const id = "conf-20261015T180000Z-eee555"
	dir := t.TempDir()
	t.Setenv("VWS_CONF_ACTIVITY_DIR", dir)
	path := filepath.Join(dir, id)
	if err := os.WriteFile(path, []byte(deadPID(t)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ClaimRun(id); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != strconv.Itoa(os.Getpid())+"\n" {
		t.Fatalf("marker %q", body)
	}
}
