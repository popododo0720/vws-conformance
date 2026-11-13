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
	"unicode/utf8"
)

type Server struct {
	*httptest.Server

	mu               sync.Mutex
	vols             map[string]map[string]string
	snaps            map[string]snapshot
	vpcs             map[string]map[string]string
	internetGateways map[string]map[string]string
	calls            map[string]int
	seq              int
	onCreate         func() CreateResult
	sizes            map[string]int32
	instances        map[string]instance
	tokens           map[string]string
}

// CreateResult is what a create hook asks CreateVolume to do. The zero value creates the volume.
type CreateResult struct {
	Code string // answer with this error code and create nothing
	Drop bool   // close the connection without an answer
	Lose bool   // create the volume, then close the connection without an answer
}

func New() *Server {
	s := &Server{
		vols: map[string]map[string]string{}, snaps: map[string]snapshot{},
		vpcs:             map[string]map[string]string{},
		internetGateways: map[string]map[string]string{}, calls: map[string]int{},
		instances: map[string]instance{}, tokens: map[string]string{},
	}
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
	case "DescribeSubnets":
		writeXML(w, action, "<subnetSet><item><subnetId>subnet-0123456789abcdef0</subnetId>"+
			"<state>available</state></item></subnetSet>")
	case "RunInstances", "DescribeInstances", "DescribeInstanceAttribute",
		"StopInstances", "StartInstances", "TerminateInstances":
		s.instanceAction(w, r, action)
	case "CreateVpc":
		s.createVpc(w, r)
	case "DescribeVpcs":
		s.describeVpc(w, r.Form.Get("VpcId.1"))
	case "DeleteVpc":
		s.deleteVpc(w, r.Form.Get("VpcId"))
	case "CreateInternetGateway":
		s.createInternetGateway(w, r)
	case "DescribeInternetGateways":
		s.describeInternetGateway(w, r.Form.Get("InternetGatewayId.1"))
	case "DeleteInternetGateway":
		s.deleteInternetGateway(w, r.Form.Get("InternetGatewayId"))
	case "CreateSnapshot":
		s.createSnapshot(w, r)
	case "DescribeSnapshots":
		s.describeSnapshot(w, r.Form.Get("SnapshotId.1"))
	case "DeleteSnapshot":
		s.deleteSnapshot(w, r.Form.Get("SnapshotId"))
	case "CreateVolume":
		s.create(w, r)
	case "DescribeVolumes":
		s.describe(w, r.Form.Get("VolumeId.1"))
	case "ModifyVolume":
		s.modifyVolume(w, r)
	case "CreateTags", "DeleteTags":
		s.tags(w, r, action)
	case "DeleteVolume":
		s.delete(w, r.Form.Get("VolumeId"))
	default:
		writeError(w, action, "InvalidAction", "unsupported action "+action)
	}
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	if r.Form.Get("Size") == "0" {
		writeError(w, "CreateVolume", "InvalidParameterValue", "Value (0) for parameter Size is invalid.")
		return
	}
	s.mu.Lock()
	hook := s.onCreate
	s.mu.Unlock()
	var res CreateResult
	if hook != nil {
		res = hook()
	}
	if res.Drop {
		s.hangUp(w)
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
	if s.sizes == nil {
		s.sizes = map[string]int32{}
	}
	s.sizes[id] = 1
	s.mu.Unlock()
	if res.Lose {
		s.hangUp(w)
		return
	}
	writeXML(w, "CreateVolume", "<volumeId>"+id+"</volumeId><size>1</size><status>available</status>")
}

func (s *Server) hangUp(w http.ResponseWriter) {
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
	writeXML(w, "DescribeVolumes", "<volumeSet><item><volumeId>"+html.EscapeString(id)+"</volumeId>"+
		"<status>available</status><size>1</size><tagSet>"+
		set.String()+"</tagSet></item></volumeSet>")
}

