package server

import (
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type memoryStatusResponse struct {
	ProcessRSSBytes  uint64 `json:"processRSSBytes"`
	SystemUsedBytes  uint64 `json:"systemUsedBytes"`
	TotalMemoryBytes uint64 `json:"totalMemoryBytes"`
}

func (s *Server) handleMemoryStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Cache-Control", "no-store")

	stats, err := readMemoryStatus()
	if err != nil {
		log.Printf("memory status error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read memory status"})
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

func readMemoryStatus() (memoryStatusResponse, error) {
	processRSS, err := readProcMemoryValueBytes("/proc/self/status", "VmRSS:")
	if err != nil {
		return memoryStatusResponse{}, err
	}

	totalMemory, err := readProcMemoryValueBytes("/proc/meminfo", "MemTotal:")
	if err != nil {
		return memoryStatusResponse{}, err
	}
	availableMemory, err := readProcMemoryValueBytes("/proc/meminfo", "MemAvailable:")
	if err != nil {
		availableMemory, err = readProcMemoryValueBytes("/proc/meminfo", "MemFree:")
		if err != nil {
			return memoryStatusResponse{}, err
		}
	}

	if totalMemory == 0 {
		return memoryStatusResponse{}, errors.New("total memory unavailable")
	}
	if availableMemory > totalMemory {
		return memoryStatusResponse{}, errors.New("available memory exceeds total memory")
	}

	return memoryStatusResponse{
		ProcessRSSBytes:  processRSS,
		SystemUsedBytes:  totalMemory - availableMemory,
		TotalMemoryBytes: totalMemory,
	}, nil
}

func readProcMemoryValueBytes(path, key string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return parseProcMemoryValueBytes(string(data), key)
}

func parseProcMemoryValueBytes(content, key string) (uint64, error) {
	for _, line := range strings.Split(content, "\n") {
		if !strings.HasPrefix(line, key) {
			continue
		}
		return parseProcMemoryLineBytes(line)
	}
	return 0, errors.New("memory key not found")
}

func parseProcMemoryLineBytes(line string) (uint64, error) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0, errors.New("memory line missing value")
	}

	value, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, err
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
