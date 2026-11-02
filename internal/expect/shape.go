package expect

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"

	"github.com/popododo0720/vws-conformance/internal/cases"
)

// ShapeOK is the value ShapeField reports when the body has the expected shape.
// Cases match it with the pattern ShapeOKPattern on the "shape" field.
const (
	ShapeOK        = "ok"
	ShapeOKPattern = "^ok$"
)

type xmlNode struct {
	XMLName xml.Name
	Nodes   []xmlNode `xml:",any"`
	Text    string    `xml:",chardata"`
}

func (n xmlNode) child(name string) (xmlNode, bool) {
	for _, c := range n.Nodes {
		if c.XMLName.Local == name {
			return c, true
		}
	}
	return xmlNode{}, false
}

// path resolves a slash-separated child path such as "instanceState/name".
func (n xmlNode) path(p string) (xmlNode, bool) {
	cur := n
	for _, part := range strings.Split(p, "/") {
		next, ok := cur.child(part)
		if !ok {
			return xmlNode{}, false
		}
		cur = next
	}
	return cur, true
}

// EC2List checks an EC2 query-protocol response body against the AWS response
// shape: the root element is <op>Response, it holds the list element, and every
// <item> in that list has each required path with non-empty text. An empty list
// is valid; a missing list element is not.
func EC2List(body []byte, op, list string, required ...string) error {
	if len(bytes.TrimSpace(body)) == 0 {
		return errors.New("response body is empty")
	}
	var root xmlNode
	if err := xml.Unmarshal(body, &root); err != nil {
		return fmt.Errorf("response body is not XML: %w", err)
	}
	if want := op + "Response"; root.XMLName.Local != want {
		return fmt.Errorf("root element is %q, want %q", root.XMLName.Local, want)
	}
	set, ok := root.child(list)
	if !ok {
		return fmt.Errorf("list element %q is missing", list)
	}
	for i, it := range set.Nodes {
		if it.XMLName.Local != "item" {
			return fmt.Errorf("%s child %d is %q, want \"item\"", list, i, it.XMLName.Local)
		}
		for _, p := range required {
			v, ok := it.path(p)
			if !ok || strings.TrimSpace(v.Text) == "" {
				return fmt.Errorf("%s item %d has no %s", list, i, p)
			}
		}
	}
	return nil
}

// ShapeField runs EC2List on a successful outcome and returns ShapeOK or the
// reason the shape check failed. Error outcomes return "" so the HTTP and code
// checks in Judge report them.
func ShapeField(o cases.Outcome, op, list string, required ...string) string {
	if o.Code != "" || o.HTTP != 200 {
		return ""
	}
	if err := EC2List(o.Body, op, list, required...); err != nil {
		return err.Error()
	}
	return ShapeOK
}
