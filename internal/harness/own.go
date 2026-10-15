package harness

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

const runIDNameLen = 28

func ClaimRun(dir, id string) error {
	if err := ValidateRunID(id); err != nil {
		return err
	}
	path := filepath.Join(dir, "active")
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("active run: %w", err)
	}
	body := []byte(strconv.Itoa(os.Getpid()) + "\n")
	err := os.WriteFile(filepath.Join(path, id), body, 0o644)
	if err != nil {
		return fmt.Errorf("active run: %w", err)
	}
	return nil
}

func ReleaseRun(dir, id string) error {
	err := os.Remove(filepath.Join(dir, "active", id))
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return fmt.Errorf("active run: %w", err)
}

func (rt *Runtime) keepRun(id string) bool {
	if ValidateRunID(id) != nil {
		return false
	}
	if id == rt.Cfg.RunID {
		return true
	}
	return !runActive(rt.Cfg.OutDir, id)
}

func validRun(id string) bool {
	return ValidateRunID(id) == nil
}

func runActive(dir, id string) bool {
	b, err := os.ReadFile(filepath.Join(dir, "active", id))
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 || pid == os.Getpid() {
		return false
	}
	return processLive(pid)
}

func processLive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func runIDFromName(name string) (string, bool) {
	lower := strings.ToLower(name)
	if len(lower) < runIDNameLen || lower[13] != 't' || lower[20] != 'z' || lower[21] != '-' {
		return "", false
	}
	if len(lower) > runIDNameLen && lower[runIDNameLen] != '-' {
		return "", false
	}
	id := lower[:13] + "T" + lower[14:20] + "Z" + lower[21:runIDNameLen]
	if ValidateRunID(id) != nil {
		return "", false
	}
	return id, true
}

func ownedName(name string, keep func(string) bool) bool {
	id, ok := runIDFromName(name)
	return ok && keep(id)
}

func ownedTag(tags []ec2types.Tag, keep func(string) bool) bool {
	for _, tag := range tags {
		key := aws.ToString(tag.Key)
		val := aws.ToString(tag.Value)
		if key == "vws:conformance-run" && keep(val) {
			return true
		}
		if key == "Name" && ownedName(val, keep) {
			return true
		}
	}
	return false
}
