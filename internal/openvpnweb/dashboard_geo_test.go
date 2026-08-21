package openvpnweb

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestDashboardGeoIPWithResolverAcceptsPublicIPv4(t *testing.T) {
	ip, region, ok := dashboardGeoIPWithResolver("8.8.8.8", func(value string) string {
		if value != "8.8.8.8" {
			t.Fatalf("resolver received %q", value)
		}
		return "美国|0|加利福尼亚州|山景城|Google"
	})
	if !ok || ip != "8.8.8.8" {
		t.Fatalf("expected public IPv4 to resolve, got ip=%q ok=%v", ip, ok)
	}
	if region.Country != "美国" || region.Province != "加利福尼亚州" || region.City != "山景城" || region.Label != "山景城" {
		t.Fatalf("unexpected region: %#v", region)
	}
}

func TestDashboardGeoIPWithResolverRejectsPrivateAndVPNAddresses(t *testing.T) {
	called := false
	resolver := func(string) string {
		called = true
		return "中国|0|浙江省|杭州|ISP"
	}
	for _, ip := range []string{
		"10.8.0.2", "192.168.1.20", "172.17.0.2", "127.0.0.1",
		"100.64.0.1", "169.254.1.1", "192.0.2.1", "198.18.0.1",
		"198.51.100.1", "203.0.113.1", "224.0.0.1", "255.255.255.255",
		"::1", "2001:4860:4860::8888", "::ffff:8.8.8.8", "192.0.0.8",
	} {
		if _, _, ok := dashboardGeoIPWithResolver(ip, resolver); ok {
			t.Fatalf("%s must not be mapped", ip)
		}
	}
	if called {
		t.Fatal("private, VPN and IPv6 addresses must not be sent to GeoIP resolver")
	}
}

func TestParseDashboardGeoRegionFallsBackToCountry(t *testing.T) {
	region := parseDashboardGeoRegion("日本|0|0|0|ISP")
	if region.Country != "日本" || region.Province != "" || region.City != "" || region.Label != "日本" {
		t.Fatalf("unexpected fallback region: %#v", region)
	}
}

func TestDashboardGeoAddWithResolverDeduplicatesIPsAndAggregatesRegions(t *testing.T) {
	points := map[string]DashboardGeoPoint{}
	seen := map[string]map[string]struct{}{dashboardGeoSourceAudit: {}}
	unknown := int64(0)
	resolver := func(ip string) string {
		switch ip {
		case "8.8.8.8", "1.1.1.1":
			return "美国|0|加利福尼亚州|山景城|ISP"
		default:
			return ""
		}
	}
	for _, ip := range []string{"8.8.8.8", "8.8.8.8", "1.1.1.1", "10.8.0.2", "10.8.0.2"} {
		dashboardGeoAddWithResolver(points, seen, &unknown, dashboardGeoSourceAudit, ip, resolver)
	}
	if len(points) != 1 {
		t.Fatalf("expected one region aggregate, got %#v", points)
	}
	for _, point := range points {
		if point.Count != 2 {
			t.Fatalf("expected two distinct public IPs, got %d", point.Count)
		}
	}
	if unknown != 1 {
		t.Fatalf("expected one deduplicated unknown IP, got %d", unknown)
	}
}

func TestDashboardGeoCanSeeOnlineClientHonorsVisibleUserScope(t *testing.T) {
	visible := map[string]struct{}{"alice": {}}
	if !dashboardGeoCanSeeOnlineClient(ClientData{Username: "alice"}, visible, false) {
		t.Fatal("visible user should be included")
	}
	if dashboardGeoCanSeeOnlineClient(ClientData{Username: "bob", CommonName: "bob-cert"}, visible, false) {
		t.Fatal("out-of-scope user must not be included")
	}
	if !dashboardGeoCanSeeOnlineClient(ClientData{Username: "bob"}, visible, true) {
		t.Fatal("unfiltered administrator scope should include client")
	}
}

