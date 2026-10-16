package auth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

// mutationRule is the cleanup-ownership rule. A resource is created, written,
// detached, disassociated, or deleted only when this run created it, or its
// ownership and every attachment peer's ownership are independently verified.
// Unverifiable means preserve.
const mutationRule = "unverifiable ownership preserves the resource"

func TestAuthWritesOnlyABucketThisRunOwns(t *testing.T) {
	const (
		runID = "conf-20261016T120000Z-aabbcc"
		other = "conf-20261016T130000Z-bbccdd"
	)
	rows := []struct {
		name       string
		create     int
		tags       string
		activity   string
		wantWrite  bool
		wantDelete bool
		wantErr    bool
	}{
		{name: "foreign tag", create: http.StatusConflict, tags: "foreign", wantErr: true},
		{name: "no tag set", create: http.StatusConflict, tags: "none", wantErr: true},
		{name: "tagging unread", create: http.StatusConflict, tags: "broken", wantErr: true},
		{name: "finished other run", create: http.StatusConflict, tags: "foreign", activity: "0", wantErr: true},
		{name: "this run tag", create: http.StatusConflict, tags: "this", wantWrite: true, wantDelete: true},
		{name: "created here", create: http.StatusOK, tags: "none", wantWrite: true, wantDelete: true},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			if row.activity != "" {
				dir := t.TempDir()
				t.Setenv("VWS_CONF_ACTIVITY_DIR", dir)
				if err := os.WriteFile(filepath.Join(dir, other), []byte(row.activity+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var mu sync.Mutex
			wrote, deleted := false, false
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path := strings.Trim(r.URL.Path, "/")
				w.Header().Set("Content-Type", "application/xml")
				if r.URL.Query().Has("tagging") {
					writeAdoptTags(w, row.tags, runID, other, path)
					return
				}
				if r.Method == http.MethodDelete {
					mu.Lock()
					deleted = true
					mu.Unlock()
					w.WriteHeader(http.StatusNoContent)
					return
				}
				if r.Method == http.MethodPut && strings.Contains(path, "/") {
					mu.Lock()
					wrote = true
					mu.Unlock()
					w.WriteHeader(http.StatusOK)
					return
				}
				if r.Method == http.MethodPut {
					if row.create != http.StatusOK {
						w.WriteHeader(row.create)
						if _, err := io.WriteString(w, bucketAlreadyXML(path)); err != nil {
							panic(err)
						}
						return
					}
				}
				if _, err := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><ok/>`); err != nil {
					panic(err)
				}
			}))
			defer srv.Close()
			cfg := harness.Config{
				AccessKey: "AKIDEXAMPLE", Secret: "secret", Region: "vws-1",
				S3Endpoint: srv.URL, AllowedHosts: []string{"127.0.0.1"},
				AllowedAccounts: []string{"123456789012"},
				Target:          "local-fake", RunID: runID, OutDir: t.TempDir(), AccountID: "123456789012",
			}
			cl, err := harness.Load(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			rt := harness.NewRuntime(cfg, cl)
			_, callErr := run(context.Background(), rt, cases.Case{ID: "AUTH-010"})
			if cleanErr := rt.Clean.Run(context.Background()); cleanErr != nil && !row.wantErr {
				t.Fatal(cleanErr)
			}
			mu.Lock()
			defer mu.Unlock()
			if row.wantErr {
				if callErr == nil || wrote || deleted {
					t.Fatalf("%s: %s err %v wrote %v deleted %v", mutationRule, row.name, callErr, wrote, deleted)
				}
				return
			}
			if callErr != nil || wrote != row.wantWrite || deleted != row.wantDelete {
				t.Fatalf("%s: %s err %v wrote %v deleted %v", mutationRule, row.name, callErr, wrote, deleted)
			}
		})
	}
}

func writeAdoptTags(w http.ResponseWriter, kind, runID, other, bucket string) {
	switch kind {
	case "foreign":
		if _, err := io.WriteString(w, adoptTagXML(other)); err != nil {
			panic(err)
		}
	case "this":
		if _, err := io.WriteString(w, adoptTagXML(runID)); err != nil {
			panic(err)
		}
	case "broken":
		w.WriteHeader(http.StatusNotImplemented)
		if _, err := io.WriteString(w, `<Error><Code>NotImplemented</Code><Message>tagging is not served</Message>`+
			`<RequestId>r</RequestId></Error>`); err != nil {
			panic(err)
		}
	default:
		w.WriteHeader(http.StatusNotFound)
		if _, err := io.WriteString(w, `<Error><Code>NoSuchTagSet</Code><Message>The TagSet does not exist</Message>`+
			`<BucketName>`+bucket+`</BucketName><RequestId>r</RequestId></Error>`); err != nil {
			panic(err)
		}
	}
}

func adoptTagXML(run string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<Tagging xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><TagSet><Tag>` +
		`<Key>vws:conformance-run</Key><Value>` + run + `</Value></Tag></TagSet></Tagging>`
}

func bucketAlreadyXML(name string) string {
	return `<?xml version="1.0" encoding="UTF-8"?><Error><Code>BucketAlreadyOwnedByYou</Code>` +
		`<Message>Your previous request to create the named bucket succeeded and you already own it.</Message>` +
		`<BucketName>` + name + `</BucketName><RequestId>r</RequestId></Error>`
}
