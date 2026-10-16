package harness

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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

func TestConcurrentClaimants(t *testing.T) {
	if os.Getenv("CONF_CLAIM_CHILD") == "1" {
		runClaimChild()
	}
	dir := t.TempDir()
	t.Setenv("VWS_CONF_ACTIVITY_DIR", dir)
	const id = "conf-20261015T120000Z-abc123"
	assertOneClaim(t, dir, id, "")
	assertOneClaim(t, dir, id, "empty")
	assertOneClaim(t, dir, id, deadPID(t))
}

func runClaimChild() {
	ready := os.Getenv("CONF_CLAIM_READY")
	if err := os.WriteFile(ready, []byte("1\n"), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	barrier := filepath.Join(os.Getenv("VWS_CONF_ACTIVITY_DIR"), "claim-go")
	for {
		if _, err := os.Stat(barrier); err == nil {
			break
		}
	}
	id := os.Getenv("CONF_CLAIM_ID")
	if err := ClaimRun(id); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	pid := strconv.Itoa(os.Getpid())
	dir := os.Getenv("VWS_CONF_ACTIVITY_DIR")
	if err := os.WriteFile(filepath.Join(dir, "won-"+pid), []byte(pid+"\n"), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	release := filepath.Join(dir, "claim-release")
	for {
		if _, err := os.Stat(release); err == nil {
			break
		}
	}
	fmt.Println(pid)
	os.Exit(0)
}

func assertOneClaim(t *testing.T, dir, id, seed string) {
	t.Helper()
	path := filepath.Join(dir, id)
	if callErr := os.Remove(path); callErr != nil && !os.IsNotExist(callErr) {
		panic(callErr)
	}
	if callErr := os.Remove(filepath.Join(dir, "claim-go")); callErr != nil && !os.IsNotExist(callErr) {
		panic(callErr)
	}
	if callErr := os.Remove(filepath.Join(dir, "claim-release")); callErr != nil && !os.IsNotExist(callErr) {
		panic(callErr)
	}
	oldWins, globErr := filepath.Glob(filepath.Join(dir, "won-*"))
	if globErr != nil {
		panic(globErr)
	}
	for _, name := range oldWins {
		if callErr := os.Remove(name); callErr != nil && !os.IsNotExist(callErr) {
			panic(callErr)
		}
	}
	switch seed {
	case "":
	case "empty":
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	default:
		if err := os.WriteFile(path, []byte(seed+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	const n = 12
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmds := make([]*exec.Cmd, n)
	outs := make([]*bytes.Buffer, n)
	for i := 0; i < n; i++ {
		ready := filepath.Join(dir, "ready-"+strconv.Itoa(i))
		if callErr := os.Remove(ready); callErr != nil && !os.IsNotExist(callErr) {
			panic(callErr)
		}
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestConcurrentClaimants$", "-test.count=1")
		cmd.Env = append(os.Environ(),
			"CONF_CLAIM_CHILD=1",
			"CONF_CLAIM_ID="+id,
			"CONF_CLAIM_READY="+ready,
			"VWS_CONF_ACTIVITY_DIR="+dir,
		)
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		cmds[i] = cmd
		outs[i] = &buf
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		readyN := 0
		for i := 0; i < n; i++ {
			if _, err := os.Stat(filepath.Join(dir, "ready-"+strconv.Itoa(i))); err == nil {
				readyN++
			}
		}
		if readyN == n {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, err := os.Stat(filepath.Join(dir, "ready-0")); err != nil {
		t.Fatal("claim children did not start")
	}
	for i := 0; i < n; i++ {
		if _, err := os.Stat(filepath.Join(dir, "ready-"+strconv.Itoa(i))); err != nil {
			t.Fatalf("ready %d missing", i)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "claim-go"), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var exited atomic.Int32
	for i := range cmds {
		go func(i int) {
			if callErr := cmds[i].Wait(); callErr != nil {
				// A losing claimant exits non-zero. One winner is the assertion.
				if _, writeErr := outs[i].WriteString(callErr.Error()); writeErr != nil {
					panic(writeErr)
				}
			}
			exited.Add(1)
		}(i)
	}
	deadline = time.Now().Add(15 * time.Second)
	var winners []string
	for time.Now().Before(deadline) {
		winners = nil
		matches, globErr := filepath.Glob(filepath.Join(dir, "won-*"))
		if globErr != nil {
			panic(globErr)
		}
		for _, name := range matches {
			b, readErr := os.ReadFile(name)
			if readErr == nil {
				winners = append(winners, strings.TrimSpace(string(b)))
			}
		}
		if len(winners)+int(exited.Load()) >= n {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	body, err := os.ReadFile(path)
	if len(winners) != 1 || err != nil || string(body) != winners[0]+"\n" {
		if callErr := os.WriteFile(filepath.Join(dir, "claim-release"), []byte("1\n"), 0o644); callErr != nil &&
			!os.IsNotExist(callErr) {
			panic(callErr)
		}
		var logs []string
		for _, buf := range outs {
			if s := strings.TrimSpace(buf.String()); s != "" {
				logs = append(logs, s)
			}
		}
		t.Fatalf("seed %q winners %q marker %q read %v logs %q", seed, winners, body, err, logs)
	}
	if err := os.WriteFile(filepath.Join(dir, "claim-release"), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitDeadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(waitDeadline) {
		if int(exited.Load()) == n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("claim children did not exit")
}
