package terminal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ianf339/roambench/internal/config"
	"github.com/ianf339/roambench/internal/systemstats"
)

var ErrResourceControlUnavailable = errors.New("per-session resource control unavailable")

type sessionCgroupManager struct {
	root      string
	resources config.TerminalResourceConfig
}

func newSessionCgroupManager(resources config.TerminalResourceConfig) (*sessionCgroupManager, error) {
	if !sessionResourceLimitsEnabled(resources) {
		return nil, nil
	}
	root, err := systemstats.ResolveTaskPoolCgroupDir("", "", "supervisor")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrResourceControlUnavailable, err)
	}
	return &sessionCgroupManager{root: root, resources: resources}, nil
}

func sessionResourceLimitsEnabled(resources config.TerminalResourceConfig) bool {
	return resources.SessionMemoryHighBytes > 0 ||
		resources.SessionMemoryMaxBytes > 0 ||
		resources.SessionSwapMaxBytes > 0 ||
		resources.SessionPIDsMax > 0 ||
		resources.SessionCPUWeight > 0 ||
		resources.SessionIOWeight > 0
}

func (m *sessionCgroupManager) Prepare(sessionID string) (string, error) {
	if m == nil {
		return "", nil
	}
	if !validCgroupSessionID(sessionID) {
		return "", errors.New("invalid terminal session ID for cgroup")
	}

	controllers := m.requiredControllers()
	if err := enableCgroupControllers(m.root, controllers); err != nil {
		return "", fmt.Errorf("%w: service cgroup: %v", ErrResourceControlUnavailable, err)
	}

	terminalsDir := filepath.Join(m.root, "terminals")
	if err := os.MkdirAll(terminalsDir, 0o755); err != nil {
		return "", fmt.Errorf("%w: create terminal cgroup root: %v", ErrResourceControlUnavailable, err)
	}
	if err := enableCgroupControllers(terminalsDir, controllers); err != nil {
		return "", fmt.Errorf("%w: terminal cgroup root: %v", ErrResourceControlUnavailable, err)
	}

	sessionDir := filepath.Join(terminalsDir, sessionID)
	if err := os.Mkdir(sessionDir, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
		return "", fmt.Errorf("%w: create session cgroup: %v", ErrResourceControlUnavailable, err)
	}

	values := map[string]int64{}
	if m.resources.SessionMemoryHighBytes > 0 {
		values["memory.high"] = m.resources.SessionMemoryHighBytes
	}
	if m.resources.SessionMemoryMaxBytes > 0 {
		values["memory.max"] = m.resources.SessionMemoryMaxBytes
	}
	if m.resources.SessionSwapMaxBytes > 0 {
		values["memory.swap.max"] = m.resources.SessionSwapMaxBytes
	}
	if m.resources.SessionPIDsMax > 0 {
		values["pids.max"] = m.resources.SessionPIDsMax
	}
	if m.resources.SessionCPUWeight > 0 {
		values["cpu.weight"] = m.resources.SessionCPUWeight
	}
	for name, value := range values {
		if err := writeCgroupValue(filepath.Join(sessionDir, name), strconv.FormatInt(value, 10)); err != nil {
			return "", fmt.Errorf("%w: set %s: %v", ErrResourceControlUnavailable, name, err)
		}
	}
	if m.resources.SessionIOWeight > 0 {
		value := "default " + strconv.FormatInt(m.resources.SessionIOWeight, 10)
		if err := writeCgroupValue(filepath.Join(sessionDir, "io.weight"), value); err != nil {
			return "", fmt.Errorf("%w: set io.weight: %v", ErrResourceControlUnavailable, err)
		}
	}
	if m.resources.SessionMemoryHighBytes > 0 || m.resources.SessionMemoryMaxBytes > 0 || m.resources.SessionSwapMaxBytes > 0 {
		if err := writeCgroupValue(filepath.Join(sessionDir, "memory.oom.group"), "1"); err != nil {
			return "", fmt.Errorf("%w: set memory.oom.group: %v", ErrResourceControlUnavailable, err)
		}
	}

	return filepath.Join(sessionDir, "cgroup.procs"), nil
}

func (m *sessionCgroupManager) Cleanup(sessionID string) {
	if m == nil || !validCgroupSessionID(sessionID) {
		return
	}
	sessionDir := filepath.Join(m.root, "terminals", sessionID)
	_ = writeCgroupValue(filepath.Join(sessionDir, "cgroup.kill"), "1")
	for attempt := 0; attempt < 10; attempt++ {
		if err := os.Remove(sessionDir); err == nil || errors.Is(err, os.ErrNotExist) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (m *sessionCgroupManager) requiredControllers() []string {
	set := make(map[string]struct{})
	if m.resources.SessionMemoryHighBytes > 0 || m.resources.SessionMemoryMaxBytes > 0 || m.resources.SessionSwapMaxBytes > 0 {
		set["memory"] = struct{}{}
	}
	if m.resources.SessionPIDsMax > 0 {
		set["pids"] = struct{}{}
	}
	if m.resources.SessionCPUWeight > 0 {
		set["cpu"] = struct{}{}
	}
	if m.resources.SessionIOWeight > 0 {
		set["io"] = struct{}{}
	}
	controllers := make([]string, 0, len(set))
	for controller := range set {
		controllers = append(controllers, controller)
	}
	sort.Strings(controllers)
	return controllers
}

func enableCgroupControllers(dir string, controllers []string) error {
	if len(controllers) == 0 {
		return nil
	}
	available, err := os.ReadFile(filepath.Join(dir, "cgroup.controllers"))
	if err != nil {
		return err
	}
	availableSet := make(map[string]struct{})
	for _, controller := range strings.Fields(string(available)) {
		availableSet[controller] = struct{}{}
	}
	commands := make([]string, 0, len(controllers))
	for _, controller := range controllers {
		if _, ok := availableSet[controller]; !ok {
			return fmt.Errorf("controller %s is not delegated", controller)
		}
		commands = append(commands, "+"+controller)
	}
	return writeCgroupValue(filepath.Join(dir, "cgroup.subtree_control"), strings.Join(commands, " "))
}

func writeCgroupValue(path, value string) error {
	return os.WriteFile(path, []byte(value), 0o644)
}

func validCgroupSessionID(sessionID string) bool {
	if !strings.HasPrefix(sessionID, "lt-") || len(sessionID) <= len("lt-") {
		return false
	}
	for _, r := range strings.TrimPrefix(sessionID, "lt-") {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
