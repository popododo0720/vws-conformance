package ec2fake

import (
	"fmt"
	"html"
	"net/http"
	"strings"
)

func (s *Server) Vpc(id string) (map[string]string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tags, ok := s.vpcs[id]
	return tags, ok
}

func (s *Server) PutVpc(id string, tags map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vpcs[id] = tags
}

func (s *Server) createVpc(w http.ResponseWriter, r *http.Request) {
	if r.Form.Get("CidrBlock") == "10.44.0.0/29" {
		writeError(w, "CreateVpc", "InvalidVpcRange", "The CIDR block range is not valid. "+
			"The block range must be between a /28 netmask and /16 netmask.")
		return
	}

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
		writeError(w, "CreateVpc", result.Code, "refused")
		return
	}
	tags := map[string]string{}
	for i := 1; r.Form.Get(fmt.Sprintf("TagSpecification.%d.ResourceType", i)) != ""; i++ {
		if r.Form.Get(fmt.Sprintf("TagSpecification.%d.ResourceType", i)) != "vpc" {
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
	id := fmt.Sprintf("vpc-%017d", s.seq)
	s.vpcs[id] = tags
	s.mu.Unlock()
	if result.Lose {
		s.hangUp(w)
		return
	}
	writeXML(w, "CreateVpc", "<vpc><vpcId>"+id+"</vpcId><state>available</state>"+
		"<cidrBlock>10.44.0.0/16</cidrBlock><isDefault>false</isDefault></vpc>")
}

func (s *Server) describeVpc(w http.ResponseWriter, id string) {
	s.mu.Lock()
	tags, ok := s.vpcs[id]
	var set strings.Builder
	for key, value := range tags {
		fmt.Fprintf(&set, "<item><key>%s</key><value>%s</value></item>", html.EscapeString(key), html.EscapeString(value))
	}
	s.mu.Unlock()
	if !ok {
		writeError(w, "DescribeVpcs", "InvalidVpcID.NotFound", "The vpc id '"+id+"' does not exist.")
		return
	}
	writeXML(w, "DescribeVpcs", "<vpcSet><item><vpcId>"+html.EscapeString(id)+"</vpcId><state>available</state>"+
		"<cidrBlock>10.44.0.0/16</cidrBlock><isDefault>false</isDefault><tagSet>"+
		set.String()+"</tagSet></item></vpcSet>")
}

func (s *Server) deleteVpc(w http.ResponseWriter, id string) {
	s.mu.Lock()
	_, ok := s.vpcs[id]
	delete(s.vpcs, id)
	s.mu.Unlock()
	if !ok {
		writeError(w, "DeleteVpc", "InvalidVpcID.NotFound", "The vpc id '"+id+"' does not exist.")
		return
	}
	writeXML(w, "DeleteVpc", "<return>true</return>")
}
