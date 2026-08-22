package openvpnweb

import (
	"context"
	"testing"
	"time"

	gormlogger "gorm.io/gorm/logger"
)

func TestCountTodayConnectionsIncludesLiveSessions(t *testing.T) {
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
	if err := database.AutoMigrate(&History{}); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	if err := database.Create(&History{ConnectionID: "finished-today", TimeUnix: todayStart + 60}).Error; err != nil {
		t.Fatal(err)
	}
	// A history row with the same management connection ID must not be counted
	// again if the session is also present in the management status response.
	if err := database.Create(&History{ConnectionID: "already-recorded", TimeUnix: todayStart + 120}).Error; err != nil {
		t.Fatal(err)
	}

	connectedToday := time.Unix(todayStart+180, 0).In(now.Location()).Format("2006-01-02 15:04:05")
	connectedYesterday := time.Unix(todayStart-60, 0).In(now.Location()).Format("2006-01-02 15:04:05")
	count, err := countTodayConnections(context.Background(), todayStart, []ClientData{
		{ID: "already-recorded", ConnDate: connectedToday, Username: "alice", CommonName: "alice", Vip: "10.8.0.2"},
		{ID: "live-today", ConnDate: connectedToday, Username: "bob", CommonName: "bob", Vip: "10.8.0.3"},
		{ID: "live-today", ConnDate: connectedToday, Username: "bob", CommonName: "bob", Vip: "10.8.0.3"},
		{ID: "live-yesterday", ConnDate: connectedYesterday, Username: "carol", CommonName: "carol", Vip: "10.8.0.4"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("today connections = %d, want 3 (two history sessions + one live session)", count)
	}
}
