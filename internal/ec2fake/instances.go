package ec2fake

import (
	"fmt"
	"html"
	"net/http"
	"strings"
)

type instance struct {
	image string
	state string
	tags  map[string]string
}

func (s *Server) instanceAction(w http.ResponseWriter, r *http.Request, action string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := r.Form.Get("InstanceId.1")
	if action == "RunInstances" {
		image := r.Form.Get("ImageId")
		if image == "" {
			writeError(w, action, "MissingParameter", "The request must contain the parameter ImageId")
			return
		}
		token := r.Form.Get("ClientToken")
		id = s.tokens[token]
		if token != "" && id != "" {
			if s.instances[id].image != image {
				writeError(w, action, "IdempotentParameterMismatch", "The client token was used with different parameters")
				return
			}
		} else {
			s.seq++
			id = fmt.Sprintf("i-%017d", s.seq)
			tags := map[string]string{}
			for i := 1; ; i++ {
				prefix := fmt.Sprintf("TagSpecification.%d.", i)
				if r.Form.Get(prefix+"ResourceType") == "" {
					break
				}
				if r.Form.Get(prefix+"ResourceType") != "instance" {
					continue
				}
				for j := 1; ; j++ {
					tag := fmt.Sprintf("%sTag.%d.", prefix, j)
					key := r.Form.Get(tag + "Key")
					if key == "" {
						break
					}
					tags[key] = r.Form.Get(tag + "Value")
				}
			}
			s.instances[id] = instance{image: image, state: "running", tags: tags}
			if token != "" {
				s.tokens[token] = id
			}
		}
		writeXML(w, action, "<instancesSet>"+instanceXML(id, s.instances[id])+"</instancesSet>")
		return
	}
	if action == "DescribeInstanceAttribute" {
		id = r.Form.Get("InstanceId")
	}
	inst, ok := s.instances[id]
	if !ok {
		code := "InvalidInstanceID.NotFound"
		message := "The instance ID '" + id + "' does not exist"
		if id == "i-xyz" {
			code = "InvalidInstanceID.Malformed"
			message = "The instance ID '" + id + "' is malformed"
		}
		writeError(w, action, code, message)
		return
	}
	switch action {
	case "DescribeInstances":
		item := instanceXML(id, inst)
		if r.Form.Get("Filter.1.Name") == "instance-state-name" && r.Form.Get("Filter.1.Value.1") != inst.state {
			item = ""
		}
		writeXML(w, action, "<reservationSet><item><instancesSet>"+item+"</instancesSet></item></reservationSet>")
	case "DescribeInstanceAttribute":
		writeXML(w, action, "<instanceId>"+html.EscapeString(id)+"</instanceId><userData><value/></userData>")
	default:
		target := map[string]string{
			"StopInstances": "stopped", "StartInstances": "running", "TerminateInstances": "terminated",
		}[action]
		previous := inst.state
		inst.state = target
		s.instances[id] = inst
		writeXML(w, action, "<instancesSet><item><instanceId>"+html.EscapeString(id)+
			"</instanceId><previousState><name>"+previous+"</name></previousState><currentState><name>"+
			target+"</name></currentState></item></instancesSet>")
	}
}

func instanceXML(id string, inst instance) string {
	var tags strings.Builder
	for k, v := range inst.tags {
		fmt.Fprintf(&tags, "<item><key>%s</key><value>%s</value></item>", html.EscapeString(k), html.EscapeString(v))
	}
	return "<item><instanceId>" + html.EscapeString(id) + "</instanceId><imageId>" + html.EscapeString(inst.image) +
		"</imageId><instanceState><name>" + inst.state + "</name></instanceState><tagSet>" +
		tags.String() + "</tagSet></item>"
}
