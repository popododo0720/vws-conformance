package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mutatingSDKCalls is the census. A mutating selector outside act.go skips the gate.
var mutatingSDKCalls = []string{
	"TerminateInstances",
	"StopInstances",
	"StartInstances",
	"RebootInstances",
	"DeleteVolume",
	"DetachVolume",
	"AttachVolume",
	"DeleteSnapshot",
	"ReleaseAddress",
	"DisassociateAddress",
	"AssociateAddress",
	"DeleteKeyPair",
	"DeleteSecurityGroup",
	"DeleteSubnet",
	"DeleteVpc",
	"DetachInternetGateway",
	"DeleteInternetGateway",
	"DeleteBucket",
	"DeleteObject",
	"CreateTags",
	"DeleteTags",
	"ModifyInstanceAttribute",
	"CreateImage",
	"DeregisterImage",
	"PutObject",
	"CreateBucket",
}

func TestMutatingCallsStayInTheGate(t *testing.T) {
	root := moduleRoot(t)
	var hits []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "vendor" || d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("rel %s: %w", path, err)
		}
		if rel == filepath.Join("internal", "harness", "act.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		for _, name := range mutatingSDKCalls {
			if strings.Contains(string(body), "."+name+"(") {
				hits = append(hits, rel+": "+name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) > 0 {
		t.Fatalf("mutating SDK call outside the gate:\n%s", strings.Join(hits, "\n"))
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
