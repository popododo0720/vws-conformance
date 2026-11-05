// Package ec2fake is a loopback EC2 query-protocol server for the volume calls the gate makes.
// Tests use it so no request leaves 127.0.0.1.
package ec2fake

import (
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
)

type Server struct {
	*httptest.Server

	mu       sync.Mutex
	vols     map[string]map[string]string
	calls    map[string]int
	seq      int
	onCreate func() CreateResult
}

// CreateResult is what a create hook asks CreateVolume to do. The zero value creates the volume.
type CreateResult struct {
	Code string // answer with this error code and create nothing
	Drop bool   // close the connection without an answer
}

func New() *Server {
	s := &Server{vols: map[string]map[string]string{}, calls: map[string]int{}}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	return s
}

// SetCreateHook runs fn on the request goroutine before CreateVolume is applied; fn may block.
func (s *Server) SetCreateHook(fn func() CreateResult) {
	s.mu.Lock()
	s.onCreate = fn
	s.mu.Unlock()
}

func (s *Server) Calls(action string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[action]
}

func (s *Server) Volume(id string) (map[string]string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tags, ok := s.vols[id]
	return tags, ok
}

func (s *Server) PutVolume(id string, tags map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vols[id] = tags
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	action := r.Form.Get("Action")
	s.mu.Lock()
	s.calls[action]++
	s.mu.Unlock()
	switch action {
	case "CreateVolume":
		s.create(w, r)
	case "DescribeVolumes":
		s.describe(w, r.Form.Get("VolumeId.1"))
	case "DeleteVolume":
		s.delete(w, r.Form.Get("VolumeId"))
	default:
		writeError(w, action, "InvalidAction", "unsupported action "+action)
	}
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	hook := s.onCreate
	s.mu.Unlock()
	var res CreateResult
	if hook != nil {
		res = hook()
	}
	if res.Drop {
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "hijack unsupported", http.StatusInternalServerError)
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			note(err)
			return
		}
		note(conn.Close())
		return
	}
	if res.Code != "" {
		writeError(w, "CreateVolume", res.Code, "refused")
		return
	}
	tags := map[string]string{}
	for i := 1; r.Form.Get(fmt.Sprintf("TagSpecification.%d.ResourceType", i)) != ""; i++ {
		if r.Form.Get(fmt.Sprintf("TagSpecification.%d.ResourceType", i)) != "volume" {
			continue
		}
		for j := 1; ; j++ {
			key := r.Form.Get(fmt.Sprintf("TagSpecification.%d.Tag.%d.Key", i, j))
			if key == "" {
				break
			}
			tags[key] = r.Form.Get(fmt.Sprintf("TagSpecification.%d.Tag.%d.Value", i, j))
		}
	}
	s.mu.Lock()
	s.seq++
	id := fmt.Sprintf("vol-%017d", s.seq)
	s.vols[id] = tags
	s.mu.Unlock()
	writeXML(w, "CreateVolume", "<volumeId>"+id+"</volumeId><size>1</size><status>creating</status>")
}

func (s *Server) describe(w http.ResponseWriter, id string) {
	s.mu.Lock()
	tags, ok := s.vols[id]
	var set strings.Builder
	for k, v := range tags {
		fmt.Fprintf(&set, "<item><key>%s</key><value>%s</value></item>", html.EscapeString(k), html.EscapeString(v))
	}
	s.mu.Unlock()
	if !ok {
		writeError(w, "DescribeVolumes", "InvalidVolume.NotFound", "The volume '"+id+"' does not exist.")
		return
	}
	writeXML(w, "DescribeVolumes", "<volumeSet><item><volumeId>"+html.EscapeString(id)+"</volumeId><tagSet>"+
		set.String()+"</tagSet></item></volumeSet>")
}

func (s *Server) delete(w http.ResponseWriter, id string) {
	s.mu.Lock()
	_, ok := s.vols[id]
	delete(s.vols, id)
	s.mu.Unlock()
	if !ok {
		writeError(w, "DeleteVolume", "InvalidVolume.NotFound", "The volume '"+id+"' does not exist.")
		return
	}
	writeXML(w, "DeleteVolume", "<return>true</return>")
}

func writeXML(w http.ResponseWriter, action, body string) {
	w.Header().Set("Content-Type", "text/xml")
	_, err := fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>`+
		`<%sResponse><requestId>req-%s</requestId>%s</%sResponse>`,
		action, action, body, action)
	note(err)
}

func writeError(w http.ResponseWriter, action, code, msg string) {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(http.StatusBadRequest)
	_, err := fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Response><Errors><Error><Code>%s</Code>`+
		`<Message>%s</Message></Error></Errors><RequestID>req-%s</RequestID></Response>`,
		code, html.EscapeString(msg), action)
	note(err)
}

func note(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "ec2fake: %v\n", err)
	}
}
