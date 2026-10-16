package ec2

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func TestFailedKeyCreateDoesNotScheduleDeletion(t *testing.T) {
	deleted := runKeyCleanup(t, "EC2-KP-001", false)
	if deleted {
		t.Fatal("failed create scheduled DeleteKeyPair")
	}
}

func TestUnexpectedImportIsDeleted(t *testing.T) {
	deleted := runKeyCleanup(t, "EC2-KP-104", true)
	if !deleted {
		t.Fatal("successful import left the key")
	}
}

func runKeyCleanup(t *testing.T, id string, succeed bool) bool {
	t.Helper()
	var mu sync.Mutex
	deleted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}
		vals, err := url.ParseQuery(string(body))
		if err != nil {
			panic(err)
		}
		action := vals.Get("Action")
		w.Header().Set("Content-Type", "text/xml")
		switch action {
		case "DeleteKeyPair":
			mu.Lock()
			deleted = true
			mu.Unlock()
			if _, writeErr := io.WriteString(w, ec2Item("DeleteKeyPairResponse", "<return>true</return>")); writeErr != nil {
				panic(writeErr)
			}
		case "CreateKeyPair":
			w.WriteHeader(http.StatusBadRequest)
			if _, writeErr := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
				`<Response><Errors><Error><Code>InvalidKeyPair.Duplicate</Code>`+
				`<Message>exists</Message></Error></Errors><RequestID>r</RequestID></Response>`); writeErr != nil {
				panic(writeErr)
			}
		case "DescribeKeyPairs":
			name := vals.Get("KeyName.1")
			if _, writeErr := io.WriteString(w, ec2Item("DescribeKeyPairsResponse",
				"<keySet><item><keyName>"+name+"</keyName><keyFingerprint>ab</keyFingerprint></item></keySet>")); writeErr != nil {
				panic(writeErr)
			}
		case "ImportKeyPair":
			if !succeed {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			name := vals.Get("KeyName")
			if _, writeErr := io.WriteString(w, ec2Item("ImportKeyPairResponse",
				"<keyName>"+name+"</keyName><keyFingerprint>ab</keyFingerprint>")); writeErr != nil {
				panic(writeErr)
			}
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()
	rt := keyRuntime(t, srv.URL)
	var spec cases.Case
	for _, c := range KeyPairs().Cases {
		if c.ID == id {
			spec = c
		}
	}
	if _, err := runKeys(context.Background(), rt, spec); id == "EC2-KP-104" && err != nil {
		t.Fatal(err)
	}
	if err := rt.Clean.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	return deleted
}

func keyRuntime(t *testing.T, endpoint string) *harness.Runtime {
	t.Helper()
	cfg := harness.Config{
		AccessKey: "AKIDEXAMPLE", Secret: "secret", Region: "vws-1",
		EC2Endpoint: endpoint, AllowedHosts: []string{"127.0.0.1"},
		AllowedAccounts: []string{"123456789012"},
		Target:          "local-fake", RunID: "conf-20261015T120000Z-abc123",
		OutDir: t.TempDir(), AccountID: "123456789012",
	}
	cl, err := harness.Load(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return harness.NewRuntime(cfg, cl)
}
