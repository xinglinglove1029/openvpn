package openvpnweb

import (
	"context"
	"log"
	"time"
)

// DashboardStatsPayload 概览页 WebSocket 推送载荷
// 一次性包含概览页所有卡片所需数据，前端订阅 dashboard:stats 即可全量更新
type DashboardStatsPayload struct {
	Summary  DashboardSummary `json:"summary"`
	Online   []ClientData     `json:"online"`
	Server   ServerData       `json:"server"`
	PushedAt int64            `json:"pushedAt"` // 推送时间戳（秒），前端可据此检测数据新鲜度
}

const dashboardStatsTopic = "dashboard:stats"

// dashboardStatsCollector 周期采集概览数据并通过 EventBus 广播 dashboard:stats。
// 设计要点：
//   - 单 goroutine 采集，所有已连接的概览页共享同一份数据；
//   - 采集间隔 5s，与 system:stats 保持一致，避免推送给前端造成抖动；
//   - 启动时立即采集一次，让前端首屏就有数据；
//   - 失败时仅记日志，绝不因采集异常影响主流程。
type dashboardStatsCollector struct {
	ov       *ovpn
	interval time.Duration
}

// StartDashboardStatsCollector 启动概览数据采集器（应用启动时调用一次）。
// ov 为 OpenVPN 管理实例，用于查询在线客户端和服务状态。
func StartDashboardStatsCollector(ov *ovpn, interval time.Duration) {
	if ov == nil {
		log.Println("[dashboard-stats] ov instance is nil, collector disabled")
		return
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}
	c := &dashboardStatsCollector{
		ov:       ov,
		interval: interval,
	}

	// 预热：立即推送一次，首屏即有数据
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if payload, err := c.collect(ctx); err == nil {
			Bus().Publish(dashboardStatsTopic, payload)
		} else {
			log.Printf("[dashboard-stats] initial collect failed: %v", err)
		}
	}()

	go c.loop()
}

func (c *dashboardStatsCollector) loop() {
	t := time.NewTicker(c.interval)
	defer t.Stop()
	for range t.C {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		payload, err := c.collect(ctx)
		cancel()
		if err != nil {
			log.Printf("[dashboard-stats] collect failed: %v", err)
			continue
		}
		Bus().Publish(dashboardStatsTopic, payload)
	}
}

// collect 采集一次概览数据快照
// 复用 buildDashboardSummary 逻辑，额外补充在线客户端明细和服务运行时信息
func (c *dashboardStatsCollector) collect(ctx context.Context) (DashboardStatsPayload, error) {
	summary := c.ov.buildDashboardSummary()

	// 在线客户端明细（与 /ovpn/online-client API 保持一致）
	online, _ := c.ov.safeOnlineClients()
	server, _ := c.ov.safeServerData()

	return DashboardStatsPayload{
		Summary:  summary,
		Online:   online,
		Server:   server,
		PushedAt: time.Now().Unix(),
	}, nil
}
