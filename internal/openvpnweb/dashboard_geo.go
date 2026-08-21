package openvpnweb

import (
	"context"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	dashboardGeoSourceOnline  = "online"
	dashboardGeoSourceAudit   = "audit"
	dashboardGeoSourceWebsite = "website"
)

// DashboardGeoPoint is deliberately an area-level aggregate. It contains no
// exact coordinate or raw IP address, preventing the dashboard from becoming
// a client location tracking surface.
type DashboardGeoPoint struct {
	Source   string `json:"source"`
	Country  string `json:"country"`
	Province string `json:"province,omitempty"`
	City     string `json:"city,omitempty"`
	Label    string `json:"label"`
	Count    int64  `json:"count"`
}

type DashboardGeoResponse struct {
	Start             int64               `json:"start"`
	End               int64               `json:"end"`
	Points            []DashboardGeoPoint `json:"points"`
	Unknown           map[string]int64    `json:"unknown"`
	WebsiteDomainOnly int64               `json:"websiteDomainOnly"`
	AvailableSources  []string            `json:"availableSources"`
	OnlineAsOf        int64               `json:"onlineAsOf,omitempty"`
	Notes             []string            `json:"notes"`
}

type dashboardGeoRegion struct {
	Country  string
	Province string
	City     string
	Label    string
}

// parseDashboardGeoRegion parses ip2region's country|region|province|city|isp
// response. A missing city is expected for many addresses; the label falls
// back to province then country so maps can still show an honest aggregation.
func parseDashboardGeoRegion(raw string) dashboardGeoRegion {
	parts := strings.Split(raw, "|")
	part := func(index int) string {
		if index < 0 || index >= len(parts) {
			return ""
		}
		value := strings.TrimSpace(parts[index])
		if value == "" || value == "0" || value == "-" || strings.EqualFold(value, "unknown") || value == "未知" || value == "未分配" {
			return ""
		}
		return value
	}
	country, province, city := part(0), part(2), part(3)
	label := city
	if label == "" {
		label = province
	}
	if label == "" {
		label = country
	}
	return dashboardGeoRegion{Country: country, Province: province, City: city, Label: label}
}

// dashboardGeoIP accepts only public IPv4. The embedded ip2region database is
// IPv4-only. IPv6 and every VPN/private/loopback address are safely counted as
// unknown instead of being looked up through an external service.
func dashboardGeoAddressText(raw string) string {
	ipText := strings.TrimSpace(raw)
	if host, _, err := net.SplitHostPort(ipText); err == nil {
		return host
	}
	return strings.Trim(ipText, "[]")
}

func dashboardGeoAddressKey(raw string) string {
	ipText := dashboardGeoAddressText(raw)
	// Keep IPv6 literals distinct from their IPv4-mapped representation. They
	// are intentionally counted as unlocatable rather than deduplicating away a
	// separately observed, real IPv4 address.
	if strings.Contains(ipText, ":") {
		return strings.ToLower(ipText)
	}
	if parsed := net.ParseIP(ipText); parsed != nil {
		return parsed.String()
	}
	return ipText
}

func dashboardGeoIPWithResolver(raw string, resolve func(string) string) (string, dashboardGeoRegion, bool) {
	ipText := dashboardGeoAddressText(raw)
	// IPv4-mapped IPv6 is still an IPv6 literal. Do not turn it into a
	// geolocated IPv4 point when this endpoint explicitly supports IPv4 only.
	if ipText == "" || strings.Contains(ipText, ":") {
		return "", dashboardGeoRegion{}, false
	}
	parsed := net.ParseIP(ipText)
	if parsed == nil || !dashboardGeoIsPublicIPv4(parsed) {
		return "", dashboardGeoRegion{}, false
	}
	ip := parsed.To4().String()
	region := parseDashboardGeoRegion(resolve(ip))
	if region.Label == "" {
		return ip, dashboardGeoRegion{}, false
	}
	return ip, region, true
}

func dashboardGeoIP(raw string) (string, dashboardGeoRegion, bool) {
	return dashboardGeoIPWithResolver(raw, GetIPRegion)
}