func TestDashboardGeoIPWithResolverNormalizesHostPort(t *testing.T) {
	ip, region, ok := dashboardGeoIPWithResolver("8.8.8.8:443", func(value string) string {
		if value != "8.8.8.8" {
			t.Fatalf("resolver received %q", value)
		}
		return "美国|0|0|0|ISP"
	})
	if !ok || ip != "8.8.8.8" || region.Label != "美国" {
		t.Fatalf("expected host:port public IPv4 to resolve, got ip=%q region=%#v ok=%v", ip, region, ok)
	}
}

func TestDashboardGeoAddInitializesSourceDeduplicationSet(t *testing.T) {
	points := map[string]DashboardGeoPoint{}
	seen := map[string]map[string]struct{}{}
	unknown := int64(0)
	dashboardGeoAddWithResolver(points, seen, &unknown, dashboardGeoSourceOnline, "8.8.8.8", func(string) string {
		return "美国|0|0|0|ISP"
	})
	if len(seen[dashboardGeoSourceOnline]) != 1 || len(points) != 1 || unknown != 0 {
		t.Fatalf("expected source set and point to be initialized, seen=%#v points=%#v unknown=%d", seen, points, unknown)
	}
}

func TestDashboardGeoDeduplicatesBeforeResolvingAndNormalizesUnknownHostPort(t *testing.T) {
	points := map[string]DashboardGeoPoint{}
	seen := map[string]map[string]struct{}{}
	unknown := int64(0)
	resolverCalls := 0
	resolver := func(string) string {
		resolverCalls++
		return "美国|0|0|0|ISP"
	}
	for _, ip := range []string{"8.8.8.8", "8.8.8.8:443", "10.8.0.2", "10.8.0.2:443"} {
		dashboardGeoAddWithResolver(points, seen, &unknown, dashboardGeoSourceOnline, ip, resolver)
	}
	if resolverCalls != 1 {
		t.Fatalf("expected one GeoIP lookup for equivalent public IP, got %d", resolverCalls)
	}
	if len(points) != 1 || unknown != 1 {
		t.Fatalf("expected one point and one canonical unknown, points=%#v unknown=%d", points, unknown)
	}
}

func TestParseDashboardGeoDateBounds(t *testing.T) {
	start, err := parseDashboardGeoUnix("2026-08-20")
	if err != nil {
		t.Fatal(err)
	}
	end, err := parseDashboardGeoEndUnix("2026-08-20")
	if err != nil {
		t.Fatal(err)
	}
	wantStart := time.Date(2026, time.August, 20, 0, 0, 0, 0, time.Local).Unix()
	wantEnd := time.Date(2026, time.August, 20, 23, 59, 59, 0, time.Local).Unix()
	if start != wantStart || end != wantEnd {
		t.Fatalf("unexpected date bounds: start=%d end=%d", start, end)
	}
	if _, err := parseDashboardGeoUnix("not-a-time"); err == nil {
		t.Fatal("invalid start must return an error")
	}
	if _, err := parseDashboardGeoEndUnix("not-a-time"); err == nil {
		t.Fatal("invalid end must return an error")
	}
}

func TestDashboardGeoHandlerRejectsInvalidSourceAndRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	ov := &ovpn{}
	router.GET("/geo", ov.dashboardGeo)
	for _, query := range []string{"?source=invalid", "?start=not-a-time", "?end=not-a-time"} {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/geo"+query, nil)
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d (%s)", query, recorder.Code, recorder.Body.String())
		}
	}
}

func TestDashboardGeoHasSourceUsesFineGrainedPermissions(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("isAdmin", false)
	ctx.Set("permissions", []string{"client:view_online"})
	if !dashboardGeoHasSource(ctx, dashboardGeoSourceOnline) {
		t.Fatal("online permission should make online source available")
	}
	if dashboardGeoHasSource(ctx, dashboardGeoSourceAudit) || dashboardGeoHasSource(ctx, dashboardGeoSourceWebsite) {
		t.Fatal("ungranted data sources must remain unavailable")
	}
}
