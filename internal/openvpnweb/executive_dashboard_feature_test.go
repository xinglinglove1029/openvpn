package openvpnweb

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

func TestExecutiveDashboardDefaultEnabledFor(t *testing.T) {
	cases := []struct {
		name        string
		cpuCores    float64
		memoryBytes uint64
		want        bool
	}{
		{name: "one core two gib", cpuCores: 1, memoryBytes: 2 * 1024 * 1024 * 1024, want: false},
		{name: "fractional cpu quota", cpuCores: 0.5, memoryBytes: 1024 * 1024 * 1024, want: false},
		{name: "one core more memory", cpuCores: 1, memoryBytes: 3 * 1024 * 1024 * 1024, want: true},
		{name: "two cores two gib", cpuCores: 2, memoryBytes: 2 * 1024 * 1024 * 1024, want: true},
		{name: "unknown capacity fails open", cpuCores: 0, memoryBytes: 0, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := executiveDashboardDefaultEnabledFor(tc.cpuCores, tc.memoryBytes); got != tc.want {
				t.Fatalf("executiveDashboardDefaultEnabledFor(%v, %d) = %t, want %t", tc.cpuCores, tc.memoryBytes, got, tc.want)
			}
		})
	}
}

func TestCgroupCapacityReaders(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "cpu.max"), []byte("100000 100000\n"), 0600); err != nil {
		t.Fatalf("write cpu.max: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "memory.max"), []byte("2147483648\n"), 0600); err != nil {
		t.Fatalf("write memory.max: %v", err)
	}
	cores, ok := cgroupCPUQuotaCores(root)
	if !ok || cores != 1 {
		t.Fatalf("cgroup CPU quota = %v, %t; want 1, true", cores, ok)
	}
	memoryBytes, ok := cgroupMemoryLimit(root)
	if !ok || memoryBytes != executiveDashboardLowSpecMemoryBytes {
		t.Fatalf("cgroup memory = %d, %t; want %d, true", memoryBytes, ok, executiveDashboardLowSpecMemoryBytes)
	}
}

func TestRequireExecutiveDashboard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	viper.Reset()
	t.Cleanup(viper.Reset)

	router := gin.New()
	router.GET("/dashboard", RequireExecutiveDashboard(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	viper.Set("system.base.executive_dashboard_enabled", false)
	blocked := httptest.NewRecorder()
	router.ServeHTTP(blocked, httptest.NewRequest(http.MethodGet, "/dashboard", nil))
	if blocked.Code != http.StatusNotFound {
		t.Fatalf("disabled dashboard status = %d, want %d", blocked.Code, http.StatusNotFound)
	}

	viper.Set("system.base.executive_dashboard_enabled", true)
	allowed := httptest.NewRecorder()
	router.ServeHTTP(allowed, httptest.NewRequest(http.MethodGet, "/dashboard", nil))
	if allowed.Code != http.StatusNoContent {
		t.Fatalf("enabled dashboard status = %d, want %d", allowed.Code, http.StatusNoContent)
	}
}

func TestInitConfigPersistsExecutiveDashboardDefaultForExistingConfig(t *testing.T) {
	previousOVData := ovData
	ovData = t.TempDir()
	viper.Reset()
	t.Cleanup(func() {
		ovData = previousOVData
		viper.Reset()
	})

	configPath := filepath.Join(ovData, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"system":{"base":{"token":"existing-token"}}}`), 0600); err != nil {
		t.Fatalf("write existing config: %v", err)
	}

	want := executiveDashboardDefaultEnabled()
	initConfig()
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read persisted config: %v", err)
	}
	var persisted struct {
		System struct {
			Base struct {
				ExecutiveDashboardEnabled *bool `json:"executive_dashboard_enabled"`
			} `json:"base"`
		} `json:"system"`
	}
	if err := json.Unmarshal(content, &persisted); err != nil {
		t.Fatalf("unmarshal persisted config: %v", err)
	}
	if persisted.System.Base.ExecutiveDashboardEnabled == nil {
		t.Fatal("executive_dashboard_enabled was not persisted to the existing config")
	}
	if got := *persisted.System.Base.ExecutiveDashboardEnabled; got != want {
		t.Fatalf("persisted executive_dashboard_enabled = %t, want %t", got, want)
	}
}

func TestInitConfigPersistsExecutiveDashboardChoice(t *testing.T) {
	previousOVData := ovData
	ovData = t.TempDir()
	viper.Reset()
	t.Cleanup(func() {
		ovData = previousOVData
		viper.Reset()
	})

	configPath := filepath.Join(ovData, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"system":{"base":{"token":"existing-token","executive_dashboard_enabled":true}}}`), 0600); err != nil {
		t.Fatalf("write existing config: %v", err)
	}
	initConfig()
	if !viper.GetBool("system.base.executive_dashboard_enabled") {
		t.Fatal("explicit executive_dashboard_enabled=true was overwritten")
	}
}
