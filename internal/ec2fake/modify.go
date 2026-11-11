package ec2fake

import (
	"fmt"
	"html"
	"net/http"
	"strconv"
)

func (s *Server) modifyVolume(w http.ResponseWriter, r *http.Request) {
	id := r.Form.Get("VolumeId")
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.vols[id]; !ok {
		writeError(w, "ModifyVolume", "InvalidVolume.NotFound", "The volume '"+id+"' does not exist.")
		return
	}
	original := s.sizes[id]
	if original == 0 {
		original = 1
	}
	target := original
	if size := r.Form.Get("Size"); size != "" {
		n, err := strconv.ParseInt(size, 10, 32)
		if err != nil || n < int64(original) {
			writeError(w, "ModifyVolume", "InvalidParameterValue", "Value ("+size+") for parameter Size is invalid.")
			return
		}
		target = int32(n)
	}
	if s.sizes == nil {
		s.sizes = map[string]int32{}
	}
	s.sizes[id] = target
	body := fmt.Sprintf("<volumeModification><volumeId>%s</volumeId><originalSize>%d</originalSize>"+
		"<targetSize>%d</targetSize><modificationState>modifying</modificationState>"+
		"<startTime>2026-11-10T00:00:00Z</startTime></volumeModification>", html.EscapeString(id), original, target)
	writeXML(w, "ModifyVolume", body)
}

func (s *Server) VolumeSize(id string) int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sizes[id]
}

func (s *Server) VolumeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.vols)
}
