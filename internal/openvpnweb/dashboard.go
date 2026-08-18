package openvpnweb

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

type DashboardSummary struct {
	Stats DashboardStats  `json:"stats"`
	Risks []DashboardRisk `json:"risks"`
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
	ServerStatus     string `json:"serverStatus"`
	ManagementOK     bool   `json:"managementOk"`
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

	return DashboardSummary{
		Stats: stats,
		Risks: risks,
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
