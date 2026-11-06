package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixtureCommit = "0123456789abcdef0123456789abcdef01234567"

// fixture copies the console-api tree shared with the inventory tests and gives it a detached HEAD.
func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("..", "..", "inventory", "testdata", "console-api"))); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte(fixtureCommit+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func read(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestWriteIsIdempotentAndCheckPasses(t *testing.T) {
	src, out := fixture(t), filepath.Join(t.TempDir(), "inventory.json")
	if code, msg := run([]string{"-src", src, "-out", out}); code != 0 {
		t.Fatalf("write exit %d: %s", code, msg)
	}
	first := read(t, out)
	if code, msg := run([]string{"-src", src, "-out", out}); code != 0 {
		t.Fatalf("rewrite exit %d: %s", code, msg)
	}
	if len(first) == 0 || !bytes.Equal(first, read(t, out)) {
		t.Errorf("rerun changed the bytes")
	}
	if code, msg := run([]string{"-src", src, "-out", out, "-check"}); code != 0 {
		t.Errorf("check on fresh output exit %d: %s", code, msg)
	}
}

func TestCheckFailsOnDriftAndWritesNothing(t *testing.T) {
	src, out := fixture(t), filepath.Join(t.TempDir(), "inventory.json")
	if code, _ := run([]string{"-src", src, "-out", out}); code != 0 {
		t.Fatal("write failed")
	}
	good := read(t, out)
	stale := bytes.Replace(good, []byte("      \"CreateUser\",\n"), nil, 1)
	if bytes.Equal(stale, good) {
		t.Fatal("fixture edit matched nothing")
	}
	if err := os.WriteFile(out, stale, 0o644); err != nil {
		t.Fatal(err)
	}
	code, msg := run([]string{"-src", src, "-out", out, "-check"})
	if code != 1 || !strings.Contains(msg, "differs") || !strings.Contains(msg, `+ "CreateUser",`) {
		t.Errorf("exit %d, message %q", code, msg)
	}
	if !bytes.Equal(read(t, out), stale) {
		t.Error("-check rewrote the file")
	}
}

func TestFailedExtractionKeepsOldFileAndLeavesNoTemp(t *testing.T) {
	src, dir := fixture(t), t.TempDir()
	out := filepath.Join(dir, "inventory.json")
	if err := os.WriteFile(out, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(src, "internal", "surface", "s3rest", "s3rest.go")); err != nil {
		t.Fatal(err)
	}
	if code, _ := run([]string{"-src", src, "-out", out}); code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	if string(read(t, out)) != "old\n" {
		t.Errorf("old file changed: %q", read(t, out))
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("directory has %d entries, want only inventory.json", len(entries))
	}
}

func TestUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"-src", "x", "extra"}, {"-nope"}} {
		if code, _ := run(args); code != 2 {
			t.Errorf("run(%v) exit %d, want 2", args, code)
		}
	}
}
