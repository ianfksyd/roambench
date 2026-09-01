package systemstats

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFixture(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func TestReaderReportsHostAndWholeServiceCgroupMemory(t *testing.T) {
	root := t.TempDir()
	procRoot := filepath.Join(root, "proc")
	cgroupRoot := filepath.Join(root, "cgroup")
	servicePath := "system.slice/roambench.service"

	writeFixture(t, procRoot, "self/status", "Name:\troambench\nVmRSS:\t32768 kB\n")
	writeFixture(t, procRoot, "meminfo", "MemTotal: 16777216 kB\nMemAvailable: 4194304 kB\nSwapTotal: 4194304 kB\nSwapFree: 1048576 kB\n")
	writeFixture(t, procRoot, "self/cgroup", "0::/"+servicePath+"/supervisor\n")
	writeFixture(t, cgroupRoot, servicePath+"/memory.current", "6442450944\n")
	writeFixture(t, cgroupRoot, servicePath+"/memory.swap.current", "536870912\n")
	writeFixture(t, cgroupRoot, servicePath+"/memory.high", "6442450944\n")
	writeFixture(t, cgroupRoot, servicePath+"/memory.max", "8589934592\n")
	writeFixture(t, cgroupRoot, servicePath+"/memory.stat", "anon 2147483648\nfile 3758096384\nkernel 536870912\n")
	writeFixture(t, cgroupRoot, servicePath+"/memory.pressure", "some avg10=3.25 avg60=1.00 avg300=0.20 total=10\nfull avg10=1.50 avg60=0.50 avg300=0.10 total=5\n")
	writeFixture(t, cgroupRoot, servicePath+"/pids.current", "141\n")
	writeFixture(t, cgroupRoot, servicePath+"/pids.max", "384\n")

	got, err := (Reader{ProcRoot: procRoot, CgroupRoot: cgroupRoot, DelegateSubgroup: "supervisor"}).Read()
	if err != nil {
		t.Fatalf("Read(): %v", err)
	}

	if got.ProcessRSSBytes != 32<<20 {
		t.Fatalf("ProcessRSSBytes = %d, want %d", got.ProcessRSSBytes, uint64(32<<20))
	}
	if got.SystemUsedBytes != 12<<30 || got.SystemSwapUsedBytes != 3<<30 {
		t.Fatalf("host usage = memory %d swap %d, want %d and %d", got.SystemUsedBytes, got.SystemSwapUsedBytes, uint64(12<<30), uint64(3<<30))
	}
	if !got.TaskPoolAvailable {
		t.Fatal("TaskPoolAvailable = false, want true")
	}
	if got.TaskPoolCurrentBytes != 6<<30 || got.TaskPoolAnonBytes != 2<<30 || got.TaskPoolFileBytes != 3584<<20 {
		t.Fatalf("task pool bytes = current %d anon %d file %d", got.TaskPoolCurrentBytes, got.TaskPoolAnonBytes, got.TaskPoolFileBytes)
	}
	if got.TaskPoolMemoryHighBytes == nil || *got.TaskPoolMemoryHighBytes != 6<<30 {
		t.Fatalf("TaskPoolMemoryHighBytes = %v, want %d", got.TaskPoolMemoryHighBytes, uint64(6<<30))
	}
	if got.TaskPoolMemoryMaxBytes == nil || *got.TaskPoolMemoryMaxBytes != 8<<30 {
		t.Fatalf("TaskPoolMemoryMaxBytes = %v, want %d", got.TaskPoolMemoryMaxBytes, uint64(8<<30))
	}
	if got.TaskPoolPIDsCurrent != 141 || got.TaskPoolPIDsMax == nil || *got.TaskPoolPIDsMax != 384 {
		t.Fatalf("task pool PIDs = current %d max %v", got.TaskPoolPIDsCurrent, got.TaskPoolPIDsMax)
	}
	if got.MemoryPressureSomeAvg10 != 3.25 || got.MemoryPressureFullAvg10 != 1.5 {
		t.Fatalf("memory pressure = some %.2f full %.2f", got.MemoryPressureSomeAvg10, got.MemoryPressureFullAvg10)
	}
}

func TestReaderTreatsUnlimitedCgroupValuesAsNoLimit(t *testing.T) {
	root := t.TempDir()
	procRoot := filepath.Join(root, "proc")
	cgroupRoot := filepath.Join(root, "cgroup")
	servicePath := "user.slice/roambench.service"

	writeFixture(t, procRoot, "self/status", "VmRSS: 1 kB\n")
	writeFixture(t, procRoot, "meminfo", "MemTotal: 8 kB\nMemAvailable: 3 kB\n")
	writeFixture(t, procRoot, "self/cgroup", "0::/"+servicePath+"\n")
	writeFixture(t, cgroupRoot, servicePath+"/memory.current", "4096\n")
	writeFixture(t, cgroupRoot, servicePath+"/memory.high", "max\n")
	writeFixture(t, cgroupRoot, servicePath+"/memory.max", "max\n")
	writeFixture(t, cgroupRoot, servicePath+"/pids.max", "max\n")

	got, err := (Reader{ProcRoot: procRoot, CgroupRoot: cgroupRoot}).Read()
	if err != nil {
		t.Fatalf("Read(): %v", err)
	}
	if got.TaskPoolMemoryHighBytes != nil || got.TaskPoolMemoryMaxBytes != nil || got.TaskPoolPIDsMax != nil {
		t.Fatalf("unlimited values should be nil: high=%v max=%v pids=%v", got.TaskPoolMemoryHighBytes, got.TaskPoolMemoryMaxBytes, got.TaskPoolPIDsMax)
	}
}

func TestReaderKeepsHostStatsWhenCgroupMetricsAreUnavailable(t *testing.T) {
	root := t.TempDir()
	procRoot := filepath.Join(root, "proc")
	cgroupRoot := filepath.Join(root, "cgroup")

	writeFixture(t, procRoot, "self/status", "VmRSS: 1024 kB\n")
	writeFixture(t, procRoot, "meminfo", "MemTotal: 4096 kB\nMemAvailable: 1024 kB\n")
	writeFixture(t, procRoot, "self/cgroup", "0::/missing.slice/roambench.service\n")

	got, err := (Reader{ProcRoot: procRoot, CgroupRoot: cgroupRoot}).Read()
	if err != nil {
		t.Fatalf("Read(): %v", err)
	}
	if got.TaskPoolAvailable {
		t.Fatal("TaskPoolAvailable = true, want false")
	}
	if got.SystemUsedBytes != 3<<20 {
		t.Fatalf("SystemUsedBytes = %d, want %d", got.SystemUsedBytes, uint64(3<<20))
	}
}

func TestReaderDoesNotReportSwapSaturationFromPartialMeminfo(t *testing.T) {
	root := t.TempDir()
	procRoot := filepath.Join(root, "proc")
	cgroupRoot := filepath.Join(root, "cgroup")

	writeFixture(t, procRoot, "self/status", "VmRSS: 1024 kB\n")
	writeFixture(t, procRoot, "meminfo", "MemTotal: 4096 kB\nMemAvailable: 1024 kB\nSwapTotal: 2048 kB\n")
	writeFixture(t, procRoot, "self/cgroup", "0::/missing.slice/roambench.service\n")

	got, err := (Reader{ProcRoot: procRoot, CgroupRoot: cgroupRoot}).Read()
	if err != nil {
		t.Fatalf("Read(): %v", err)
	}
	if got.SystemSwapTotalBytes != 0 || got.SystemSwapUsedBytes != 0 {
		t.Fatalf("partial swap stats = total %d used %d, want unavailable zeros", got.SystemSwapTotalBytes, got.SystemSwapUsedBytes)
	}
}

func TestSnapshotAdmissionPolicyRejectsEachPressureSignal(t *testing.T) {
	policy := AdmissionPolicy{
		MinSystemAvailableBytes:  2 << 30,
		MaxSystemSwapUsedPercent: 75,
		MaxMemoryPressureAvg10:   10,
	}

	tests := []struct {
		name string
		stat Snapshot
	}{
		{
			name: "low available memory",
			stat: Snapshot{SystemAvailableBytes: 1 << 30},
		},
		{
			name: "swap saturation",
			stat: Snapshot{SystemAvailableBytes: 4 << 30, SystemSwapTotalBytes: 4 << 30, SystemSwapUsedBytes: 3 << 30},
		},
		{
			name: "sustained cgroup pressure",
			stat: Snapshot{SystemAvailableBytes: 4 << 30, MemoryPressureFullAvg10: 12},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.stat.CheckAdmission(policy); err == nil {
				t.Fatal("CheckAdmission error = nil, want pressure rejection")
			}
		})
	}
}

func TestSnapshotAdmissionPolicyAllowsHealthyHost(t *testing.T) {
	stat := Snapshot{
		SystemAvailableBytes:    5 << 30,
		SystemSwapTotalBytes:    4 << 30,
		SystemSwapUsedBytes:     1 << 30,
		MemoryPressureFullAvg10: 0.25,
	}
	policy := AdmissionPolicy{
		MinSystemAvailableBytes:  2 << 30,
		MaxSystemSwapUsedPercent: 75,
		MaxMemoryPressureAvg10:   10,
	}

	if err := stat.CheckAdmission(policy); err != nil {
		t.Fatalf("CheckAdmission(): %v", err)
	}
}