// dashboardGeoIsPublicIPv4 rejects every non-global IPv4 allocation before
// ip2region is invoked. Besides RFC1918 this includes the VPN/CGNAT,
// loopback, link-local, documentation, benchmark, multicast and reserved
// ranges. This keeps the map honest and makes it impossible to geolocate a
// virtual address merely because a resolver happens to return a value.
func dashboardGeoIsPublicIPv4(parsed net.IP) bool {
	ip := parsed.To4()
	if ip == nil {
		return false
	}
	a, b, c := ip[0], ip[1], ip[2]
	switch {
	case a == 0, a == 10, a == 127, a >= 224:
		return false
	case a == 100 && b >= 64 && b <= 127: // RFC 6598 carrier-grade NAT
		return false
	case a == 169 && b == 254: // link-local
		return false
	case a == 172 && b >= 16 && b <= 31: // RFC 1918
		return false
	case a == 192 && b == 0 && c == 0: // IANA special-purpose block
		return false
	case a == 192 && b == 0 && c == 2: // TEST-NET-1
		return false
	case a == 192 && b == 88 && c == 99: // deprecated 6to4 relay anycast
		return false
	case a == 192 && b == 168: // RFC 1918
		return false
	case a == 198 && (b == 18 || b == 19): // benchmarking
		return false
	case a == 198 && b == 51 && c == 100: // TEST-NET-2
		return false
	case a == 203 && b == 0 && c == 113: // TEST-NET-3
		return false
	case a == 255 && b == 255 && c == 255 && ip[3] == 255:
		return false
	default:
		return true
	}
}

func dashboardGeoSeenSet(seen map[string]map[string]struct{}, source string) map[string]struct{} {
	values, ok := seen[source]
	if !ok || values == nil {
		values = make(map[string]struct{})
		seen[source] = values
	}
	return values
}

func dashboardGeoAddWithResolver(points map[string]DashboardGeoPoint, seen map[string]map[string]struct{}, unknown *int64, source, rawIP string, resolve func(string) string) {
	sourceSeen := dashboardGeoSeenSet(seen, source)
	addressKey := dashboardGeoAddressKey(rawIP)
	if addressKey == "" {
		addressKey = "(empty)"
	}
	// Canonicalize and deduplicate before calling the resolver. The dashboard
	// measures distinct observed IPs, so repeated events must not cause repeated
	// ip2region lookups or inflate the unknown counter.
	if _, exists := sourceSeen[addressKey]; exists {
		return
	}
	sourceSeen[addressKey] = struct{}{}
	ip, region, ok := dashboardGeoIPWithResolver(rawIP, resolve)
	if !ok {
		(*unknown)++
		return
	}
	if ip != addressKey {
		// Keep the canonical IPv4 key in the set even if the input used a port.
		delete(sourceSeen, addressKey)
		if _, exists := sourceSeen[ip]; exists {
			return
		}
		sourceSeen[ip] = struct{}{}
	}
	key := source + "\x00" + region.Country + "\x00" + region.Province + "\x00" + region.City
	point := points[key]
	point.Source = source
	point.Country = region.Country
	point.Province = region.Province
	point.City = region.City
	point.Label = region.Label
	point.Count++
	points[key] = point
}

func dashboardGeoAdd(points map[string]DashboardGeoPoint, seen map[string]map[string]struct{}, unknown *int64, source, rawIP string) {
	dashboardGeoAddWithResolver(points, seen, unknown, source, rawIP, GetIPRegion)
}

func dashboardGeoScope(c *gin.Context) ([]uint, bool) {
	return webAuditAccessScope(c)
}

// dashboardGeoHasSource keeps map sources within the permission boundary of
// their corresponding details. The route itself is protected by menu:overview;
// unavailable sources are omitted rather than returning a 403 for the whole
// large screen.
func dashboardGeoHasSource(c *gin.Context, source string) bool {
	if isAdmin, _ := c.Get("isAdmin"); isAdmin == true {
		return true
	}
	switch source {
	case dashboardGeoSourceOnline:
		return hasPermissionCode(c, "client:view_online")
	case dashboardGeoSourceAudit:
		return hasPermissionCode(c, "audit:view")
	case dashboardGeoSourceWebsite:
		return hasPermissionCode(c, "web-audit:view")
	default:
		return false
	}
}

