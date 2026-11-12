// Command conf-probe sends local CI setup requests through the conformance egress guard.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/popododo0720/vws-conformance/internal/harness"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "conf-probe: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, input io.Reader, output io.Writer) (err error) {
	fs := flag.NewFlagSet("conf-probe", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	method := fs.String("method", http.MethodGet, "GET or POST")
	auth := fs.String("authorization", "", "synthetic test authorization")
	region := fs.String("region", "", "console region")
	if fs.Parse(args) != nil || fs.NArg() != 1 || (*method != http.MethodGet && *method != http.MethodPost) {
		return errors.New("expected GET or POST flags and one URL")
	}
	allowed := []string{"127.0.0.1"}
	if err := harness.CheckEndpoint(fs.Arg(0), allowed); err != nil {
		return fmt.Errorf("probe endpoint: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if *method == http.MethodGet {
		input = nil
	}
	req, err := http.NewRequestWithContext(ctx, *method, fs.Arg(0), input)
	if err != nil {
		return fmt.Errorf("probe request: %w", err)
	}
	if req.URL.User != nil {
		return errors.New("probe URL user info is refused")
	}
	req.Header.Set("Authorization", *auth)
	req.Header.Set("X-Vws-Region", *region)
	if *method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := harness.NewHTTPClient(allowed, nil).Do(req)
	if err != nil {
		return fmt.Errorf("probe send: %w", err)
	}
	defer func() { err = errors.Join(err, resp.Body.Close()) }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("probe HTTP status %d", resp.StatusCode)
	}
	if _, err := io.Copy(output, resp.Body); err != nil {
		return fmt.Errorf("probe response: %w", err)
	}
	return nil
}
