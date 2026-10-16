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

const (
	runIDNameLen     = 28
	activityFinished = "0"
)

func ClaimRun(id string) error {
	if err := ValidateRunID(id); err != nil {
		return err
	}
	dir := activityDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("active run: %w", err)
	}
	path := filepath.Join(dir, id)
	for range 3 {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			_, werr := f.Write([]byte(strconv.Itoa(os.Getpid()) + "\n"))
			cerr := f.Close()
			if werr != nil || cerr != nil {
				_ = os.Remove(path)
				return fmt.Errorf("active run: %w", errors.Join(werr, cerr))
			}
			return nil
		}
		if !os.IsExist(err) {
			return fmt.Errorf("active run: %w", err)
		}
		pid, held := readActivityPID(path)
		if held && pid == os.Getpid() {
			return nil
		}
		if held && processLive(pid) {
			return fmt.Errorf("active run %s is held by pid %d", id, pid)
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("active run: %w", err)
		}
	}
	return fmt.Errorf("active run %s: claim raced", id)
}

func ReleaseRun(id string) error {
	if err := ValidateRunID(id); err != nil {
		return err
	}
	path := filepath.Join(activityDir(), id)
	pid, held := readActivityPID(path)
	if held && pid != os.Getpid() && processLive(pid) {
		return fmt.Errorf("active run %s is held by pid %d", id, pid)
	}
	return writeActivity(id, activityFinished)
}

func writeActivity(id, body string) error {
	if err := ValidateRunID(id); err != nil {
		return err
	}
	dir := activityDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("active run: %w", err)
	}
	err := os.WriteFile(filepath.Join(dir, id), []byte(body+"\n"), 0o644)
	if err != nil {
		return fmt.Errorf("active run: %w", err)
	}
	return nil
}

func activityDir() string {
	if dir := os.Getenv("VWS_CONF_ACTIVITY_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(os.TempDir(), "vws-conformance-active")
}

func (rt *Runtime) keepRun(id string) bool {
	if ValidateRunID(id) != nil {
		return false
	}
	if liveForeign(id) {
		return false
	}
	if id == rt.Cfg.RunID {
		return true
	}
	return runFinished(id)
}

func liveForeign(id string) bool {
	pid, held := readActivityPID(filepath.Join(activityDir(), id))
	if !held || pid == os.Getpid() {
		return false
	}
	return processLive(pid)
}

func readActivityPID(path string) (int, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

func validRun(id string) bool {
	return ValidateRunID(id) == nil
}

func runFinished(id string) bool {
	b, err := os.ReadFile(filepath.Join(activityDir(), id))
	if err != nil {
		return false
	}
	if strings.TrimSpace(string(b)) == activityFinished {
		return true
	}
	pid, held := readActivityPID(filepath.Join(activityDir(), id))
	if !held || pid == os.Getpid() {
		return false
	}
	return !processLive(pid)
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
