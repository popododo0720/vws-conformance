package inventory

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const fixtureCommit = "0123456789abcdef0123456789abcdef01234567"

// checkout copies the fixture console-api tree into a temp dir at fixtureCommit, then applies
// edits {path, old, new}. An edit that matches nothing fails the test, so no mutation case can
// pass by changing nothing.
func checkout(t *testing.T, edits ...[3]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("testdata", "console-api"))); err != nil {
		t.Fatal(err)
	}
	for _, e := range edits {
		path := filepath.Join(dir, filepath.FromSlash(e[0]))
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), e[1]) {
			t.Fatalf("edit of %s matches nothing: %q", e[0], e[1])
		}
		if err := os.WriteFile(path, []byte(strings.Replace(string(b), e[1], e[2], 1)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeHead(t, dir, fixtureCommit)
	return dir
}

func writeHead(t *testing.T, dir, head string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte(head+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExtract(t *testing.T) {
	inv, err := Extract(checkout(t))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for op := range inv.Ops {
		got = append(got, op.String())
	}
	slices.Sort(got)
	want := []string{
		"ec2:DescribeVpcs", "ec2:RunInstances",
		"s3:CreateBucket", "s3:DeleteBucket", "s3:DeleteObject", "s3:GetObject", "s3:HeadBucket",
		"s3:HeadObject", "s3:ListBuckets", "s3:ListObjects", "s3:ListObjectsV2", "s3:PutObject",
	}
	if !slices.Equal(got, want) {
		t.Errorf("operations\n got %v\nwant %v", got, want)
	}
	if inv.Commit != fixtureCommit {
		t.Errorf("commit %s, want %s", inv.Commit, fixtureCommit)
	}
}

func TestExtractFollowsGitdirFile(t *testing.T) {
	dir := checkout(t)
	gitDir := t.TempDir()
	writeHead(t, gitDir, strings.Repeat("ab", 20))
	if err := os.RemoveAll(filepath.Join(dir, ".git")); err != nil {
		t.Fatal(err)
	}
	pointer := []byte("gitdir: " + filepath.Join(gitDir, ".git") + "\n")
	if err := os.WriteFile(filepath.Join(dir, ".git"), pointer, 0o644); err != nil {
		t.Fatal(err)
	}
	inv, err := Extract(dir)
	if err != nil {
		t.Fatal(err)
	}
	if inv.Commit != strings.Repeat("ab", 20) {
		t.Errorf("commit %s", inv.Commit)
	}
}

// TestExtractRejectsShapeChange is the "fails loudly" contract: a console-api refactor the tool
// cannot read must be an error naming the file, not a smaller inventory.
func TestExtractRejectsShapeChange(t *testing.T) {
	const (
		ec2  = fileEC2
		s3   = fileS3
		opfn = fileOperation
	)
	tests := []struct {
		name string
		edit [3]string
		want string
	}{
		{
			"second Unimplemented switch",
			[3]string{
				ec2, "switch action {",
				"switch other {\n\tdefault:\n\t\tx(resource.Unimplemented(a))\n\t}\n\tswitch action {",
			},
			ec2 + ":8: second switch whose default",
		},
		{
			"no Unimplemented default",
			[3]string{ec2, "resource.Unimplemented(action)", "nil"},
			ec2 + ":4: switch on action whose default does not call",
		},
		{
			"unresolvable Action ident",
			[3]string{ec2, "ActionRunInstances.String()", "ActionBogus.String()"},
			"ActionBogus is not in Action.String",
		},
		{"non-literal case", [3]string{ec2, `"DescribeVpcs"`, "name"}, "neither a string literal"},
		{
			"malformed case name",
			[3]string{ec2, `"DescribeVpcs"`, `"Describe Vpcs"`},
			ec2 + `:7: dispatch case "Describe Vpcs" is not`,
		},
		{"repeated dispatch case", [3]string{ec2, `"DescribeVpcs"`, `"RunInstances"`}, "dispatched twice"},
		{
			"Action.String not a switch",
			[3]string{opfn, "switch a {", "if a == 0 {\n\t\treturn \"\"\n\t}\n\tswitch a {"},
			"not a single switch",
		},
		{"case returns a call", [3]string{opfn, `return "RunInstances"`, "return name()"}, "string literal"},
		{
			"extra mux route",
			[3]string{s3, "return mux", "mux.HandleFunc(\"POST /{bucket}\", h.post)\n\treturn mux"},
			"registered but not in the S3 route table",
		},
		{
			"missing mux route",
			[3]string{s3, "\tmux.HandleFunc(\"HEAD /{bucket}\", h.headBucket)\n", ""},
			"in the S3 route table but not registered",
		},
		{
			"dispatch in an if before the switch",
			[3]string{
				ec2, "\tswitch action {",
				"\tif action == \"DescribeRegions\" {\n\t\th.describeRegions()\n\t}\n\tswitch action {",
			},
			ec2 + ":4: action is compared outside the dispatch switch",
		},
		{
			"second switch on action",
			[3]string{
				ec2, "\tswitch action {",
				"\tswitch action {\n\tcase \"DescribeRegions\":\n\t\th.describeRegions()\n\t}\n\tswitch action {",
			},
			ec2 + ":4: switch on action whose default",
		},
		{
			"route registered with Handle",
			[3]string{s3, "return mux", "mux.Handle(\"POST /{bucket}\", http.HandlerFunc(h.post))\n\treturn mux"},
			"registered but not in the S3 route table",
		},
		{"computed mux pattern", [3]string{s3, `"HEAD /{bucket}"`, "pattern"}, "not a unique string literal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Extract(checkout(t, tt.edit))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestExtractRejectsBadCheckout(t *testing.T) {
	t.Run("symbolic HEAD", func(t *testing.T) {
		dir := checkout(t)
		writeHead(t, dir, "ref: refs/heads/main")
		if _, err := Extract(dir); err == nil || !strings.Contains(err.Error(), "detached") {
			t.Fatalf("error %v", err)
		}
	})
	t.Run("short HEAD", func(t *testing.T) {
		dir := checkout(t)
		writeHead(t, dir, "0123abcd")
		if _, err := Extract(dir); err == nil {
			t.Fatal("want an error")
		}
	})
	t.Run("missing git dir", func(t *testing.T) {
		if _, err := Extract(t.TempDir()); err == nil {
			t.Fatal("want an error")
		}
	})
	t.Run("missing source file", func(t *testing.T) {
		dir := checkout(t)
		if err := os.Remove(filepath.Join(dir, "internal/surface/s3rest/s3rest.go")); err != nil {
			t.Fatal(err)
		}
		if _, err := Extract(dir); err == nil || !strings.Contains(err.Error(), "s3rest.go") {
			t.Fatalf("error %v", err)
		}
	})
	t.Run("syntax error", func(t *testing.T) {
		if _, err := Extract(checkout(t, [3]string{fileEC2, "func (h *handler) serve() {", "func ("})); err == nil {
			t.Fatal("want an error")
		}
	})
}
