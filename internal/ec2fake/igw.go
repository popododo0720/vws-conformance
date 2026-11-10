package ec2fake

import (
	"fmt"
	"html"
	"net/http"
	"strings"
)

func (s *Server) InternetGateway(id string) (map[string]string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tags, ok := s.internetGateways[id]
	return tags, ok
}

func (s *Server) PutInternetGateway(id string, tags map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.internetGateways[id] = tags
}

func (s *Server) createInternetGateway(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	hook := s.onCreate
	s.mu.Unlock()
	var result CreateResult
	if hook != nil {
		result = hook()
	}
	if result.Drop {
		s.hangUp(w)
		return
	}
	if result.Code != "" {
		writeError(w, "CreateInternetGateway", result.Code, "refused")
		return
	}
	tags := map[string]string{}
	for i := 1; r.Form.Get(fmt.Sprintf("TagSpecification.%d.ResourceType", i)) != ""; i++ {
		if r.Form.Get(fmt.Sprintf("TagSpecification.%d.ResourceType", i)) != "internet-gateway" {
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
	id := fmt.Sprintf("igw-%017d", s.seq)
	s.internetGateways[id] = tags
	s.mu.Unlock()
	if result.Lose {
		s.hangUp(w)
		return
	}
	writeXML(w, "CreateInternetGateway", "<internetGateway><internetGatewayId>"+id+
		"</internetGatewayId><attachmentSet/></internetGateway>")
}

func (s *Server) describeInternetGateway(w http.ResponseWriter, id string) {
	s.mu.Lock()
	tags, ok := s.internetGateways[id]
	var set strings.Builder
	for key, value := range tags {
		fmt.Fprintf(&set, "<item><key>%s</key><value>%s</value></item>", html.EscapeString(key), html.EscapeString(value))
	}
	s.mu.Unlock()
	if !ok {
		writeError(w, "DescribeInternetGateways", "InvalidInternetGatewayID.NotFound",
			"The resource '"+id+"' does not exist.")
		return
	}
	writeXML(w, "DescribeInternetGateways", "<internetGatewaySet><item><internetGatewayId>"+html.EscapeString(id)+
		"</internetGatewayId><attachmentSet/><tagSet>"+set.String()+"</tagSet></item></internetGatewaySet>")
}

func (s *Server) deleteInternetGateway(w http.ResponseWriter, id string) {
	s.mu.Lock()
	_, ok := s.internetGateways[id]
	delete(s.internetGateways, id)
	s.mu.Unlock()
	if !ok {
		writeError(w, "DeleteInternetGateway", "InvalidInternetGatewayID.NotFound",
			"The resource '"+id+"' does not exist.")
		return
	}
	writeXML(w, "DeleteInternetGateway", "<return>true</return>")
}
