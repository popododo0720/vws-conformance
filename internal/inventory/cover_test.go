package inventory_test

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/popododo0720/vws-conformance/internal/engine"
	"github.com/popododo0720/vws-conformance/internal/inventory"
)

func TestInventoryCoversCases(t *testing.T) {
	path, err := inventory.Find()
	if err != nil {
		t.Fatal(err)
	}
	ops, err := inventory.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := inventory.Cover(ops, engine.Groups()); err != nil {
		t.Fatal(err)
	}
	for _, g := range engine.Groups() {
		for _, c := range g.Cases {
			if !strings.HasPrefix(c.DocURL, "https://docs.aws.amazon.com/") {
				t.Errorf("%s doc %q", c.ID, c.DocURL)
			}
		}
	}
}

func TestCheckoutMatchesInventory(t *testing.T) {
	root := os.Getenv("VWS_CONF_CONSOLE_API_SRC")
	if root == "" {
		t.Skip("VWS_CONF_CONSOLE_API_SRC is unset")
	}
	path, err := inventory.Find()
	if err != nil {
		t.Fatal(err)
	}
	file, err := inventory.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	live, err := inventory.FromCheckout(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := inventory.SameImpl(file, live); err != nil {
		t.Fatal(err)
	}
}

func TestDocURLsReturn200(t *testing.T) {
	if os.Getenv("VWS_CONF_DOC_CHECK") != "1" {
		t.Skip("VWS_CONF_DOC_CHECK is unset")
	}
	seen := map[string]bool{}
	var urls []string
	for _, g := range engine.Groups() {
		for _, c := range g.Cases {
			if seen[c.DocURL] {
				continue
			}
			seen[c.DocURL] = true
			urls = append(urls, c.DocURL)
			if len(urls) == 10 {
				break
			}
		}
		if len(urls) == 10 {
			break
		}
	}
	client := &http.Client{Timeout: 20 * time.Second}
	for _, u := range urls {
		resp, err := client.Get(u)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("%s status %d", u, resp.StatusCode)
		}
	}
}
