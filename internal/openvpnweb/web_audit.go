package openvpnweb

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// WebsiteAccessLog is a privacy-preserving DNS audit event. It deliberately
// stores only a queried DNS name and metadata: never URL paths, payloads,
// cookies, credentials, or DNS response bodies.
type WebsiteAccessLog struct {
	ID           uint      `gorm:"primarykey" json:"id"`
	UserID       uint      `gorm:"index;default:0" json:"userId"`
	Username     string    `gorm:"index" json:"username"`
	CommonName   string    `gorm:"index" json:"commonName"`
	ConnectionID string    `gorm:"index" json:"connectionId"`
	VPNIP        string    `gorm:"index" json:"vpnIp"`
	Domain       string    `gorm:"index" json:"domain"`
	QueryType    string    `json:"queryType"`
	ResponseCode string    `json:"responseCode"`
	QueriedAt    int64     `gorm:"index" json:"queriedAt"`
	CreatedAt    time.Time `json:"createdAt"`
}

func (WebsiteAccessLog) TableName() string { return "website_access_logs" }

// SuricataNetworkEvent is a privacy-preserving EVE event. It deliberately has
// no raw JSON, HTTP body, response body, cookie, authorization, or payload field.
type SuricataNetworkEvent struct {
	ID              uint      `gorm:"primarykey" json:"id"`
	UserID          uint      `gorm:"index;default:0" json:"userId"`
	Username        string    `gorm:"index" json:"username"`
	CommonName      string    `gorm:"index" json:"commonName"`
	ConnectionID    string    `gorm:"index" json:"connectionId"`
	VPNIP           string    `gorm:"index" json:"vpnIp"`
	EventType       string    `gorm:"index" json:"eventType"`
	DestinationIP   string    `gorm:"index" json:"destinationIp"`
	Protocol        string    `json:"protocol"`
	AppProtocol     string    `json:"appProtocol"`
	SourcePort      uint16    `json:"sourcePort"`
	DestinationPort uint16    `json:"destinationPort"`
	BytesToServer   uint64    `json:"bytesToServer"`
	BytesToClient   uint64    `json:"bytesToClient"`
	PacketsToServer uint64    `json:"packetsToServer"`
	PacketsToClient uint64    `json:"packetsToClient"`
	DNSName         string    `gorm:"index" json:"dnsName,omitempty"`
	DNSRecordType   string    `json:"dnsRecordType,omitempty"`
	TLSSNI          string    `gorm:"index" json:"tlsSni,omitempty"`
	TLSVersion      string    `json:"tlsVersion,omitempty"`
	HTTPHostname    string    `gorm:"index" json:"httpHostname,omitempty"`
	HTTPURL         string    `json:"httpUrl,omitempty"`
	HTTPMethod      string    `json:"httpMethod,omitempty"`
	AlertSignature  string    `json:"alertSignature,omitempty"`
	AlertCategory   string    `json:"alertCategory,omitempty"`
	AlertSeverity   int       `json:"alertSeverity,omitempty"`
	ObservedAt      int64     `gorm:"index" json:"observedAt"`
	CreatedAt       time.Time `json:"createdAt"`
}

func (SuricataNetworkEvent) TableName() string { return "suricata_network_events" }

func (SuricataNetworkEvent) Clear() error {
	days := suricataEVEMaxDays()
	if days <= 0 {
		return nil
	}
	return db.Where("observed_at < ?", time.Now().AddDate(0, 0, -days).Unix()).Delete(&SuricataNetworkEvent{}).Error
}

type SuricataNetworkAuditFilter struct {
	Start     int64
	End       int64
	Username  string
	EventType string
}

type SuricataNetworkAuditRecordsResponse struct {
	Start int64                  `json:"start"`
	End   int64                  `json:"end"`
	Total int64                  `json:"total"`
	Data  []SuricataNetworkEvent `json:"data"`
}

func normalizeSuricataNetworkAuditFilter(filter SuricataNetworkAuditFilter) SuricataNetworkAuditFilter {
	filter.Start, filter.End = normalizeWebsiteAuditRange(filter.Start, filter.End)
	filter.Username = strings.TrimSpace(filter.Username)
	filter.EventType = strings.ToLower(strings.TrimSpace(filter.EventType))
	return filter
}

