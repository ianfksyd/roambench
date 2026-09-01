package systemstats

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var ErrAdmissionDenied = errors.New("task admission blocked by resource pressure")

type Snapshot struct {
	ProcessRSSBytes         uint64  `json:"processRSSBytes"`
	SystemUsedBytes         uint64  `json:"systemUsedBytes"`
	SystemAvailableBytes    uint64  `json:"systemAvailableBytes"`
	SystemTotalBytes        uint64  `json:"totalMemoryBytes"`
	SystemSwapUsedBytes     uint64  `json:"systemSwapUsedBytes"`
	SystemSwapTotalBytes    uint64  `json:"systemSwapTotalBytes"`
	TaskPoolAvailable       bool    `json:"taskPoolAvailable"`
	TaskPoolCurrentBytes    uint64  `json:"taskPoolCurrentBytes"`
	TaskPoolAnonBytes       uint64  `json:"taskPoolAnonBytes"`
	TaskPoolFileBytes       uint64  `json:"taskPoolFileBytes"`
	TaskPoolSwapBytes       uint64  `json:"taskPoolSwapBytes"`
	TaskPoolMemoryHighBytes *uint64 `json:"taskPoolMemoryHighBytes"`
	TaskPoolMemoryMaxBytes  *uint64 `json:"taskPoolMemoryMaxBytes"`
	TaskPoolPIDsCurrent     uint64  `json:"taskPoolPidsCurrent"`
	TaskPoolPIDsMax         *uint64 `json:"taskPoolPidsMax"`
	MemoryPressureSomeAvg10 float64 `json:"memoryPressureSomeAvg10"`
	MemoryPressureFullAvg10 float64 `json:"memoryPressureFullAvg10"`
}

type Reader struct {
	ProcRoot         string
	CgroupRoot       string
	DelegateSubgroup string
}

type AdmissionPolicy struct {
	MinSystemAvailableBytes  uint64
	MaxSystemSwapUsedPercent float64
	MaxMemoryPressureAvg10   float64
}

func Read() (Snapshot, error) {
	return (Reader{}).Read()
}

func (r Reader) Read() (Snapshot, error) {
	procRoot := r.ProcRoot
	if procRoot == "" {
		procRoot = "/proc"
	}
	cgroupRoot := r.CgroupRoot
	if cgroupRoot == "" {
		cgroupRoot = "/sys/fs/cgroup"
	}
	delegateSubgroup := r.DelegateSubgroup
	if delegateSubgroup == "" {
		delegateSubgroup = "supervisor"
	}

	processRSS, err := readProcMemoryValueBytes(filepath.Join(procRoot, "self/status"), "VmRSS:")
	if err != nil {
		return Snapshot{}, err
	}
	totalMemory, err := readProcMemoryValueBytes(filepath.Join(procRoot, "meminfo"), "MemTotal:")
	if err != nil {
		return Snapshot{}, err
	}
	availableMemory, err := readProcMemoryValueBytes(filepath.Join(procRoot, "meminfo"), "MemAvailable:")
	if err != nil {
		availableMemory, err = readProcMemoryValueBytes(filepath.Join(procRoot, "meminfo"), "MemFree:")
		if err != nil {
			return Snapshot{}, err
		}
	}
	if totalMemory == 0 {
		return Snapshot{}, errors.New("total memory unavailable")
	}
	if availableMemory > totalMemory {
		return Snapshot{}, errors.New("available memory exceeds total memory")
	}

	snapshot := Snapshot{
		ProcessRSSBytes:      processRSS,
		SystemUsedBytes:      totalMemory - availableMemory,
		SystemAvailableBytes: availableMemory,
		SystemTotalBytes:     totalMemory,
	}

	swapTotal, swapTotalErr := readProcMemoryValueBytes(filepath.Join(procRoot, "meminfo"), "SwapTotal:")
	swapFree, swapFreeErr := readProcMemoryValueBytes(filepath.Join(procRoot, "meminfo"), "SwapFree:")
	if swapTotalErr == nil && swapFreeErr == nil && swapFree <= swapTotal {
		snapshot.SystemSwapTotalBytes = swapTotal
		snapshot.SystemSwapUsedBytes = swapTotal - swapFree
	}

	cgroupDir, err := ResolveTaskPoolCgroupDir(procRoot, cgroupRoot, delegateSubgroup)
	if err != nil {
		return snapshot, nil
	}
	current, err := readUintFile(filepath.Join(cgroupDir, "memory.current"))
	if err != nil {
		return snapshot, nil
	}

	snapshot.TaskPoolAvailable = true
	snapshot.TaskPoolCurrentBytes = current
	snapshot.TaskPoolSwapBytes, _ = readUintFile(filepath.Join(cgroupDir, "memory.swap.current"))
	snapshot.TaskPoolMemoryHighBytes, _ = readLimitFile(filepath.Join(cgroupDir, "memory.high"))
	snapshot.TaskPoolMemoryMaxBytes, _ = readLimitFile(filepath.Join(cgroupDir, "memory.max"))
	snapshot.TaskPoolPIDsCurrent, _ = readUintFile(filepath.Join(cgroupDir, "pids.current"))
	snapshot.TaskPoolPIDsMax, _ = readLimitFile(filepath.Join(cgroupDir, "pids.max"))

	if stat, statErr := os.ReadFile(filepath.Join(cgroupDir, "memory.stat")); statErr == nil {
		values := parseKeyValueLines(string(stat))
		snapshot.TaskPoolAnonBytes = values["anon"]
		snapshot.TaskPoolFileBytes = values["file"]
	}
	if pressure, pressureErr := os.ReadFile(filepath.Join(cgroupDir, "memory.pressure")); pressureErr == nil {
		snapshot.MemoryPressureSomeAvg10 = parsePressureAvg10(string(pressure), "some")
		snapshot.MemoryPressureFullAvg10 = parsePressureAvg10(string(pressure), "full")
	}

	return snapshot, nil
}

