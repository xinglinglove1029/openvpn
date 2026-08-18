package openvpnweb

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gavintan/gopkg/tools"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

// ClientTrafficSample records the positive traffic delta observed for an
// OpenVPN client connection. Cumulative counters come from management status
// and deltas make arbitrary time-range aggregation possible.
type ClientTrafficSample struct {
	ID                 uint      `gorm:"primarykey" json:"id"`
	ConnectionID       string    `gorm:"index" json:"connectionId"`
	ConnectionKey      string    `gorm:"index" json:"connectionKey"`
	UserID             uint      `gorm:"index;default:0" json:"userId"`
	Username           string    `gorm:"index" json:"username"`
	CommonName         string    `gorm:"index" json:"commonName"`
	SampleTime         int64     `gorm:"index" json:"sampleTime"`
	CumulativeReceived float64   `json:"cumulativeReceived"`
	CumulativeSent     float64   `json:"cumulativeSent"`
	ReceivedDelta      float64   `json:"receivedDelta"`
	SentDelta          float64   `json:"sentDelta"`
	CreatedAt          time.Time `json:"createdAt,omitempty"`
}

func (ClientTrafficSample) TableName() string { return "client_traffic_samples" }

func (ClientTrafficSample) Clear() error {
	retentionDays := 90
	if n := configHistoryMaxDays(); n > 0 {
		retentionDays = n
	}
	return db.Where("sample_time < ?", time.Now().AddDate(0, 0, -retentionDays).Unix()).Delete(&ClientTrafficSample{}).Error
}

func configHistoryMaxDays() int {
	// Keep this helper local so traffic sampling follows the existing history
	// retention setting without introducing another configuration key.
	return viper.GetInt("system.base.history_max_days")
}

// trafficUserStat is the API shape used by the redesigned overview widget.
type DashboardTrafficUser struct {
	Username      string  `json:"username"`
	CommonName    string  `json:"commonName"`
	Online        bool    `json:"online"`
	Connections   int64   `json:"connections"`
	OnlineSeconds int64   `json:"onlineSeconds"`
	Received      float64 `json:"received"`
	Sent          float64 `json:"sent"`
	Total         float64 `json:"total"`
	ReceivedText  string  `json:"receivedText"`
	SentText      string  `json:"sentText"`
	TotalText     string  `json:"totalText"`
	LastSeen      int64   `json:"lastSeen"`
}

type DashboardTrafficTotals struct {
	ActiveUsers  int64   `json:"activeUsers"`
	Received     float64 `json:"received"`
	Sent         float64 `json:"sent"`
	Total        float64 `json:"total"`
	ReceivedText string  `json:"receivedText"`
	SentText     string  `json:"sentText"`
	TotalText    string  `json:"totalText"`
}

type DashboardTrafficUsersResponse struct {
	Start         int64                  `json:"start"`
	End           int64                  `json:"end"`
	SampleSeconds int64                  `json:"sampleSeconds"`
	Users         []DashboardTrafficUser `json:"users"`
	Totals        DashboardTrafficTotals `json:"totals"`
}

type trafficUserAccumulator struct {
	Username      string
	CommonName    string
	Online        bool
	Connections   int64
	OnlineSeconds int64
	Received      float64
	Sent          float64
	LastSeen      int64
	connections   map[string]struct{}
}

func (a *trafficUserAccumulator) addConnection(key string) {
	if a.connections == nil {
		a.connections = make(map[string]struct{})
	}
	if key == "" {
		a.Connections++
		return
	}
	if _, exists := a.connections[key]; !exists {
		a.connections[key] = struct{}{}
		a.Connections++
	}
}

func trafficUsername(username, commonName string) string {
	username = strings.TrimSpace(username)
	if username != "" && !strings.EqualFold(username, "UNDEF") {
		return username
	}
	commonName = strings.TrimSpace(commonName)
	if commonName != "" {
		return commonName
	}
	return "未知用户"
}

func (a *trafficUserAccumulator) result() DashboardTrafficUser {
	username := trafficUsername(a.Username, a.CommonName)
	return DashboardTrafficUser{
		Username:      username,
		CommonName:    a.CommonName,
		Online:        a.Online,
		Connections:   a.Connections,
		OnlineSeconds: a.OnlineSeconds,
		Received:      a.Received,
		Sent:          a.Sent,
		Total:         a.Received + a.Sent,
		ReceivedText:  tools.FormatBytes(a.Received),
		SentText:      tools.FormatBytes(a.Sent),
		TotalText:     tools.FormatBytes(a.Received + a.Sent),
		LastSeen:      a.LastSeen,
	}
}