func suricataNetworkAuditQuery(ctx context.Context, filter SuricataNetworkAuditFilter, accessibleUserIDs []uint, skipFilter bool) *gorm.DB {
	filter = normalizeSuricataNetworkAuditFilter(filter)
	q := db.WithContext(ctx).Model(&SuricataNetworkEvent{}).Where("observed_at >= ? AND observed_at <= ?", filter.Start, filter.End)
	if !skipFilter {
		q = q.Where("user_id IN ?", accessibleUserIDs)
	}
	if filter.Username != "" {
		q = q.Where("LOWER(username) LIKE ? ESCAPE '\\'", "%"+escapeWebsiteAuditLike(strings.ToLower(filter.Username))+"%")
	}
	if filter.EventType != "" {
		q = q.Where("event_type = ?", filter.EventType)
	}
	return q
}

func querySuricataNetworkAuditRecords(ctx context.Context, filter SuricataNetworkAuditFilter, accessibleUserIDs []uint, skipFilter bool, offset, limit int) (SuricataNetworkAuditRecordsResponse, error) {
	filter = normalizeSuricataNetworkAuditFilter(filter)
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	result := SuricataNetworkAuditRecordsResponse{Start: filter.Start, End: filter.End, Data: make([]SuricataNetworkEvent, 0)}
	q := func() *gorm.DB { return suricataNetworkAuditQuery(ctx, filter, accessibleUserIDs, skipFilter) }
	if err := q().Count(&result.Total).Error; err != nil {
		return result, err
	}
	err := q().Order("observed_at DESC, id DESC").Offset(offset).Limit(limit).Find(&result.Data).Error
	return result, err
}

func parseSuricataNetworkAuditFilter(c *gin.Context) SuricataNetworkAuditFilter {
	start, _ := strconv.ParseInt(c.Query("start"), 10, 64)
	end, _ := strconv.ParseInt(c.Query("end"), 10, 64)
	return normalizeSuricataNetworkAuditFilter(SuricataNetworkAuditFilter{Start: start, End: end, Username: c.Query("username"), EventType: c.Query("eventType")})
}

func (ov *ovpn) suricataNetworkAuditRecords(c *gin.Context) {
	ids, skip := webAuditAccessScope(c)
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	result, err := querySuricataNetworkAuditRecords(c.Request.Context(), parseSuricataNetworkAuditFilter(c), ids, skip, offset, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "查询网络审计明细失败"})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (ov *ovpn) suricataNetworkAuditExport(c *gin.Context) {
	ids, skip := webAuditAccessScope(c)
	q := suricataNetworkAuditQuery(c.Request.Context(), parseSuricataNetworkAuditFilter(c), ids, skip)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "统计网络审计数据失败"})
		return
	}
	if total > websiteAuditMaxExportRows {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"message": fmt.Sprintf("导出结果超过 %d 条，请缩小时间范围或筛选条件", websiteAuditMaxExportRows)})
		return
	}
	rows, err := q.Order("observed_at DESC, id DESC").Rows()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "导出网络审计明细失败"})
		return
	}
	defer rows.Close()
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=suricata_network_%s.csv", time.Now().Format("20060102150405")))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	_, _ = c.Writer.Write([]byte("\xEF\xBB\xBF"))
	writer := csv.NewWriter(c.Writer)
	defer writer.Flush()
	if err := writer.Write([]string{"时间", "用户", "VPN IP", "事件", "目的地址", "协议", "目的端口", "DNS", "TLS SNI", "HTTP 主机", "HTTP URL", "方法", "告警签名", "告警类别", "严重度"}); err != nil {
		return
	}
	for rows.Next() {
		var entry SuricataNetworkEvent
		if err := db.ScanRows(rows, &entry); err != nil {
			return
		}
		if err := writer.Write([]string{time.Unix(entry.ObservedAt, 0).Format("2006-01-02 15:04:05"), csvSafeWebsiteAuditField(entry.Username), csvSafeWebsiteAuditField(entry.VPNIP), csvSafeWebsiteAuditField(entry.EventType), csvSafeWebsiteAuditField(entry.DestinationIP), csvSafeWebsiteAuditField(entry.Protocol), strconv.Itoa(int(entry.DestinationPort)), csvSafeWebsiteAuditField(entry.DNSName), csvSafeWebsiteAuditField(entry.TLSSNI), csvSafeWebsiteAuditField(entry.HTTPHostname), csvSafeWebsiteAuditField(entry.HTTPURL), csvSafeWebsiteAuditField(entry.HTTPMethod), csvSafeWebsiteAuditField(entry.AlertSignature), csvSafeWebsiteAuditField(entry.AlertCategory), strconv.Itoa(entry.AlertSeverity)}); err != nil {
			return
		}
	}
}

