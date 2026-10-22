package harness

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var RunIDPattern = regexp.MustCompile(`^conf-[0-9]{8}T[0-9]{6}Z-[0-9a-f]{6}$`)

const (
	InstanceType          = "t2.nano"
	VolumeGiB       int32 = 1
	DefaultParallel       = 1
)

func ValidateRunID(id string) error {
	if !RunIDPattern.MatchString(id) {
		return fmt.Errorf("run id %q does not match the report pattern", id)
	}
	return nil
}

func NewRunID(now time.Time) (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("run id: %w", err)
	}
	n := binary.BigEndian.Uint32(b[:]) & 0xffffff
	id := fmt.Sprintf("conf-%s-%06x", now.UTC().Format("20060102T150405Z"), n)
	if err := ValidateRunID(id); err != nil {
		return "", err
	}
	return id, nil
}

func NamePrefix(runID string) string {
	return strings.ToLower(runID)
}

func ResourceName(runID, suffix string) string {
	return NamePrefix(runID) + "-" + suffix
}
