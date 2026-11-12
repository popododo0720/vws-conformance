package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProbeRequestsAndRerun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test" || r.Header.Get("X-Vws-Region") != "pve-test-1" {
			t.Error("lost request headers")
		}
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			if err != nil || string(body) != "{}" || r.Header.Get("Content-Type") != "application/json" {
				t.Error("lost registration body")
			}
		}
		if _, err := io.WriteString(w, `{"items":[{}]}`); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()
	for range 2 {
		for _, method := range []string{"GET", "POST"} {
			var out bytes.Buffer
			err := run(t.Context(), []string{
				"-method", method, "-authorization", "Bearer test",
				"-region", "pve-test-1", srv.URL,
			}, strings.NewReader("{}"), &out)
			if err != nil || out.String() != `{"items":[{}]}` {
				t.Fatalf("%s: %v %s", method, err, out.String())
			}
		}
	}
}

func TestProbeRefusesBoundaryInput(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits++ }))
	defer srv.Close()
	for _, args := range [][]string{
		nil,
		{"-unknown"},
		{"-method", "DELETE", "http://127.0.0.1"},
		{"http://192.0.2.1"},
		{"http://localhost"},
		{"https://s3.amazonaws.com"},
		{strings.Replace(srv.URL, "//", "//user:secret@", 1)},
		{"http://127.0.0.1", "extra"},
	} {
		if err := run(t.Context(), args, nil, io.Discard); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	if hits != 0 {
		t.Fatal("invalid URL reached the server")
	}
}

func TestProbeRefusesRedirectAndHTTPFailure(t *testing.T) {
	hits := 0
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits++ }))
	defer destination.Close()
	for _, status := range []int{http.StatusFound, http.StatusInternalServerError} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", destination.URL)
			w.WriteHeader(status)
		}))
		err := run(t.Context(), []string{srv.URL}, nil, io.Discard)
		srv.Close()
		if err == nil {
			t.Fatalf("accepted HTTP %d", status)
		}
	}
	if hits != 0 {
		t.Fatal("followed a redirect")
	}
}
