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
)

func TestUnexpectedNegativeCreateIsTracked(t *testing.T) {
	for _, row := range []struct {
		id     string
		create string
		clean  string
	}{
		{id: "EC2-RUN-101", create: "RunInstances", clean: "TerminateInstances"},
		{id: "EC2-EIP-105", create: "AllocateAddress", clean: "ReleaseAddress"},
	} {
		t.Run(row.id, func(t *testing.T) {
			if !negativeCreateCleans(t, row.id, row.create, row.clean, Addresses().Cases, Instances().Cases) {
				t.Fatalf("%s left the created resource", row.id)
			}
		})
	}
}

func negativeCreateCleans(t *testing.T, id, create, clean string, groups ...[]cases.Case) bool {
	t.Helper()
	var mu sync.Mutex
	cleaned := false
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
		if action == clean {
			mu.Lock()
			cleaned = true
			mu.Unlock()
			if _, writeErr := io.WriteString(w, ec2Item(clean+"Response", "<return>true</return>")); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		if action == "DescribeVolumes" || action == "DescribeInstances" {
			if _, writeErr := io.WriteString(w, emptyDescribe(action)); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		if action == "DescribeAddresses" {
			if _, writeErr := io.WriteString(w, ec2Item("DescribeAddressesResponse",
				"<addressesSet><item><allocationId>eipalloc-leak</allocationId><domain>vpc</domain>"+
					"<publicIp>203.0.113.10</publicIp></item></addressesSet>")); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		if action != create {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, writeErr := io.WriteString(w, createdXML(action)); writeErr != nil {
			panic(writeErr)
		}
	}))
	defer srv.Close()
	rt := keyRuntime(t, srv.URL)
	var spec cases.Case
	for _, group := range groups {
		for _, c := range group {
			if c.ID == id {
				spec = c
			}
		}
	}
	var err error
	switch id {
	case "EC2-RUN-101":
		_, err = runInstances(context.Background(), rt, spec)
	default:
		_, err = runAddr(context.Background(), rt, spec)
	}
	if err != nil {
		t.Fatal(err)
	}
	if cleanErr := rt.Clean.Run(context.Background()); cleanErr != nil {
		t.Fatal(cleanErr)
	}
	mu.Lock()
	defer mu.Unlock()
	return cleaned
}

func createdXML(action string) string {
	switch action {
	case "RunInstances":
		return runInstanceXML("i-leak")
	case "AllocateAddress":
		return `<?xml version="1.0" encoding="UTF-8"?>` +
			`<AllocateAddressResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">` +
			`<requestId>r</requestId><publicIp>203.0.113.10</publicIp><domain>vpc</domain>` +
			`<allocationId>eipalloc-leak</allocationId></AllocateAddressResponse>`
	default:
		return ec2Item(action+"Response", "<return>true</return>")
	}
}

func emptyDescribe(action string) string {
	inner := "<volumeSet/>"
	switch action {
	case "DescribeAddresses":
		inner = "<addressesSet/>"
	case "DescribeInstances":
		inner = "<reservationSet/>"
	}
	return ec2Item(action+"Response", inner)
}
