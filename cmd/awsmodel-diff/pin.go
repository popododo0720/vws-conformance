package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

// releaseTag is the shape of a botocore release tag. A branch name or anything else is refused, so
// a run always compares fixed files and the tag is safe to put in an issue title.
var releaseTag = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

var errEmptyPin = errors.New("awsmodel pin: no models")

// TagError is a botocore version that is not a release tag.
type TagError struct {
	Tag string
}

func (e *TagError) Error() string {
	return fmt.Sprintf("awsmodel: %q is not a botocore release tag", e.Tag)
}

// DigestError is a baseline model file whose sha256 is not the one pinned for its service, so it
// is not the pinned copy.
type DigestError struct {
	Service string
	Want    string
	Got     string
}

func (e *DigestError) Error() string {
	return fmt.Sprintf("awsmodel: %s model sha256 is %s, pin.json says %s", e.Service, e.Got, e.Want)
}

// pinModel is one service model at the pinned release: its path in the botocore repository and
// the sha256 of the file.
type pinModel struct {
	Service string `json:"service"`
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
}

// pin is pin.json: the botocore release every run diffs against. Moving it forward is a pull
// request that edits the version and the sha256 lines (printed by -repin).
type pin struct {
	Botocore string     `json:"botocore"`
	Models   []pinModel `json:"models"`
}

func parsePin(data []byte) (*pin, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var p pin
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("parse awsmodel pin: %w", err)
	}
	if !releaseTag.MatchString(p.Botocore) {
		return nil, &TagError{Tag: p.Botocore}
	}
	if len(p.Models) == 0 {
		return nil, errEmptyPin
	}
	seen := make(map[string]bool, len(p.Models))
	for _, m := range p.Models {
		if m.Service == "" || m.Path == "" || seen[m.Service] {
			return nil, fmt.Errorf("awsmodel pin: model %q needs a unique service and a path", m.Service)
		}
		if _, err := hex.DecodeString(m.SHA256); err != nil || len(m.SHA256) != sha256.Size*2 {
			return nil, fmt.Errorf("awsmodel pin: %s sha256 %q is not a sha256", m.Service, m.SHA256)
		}
		seen[m.Service] = true
	}
	return &p, nil
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// check confirms that data is the pinned copy of m.
func (m pinModel) check(data []byte) error {
	if got := digest(data); got != m.SHA256 {
		return &DigestError{Service: m.Service, Want: m.SHA256, Got: got}
	}
	return nil
}
