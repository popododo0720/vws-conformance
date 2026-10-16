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
	return withActivityLock(func() error {
		path := filepath.Join(activityDir(), id)
		pid, held := readActivityPID(path)
		if held && pid == os.Getpid() {
			return nil
		}
		if held && processLive(pid) {
			return fmt.Errorf("active run %s is held by pid %d", id, pid)
		}
		return publishActivity(id, strconv.Itoa(os.Getpid()))
	})
}

func ReleaseRun(id string) error {
	if err := ValidateRunID(id); err != nil {
		return err
	}
	return withActivityLock(func() error {
		path := filepath.Join(activityDir(), id)
		pid, held := readActivityPID(path)
		if held && pid != os.Getpid() && processLive(pid) {
			return fmt.Errorf("active run %s is held by pid %d", id, pid)
		}
		return publishActivity(id, activityFinished)
	})
}

func withActivityLock(fn func() error) error {
	dir := activityDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("active run: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, ".lock"), os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("active run: %w", err)
	}
	defer closeChecked(f)
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("active run: %w", err)
	}
	defer func() {
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
			fmt.Fprintf(os.Stderr, "unlock activity: %v\n", err)
		}
	}()
	return fn()
}

func publishActivity(id, body string) error {
	dir := activityDir()
	f, err := os.CreateTemp(dir, ".marker-*")
	if err != nil {
		return fmt.Errorf("active run: %w", err)
	}
	tmp := f.Name()
	_, werr := f.Write([]byte(body + "\n"))
	cerr := f.Close()
	if werr != nil || cerr != nil {
		rmErr := os.Remove(tmp)
		return fmt.Errorf("active run: %w", errors.Join(werr, cerr, rmErr))
	}
	if err := os.Rename(tmp, filepath.Join(dir, id)); err != nil {
		rmErr := os.Remove(tmp)
		return fmt.Errorf("active run: %w", errors.Join(err, rmErr))
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

func ownerOutsideKeep(tags []ec2types.Tag, names []string, keep func(string) bool) bool {
	owners := tagOwners(tags)
	for _, name := range names {
		owners = mergeOwners(owners, runOwners(name))
	}
	for _, id := range owners {
		if !keep(id) {
			return true
		}
	}
	return false
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

func appendOwner(owners []string, id string) []string {
	if ValidateRunID(id) != nil {
		return owners
	}
	for _, owner := range owners {
		if owner == id {
			return owners
		}
	}
	return append(owners, id)
}

func mergeOwners(dst, src []string) []string {
	for _, id := range src {
		dst = appendOwner(dst, id)
	}
	return dst
}

func runOwners(name string) []string {
	id, ok := runIDFromName(name)
	if !ok {
		return nil
	}
	return []string{id}
}

func tagOwners(tags []ec2types.Tag) []string {
	var owners []string
	for _, tag := range tags {
		key := aws.ToString(tag.Key)
		val := aws.ToString(tag.Value)
		if key == "vws:conformance-run" {
			owners = appendOwner(owners, val)
		}
		if key == "Name" {
			owners = mergeOwners(owners, runOwners(val))
		}
	}
	return owners
}