func normalizeDashboardGeoRange(start, end int64) (int64, int64) {
	return normalizeWebsiteAuditRange(start, end)
}

func dashboardGeoVisibleUsers(ctx context.Context, userIDs []uint, skipFilter bool) map[string]struct{} {
	if skipFilter {
		return nil
	}
	visible := make(map[string]struct{})
	if len(userIDs) == 0 || db == nil {
		return visible
	}
	var users []User
	if err := db.WithContext(ctx).Select("username", "ovpn_config").Where("id IN ?", userIDs).Find(&users).Error; err != nil {
		return visible
	}
	for _, user := range users {
		for _, identity := range []string{user.Username, user.OvpnConfig} {
			if identity = strings.TrimSpace(identity); identity != "" {
				visible[identity] = struct{}{}
			}
		}
	}
	return visible
}

func dashboardGeoCanSeeOnlineClient(client ClientData, visible map[string]struct{}, skipFilter bool) bool {
	if skipFilter {
		return true
	}
	if _, ok := visible[strings.TrimSpace(client.Username)]; ok {
		return true
	}
	_, ok := visible[strings.TrimSpace(client.CommonName)]
	return ok
}

func (ov *ovpn) dashboardGeoData(ctx context.Context, start, end int64, requested string, c *gin.Context) DashboardGeoResponse {
	start, end = normalizeDashboardGeoRange(start, end)
	result := DashboardGeoResponse{
		Start: start, End: end,
		Points:  make([]DashboardGeoPoint, 0),
		Unknown: map[string]int64{}, AvailableSources: make([]string, 0), Notes: make([]string, 0),
	}

	allowed := map[string]bool{}
	for _, source := range []string{dashboardGeoSourceOnline, dashboardGeoSourceAudit, dashboardGeoSourceWebsite} {
		if dashboardGeoHasSource(c, source) {
			allowed[source] = true
			result.AvailableSources = append(result.AvailableSources, source)
		}
	}
	requested = strings.TrimSpace(strings.ToLower(requested))
	if requested != "" && requested != "all" {
		for source := range allowed {
			if source != requested {
				delete(allowed, source)
			}
		}
	}

	userIDs, skipFilter := dashboardGeoScope(c)
	visibleUsers := dashboardGeoVisibleUsers(ctx, userIDs, skipFilter)
	points := map[string]DashboardGeoPoint{}
	seen := map[string]map[string]struct{}{
		dashboardGeoSourceOnline: {}, dashboardGeoSourceAudit: {}, dashboardGeoSourceWebsite: {},
	}

	if allowed[dashboardGeoSourceOnline] && ov != nil {
		result.OnlineAsOf = time.Now().Unix()
		result.Notes = append(result.Notes, "在线客户端来源为当前实时快照，不按所选时间范围筛选。")
		clients, managementOK := ov.safeOnlineClients()
		if !managementOK {
			result.Notes = append(result.Notes, "OpenVPN management 当前不可用，在线客户端来源暂时无法读取。")
		} else {
			for _, client := range clients {
				if !dashboardGeoCanSeeOnlineClient(client, visibleUsers, skipFilter) {
					continue
				}
				unknown := result.Unknown[dashboardGeoSourceOnline]
				dashboardGeoAdd(points, seen, &unknown, dashboardGeoSourceOnline, client.Rip)
				result.Unknown[dashboardGeoSourceOnline] = unknown
			}
		}
	}

	if allowed[dashboardGeoSourceAudit] && db != nil {
		var ips []string
		query := db.WithContext(ctx).Model(&AuditLog{}).Where("created_at >= ? AND created_at <= ?", time.Unix(start, 0), time.Unix(end, 0))
		if !skipFilter {
			if len(userIDs) == 0 {
				query = query.Where("1 = 0")
			} else {
				query = query.Where("operator_id IN ?", userIDs)
			}
		}
		if err := query.Distinct("ip").Pluck("ip", &ips).Error; err != nil {
			result.Notes = append(result.Notes, "操作审计地理数据暂时无法读取。")
		} else {
			for _, ip := range ips {
				unknown := result.Unknown[dashboardGeoSourceAudit]
				dashboardGeoAdd(points, seen, &unknown, dashboardGeoSourceAudit, ip)
				result.Unknown[dashboardGeoSourceAudit] = unknown
			}
		}
	}

	if allowed[dashboardGeoSourceWebsite] && db != nil {
		var ips []string
		query := db.WithContext(ctx).Model(&SuricataNetworkEvent{}).Where("observed_at >= ? AND observed_at <= ? AND destination_ip <> ''", start, end)
		if !skipFilter {
			if len(userIDs) == 0 {
				query = query.Where("1 = 0")
			} else {
				query = query.Where("user_id IN ?", userIDs)
			}
		}
		if err := query.Distinct("destination_ip").Pluck("destination_ip", &ips).Error; err != nil {
			result.Notes = append(result.Notes, "网站目标地理数据暂时无法读取。")
		} else {
			for _, ip := range ips {
				unknown := result.Unknown[dashboardGeoSourceWebsite]
				dashboardGeoAdd(points, seen, &unknown, dashboardGeoSourceWebsite, ip)
				result.Unknown[dashboardGeoSourceWebsite] = unknown
			}
		}

		domainQuery := db.WithContext(ctx).Model(&WebsiteAccessLog{}).Where("queried_at >= ? AND queried_at <= ?", start, end)
		if !skipFilter {
			if len(userIDs) == 0 {
				domainQuery = domainQuery.Where("1 = 0")
			} else {
				domainQuery = domainQuery.Where("user_id IN ?", userIDs)
			}
		}
		if err := domainQuery.Count(&result.WebsiteDomainOnly).Error; err != nil {
			result.Notes = append(result.Notes, "普通 DNS 审计数量暂时无法读取。")
		} else if result.WebsiteDomainOnly > 0 {
			result.Notes = append(result.Notes, "普通 DNS 审计只记录域名，没有目标 IP；网站地图仅使用 Suricata 记录的目标 IP，不能代表用户所在地。")
		}
	}

	if len(result.AvailableSources) == 0 {
		result.Notes = append(result.Notes, "当前账号没有可用于地图展示的明细权限。")
	}
	for _, point := range points {
		result.Points = append(result.Points, point)
	}
	sort.Slice(result.Points, func(i, j int) bool {
		if result.Points[i].Source != result.Points[j].Source {
			return result.Points[i].Source < result.Points[j].Source
		}
		if result.Points[i].Count != result.Points[j].Count {
			return result.Points[i].Count > result.Points[j].Count
		}
		return result.Points[i].Label < result.Points[j].Label
	})
	return result
}

