// Package iamfake is a loopback IAM query-protocol server for the virtual MFA device calls the
// gate and the MFA device and tag cases make. It answers as the AWS IAM API reference documents, so a case
// that passes here fails only on a real parity gap. Tests use it so no request leaves 127.0.0.1.
package iamfake

import (
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"
)

// Account is the account every device serial number names.
const Account = "123456789012"

// invalidInputMessage is the description API_TagMFADevice documents for InvalidInput.
const invalidInputMessage = "The request was rejected because an invalid or out-of-range value " +
	"was supplied for an input parameter."

// Seed is the Base32StringSeed every created device answers with; no report may contain it.
const Seed = "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXPFAKESEED"

// maxTags is the AWS limit of tags per IAM resource (TagMFADevice LimitExceeded).
const maxTags = 50

var (
	tagKeyRE   = regexp.MustCompile(`^[\p{L}\p{Z}\p{N}_.:/=+\-@]{1,128}$`)
	tagValueRE = regexp.MustCompile(`^[\p{L}\p{Z}\p{N}_.:/=+\-@]*$`)
)

type Server struct {
	*httptest.Server

	mu       sync.Mutex
	devices  map[string]map[string]string
	calls    map[string]int
	onCreate func() string
}

func New() *Server {
	s := &Server{devices: map[string]map[string]string{}, calls: map[string]int{}}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	return s
}

// SetCreateHook runs fn before CreateVirtualMFADevice is applied. A non-empty result is the
// 400 error code to answer with, and nothing is created.
func (s *Server) SetCreateHook(fn func() string) {
	s.mu.Lock()
	s.onCreate = fn
	s.mu.Unlock()
}

func (s *Server) Calls(action string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[action]
}

// Device returns a copy of the tags of the device with this serial number.
func (s *Server) Device(serial string) (map[string]string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tags, ok := s.devices[serial]
	if !ok {
		return nil, false
	}
	out := make(map[string]string, len(tags))
	for k, v := range tags {
		out[k] = v
	}
	return out, true
}