func (ov *ovpn) suricataEVEStatus(c *gin.Context) { c.JSON(http.StatusOK, getSuricataEVEStatus()) }

func (WebsiteAccessLog) Clear() error {
	days := configHistoryMaxDays()
	// Existing historical data defaults to 90 days. A value of 0 explicitly
	// disables automatic cleanup rather than unexpectedly deleting all audit data.
	if days <= 0 {
		return nil
	}
	return db.Where("queried_at < ?", time.Now().AddDate(0, 0, -days).Unix()).Delete(&WebsiteAccessLog{}).Error
}

type WebsiteAuditFilter struct {
	Start    int64
	End      int64
	Username string
	Domain   string
}

type WebsiteAuditTopItem struct {
	Username   string `json:"username,omitempty"`
	CommonName string `json:"commonName,omitempty"`
	Domain     string `json:"domain,omitempty"`
	Queries    int64  `json:"queries"`
}

type WebsiteAuditTrendItem struct {
	Time    int64 `json:"time"`
	Queries int64 `json:"queries"`
}

type WebsiteAuditSummary struct {
	Start         int64                   `json:"start"`
	End           int64                   `json:"end"`
	TotalQueries  int64                   `json:"totalQueries"`
	ActiveUsers   int64                   `json:"activeUsers"`
	UniqueDomains int64                   `json:"uniqueDomains"`
	TopUsers      []WebsiteAuditTopItem   `json:"topUsers"`
	TopDomains    []WebsiteAuditTopItem   `json:"topDomains"`
	Trend         []WebsiteAuditTrendItem `json:"trend"`
}

type WebsiteAuditRecordsResponse struct {
	Start        int64              `json:"start"`
	End          int64              `json:"end"`
	Total        int64              `json:"total"`
	Data         []WebsiteAccessLog `json:"data"`
	HistoryID    uint               `json:"historyId,omitempty"`
	ConnectionID string             `json:"connectionId,omitempty"`
	MatchedBy    string             `json:"matchedBy,omitempty"`
}

func normalizeWebsiteAuditRange(start, end int64) (int64, int64) {
	now := time.Now().Unix()
	if end <= 0 || end > now+300 {
		end = now
	}
	if start <= 0 || start >= end {
		start = end - int64((24 * time.Hour).Seconds())
	}
	maxStart := end - int64((90 * 24 * time.Hour).Seconds())
	if start < maxStart {
		start = maxStart
	}
	return start, end
}

func normalizeWebsiteAuditFilter(filter WebsiteAuditFilter) WebsiteAuditFilter {
	filter.Start, filter.End = normalizeWebsiteAuditRange(filter.Start, filter.End)
	filter.Username = strings.TrimSpace(filter.Username)
	filter.Domain = normalizeDNSName(filter.Domain)
	return filter
}

// escapeWebsiteAuditLike turns user input into a literal substring match. Without
// this, a search for % or _ would unexpectedly match unrelated DNS records.
func escapeWebsiteAuditLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	value = strings.ReplaceAll(value, `_`, `\_`)
	return value
}

func websiteAuditQuery(ctx context.Context, filter WebsiteAuditFilter, accessibleUserIDs []uint, skipFilter bool) *gorm.DB {
	filter = normalizeWebsiteAuditFilter(filter)
	q := db.WithContext(ctx).Model(&WebsiteAccessLog{}).Where("queried_at >= ? AND queried_at <= ?", filter.Start, filter.End)
	if !skipFilter {
		q = q.Where("user_id IN ?", accessibleUserIDs)
	}
	if filter.Username != "" {
		q = q.Where("LOWER(username) LIKE ? ESCAPE '\\'", "%"+escapeWebsiteAuditLike(strings.ToLower(filter.Username))+"%")
	}
	if filter.Domain != "" {
		q = q.Where("LOWER(domain) LIKE ? ESCAPE '\\'", "%"+escapeWebsiteAuditLike(strings.ToLower(filter.Domain))+"%")
	}
	return q
}