func (s *Server) delete(w http.ResponseWriter, id string) {
	s.mu.Lock()
	_, ok := s.vols[id]
	delete(s.vols, id)
	delete(s.sizes, id)
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

func (s *Server) tags(w http.ResponseWriter, r *http.Request, action string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := r.Form.Get("ResourceId.1")
	tags, ok := s.vols[id]
	if !ok {
		writeError(w, action, "InvalidVolume.NotFound", "The volume does not exist.")
		return
	}
	for i := 1; r.Form.Has(fmt.Sprintf("Tag.%d.Key", i)); i++ {
		value := r.Form.Get(fmt.Sprintf("Tag.%d.Value", i))
		if utf8.RuneCountInString(value) > 256 {
			writeError(w, action, "InvalidParameterValue", fmt.Sprintf("Value (%s) for parameter tagValue is invalid.", value))
			return
		}
	}
	for i := 1; r.Form.Has(fmt.Sprintf("Tag.%d.Key", i)); i++ {
		key := r.Form.Get(fmt.Sprintf("Tag.%d.Key", i))
		valueKey := fmt.Sprintf("Tag.%d.Value", i)
		value := r.Form.Get(valueKey)
		if action == "CreateTags" {
			tags[key] = value
			continue
		}
		if !r.Form.Has(valueKey) || tags[key] == value {
			delete(tags, key)
		}
	}
	writeXML(w, action, "<return>true</return>")
}

type snapshot struct {
	volume string
	tags   map[string]string
}

func (s *Server) Snapshot(id string) (map[string]string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, ok := s.snaps[id]
	return snap.tags, ok
}

func (s *Server) PutSnapshot(id, volume string, tags map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snaps[id] = snapshot{volume: volume, tags: tags}
}

func (s *Server) createSnapshot(w http.ResponseWriter, r *http.Request) {
	volume := r.Form.Get("VolumeId")
	if volume == "" {
		writeError(w, "CreateSnapshot", "MissingParameter", "The request must contain the parameter VolumeId")
		return
	}
	s.mu.Lock()
	_, exists := s.vols[volume]
	if !exists {
		s.mu.Unlock()
		writeError(w, "CreateSnapshot", "InvalidVolume.NotFound", "The volume '"+volume+"' does not exist.")
		return
	}
	tags := map[string]string{}
	for i := 1; r.Form.Get(fmt.Sprintf("TagSpecification.%d.ResourceType", i)) != ""; i++ {
		if r.Form.Get(fmt.Sprintf("TagSpecification.%d.ResourceType", i)) != "snapshot" {
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
	s.seq++
	id := fmt.Sprintf("snap-%017d", s.seq)
	s.snaps[id] = snapshot{volume: volume, tags: tags}
	s.mu.Unlock()
	writeXML(w, "CreateSnapshot", "<snapshotId>"+id+"</snapshotId><volumeId>"+html.EscapeString(volume)+"</volumeId>"+
		"<volumeSize>1</volumeSize><status>pending</status>")
}

func (s *Server) describeSnapshot(w http.ResponseWriter, id string) {
	s.mu.Lock()
	snap, ok := s.snaps[id]
	var tags strings.Builder
	for k, v := range snap.tags {
		fmt.Fprintf(&tags, "<item><key>%s</key><value>%s</value></item>", html.EscapeString(k), html.EscapeString(v))
	}
	s.mu.Unlock()
	if !ok {
		writeError(w, "DescribeSnapshots", "InvalidSnapshot.NotFound", "The snapshot '"+id+"' does not exist.")
		return
	}
	writeXML(w, "DescribeSnapshots", "<snapshotSet><item><snapshotId>"+html.EscapeString(id)+"</snapshotId>"+
		"<volumeId>"+html.EscapeString(snap.volume)+"</volumeId>"+
		"<status>completed</status><tagSet>"+tags.String()+"</tagSet></item></snapshotSet>")
}

func (s *Server) deleteSnapshot(w http.ResponseWriter, id string) {
	s.mu.Lock()
	_, ok := s.snaps[id]
	delete(s.snaps, id)
	s.mu.Unlock()
	if !ok {
		writeError(w, "DeleteSnapshot", "InvalidSnapshot.NotFound", "The snapshot '"+id+"' does not exist.")
		return
	}
	writeXML(w, "DeleteSnapshot", "<return>true</return>")
}

func (s *Server) ResourceCounts() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.vols), len(s.snaps)
}
