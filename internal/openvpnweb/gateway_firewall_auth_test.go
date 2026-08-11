package openvpnweb

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

func TestInitConfigDefaultsGatewayEnabled(t *testing.T) {
	previousOVData := ovData
	ovData = t.TempDir()
	viper.Reset()
	t.Cleanup(func() {
		ovData = previousOVData
		viper.Reset()
	})

	initConfig()

	if !viper.GetBool("openvpn.ovpn_gateway") {
		t.Fatal("new configuration defaults ovpn_gateway to false, want true")
	}

	contents, err := os.ReadFile(filepath.Join(ovData, "config.json"))
	if err != nil {
		t.Fatalf("read generated config.json: %v", err)
	}
	var config struct {
		OpenVPN struct {
			Gateway  *bool  `json:"ovpn_gateway"`
			PushDNS1 string `json:"ovpn_push_dns1"`
			PushDNS2 string `json:"ovpn_push_dns2"`
		} `json:"openvpn"`
	}
	if err := json.Unmarshal(contents, &config); err != nil {
		t.Fatalf("parse generated config.json: %v", err)
	}
	if config.OpenVPN.Gateway == nil || !*config.OpenVPN.Gateway {
		t.Fatalf("generated ovpn_gateway = %v, want true", config.OpenVPN.Gateway)
	}
	if config.OpenVPN.PushDNS1 != "8.8.8.8" || config.OpenVPN.PushDNS2 != "2001:4860:4860::8888" {
		t.Fatalf("generated DNS defaults = %q, %q", config.OpenVPN.PushDNS1, config.OpenVPN.PushDNS2)
	}
}

func TestInitConfigPreservesExplicitGatewayDisabled(t *testing.T) {
	previousOVData := ovData
	ovData = t.TempDir()
	viper.Reset()
	t.Cleanup(func() {
		ovData = previousOVData
		viper.Reset()
	})

	configPath := filepath.Join(ovData, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"system":{"base":{"token":"existing-token"}},"openvpn":{"ovpn_gateway":false}}`), 0600); err != nil {
		t.Fatalf("write existing config.json: %v", err)
	}

	initConfig()

	if viper.GetBool("openvpn.ovpn_gateway") {
		t.Fatal("explicit ovpn_gateway=false was overwritten")
	}
}

func TestDockerEntrypointDefaultsMissingGatewayToEnabled(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source file")
	}
	entrypointPath := filepath.Join(filepath.Dir(currentFile), "..", "..", "build", "docker-entrypoint.sh")
	contents, err := os.ReadFile(entrypointPath)
	if err != nil {
		t.Fatalf("read docker entrypoint: %v", err)
	}
	entrypoint := string(contents)
	if !strings.Contains(entrypoint, `OVPN_GATEWAY=$(jq -r '.openvpn.ovpn_gateway // "true"' $SYSTEM_CONFIG)`) {
		t.Fatal("entrypoint does not default a missing ovpn_gateway field to true")
	}
	for _, push := range []string{
		`push "dhcp-option DNS 8.8.8.8"`,
		`push "dhcp-option DNS 2001:4860:4860::8888"`,
		`push "redirect-gateway def1 ipv6 bypass-dhcp"`,
	} {
		if !strings.Contains(entrypoint, push) {
			t.Fatalf("entrypoint is missing gateway push %q", push)
		}
	}
}

func newFirewallHookTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions("test-session", cookie.NewStore([]byte("test-session-secret"))))
	router.Use(AuthMiddleWare())
	router.Any("/ovpn/firewall", RequirePermission("firewall:create"), func(c *gin.Context) {
		internal := hasInternalFirewallHookIdentity(c)
		_, hasAdminIdentity := c.Get("isAdmin")
		c.JSON(http.StatusOK, gin.H{"internal": internal, "hasAdminIdentity": hasAdminIdentity})
	})
	return router
}

func TestOpenVPNFirewallHookReceivesOnlyNarrowInternalIdentity(t *testing.T) {
	viper.Reset()
	viper.Set("system.base.token", "test-internal-token")
	t.Cleanup(viper.Reset)

	router := newFirewallHookTestRouter()
	for _, testCase := range []struct {
		action     string
		remoteAddr string
	}{
		{action: "add_ovips", remoteAddr: "127.0.0.1:12000"},
		{action: "delete_ovips", remoteAddr: "127.0.0.1:12000"},
		{action: "add_ovips", remoteAddr: "[::1]:12000"},
	} {
		req := httptest.NewRequest(http.MethodPost, "/ovpn/firewall?a="+testCase.action, nil)
		req.RemoteAddr = testCase.remoteAddr
		req.Header.Set("O-Token", "test-internal-token")
		response := httptest.NewRecorder()

		router.ServeHTTP(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("%s from %s status = %d, want %d; body=%s", testCase.action, testCase.remoteAddr, response.Code, http.StatusOK, response.Body.String())
		}
		var body struct {
			Internal         bool `json:"internal"`
			HasAdminIdentity bool `json:"hasAdminIdentity"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode %s response: %v", testCase.action, err)
		}
		if !body.Internal || body.HasAdminIdentity {
			t.Fatalf("%s from %s identity = %#v, want narrow internal identity without admin", testCase.action, testCase.remoteAddr, body)
		}
	}
}