func buildWebsiteAuditSummary(ctx context.Context, filter WebsiteAuditFilter, accessibleUserIDs []uint, skipFilter bool, topLimit int) (WebsiteAuditSummary, error) {
	filter = normalizeWebsiteAuditFilter(filter)
	if topLimit <= 0 || topLimit > 100 {
		topLimit = 10
	}
	result := WebsiteAuditSummary{Start: filter.Start, End: filter.End, TopUsers: make([]WebsiteAuditTopItem, 0), TopDomains: make([]WebsiteAuditTopItem, 0), Trend: make([]WebsiteAuditTrendItem, 0)}
	query := func() *gorm.DB { return websiteAuditQuery(ctx, filter, accessibleUserIDs, skipFilter) }
	if err := query().Count(&result.TotalQueries).Error; err != nil {
		return result, err
	}
	if err := query().Where("user_id > 0").Distinct("user_id").Count(&result.ActiveUsers).Error; err != nil {
		return result, err
	}
	if err := query().Distinct("domain").Count(&result.UniqueDomains).Error; err != nil {
		return result, err
	}
	// Historical unowned records are diagnostic-only and must never affect any
	// per-user metric. New records are only written after cache ownership lookup.
	if err := query().Where("user_id > 0").Select("username, common_name, COUNT(*) AS queries").Group("username, common_name").Order("queries DESC, username ASC").Limit(topLimit).Scan(&result.TopUsers).Error; err != nil {
		return result, err
	}
	if err := query().Select("domain, COUNT(*) AS queries").Group("domain").Order("queries DESC, domain ASC").Limit(topLimit).Scan(&result.TopDomains).Error; err != nil {
		return result, err
	}

	bucket := int64(time.Hour.Seconds())
	if filter.End-filter.Start > int64((48 * time.Hour).Seconds()) {
		bucket = int64((24 * time.Hour).Seconds())
	}
	expr := websiteAuditBucketExpression(bucket)
	type trendRow struct {
		Time    int64
		Queries int64
	}
	var rows []trendRow
	if err := query().Select(fmt.Sprintf("%s AS time, COUNT(*) AS queries", expr)).Group(expr).Order("time ASC").Scan(&rows).Error; err != nil {
		return result, err
	}
	buckets := make(map[int64]int64, len(rows))
	for _, row := range rows {
		buckets[row.Time] = row.Queries
	}
	for t := (filter.Start / bucket) * bucket; t <= filter.End; t += bucket {
		result.Trend = append(result.Trend, WebsiteAuditTrendItem{Time: t, Queries: buckets[t]})
	}
	return result, nil
}

func websiteAuditBucketExpression(bucket int64) string {
	// bucket is internal and not user-controlled. Keep the expression portable
	// while aggregating in SQL instead of loading every timestamp into memory.
	switch db.Dialector.Name() {
	case "postgres":
		return fmt.Sprintf("(FLOOR(queried_at::numeric / %d)::bigint * %d)", bucket, bucket)
	case "mysql":
		return fmt.Sprintf("(FLOOR(queried_at / %d) * %d)", bucket, bucket)
	default: // sqlite and compatible engines use integer division for integer columns.
		return fmt.Sprintf("((queried_at / %d) * %d)", bucket, bucket)
	}
}

func queryWebsiteAuditRecords(ctx context.Context, filter WebsiteAuditFilter, accessibleUserIDs []uint, skipFilter bool, offset, limit int) (WebsiteAuditRecordsResponse, error) {
	filter = normalizeWebsiteAuditFilter(filter)
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	result := WebsiteAuditRecordsResponse{Start: filter.Start, End: filter.End, Data: make([]WebsiteAccessLog, 0)}
	query := func() *gorm.DB { return websiteAuditQuery(ctx, filter, accessibleUserIDs, skipFilter) }
	if err := query().Count(&result.Total).Error; err != nil {
		return result, err
	}
	err := query().Order("queried_at DESC, id DESC").Offset(offset).Limit(limit).Find(&result.Data).Error
	return result, err
}

