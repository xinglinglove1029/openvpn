package openvpnweb

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	gormlogger "gorm.io/gorm/logger"
)

func TestDashboardTrafficUsersUsesSamplesAndHistoryFallback(t *testing.T) {
	originalDB := db
	database, err := OpenDatabase(DatabaseConfig{Type: "sqlite", Path: ":memory:"}, "", gormlogger.Default)
	if err != nil {
		t.Fatal(err)
	}
	db = database
	defer func() { db = originalDB }()

	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := database.AutoMigrate(&History{}, &ClientTrafficSample{}); err != nil {
		t.Fatal(err)
	}

	now := time.Now().Unix()
	if err := database.Create(&ClientTrafficSample{
		ConnectionID:  "alice-1",
		ConnectionKey: "alice-1|alice|start",
		Username:      "alice",
		CommonName:    "alice",
		SampleTime:    now - 300,
		ReceivedDelta: 100,
		SentDelta:     20,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&History{
		Username:      "alice",
		CommonName:    "alice",
		ConnectionID:  "alice-1",
		BytesReceived: 999,
		BytesSent:     999,
		TimeUnix:      now - 200,
		TimeDuration:  120,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&History{
		Username:      "bob",
		CommonName:    "bob",
		BytesReceived: 50,
		BytesSent:     5,
		TimeUnix:      now - 200,
		TimeDuration:  60,
	}).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest(http.MethodGet, "/ovpn/dashboard/traffic-users?start="+formatInt64(now-3600)+"&end="+formatInt64(now-121), nil)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = request
	(&ovpn{}).dashboardTrafficUsers(context)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	var payload DashboardTrafficUsersResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Users) != 2 {
		t.Fatalf("users = %#v, want two users", payload.Users)
	}
	if payload.Totals.Received != 150 || payload.Totals.Sent != 25 || payload.Totals.Total != 175 {
		t.Fatalf("totals = %#v, want received=150 sent=25 total=175", payload.Totals)
	}
	if payload.Users[0].Username != "alice" || payload.Users[0].Received != 100 || payload.Users[0].Sent != 20 {
		t.Fatalf("alice row = %#v, expected sample-only traffic", payload.Users[0])
	}
}

func formatInt64(value int64) string {
	return strconv.FormatInt(value, 10)
}