func addTrafficAccumulator(users map[string]*trafficUserAccumulator, username, commonName string) *trafficUserAccumulator {
	key := trafficUsername(username, commonName)
	item := users[key]
	if item == nil {
		item = &trafficUserAccumulator{Username: key, CommonName: strings.TrimSpace(commonName)}
		users[key] = item
	}
	if item.CommonName == "" {
		item.CommonName = strings.TrimSpace(commonName)
	}
	return item
}

func normalizeTrafficRange(start, end int64) (int64, int64) {
	const daySeconds int64 = 24 * 60 * 60
	const maxRangeSeconds int64 = 90 * daySeconds
	now := time.Now().Unix()
	if end <= 0 || end > now+300 {
		end = now
	}
	if start <= 0 {
		start = end - daySeconds
	}
	if start >= end {
		start = end - daySeconds
	}
	if end-start > maxRangeSeconds {
		start = end - maxRangeSeconds
	}
	return start, end
}

func (ov *ovpn) dashboardTrafficUsers(c *gin.Context) {
	start, _ := strconv.ParseInt(c.Query("start"), 10, 64)
	end, _ := strconv.ParseInt(c.Query("end"), 10, 64)
	start, end = normalizeTrafficRange(start, end)

	users := make(map[string]*trafficUserAccumulator)
	ctx := context.Background()

	// Samples are the primary source for time-range traffic. They contain
	// minute-level deltas and therefore do not misattribute an entire session
	// to the hour in which it disconnected.
	var samples []ClientTrafficSample
	if err := db.WithContext(ctx).
		Where("sample_time BETWEEN ? AND ?", start, end).
		Find(&samples).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "查询流量采样失败: " + err.Error()})
		return
	}

	sampledConnections := make(map[string]struct{})
	for _, sample := range samples {
		item := addTrafficAccumulator(users, sample.Username, sample.CommonName)
		item.Received += maxPositive(sample.ReceivedDelta)
		item.Sent += maxPositive(sample.SentDelta)
		if sample.ConnectionID != "" {
			sampledConnections[sample.ConnectionID] = struct{}{}
		}
		item.addConnection(sample.ConnectionID)
		if sample.SampleTime > item.LastSeen {
			item.LastSeen = sample.SampleTime
		}
	}

	// Keep pre-sampling and very short sessions visible. For connections that
	// already have samples in this range, history contains the same cumulative
	// traffic and is used only for session metadata.
	var histories []History
	if err := db.WithContext(ctx).
		Where("time_unix BETWEEN ? AND ?", start, end).
		Find(&histories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "查询连接历史失败: " + err.Error()})
		return
	}
	for _, history := range histories {
		item := addTrafficAccumulator(users, history.Username, history.CommonName)
		connectionID := strings.TrimSpace(history.ConnectionID)
		item.addConnection(connectionID)
		if history.TimeDuration > 0 {
			item.OnlineSeconds += history.TimeDuration
		}
		if history.TimeUnix > item.LastSeen {
			item.LastSeen = history.TimeUnix
		}
		if connectionID == "" {
			item.Received += maxPositive(history.BytesReceived)
			item.Sent += maxPositive(history.BytesSent)
		} else if _, sampled := sampledConnections[connectionID]; !sampled {
			item.Received += maxPositive(history.BytesReceived)
			item.Sent += maxPositive(history.BytesSent)
		}
	}

	// Add the unsampled tail of currently online connections. The management
	// counters are cumulative per connection; subtracting the latest stored
	// cumulative value avoids double counting the sample rows.
	now := time.Now()
	if start <= now.Unix() && end >= now.Unix()-120 {
		clients, _ := ov.safeOnlineClients()
		for _, client := range clients {
			item := addTrafficAccumulator(users, client.Username, client.CommonName)
			item.Online = true
			item.addConnection(client.ID)
			item.LastSeen = now.Unix()
			connStart := parseClientConnTime(client.ConnDate, now)
			item.OnlineSeconds += overlapSeconds(connStart, now.Unix(), start, end)

			var latest ClientTrafficSample
			query := db.WithContext(ctx).Where("connection_key = ?", clientTrafficKey(client)).Order("sample_time DESC").First(&latest)
			if query.Error == nil {
				// The last counter difference covers [latest.SampleTime, now].
				// Attribute only the overlapping part when a custom range begins
				// inside this not-yet-sampled interval.
				tailSeconds := now.Unix() - latest.SampleTime
				includedSeconds := overlapSeconds(latest.SampleTime, now.Unix(), start, end)
				portion := 0.0
				if tailSeconds > 0 && includedSeconds > 0 {
					portion = float64(includedSeconds) / float64(tailSeconds)
				}
				item.Received += maxPositive(client.RecvBytes-latest.CumulativeReceived) * portion
				item.Sent += maxPositive(client.SendBytes-latest.CumulativeSent) * portion
			} else if query.Error == gorm.ErrRecordNotFound {
				// No baseline yet: do not attribute traffic from before the first
				// sample to the selected range.
			}
		}
	}

	result := make([]DashboardTrafficUser, 0, len(users))
	var totals DashboardTrafficTotals
	for _, item := range users {
		row := item.result()
		result = append(result, row)
		if row.Received > 0 || row.Sent > 0 || row.Online || row.Connections > 0 {
			totals.ActiveUsers++
		}
		totals.Received += row.Received
		totals.Sent += row.Sent
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Total > result[j].Total })
	totals.Total = totals.Received + totals.Sent
	totals.ReceivedText = tools.FormatBytes(totals.Received)
	totals.SentText = tools.FormatBytes(totals.Sent)
	totals.TotalText = tools.FormatBytes(totals.Total)

	c.JSON(http.StatusOK, DashboardTrafficUsersResponse{
		Start:         start,
		End:           end,
		SampleSeconds: 60,
		Users:         result,
		Totals:        totals,
	})
}