// queryHistoryWebsiteAuditRecords associates DNS audit events with one completed
// VPN connection. A connection ID is the primary key for the association. Older
// history rows may not have one, so those rows fall back to the exact user and
// the connection's start/end interval. The caller must already have verified
// that the history row is inside the operator's data scope.
func queryHistoryWebsiteAuditRecords(ctx context.Context, history History, accessibleUserIDs []uint, skipFilter bool, offset, limit int) (WebsiteAuditRecordsResponse, error) {
	start := history.TimeUnix
	end := history.TimeUnix + history.TimeDuration
	if start <= 0 {
		return WebsiteAuditRecordsResponse{HistoryID: history.ID, Data: make([]WebsiteAccessLog, 0)}, nil
	}
	// A zero duration is valid for legacy records. Keep the range narrow rather
	// than expanding it to the default one-day website-audit range.
	if end <= start {
		end = start + 1
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	filter := WebsiteAuditFilter{Start: start, End: end}
	query := websiteAuditQuery(ctx, filter, accessibleUserIDs, skipFilter)
	connectionID := strings.TrimSpace(history.ConnectionID)
	matchedBy := "time_range"
	if connectionID != "" {
		query = query.Where("connection_id = ?", connectionID)
		// Connection IDs should be unique, but keep the user identity as a
		// second boundary so a stale/reused ID can never cross users.
		if history.UserID > 0 {
			query = query.Where("user_id = ?", history.UserID)
		} else if strings.TrimSpace(history.Username) != "" {
			query = query.Where("username = ?", strings.TrimSpace(history.Username))
		}
		matchedBy = "connection_id"
	} else if history.UserID > 0 {
		query = query.Where("user_id = ?", history.UserID)
	} else if strings.TrimSpace(history.Username) != "" {
		query = query.Where("username = ?", strings.TrimSpace(history.Username))
	} else {
		// Never return all audit rows for a legacy record without an identity.
		query = query.Where("1 = 0")
	}

	result := WebsiteAuditRecordsResponse{
		Start:        start,
		End:          end,
		HistoryID:    history.ID,
		ConnectionID: connectionID,
		MatchedBy:    matchedBy,
		Data:         make([]WebsiteAccessLog, 0),
	}
	if err := query.Count(&result.Total).Error; err != nil {
		return result, err
	}
	err := query.Order("queried_at DESC, id DESC").Offset(offset).Limit(limit).Find(&result.Data).Error
	return result, err
}

func webAuditAccessScope(c *gin.Context) ([]uint, bool) {
	isAdmin, _ := c.Get("isAdmin")
	if isAdmin == true {
		return nil, true
	}
	username, _ := c.Get("user")
	current, _ := username.(string)
	if current == "" {
		return []uint{}, false
	}
	return GetAccessibleUserIDs(current)
}

func parseWebsiteAuditFilter(c *gin.Context) WebsiteAuditFilter {
	start, _ := strconv.ParseInt(c.Query("start"), 10, 64)
	end, _ := strconv.ParseInt(c.Query("end"), 10, 64)
	return normalizeWebsiteAuditFilter(WebsiteAuditFilter{Start: start, End: end, Username: c.Query("username"), Domain: c.Query("domain")})
}

func (ov *ovpn) websiteAuditSummary(c *gin.Context) {
	filter := parseWebsiteAuditFilter(c)
	ids, skip := webAuditAccessScope(c)
	result, err := buildWebsiteAuditSummary(c.Request.Context(), filter, ids, skip, 10)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "查询网站访问统计失败"})
		return
	}
	c.JSON(http.StatusOK, result)
}
func (ov *ovpn) websiteAuditRecords(c *gin.Context) {
	filter := parseWebsiteAuditFilter(c)
	ids, skip := webAuditAccessScope(c)
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	result, err := queryWebsiteAuditRecords(c.Request.Context(), filter, ids, skip, offset, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "查询网站访问明细失败"})
		return
	}
	c.JSON(http.StatusOK, result)
}

// historyWebsiteAudit returns DNS names observed while the selected VPN
// connection was active. The route carries history:view at the router level;
// web-audit:view is checked here so users with connection-history permission
// alone do not gain access to browsing metadata.
func (ov *ovpn) historyWebsiteAudit(c *gin.Context) {
	if !requirePermissionCode(c, "web-audit:view") {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的连接历史 ID"})
		return
	}

	var history History
	if err := db.WithContext(c.Request.Context()).First(&history, uint(id)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"message": "连接历史不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": "查询连接历史失败"})
		return
	}

	accessibleUserIDs, skipFilter := webAuditAccessScope(c)
	if !skipFilter {
		visible := false
		for _, userID := range accessibleUserIDs {
			if userID != 0 && userID == history.UserID {
				visible = true
				break
			}
		}
		if !visible {
			// Do not reveal whether another user's history ID exists.
			c.JSON(http.StatusNotFound, gin.H{"message": "连接历史不存在"})
			return
		}
	}

	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	result, err := queryHistoryWebsiteAuditRecords(c.Request.Context(), history, accessibleUserIDs, skipFilter, offset, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "查询连接期间的网站访问记录失败"})
		return
	}
	c.JSON(http.StatusOK, result)
}
func csvSafeWebsiteAuditField(value string) string {
	// Excel and similar spreadsheet programs ignore leading whitespace/control
	// characters before evaluating formulas. Preserve the display text but prefix
	// it when the first meaningful character is a formula operator.
	for _, r := range value {
		switch r {
		case ' ', '\t', '\r', '\n':
			continue
		case '=', '+', '-', '@':
			return "'" + value
		default:
			return value
		}
	}
	return value
}

