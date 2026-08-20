package openvpnweb

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	gormlogger "gorm.io/gorm/logger"
)

func setupSuricataEVETestDB(t *testing.T) func() {
	t.Helper()
	originalDB := db
	database, err := OpenDatabase(DatabaseConfig{Type: "sqlite", Path: ":memory:"}, "", gormlogger.Default)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	db = database
	if err := database.AutoMigrate(&SuricataNetworkEvent{}, &SuricataEVEOffset{}); err != nil {
		t.Fatal(err)
	}
	return func() { db = originalDB; _ = sqlDB.Close() }
}

func TestParseSuricataEVELineWhitelistsMetadata(t *testing.T) {
	line := []byte(`{"timestamp":"2026-08-20T12:00:00.123Z","event_type":"http","src_ip":"10.8.0.2","dest_ip":"198.51.100.10","proto":"TCP","src_port":51000,"dest_port":80,"http":{"hostname":"example.test","url":"/only/a/path","http_method":"GET","request_body":"secret","cookie":"session=secret","authorization":"Bearer secret"},"payload":"secret"}`)
	event, ok, err := parseSuricataEVELine(line)
	if err != nil || !ok {
		t.Fatalf("parse = %#v, %v, %v", event, ok, err)
	}
	if event.HTTPHostname != "example.test" || event.HTTPURL != "/only/a/path" || event.HTTPMethod != "GET" {
		t.Fatalf("unexpected HTTP metadata: %#v", event)
	}
	if event.AlertSignature != "" || event.DNSName != "" {
		t.Fatalf("unexpected non-whitelist fields: %#v", event)
	}
	if _, ok, err := parseSuricataEVELine([]byte(`{"event_type":"fileinfo","src_ip":"10.8.0.2","dest_ip":"198.51.100.10"}`)); err != nil || ok {
		t.Fatalf("unsupported type accepted: ok=%v err=%v", ok, err)
	}
}

func TestImportSuricataEVEAttributesCurrentIdentityAndSkipsUnknown(t *testing.T) {
	cleanup := setupSuricataEVETestDB(t)
	defer cleanup()
	path := filepath.Join(t.TempDir(), "eve.json")
	contents := "{\"event_type\":\"flow\",\"src_ip\":\"10.8.0.2\",\"dest_ip\":\"198.51.100.1\",\"proto\":\"TCP\",\"src_port\":1,\"dest_port\":443,\"flow\":{\"bytes_toserver\":10}}\n" +
		"{\"event_type\":\"tls\",\"src_ip\":\"10.8.0.99\",\"dest_ip\":\"198.51.100.1\"}\n"
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	oldClients := webAuditDNS.clients
	webAuditDNS.clients = map[string]auditClientIdentity{"10.8.0.2": {UserID: 7, Username: "alice", CommonName: "alice-cn", ConnectionID: "conn-a"}}
	defer func() { webAuditDNS.clients = oldClients }()
	_, imported, dropped, malformed, err := importSuricataEVEFile(path)
	if err != nil || imported != 1 || dropped != 1 || malformed != 0 {
		t.Fatalf("imported=%d dropped=%d malformed=%d err=%v", imported, dropped, malformed, err)
	}
	var event SuricataNetworkEvent
	if err := db.First(&event).Error; err != nil {
		t.Fatal(err)
	}
	if event.UserID != 7 || event.Username != "alice" || event.ConnectionID != "conn-a" || event.BytesToServer != 10 {
		t.Fatalf("identity snapshot/event = %#v", event)
	}
}

func TestImportSuricataEVESnapshotsIdentityWithoutDNSRun(t *testing.T) {
	cleanup := setupSuricataEVETestDB(t)
	defer cleanup()
	path := filepath.Join(t.TempDir(), "eve.json")
	if err := os.WriteFile(path, []byte("{\"event_type\":\"flow\",\"src_ip\":\"10.8.0.2\",\"dest_ip\":\"198.51.100.1\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	oldClients, oldRun := webAuditDNS.clients, webAuditDNS.run
	webAuditDNS.clients, webAuditDNS.run = map[string]auditClientIdentity{"10.8.0.2": {UserID: 9, Username: "hook-only", ConnectionID: "conn-hook"}}, nil
	defer func() { webAuditDNS.clients, webAuditDNS.run = oldClients, oldRun }()
	if _, imported, _, _, err := importSuricataEVEFile(path); err != nil || imported != 1 {
		t.Fatalf("imported=%d err=%v", imported, err)
	}
	var event SuricataNetworkEvent
	if err := db.First(&event).Error; err != nil || event.Username != "hook-only" || event.ConnectionID != "conn-hook" {
		t.Fatalf("event=%#v err=%v", event, err)
	}
}

func TestImportSuricataEVERecoversOffsetAndTruncation(t *testing.T) {
	cleanup := setupSuricataEVETestDB(t)
	defer cleanup()
	path := filepath.Join(t.TempDir(), "eve.json")
	oldClients := webAuditDNS.clients
	webAuditDNS.clients = map[string]auditClientIdentity{"10.8.0.2": {UserID: 7, Username: "alice"}}
	defer func() { webAuditDNS.clients = oldClients }()
	line := "{\"event_type\":\"flow\",\"src_ip\":\"10.8.0.2\",\"dest_ip\":\"198.51.100.1\"}\n"
	if err := os.WriteFile(path, []byte(line), 0600); err != nil {
		t.Fatal(err)
	}
	if _, imported, _, _, err := importSuricataEVEFile(path); err != nil || imported != 1 {
		t.Fatalf("first import=%d err=%v", imported, err)
	}
	if _, imported, _, _, err := importSuricataEVEFile(path); err != nil || imported != 0 {
		t.Fatalf("replay import=%d err=%v", imported, err)
	}
	if err := os.WriteFile(path, []byte(line+line), 0600); err != nil {
		t.Fatal(err)
	}
	if _, imported, _, _, err := importSuricataEVEFile(path); err != nil || imported != 1 {
		t.Fatalf("append import=%d err=%v", imported, err)
	}
	if err := os.WriteFile(path, []byte(line), 0600); err != nil {
		t.Fatal(err)
	}
	if _, imported, _, _, err := importSuricataEVEFile(path); err != nil || imported != 1 {
		t.Fatalf("truncation import=%d err=%v", imported, err)
	}
}

func TestSuricataNetworkAuditQueryRespectsScope(t *testing.T) {
	cleanup := setupSuricataEVETestDB(t)
	defer cleanup()
	now := time.Now().Unix()
	entries := []SuricataNetworkEvent{{UserID: 1, Username: "alice", EventType: "flow", ObservedAt: now}, {UserID: 2, Username: "bob", EventType: "alert", ObservedAt: now}}
	if err := db.Create(&entries).Error; err != nil {
		t.Fatal(err)
	}
	result, err := querySuricataNetworkAuditRecords(context.Background(), SuricataNetworkAuditFilter{Start: now - 60, End: now + 1}, []uint{1}, false, 0, 20)
	if err != nil || result.Total != 1 || len(result.Data) != 1 || result.Data[0].Username != "alice" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestValidateSuricataEVEPath(t *testing.T) {
	if _, err := validateSuricataEVEPath("relative/eve.json"); err == nil {
		t.Fatal("relative path accepted")
	}
	if _, err := validateSuricataEVEPath(""); err == nil {
		t.Fatal("empty path accepted")
	}
	if got, err := validateSuricataEVEPath(filepath.Join(t.TempDir(), "eve.json")); err != nil || !filepath.IsAbs(got) {
		t.Fatalf("valid path=%q err=%v", got, err)
	}
}