func logTrafficSampleError(err error) {
	log.Printf("[traffic-sample] %v", err)
}

func maxPositive(value float64) float64 {
	if value > 0 {
		return value
	}
	return 0
}

func overlapSeconds(sessionStart, sessionEnd, rangeStart, rangeEnd int64) int64 {
	if sessionStart <= 0 {
		sessionStart = rangeStart
	}
	start := sessionStart
	if start < rangeStart {
		start = rangeStart
	}
	end := sessionEnd
	if end > rangeEnd {
		end = rangeEnd
	}
	if end <= start {
		return 0
	}
	return end - start
}

func parseClientConnTime(value string, now time.Time) int64 {
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(value), time.Local)
	if err != nil {
		return now.Unix()
	}
	return parsed.Unix()
}

func clientTrafficKey(client ClientData) string {
	return fmt.Sprintf("%s|%s|%s", strings.TrimSpace(client.ID), strings.TrimSpace(client.CommonName), strings.TrimSpace(client.ConnDate))
}

func (c *dashboardStatsCollector) sampleOnlineTraffic(clients []ClientData) {
	if db == nil {
		return
	}
	now := time.Now().Unix()
	for _, client := range clients {
		key := clientTrafficKey(client)
		var previous ClientTrafficSample
		query := db.Where("connection_key = ?", key).Order("sample_time DESC").First(&previous)
		if query.Error != nil && query.Error != gorm.ErrRecordNotFound {
			logTrafficSampleError(query.Error)
			continue
		}

		if query.Error == gorm.ErrRecordNotFound {
			_ = db.Create(&ClientTrafficSample{
				ConnectionID:       strings.TrimSpace(client.ID),
				ConnectionKey:      key,
				Username:           trafficUsername(client.Username, client.CommonName),
				CommonName:         strings.TrimSpace(client.CommonName),
				SampleTime:         now,
				CumulativeReceived: maxPositive(client.RecvBytes),
				CumulativeSent:     maxPositive(client.SendBytes),
			})
			continue
		}

		receivedDelta := client.RecvBytes - previous.CumulativeReceived
		sentDelta := client.SendBytes - previous.CumulativeSent
		if receivedDelta < 0 || sentDelta < 0 {
			// Counter reset/reconnect: persist a replacement baseline. Without
			// this row every following observation would still be compared with
			// the stale, larger counter.
			if err := db.Create(&ClientTrafficSample{
				ConnectionID:       strings.TrimSpace(client.ID),
				ConnectionKey:      key,
				Username:           trafficUsername(client.Username, client.CommonName),
				CommonName:         strings.TrimSpace(client.CommonName),
				SampleTime:         now,
				CumulativeReceived: maxPositive(client.RecvBytes),
				CumulativeSent:     maxPositive(client.SendBytes),
			}).Error; err != nil {
				logTrafficSampleError(err)
			}
			continue
		}
		if receivedDelta <= 0 && sentDelta <= 0 {
			continue
		}
		if err := db.Create(&ClientTrafficSample{
			ConnectionID:       strings.TrimSpace(client.ID),
			ConnectionKey:      key,
			Username:           trafficUsername(client.Username, client.CommonName),
			CommonName:         strings.TrimSpace(client.CommonName),
			SampleTime:         now,
			CumulativeReceived: maxPositive(client.RecvBytes),
			CumulativeSent:     maxPositive(client.SendBytes),
			ReceivedDelta:      receivedDelta,
			SentDelta:          sentDelta,
		}).Error; err != nil {
			logTrafficSampleError(err)
		}
	}
}