const websiteAuditMaxExportRows = 100000

var websiteAuditExportSem = make(chan struct{}, 2)

func (ov *ovpn) websiteAuditExport(c *gin.Context) {
	// Export can run a long streaming query. Limit concurrent work and the total
	// rows so a broad request cannot exhaust the database for VPN control paths.
	select {
	case websiteAuditExportSem <- struct{}{}:
		defer func() { <-websiteAuditExportSem }()
	default:
		c.JSON(http.StatusTooManyRequests, gin.H{"message": "导出任务较多，请稍后重试"})
		return
	}
	filter := parseWebsiteAuditFilter(c)
	ids, skip := webAuditAccessScope(c)
	query := websiteAuditQuery(c.Request.Context(), filter, ids, skip)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "统计导出数据失败"})
		return
	}
	if total > websiteAuditMaxExportRows {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"message": fmt.Sprintf("导出结果超过 %d 条，请缩小时间范围或筛选条件", websiteAuditMaxExportRows)})
		return
	}
	rows, err := query.Order("queried_at DESC, id DESC").Rows()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "导出网站访问明细失败"})
		return
	}
	defer rows.Close()
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=website_access_%s.csv", time.Now().Format("20060102150405")))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	_, _ = c.Writer.Write([]byte("\xEF\xBB\xBF"))
	writer := csv.NewWriter(c.Writer)
	defer writer.Flush()
	if err := writer.Write([]string{"查询时间", "用户", "证书名称", "VPN IP", "域名", "类型", "响应状态"}); err != nil {
		return
	}
	for rows.Next() {
		var record WebsiteAccessLog
		if err := db.ScanRows(rows, &record); err != nil {
			return
		}
		if err := writer.Write([]string{
			csvSafeWebsiteAuditField(time.Unix(record.QueriedAt, 0).Format("2006-01-02 15:04:05")),
			csvSafeWebsiteAuditField(record.Username), csvSafeWebsiteAuditField(record.CommonName), csvSafeWebsiteAuditField(record.VPNIP),
			csvSafeWebsiteAuditField(record.Domain), csvSafeWebsiteAuditField(record.QueryType), csvSafeWebsiteAuditField(record.ResponseCode),
		}); err != nil {
			return
		}
	}
}
func (ov *ovpn) websiteAuditStatus(c *gin.Context) { c.JSON(http.StatusOK, getWebAuditDNSStatus()) }

// websiteAuditClientMap accepts only the local, token-authenticated OpenVPN hook
// admitted by AuthMiddleWare. It updates transient memory only; DNS audit records
// remain append-only and are never created by this endpoint.
func (ov *ovpn) websiteAuditClientMap(c *gin.Context) {
	internal, _ := c.Get(internalWebAuditClientMapContextKey)
	if internal != true || !IsLocalRequest(c) || !hasMatchingLocalServiceToken(c) {
		c.JSON(http.StatusForbidden, gin.H{"message": "仅允许本机 OpenVPN 生命周期钩子调用"})
		return
	}
	action := strings.ToLower(strings.TrimSpace(c.PostForm("action")))
	if action != "upsert" && action != "delete" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "action 必须为 upsert 或 delete"})
		return
	}
	username := strings.TrimSpace(c.PostForm("username"))
	identity := auditClientIdentity{Username: username, CommonName: strings.TrimSpace(c.PostForm("common_name")), ConnectionID: strings.TrimSpace(c.PostForm("connection_id"))}
	if updatedAt, err := strconv.ParseInt(strings.TrimSpace(c.PostForm("event_time_ns")), 10, 64); err == nil && updatedAt > 0 {
		identity.UpdatedAt = updatedAt
	}
	if action == "upsert" {
		if username == "" {
			c.JSON(http.StatusBadRequest, gin.H{"message": "upsert 缺少用户名"})
			return
		}
		var user User
		if err := db.Select("id", "username").Where("username = ?", username).First(&user).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				c.JSON(http.StatusNotFound, gin.H{"message": "用户不存在，不更新 DNS 审计映射"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"message": "查询 DNS 审计用户失败"})
			return
		}
		identity.UserID = user.ID
	}
	webAuditDNS.updateClientIdentity(action, identity, c.PostForm("vip"), c.PostForm("vip6"))
	c.Status(http.StatusNoContent)
}