func dashboardGeoSourceValid(source string) bool {
	return source == "" || source == "all" || source == dashboardGeoSourceOnline || source == dashboardGeoSourceAudit || source == dashboardGeoSourceWebsite
}

func (ov *ovpn) dashboardGeo(c *gin.Context) {
	requested := strings.TrimSpace(strings.ToLower(c.Query("source")))
	if !dashboardGeoSourceValid(requested) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "source 参数无效"})
		return
	}
	start, err := parseDashboardGeoUnix(strings.TrimSpace(c.Query("start")))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "start 参数格式无效"})
		return
	}
	end, err := parseDashboardGeoEndUnix(strings.TrimSpace(c.Query("end")))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "end 参数格式无效"})
		return
	}
	c.JSON(http.StatusOK, ov.dashboardGeoData(c.Request.Context(), start, end, requested, c))
}

func parseDashboardGeoUnix(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	if parsed, err := time.ParseInLocation("2006-01-02", value, time.Local); err == nil {
		return parsed.Unix(), nil
	}
	return strconv.ParseInt(value, 10, 64)
}

func parseDashboardGeoEndUnix(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	if parsed, err := time.ParseInLocation("2006-01-02", value, time.Local); err == nil {
		return parsed.AddDate(0, 0, 1).Add(-time.Second).Unix(), nil
	}
	return strconv.ParseInt(value, 10, 64)
}
