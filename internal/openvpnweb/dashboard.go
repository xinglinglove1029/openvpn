package openvpnweb

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gavintan/gopkg/tools"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

type DashboardSummary struct {
	Stats    DashboardStats        `json:"stats"`
	Trends   []DashboardTrendPoint `json:"trends"`
	TopUsers []DashboardTopUser    `json:"topUsers"`
	Risks    []DashboardRisk       `json:"risks"`
}

type DashboardStats struct {
	OnlineClients    int    `json:"onlineClients"`
	ClientConfigs    int    `json:"clientConfigs"`
	TotalUsers       int64  `json:"totalUsers"`
	EnabledUsers     int64  `json:"enabledUsers"`
	ExpiredUsers     int64  `json:"expiredUsers"`
	ExpiringUsers    int64  `json:"expiringUsers"`
	FirewallRules    int64  `json:"firewallRules"`
	TodayConnections int64  `json:"todayConnections"`
	BytesReceived24h string `json:"bytesReceived24h"`
	BytesSent24h     string `json:"bytesSent24h"`
	ServerStatus     string `json:"serverStatus"`
	ManagementOK     bool   `json:"managementOk"`
}

type DashboardTrendPoint struct {
	Hour        string  `json:"hour"`
	Connections int64   `json:"connections"`
	Received    float64 `json:"received"`
	Sent        float64 `json:"sent"`
}

type DashboardTopUser struct {
	Username string  `json:"username"`
	Bytes    float64 `json:"bytes"`
	Text     string  `json:"text"`
}

type DashboardRisk struct {
	Level   string `json:"level"`
	Title   string `json:"title"`
	Message string `json:"message"`
}

// buildDashboardSummary 构造一份完整的概览数据快照。
// 由 HTTP handler 和 WebSocket 周期采集器共同复用，避免逻辑分叉。
func (ov *ovpn) buildDashboardSummary() DashboardSummary {
	now := time.Now()
	since24h := now.Add(-24 * time.Hour).Unix()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()

	clients, managementOK := ov.safeOnlineClients()
	server, serverOK := ov.safeServerData()
	managementOK = managementOK && serverOK

	stats := DashboardStats{
		OnlineClients: len(clients),
		ClientConfigs: countClientConfigs(),
		ServerStatus:  strings.TrimSpace(server.Status),
		ManagementOK:  managementOK,
	}
	if stats.ServerStatus == "" {
		stats.ServerStatus = "UNKNOWN"
	}

	risks := make([]DashboardRisk, 0)
	if !managementOK {
		risks = append(risks, DashboardRisk{Level: "danger", Title: "OpenVPN Management 不可用", Message: "无法连接 OpenVPN management 端口，请检查服务状态和 management 配置。"})
	}
	if viper.GetBool("system.notify.enabled") && strings.TrimSpace(viper.GetString("system.notify.webhook")) == "" {
		risks = append(risks, DashboardRisk{Level: "warning", Title: "通知 Webhook 未配置", Message: "已启用上线/下线通知，但没有填写机器人 Webhook，告警不会真正送达。"})
	}

	if err := db.WithContext(context.Background()).Model(&User{}).Count(&stats.TotalUsers).Error; err != nil {
		risks = append(risks, DashboardRisk{Level: "warning", Title: "账号统计异常", Message: err.Error()})
	}
	if err := db.WithContext(context.Background()).Model(&User{}).Where("is_enable = ?", true).Count(&stats.EnabledUsers).Error; err != nil {
		risks = append(risks, DashboardRisk{Level: "warning", Title: "启用账号统计异常", Message: err.Error()})
	}
	if err := db.WithContext(context.Background()).Model(&User{}).Where("expire_date <> '' AND expire_date < ?", now.Format("2006-01-02")).Count(&stats.ExpiredUsers).Error; err != nil {
		risks = append(risks, DashboardRisk{Level: "warning", Title: "过期账号统计异常", Message: err.Error()})
	}
	if err := db.WithContext(context.Background()).Model(&User{}).Where("expire_date <> '' AND expire_date >= ? AND expire_date <= ?", now.Format("2006-01-02"), now.AddDate(0, 0, 7).Format("2006-01-02")).Count(&stats.ExpiringUsers).Error; err != nil {
		risks = append(risks, DashboardRisk{Level: "warning", Title: "即将过期账号统计异常", Message: err.Error()})
	}
	if err := db.WithContext(context.Background()).Model(&Firewall{}).Count(&stats.FirewallRules).Error; err != nil {
		risks = append(risks, DashboardRisk{Level: "warning", Title: "防火墙规则统计异常", Message: err.Error()})
	}
	if err := db.WithContext(context.Background()).Model(&History{}).Where("time_unix >= ?", todayStart).Count(&stats.TodayConnections).Error; err != nil {
		risks = append(risks, DashboardRisk{Level: "warning", Title: "今日上线统计异常", Message: err.Error()})
	}

	var traffic struct {
		Received float64
		Sent     float64
	}
	if err := db.WithContext(context.Background()).Model(&History{}).Select("COALESCE(SUM(bytes_received), 0) as received, COALESCE(SUM(bytes_sent), 0) as sent").Where("time_unix >= ?", since24h).Scan(&traffic).Error; err != nil {
		risks = append(risks, DashboardRisk{Level: "warning", Title: "历史流量统计异常", Message: err.Error()})
	}
	stats.BytesReceived24h = tools.FormatBytes(traffic.Received)
	stats.BytesSent24h = tools.FormatBytes(traffic.Sent)

	return DashboardSummary{
		Stats:    stats,
		Trends:   dashboardTrends(since24h),
		TopUsers: dashboardTopUsers(since24h),
		Risks:    risks,
	}
}

