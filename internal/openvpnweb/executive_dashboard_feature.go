package openvpnweb

import (
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/spf13/viper"
)

const executiveDashboardLowSpecMemoryBytes uint64 = 2 * 1024 * 1024 * 1024

// executiveDashboardDefaultEnabled returns the first-start default only. A
// deployment is considered constrained when both its effective CPU allocation
// is at most one core and its effective memory allocation is at most 2 GiB.
// The AND is deliberate: a 1-core machine with more memory (or 2 cores with
// 2 GiB memory) keeps the feature enabled by default.
func executiveDashboardDefaultEnabled() bool {
	cpuCores, memoryBytes := effectiveMachineCapacity()
	return executiveDashboardDefaultEnabledFor(cpuCores, memoryBytes)
}

func executiveDashboardDefaultEnabledFor(cpuCores float64, memoryBytes uint64) bool {
	return !(cpuCores > 0 && cpuCores <= 1 && memoryBytes > 0 && memoryBytes <= executiveDashboardLowSpecMemoryBytes)
}

// effectiveMachineCapacity honours Docker cgroup limits when available. Using
// host-only metrics would incorrectly enable the dashboard for a container
// limited to 1 CPU / 2 GiB on a larger host.
func effectiveMachineCapacity() (float64, uint64) {
	cpuCores := float64(runtime.NumCPU())
	memoryBytes := uint64(0)
	if info, err := mem.VirtualMemory(); err == nil {
		memoryBytes = info.Total
	}

	if quotaCores, ok := cgroupCPUQuotaCores("/sys/fs/cgroup"); ok && (cpuCores <= 0 || quotaCores < cpuCores) {
		cpuCores = quotaCores
	}
	if limit, ok := cgroupMemoryLimit("/sys/fs/cgroup"); ok && (memoryBytes == 0 || limit < memoryBytes) {
		memoryBytes = limit
	}
	return cpuCores, memoryBytes
}

func cgroupCPUQuotaCores(root string) (float64, bool) {
	// cgroup v2: cpu.max contains either "max <period>" or "<quota> <period>".
	if raw, err := os.ReadFile(filepath.Join(root, "cpu.max")); err == nil {
		parts := strings.Fields(string(raw))
		if len(parts) == 2 && parts[0] != "max" {
			quota, quotaErr := strconv.ParseFloat(parts[0], 64)
			period, periodErr := strconv.ParseFloat(parts[1], 64)
			if quotaErr == nil && periodErr == nil && quota > 0 && period > 0 {
				return math.Max(quota/period, 0.01), true
			}
		}
	}

	// cgroup v1: quota and period are stored in separate files.
	quota, quotaErr := readInt64File(filepath.Join(root, "cpu", "cpu.cfs_quota_us"))
	period, periodErr := readInt64File(filepath.Join(root, "cpu", "cpu.cfs_period_us"))
	if quotaErr == nil && periodErr == nil && quota > 0 && period > 0 {
		return math.Max(float64(quota)/float64(period), 0.01), true
	}
	return 0, false
}

func cgroupMemoryLimit(root string) (uint64, bool) {
	// cgroup v2: "max" means no limit.
	if raw, err := os.ReadFile(filepath.Join(root, "memory.max")); err == nil {
		value := strings.TrimSpace(string(raw))
		if value != "" && value != "max" {
			if limit, err := strconv.ParseUint(value, 10, 64); err == nil && isUsableMemoryLimit(limit) {
				return limit, true
			}
		}
	}

	// cgroup v1: a very large sentinel value represents an unlimited cgroup.
	if limit, err := readUint64File(filepath.Join(root, "memory", "memory.limit_in_bytes")); err == nil && isUsableMemoryLimit(limit) {
		return limit, true
	}
	return 0, false
}

func isUsableMemoryLimit(limit uint64) bool {
	return limit > 0 && limit < (1<<60)
}

func readInt64File(path string) (int64, error) {
	value, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(value)), 10, 64)
}

func readUint64File(path string) (uint64, error) {
	value, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(value)), 10, 64)
}

func executiveDashboardEnabled() bool {
	return viper.GetBool("system.base.executive_dashboard_enabled")
}

// RequireExecutiveDashboard prevents direct API calls from initializing the
// expensive geographic aggregation and boundary-fetch paths while the optional
// operations screen is turned off.
func RequireExecutiveDashboard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !executiveDashboardEnabled() {
			c.AbortWithStatusJSON(404, gin.H{"message": "运营大屏功能未启用，可在系统设置中开启"})
			return
		}
		c.Next()
	}
}