func TestUntrustedFirewallRequestsDoNotReceiveInternalIdentity(t *testing.T) {
	viper.Reset()
	viper.Set("system.base.token", "test-internal-token")
	t.Cleanup(viper.Reset)

	router := newFirewallHookTestRouter()
	testCases := []struct {
		name       string
		method     string
		path       string
		remoteAddr string
		token      string
	}{
		{name: "non-loopback source", method: http.MethodPost, path: "/ovpn/firewall?a=add_ovips", remoteAddr: "198.51.100.20:12000", token: "test-internal-token"},
		{name: "empty token", method: http.MethodPost, path: "/ovpn/firewall?a=add_ovips", remoteAddr: "127.0.0.1:12000", token: ""},
		{name: "wrong token", method: http.MethodPost, path: "/ovpn/firewall?a=add_ovips", remoteAddr: "127.0.0.1:12000", token: "wrong-token"},
		{name: "wrong method", method: http.MethodGet, path: "/ovpn/firewall?a=add_ovips", remoteAddr: "127.0.0.1:12000", token: "test-internal-token"},
		{name: "other firewall action", method: http.MethodPost, path: "/ovpn/firewall?a=add_blacklist", remoteAddr: "127.0.0.1:12000", token: "test-internal-token"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			req := httptest.NewRequest(testCase.method, testCase.path, nil)
			req.RemoteAddr = testCase.remoteAddr
			if testCase.token != "" {
				req.Header.Set("O-Token", testCase.token)
			}
			response := httptest.NewRecorder()

			router.ServeHTTP(response, req)
			if response.Code != http.StatusFound {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusFound, response.Body.String())
			}
		})
	}
}

func TestRequirePermissionDoesNotExpandInternalFirewallIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions("test-session", cookie.NewStore([]byte("test-session-secret"))))
	router.Use(func(c *gin.Context) {
		c.Set(internalFirewallHookContextKey, true)
		c.Next()
	})
	router.GET("/ovpn/other", RequirePermission("user:delete"), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.POST("/ovpn/firewall", RequirePermission("firewall:update"), func(c *gin.Context) { c.Status(http.StatusNoContent) })

	for _, testCase := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/ovpn/other"},
		{method: http.MethodPost, path: "/ovpn/firewall?a=add_ovips"},
	} {
		req := httptest.NewRequest(testCase.method, testCase.path, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s %s status = %d, want %d", testCase.method, testCase.path, response.Code, http.StatusForbidden)
		}
	}
}

func TestInternalFirewallHookUsesDedicatedAuditOperator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Set(internalFirewallHookContextKey, true)

	if got := auditOperator(context); got != internalFirewallHookAuditActor {
		t.Fatalf("audit operator = %q, want %q", got, internalFirewallHookAuditActor)
	}
}
