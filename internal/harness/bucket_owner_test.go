package harness

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBucketTaggingErrorLeavesTheBucket(t *testing.T) {
	const (
		current = "conf-20261015T120000Z-abc123"
		name    = "conf-20261015t120000z-abc123"
	)
	var mu sync.Mutex
	present := true
	deleted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			mu.Lock()
			present = false
			deleted = true
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Query().Has("tagging") {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotImplemented)
			if _, writeErr := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
				`<Error><Code>NotImplemented</Code><Message>tagging is not served</Message>`+
				`<RequestId>r</RequestId></Error>`); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		if r.URL.Query().Get("list-type") == "2" {
			w.Header().Set("Content-Type", "application/xml")
			if _, writeErr := io.WriteString(w, emptyListXML(name)); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		if r.URL.Query().Has("versions") {
			w.WriteHeader(http.StatusNotImplemented)
			if _, writeErr := io.WriteString(w, "NotImplemented"); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		if r.Method == http.MethodGet {
			mu.Lock()
			names := map[string]bool{}
			if present {
				names[name] = true
			}
			body := listBucketsXML(names)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/xml")
			if _, writeErr := io.WriteString(w, body); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		status, body := ec2Describe(ec2Action(r), false)
		writeEC2(w, status, body)
	}))
	defer srv.Close()
	rt := runtimeAt(t, srv.URL, t.TempDir(), current)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := rt.Sweep(ctx)
	mu.Lock()
	defer mu.Unlock()
	if err == nil || !strings.Contains(err.Error(), "NotImplemented") || !strings.Contains(err.Error(), name) {
		t.Fatalf("deleted %v err %v", deleted, err)
	}
	if deleted {
		t.Fatal("deleted a bucket whose owner tag could not be read")
	}
}

func TestForeignRunTagVetoesAFinishedBucket(t *testing.T) {
	const (
		current  = "conf-20261015T120000Z-abc123"
		finished = "conf-20261015T110000Z-aaa111"
		active   = "conf-20261015T130000Z-def456"
	)
	finishedName := "conf-20261015t110000z-aaa111"
	currentName := "conf-20261015t120000z-abc123"
	activity := t.TempDir()
	t.Setenv("VWS_CONF_ACTIVITY_DIR", activity)
	if err := writeActivity(t, activity, finished, "0"); err != nil {
		t.Fatal(err)
	}
	if err := writeActivity(t, activity, active, "1"); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	present := map[string]bool{finishedName: true, currentName: true}
	deleted := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.Trim(r.URL.Path, "/")
		if r.Method == http.MethodDelete {
			mu.Lock()
			delete(present, name)
			deleted[name] = true
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Query().Has("tagging") {
			w.Header().Set("Content-Type", "application/xml")
			if name == finishedName {
				if _, writeErr := io.WriteString(w, taggingXML(active)); writeErr != nil {
					panic(writeErr)
				}
				return
			}
			w.WriteHeader(http.StatusNotFound)
			if _, writeErr := io.WriteString(w, noSuchTagSetXML(name)); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		if r.URL.Query().Get("list-type") == "2" {
			w.Header().Set("Content-Type", "application/xml")
			if _, writeErr := io.WriteString(w, emptyListXML(name)); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		if r.URL.Query().Has("versions") {
			w.WriteHeader(http.StatusNotImplemented)
			if _, writeErr := io.WriteString(w, "NotImplemented"); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		if r.Method == http.MethodGet {
			mu.Lock()
			body := listBucketsXML(present)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/xml")
			if _, writeErr := io.WriteString(w, body); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		status, body := ec2Describe(ec2Action(r), false)
		writeEC2(w, status, body)
	}))
	defer srv.Close()
	rt := runtimeAt(t, srv.URL, t.TempDir(), current)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rt.Sweep(ctx); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if deleted[finishedName] {
		t.Fatalf("deleted finished bucket tagged for active run %s", active)
	}
	if !deleted[currentName] {
		t.Fatal("untagged current bucket was kept")
	}
}

func writeActivity(t *testing.T, dir, id, body string) error {
	t.Helper()
	if err := os.WriteFile(dir+"/"+id, []byte(body+"\n"), 0o644); err != nil {
		return fmt.Errorf("activity %s: %w", id, err)
	}
	return nil
}

func taggingXML(run string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<Tagging xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><TagSet><Tag>` +
		`<Key>vws:conformance-run</Key><Value>` + run + `</Value>` +
		`</Tag></TagSet></Tagging>`
}

func noSuchTagSetXML(name string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<Error><Code>NoSuchTagSet</Code><Message>The TagSet does not exist</Message>` +
		`<BucketName>` + name + `</BucketName><RequestId>r</RequestId></Error>`
}

func emptyListXML(name string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">` +
		`<Name>` + name + `</Name><IsTruncated>false</IsTruncated></ListBucketResult>`
}