func (s *Server) PutDevice(serial string, tags map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.devices[serial] = tags
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	action := r.Form.Get("Action")
	s.mu.Lock()
	s.calls[action]++
	hook := s.onCreate
	s.mu.Unlock()
	if action == "CreateVirtualMFADevice" && hook != nil {
		if code := hook(); code != "" {
			writeError(w, http.StatusBadRequest, code, "refused")
			return
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch action {
	case "CreateVirtualMFADevice":
		s.create(w, r)
	case "ListVirtualMFADevices":
		s.list(w, r)
	case "DeleteVirtualMFADevice", "TagMFADevice", "UntagMFADevice", "ListMFADeviceTags":
		tags, ok := s.devices[r.Form.Get("SerialNumber")]
		if !ok {
			writeError(w, http.StatusNotFound, "NoSuchEntity",
				"The MFA device with serial number "+r.Form.Get("SerialNumber")+" cannot be found.")
			return
		}
		s.device(w, r, action, tags)
	default:
		writeError(w, http.StatusBadRequest, "InvalidAction", "unsupported action "+action)
	}
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	name := r.Form.Get("VirtualMFADeviceName")
	serial := "arn:aws:iam::" + Account + ":mfa/" + name
	if _, ok := s.devices[serial]; ok {
		writeError(w, http.StatusConflict, "EntityAlreadyExists", "MFA device already exists.")
		return
	}
	tags := map[string]string{}
	if status, code, msg := merge(tags, members(r, "Tags")); code != "" {
		writeError(w, status, code, msg)
		return
	}
	s.devices[serial] = tags
	writeXML(w, "CreateVirtualMFADevice", "<VirtualMFADevice><SerialNumber>"+html.EscapeString(serial)+
		"</SerialNumber><Base32StringSeed>"+Seed+"</Base32StringSeed></VirtualMFADevice>")
}

// list answers in one page. No device here is ever enabled for a user, so every device is
// Unassigned and an Assigned filter lists none.
func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	if r.Form.Get("AssignmentStatus") != "Assigned" {
		serials := make([]string, 0, len(s.devices))
		for serial := range s.devices {
			serials = append(serials, serial)
		}
		slices.Sort(serials)
		for _, serial := range serials {
			fmt.Fprintf(&b, "<member><SerialNumber>%s</SerialNumber></member>", html.EscapeString(serial))
		}
	}
	writeXML(w, "ListVirtualMFADevices", "<VirtualMFADevices>"+b.String()+
		"</VirtualMFADevices><IsTruncated>false</IsTruncated>")
}

func (s *Server) device(w http.ResponseWriter, r *http.Request, action string, tags map[string]string) {
	serial := r.Form.Get("SerialNumber")
	switch action {
	case "DeleteVirtualMFADevice":
		delete(s.devices, serial)
		writeXML(w, action, "")
	case "TagMFADevice":
		merged := make(map[string]string, len(tags))
		for k, v := range tags {
			merged[k] = v
		}
		if status, code, msg := merge(merged, members(r, "Tags")); code != "" {
			writeError(w, status, code, msg)
			return
		}
		s.devices[serial] = merged
		writeXML(w, action, "")
	case "UntagMFADevice":
		var keys []string
		for i := 1; r.Form.Has(fmt.Sprintf("TagKeys.member.%d", i)); i++ {
			key := r.Form.Get(fmt.Sprintf("TagKeys.member.%d", i))
			if !tagKeyRE.MatchString(key) {
				writeError(w, http.StatusBadRequest, "InvalidInput", invalidInputMessage)
				return
			}
			keys = append(keys, key)
		}
		for _, key := range keys {
			delete(tags, key)
		}
		writeXML(w, action, "")
	default:
		keys := make([]string, 0, len(tags))
		for k := range tags {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		var b strings.Builder
		for _, k := range keys {
			fmt.Fprintf(&b, "<member><Key>%s</Key><Value>%s</Value></member>", html.EscapeString(k),
				html.EscapeString(tags[k]))
		}
		writeXML(w, action, "<Tags>"+b.String()+"</Tags><IsTruncated>false</IsTruncated>")
	}
}

type tag struct{ key, value string }

func members(r *http.Request, list string) []tag {
	var out []tag
	for i := 1; r.Form.Has(fmt.Sprintf("%s.member.%d.Key", list, i)); i++ {
		out = append(out, tag{
			key:   r.Form.Get(fmt.Sprintf("%s.member.%d.Key", list, i)),
			value: r.Form.Get(fmt.Sprintf("%s.member.%d.Value", list, i)),
		})
	}
	return out
}

// merge validates every tag before it writes any into dst, as AWS stores nothing from a request
// that fails. It returns the HTTP status, code and message of the first failure.
func merge(dst map[string]string, tags []tag) (int, string, string) {
	for _, t := range tags {
		switch {
		case !tagKeyRE.MatchString(t.key), utf8.RuneCountInString(t.value) > 256, !tagValueRE.MatchString(t.value):
			return http.StatusBadRequest, "InvalidInput", invalidInputMessage
		case strings.HasPrefix(strings.ToLower(t.key), "aws:"):
			return http.StatusBadRequest, "InvalidInput", invalidInputMessage
		}
	}
	for _, t := range tags {
		dst[t.key] = t.value
	}
	if len(dst) > maxTags {
		return http.StatusConflict, "LimitExceeded", "Cannot exceed quota for TagsPerMfaDevice: 50."
	}
	return 0, "", ""
}

func writeXML(w http.ResponseWriter, action, result string) {
	w.Header().Set("Content-Type", "text/xml")
	_, err := fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><%[1]sResponse><%[1]sResult>%[2]s</%[1]sResult>`+
		`<ResponseMetadata><RequestId>req-%[1]s</RequestId></ResponseMetadata></%[1]sResponse>`, action, result)
	note(err)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(status)
	_, err := fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><ErrorResponse><Error><Type>Sender</Type>`+
		`<Code>%s</Code><Message>%s</Message></Error><RequestId>req-error</RequestId></ErrorResponse>`,
		code, html.EscapeString(msg))
	note(err)
}

func note(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "iamfake: %v\n", err)
	}
}