func (ov *ovpn) dashboardSummary(c *gin.Context) {
	c.JSON(http.StatusOK, ov.buildDashboardSummary())
}

func (ov *ovpn) safeOnlineClients() ([]ClientData, bool) {
	clients := ov.getClient()
	if len(clients) > 0 {
		return clients, true
	}
	if _, err := ov.sendCommand("status 3"); err != nil {
		return clients, false
	}
	return clients, true
}

func (ov *ovpn) safeServerData() (ServerData, bool) {
	server := ov.getServer()
	return server, strings.TrimSpace(server.Status) != ""
}

func countClientConfigs() int {
	files, err := os.ReadDir(filepath.Join(ovData, "clients"))
	if err != nil {
		return 0
	}

	count := 0
	for _, file := range files {
		if !file.IsDir() && strings.EqualFold(filepath.Ext(file.Name()), ".ovpn") {
			count++
		}
	}
	return count
}

func dashboardTrends(since int64) []DashboardTrendPoint {
	points := make([]DashboardTrendPoint, 24)
	now := time.Now().Truncate(time.Hour)
	indexByHour := map[string]int{}
	for i := 23; i >= 0; i-- {
		hour := now.Add(-time.Duration(i) * time.Hour)
		key := hour.Format("2006010215")
		points[23-i] = DashboardTrendPoint{Hour: hour.Format("15:00")}
		indexByHour[key] = 23 - i
	}

	var rows []struct {
		Hour        string
		Connections int64
		Received    float64
		Sent        float64
	}
	db.WithContext(context.Background()).Model(&History{}).
		Select("strftime('%Y%m%d%H', datetime(time_unix, 'unixepoch', 'localtime')) as hour, COUNT(*) as connections, COALESCE(SUM(bytes_received), 0) as received, COALESCE(SUM(bytes_sent), 0) as sent").
		Where("time_unix >= ?", since).
		Group("hour").
		Scan(&rows)

	for _, row := range rows {
		if index, ok := indexByHour[row.Hour]; ok {
			points[index].Connections = row.Connections
			points[index].Received = row.Received
			points[index].Sent = row.Sent
		}
	}

	return points
}

func dashboardTopUsers(since int64) []DashboardTopUser {
	var rows []struct {
		Username string
		Bytes    float64
	}
	db.WithContext(context.Background()).Model(&History{}).
		Select("COALESCE(NULLIF(username, ''), common_name, 'unknown') as username, COALESCE(SUM(bytes_received + bytes_sent), 0) as bytes").
		Where("time_unix >= ?", since).
		Group("username").
		Order("bytes DESC").
		Limit(5).
		Scan(&rows)

	topUsers := make([]DashboardTopUser, 0, len(rows))
	for _, row := range rows {
		username := strings.TrimSpace(row.Username)
		if username == "" {
			username = "unknown"
		}
		topUsers = append(topUsers, DashboardTopUser{Username: username, Bytes: row.Bytes, Text: tools.FormatBytes(row.Bytes)})
	}
	sort.SliceStable(topUsers, func(i, j int) bool { return topUsers[i].Bytes > topUsers[j].Bytes })
	return topUsers
}
