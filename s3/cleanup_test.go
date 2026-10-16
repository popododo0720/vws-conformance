package s3

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/harness"
)

func TestCreateFreshDeletesEveryObject(t *testing.T) {
	cfg := harness.Config{
		AccessKey: "AKIDEXAMPLE", Secret: "secret", Region: "vws-1",
		AllowedHosts: []string{"127.0.0.1"}, AllowedAccounts: []string{"123456789012"},
		Target: "local-fake", RunID: "conf-20261015T120000Z-abc123", OutDir: t.TempDir(),
	}
	name := bucketName(harness.NewRuntime(cfg, nil), "S3-GAP-001")
	var mu sync.Mutex
	deleted := map[string]bool{}
	bucketGone := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if missingBucket(w, r) {
			return
		}
		if r.Method == http.MethodGet && strings.Contains(r.URL.RawQuery, "versions") {
			w.Header().Set("Content-Type", "application/xml")
			if _, writeErr := io.WriteString(w, `<ListVersionsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`+
				`<IsTruncated>false</IsTruncated></ListVersionsResult>`); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		if r.Method == http.MethodGet && strings.Contains(r.URL.RawQuery, "list-type=2") {
			w.Header().Set("Content-Type", "application/xml")
			if _, writeErr := io.WriteString(w, listOne("c")); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		if r.Method == http.MethodDelete {
			path := strings.TrimPrefix(r.URL.Path, "/")
			if path == name {
				mu.Lock()
				ok := deleted["c"]
				mu.Unlock()
				if !ok {
					w.Header().Set("Content-Type", "application/xml")
					w.WriteHeader(http.StatusConflict)
					if _, writeErr := io.WriteString(w, `<Error><Code>BucketNotEmpty</Code>`+
						`<Message>The bucket you tried to delete is not empty</Message>`+
						`<RequestId>r</RequestId></Error>`); writeErr != nil {
						panic(writeErr)
					}
					return
				}
				mu.Lock()
				bucketGone = true
				mu.Unlock()
				w.WriteHeader(http.StatusNoContent)
				return
			}
			key := strings.TrimPrefix(path, name+"/")
			mu.Lock()
			deleted[key] = true
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	cfg.S3Endpoint = srv.URL
	cl, err := harness.Load(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	rt := harness.NewRuntime(cfg, cl)
	if err := createFresh(context.Background(), rt, cl.S3, name); err != nil {
		t.Fatal(err)
	}
	if err := rt.Clean.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !deleted["c"] || !bucketGone {
		t.Fatalf("deleted %v bucket %v", deleted, bucketGone)
	}
}

func TestCleanupDeletesVersionsAndMarkers(t *testing.T) {
	cfg := harness.Config{
		AccessKey: "AKIDEXAMPLE", Secret: "secret", Region: "vws-1",
		AllowedHosts: []string{"127.0.0.1"}, AllowedAccounts: []string{"123456789012"},
		Target: "local-fake", RunID: "conf-20261015T120000Z-abc123", OutDir: t.TempDir(),
	}
	name := bucketName(harness.NewRuntime(cfg, nil), "S3-GAP-001")
	var mu sync.Mutex
	versions := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if missingBucket(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		if r.Method == http.MethodGet && strings.Contains(r.URL.RawQuery, "versions") {
			if _, writeErr := io.WriteString(w, versionList()); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		if r.Method == http.MethodGet && strings.Contains(r.URL.RawQuery, "list-type=2") {
			if _, writeErr := io.WriteString(w, emptyList()); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		if r.Method == http.MethodDelete {
			path := strings.TrimPrefix(r.URL.Path, "/")
			if path == name {
				mu.Lock()
				ok := versions["v1"] && versions["m1"]
				mu.Unlock()
				if !ok {
					w.WriteHeader(http.StatusConflict)
					if _, writeErr := io.WriteString(w,
						`<Error><Code>BucketNotEmpty</Code><Message>not empty</Message></Error>`); writeErr != nil {
						panic(writeErr)
					}
					return
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
			id := r.URL.Query().Get("versionId")
			mu.Lock()
			if id != "" {
				versions[id] = true
			}
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	cfg.S3Endpoint = srv.URL
	cl, err := harness.Load(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	rt := harness.NewRuntime(cfg, cl)
	if err := createFresh(context.Background(), rt, cl.S3, name); err != nil {
		t.Fatal(err)
	}
	if err := rt.Clean.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !versions["v1"] || !versions["m1"] {
		t.Fatalf("versions %v", versions)
	}
}

func missingBucket(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodHead {
		return false
	}
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusNotFound)
	if _, err := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
		`<Error><Code>NoSuchBucket</Code><Message>The specified bucket does not exist</Message>`+
		`<RequestId>r</RequestId></Error>`); err != nil {
		panic(err)
	}
	return true
}

func emptyList() string {
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">` +
		`<KeyCount>0</KeyCount><IsTruncated>false</IsTruncated></ListBucketResult>`
}

func versionList() string {
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<ListVersionsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">` +
		`<IsTruncated>false</IsTruncated>` +
		`<Version><Key>a</Key><VersionId>v1</VersionId><IsLatest>true</IsLatest>` +
		`<LastModified>2026-10-15T00:00:00.000Z</LastModified>` +
		`<ETag>&quot;abc&quot;</ETag><Size>1</Size><StorageClass>STANDARD</StorageClass></Version>` +
		`<DeleteMarker><Key>b</Key><VersionId>m1</VersionId><IsLatest>true</IsLatest>` +
		`<LastModified>2026-10-15T00:00:00.000Z</LastModified></DeleteMarker>` +
		`</ListVersionsResult>`
}

func listOne(key string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">` +
		`<Name>b</Name><KeyCount>1</KeyCount><MaxKeys>1000</MaxKeys><IsTruncated>false</IsTruncated>` +
		`<Contents><Key>` + key + `</Key><LastModified>2026-10-15T00:00:00.000Z</LastModified>` +
		`<ETag>&quot;abc&quot;</ETag><Size>1</Size><StorageClass>STANDARD</StorageClass>` +
		`</Contents></ListBucketResult>`
}