func ResolveTaskPoolCgroupDir(procRoot, cgroupRoot, delegateSubgroup string) (string, error) {
	if procRoot == "" {
		procRoot = "/proc"
	}
	if cgroupRoot == "" {
		cgroupRoot = "/sys/fs/cgroup"
	}
	if delegateSubgroup == "" {
		delegateSubgroup = "supervisor"
	}

	cgroupPath, err := readUnifiedCgroupPath(filepath.Join(procRoot, "self/cgroup"))
	if err != nil {
		return "", err
	}
	if filepath.Base(cgroupPath) == delegateSubgroup {
		cgroupPath = filepath.Dir(cgroupPath)
	}
	return filepath.Join(cgroupRoot, strings.TrimPrefix(filepath.Clean(cgroupPath), string(filepath.Separator))), nil
}

func (s Snapshot) CheckAdmission(policy AdmissionPolicy) error {
	if policy.MinSystemAvailableBytes > 0 && s.SystemAvailableBytes < policy.MinSystemAvailableBytes {
		return fmt.Errorf("%w: only %d bytes of system memory are available", ErrAdmissionDenied, s.SystemAvailableBytes)
	}
	if policy.MaxSystemSwapUsedPercent > 0 && s.SystemSwapTotalBytes > 0 {
		usedPercent := float64(s.SystemSwapUsedBytes) * 100 / float64(s.SystemSwapTotalBytes)
		if usedPercent >= policy.MaxSystemSwapUsedPercent {
			return fmt.Errorf("%w: system swap is %.1f%% used", ErrAdmissionDenied, usedPercent)
		}
	}
	if policy.MaxMemoryPressureAvg10 > 0 && s.MemoryPressureFullAvg10 >= policy.MaxMemoryPressureAvg10 {
		return fmt.Errorf("%w: task-pool memory pressure avg10 is %.2f", ErrAdmissionDenied, s.MemoryPressureFullAvg10)
	}
	return nil
}

func readUnifiedCgroupPath(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), ":", 3)
		if len(parts) == 3 && parts[0] == "0" && parts[1] == "" && strings.HasPrefix(parts[2], "/") {
			return parts[2], nil
		}
	}
	return "", errors.New("unified cgroup path unavailable")
}

func readProcMemoryValueBytes(path, key string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, key) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, errors.New("memory line missing value")
		}
		value, parseErr := strconv.ParseUint(fields[1], 10, 64)
		if parseErr != nil {
			return 0, parseErr
		}
		unit := "b"
		if len(fields) >= 3 {
			unit = strings.ToLower(fields[2])
		}
		switch unit {
		case "b":
			return value, nil
		case "kb":
			return value * 1024, nil
		case "mb":
			return value * 1024 * 1024, nil
		case "gb":
			return value * 1024 * 1024 * 1024, nil
		default:
			return 0, errors.New("unsupported memory unit")
		}
	}
	return 0, errors.New("memory key not found")
}

func readUintFile(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
}

func readLimitFile(path string) (*uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(string(data))
	if raw == "max" {
		return nil, nil
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func parseKeyValueLines(content string) map[string]uint64 {
	result := make(map[string]uint64)
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err == nil {
			result[fields[0]] = value
		}
	}
	return result
}

func parsePressureAvg10(content, kind string) float64 {
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != kind {
			continue
		}
		for _, field := range fields[1:] {
			if !strings.HasPrefix(field, "avg10=") {
				continue
			}
			value, _ := strconv.ParseFloat(strings.TrimPrefix(field, "avg10="), 64)
			return value
		}
	}
	return 0
}
