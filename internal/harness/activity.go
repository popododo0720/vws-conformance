package harness

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const activityFinished = "0"

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

func processLive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
