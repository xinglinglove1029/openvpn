package openvpnweb

import (
	"context"
	"encoding/binary"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"openvpn-web/internal/openvpnweb/ai"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"golang.org/x/net/dns/dnsmessage"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool/toolconfirmation"
	"google.golang.org/genai"
	gormlogger "gorm.io/gorm/logger"
)

func TestNormalizeDNSNameAndQueryInfo(t *testing.T) {
	if got := normalizeDNSName("  WWW.Example.COM. "); got != "www.example.com" {
		t.Fatalf("normalizeDNSName = %q", got)
	}
	name, err := dnsmessage.NewName("www.example.com.")
	if err != nil {
		t.Fatal(err)
	}
	message := dnsmessage.Message{Header: dnsmessage.Header{ID: 7, RecursionDesired: true}, Questions: []dnsmessage.Question{{Name: name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}}}
	packet, err := message.Pack()
	if err != nil {
		t.Fatal(err)
	}
	domain, queryType, err := dnsQueryInfo(packet)
	if err != nil {
		t.Fatal(err)
	}
	if domain != "www.example.com" || queryType != "TypeA" {
		t.Fatalf("query info = %q, %q", domain, queryType)
	}
}

func TestFindWebAuditClientByIP(t *testing.T) {
	clients := []ClientData{{ID: "v4", Username: "alice", Vip: "10.8.0.2"}, {ID: "v6", Username: "bob", Vip6: "fdaf::2"}}
	got, ok := findWebAuditClientByIP(clients, "fdaf::2")
	if !ok || got.Username != "bob" {
		t.Fatalf("got %#v, ok=%v", got, ok)
	}
	if _, ok := findWebAuditClientByIP(clients, "10.8.0.99"); ok {
		t.Fatal("unexpected client match")
	}
}

func TestWebsiteAuditSummaryRespectsScopeAndFilters(t *testing.T) {
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
	if err := database.AutoMigrate(&WebsiteAccessLog{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	logs := []WebsiteAccessLog{
		{UserID: 1, Username: "alice", Domain: "github.com", QueryType: "A", ResponseCode: "RCodeSuccess", QueriedAt: now - 10},
		{UserID: 1, Username: "alice", Domain: "github.com", QueryType: "AAAA", ResponseCode: "RCodeSuccess", QueriedAt: now - 20},
		{UserID: 2, Username: "bob", Domain: "example.com", QueryType: "A", ResponseCode: "RCodeSuccess", QueriedAt: now - 30},
	}
	if err := database.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}
	filter := WebsiteAuditFilter{Start: now - 3600, End: now}
	summary, err := buildWebsiteAuditSummary(context.Background(), filter, []uint{1}, false, 10)
	if err != nil {
		t.Fatal(err)
	}
	if summary.TotalQueries != 2 || summary.ActiveUsers != 1 || summary.UniqueDomains != 1 {
		t.Fatalf("unexpected scoped summary: %#v", summary)
	}
	if len(summary.TopDomains) != 1 || summary.TopDomains[0].Domain != "github.com" {
		t.Fatalf("top domains=%#v", summary.TopDomains)
	}
	records, err := queryWebsiteAuditRecords(context.Background(), WebsiteAuditFilter{Start: now - 3600, End: now, Domain: "github"}, []uint{1}, false, 0, 20)
	if err != nil || records.Total != 2 {
		t.Fatalf("records=%#v err=%v", records, err)
	}
}

func TestQueryHistoryWebsiteAuditRecordsUsesConnectionAndTimeRange(t *testing.T) {
	originalDB := db
	database, err := OpenDatabase(DatabaseConfig{Type: "sqlite", Path: ":memory:"}, "", gormlogger.Default)
	if err != nil {
		t.Fatal(err)
	}
	// SQLite :memory: creates one database per physical connection. The worker
	// writes concurrently, so keep this regression fixture on its migrated
	// connection rather than intermittently observing an empty database.
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	db = database
	defer func() {
		db = originalDB
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	}()
	if err := database.AutoMigrate(&History{}, &WebsiteAccessLog{}); err != nil {
		t.Fatal(err)
	}

	now := time.Now().Unix()
	history := History{ID: 10, UserID: 1, Username: "alice", ConnectionID: "conn-a", TimeUnix: now - 100, TimeDuration: 50}
	if err := database.Create(&history).Error; err != nil {
		t.Fatal(err)
	}
	logs := []WebsiteAccessLog{
		{UserID: 1, Username: "alice", ConnectionID: "conn-a", Domain: "inside.example", QueriedAt: now - 90},
		{UserID: 1, Username: "alice", ConnectionID: "conn-b", Domain: "other-connection.example", QueriedAt: now - 90},
		{UserID: 1, Username: "alice", ConnectionID: "conn-a", Domain: "after.example", QueriedAt: now - 20},
		{UserID: 2, Username: "bob", ConnectionID: "conn-a", Domain: "other-user.example", QueriedAt: now - 90},
	}
	if err := database.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}

	result, err := queryHistoryWebsiteAuditRecords(context.Background(), history, []uint{1}, false, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || result.MatchedBy != "connection_id" || len(result.Data) != 1 || result.Data[0].Domain != "inside.example" {
		t.Fatalf("connection-scoped result = %#v", result)
	}
	// The same connection ID from another user must stay excluded even for an
	// administrator's unscoped query.
	result, err = queryHistoryWebsiteAuditRecords(context.Background(), history, nil, true, 0, 50)
	if err != nil || result.Total != 1 || result.Data[0].Domain != "inside.example" {
		t.Fatalf("connection user-boundary result = %#v err=%v", result, err)
	}

	legacy := history
	legacy.ID = 11
	legacy.ConnectionID = ""
	if err := database.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	result, err = queryHistoryWebsiteAuditRecords(context.Background(), legacy, []uint{1}, false, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 2 || result.MatchedBy != "time_range" {
		t.Fatalf("legacy time-range result = %#v", result)
	}
}

func TestDNSFailureResponseReturnsSERVFAIL(t *testing.T) {
	name, err := dnsmessage.NewName("resolver-failure.example.")
	if err != nil {
		t.Fatal(err)
	}
	message := dnsmessage.Message{
		Header:    dnsmessage.Header{ID: 42, RecursionDesired: true},
		Questions: []dnsmessage.Question{{Name: name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}},
	}
	request, err := message.Pack()
	if err != nil {
		t.Fatal(err)
	}
	response := dnsFailureResponse(request)
	if len(response) == 0 {
		t.Fatal("dnsFailureResponse returned an empty packet")
	}
	var parser dnsmessage.Parser
	header, err := parser.Start(response)
	if err != nil {
		t.Fatal(err)
	}
	if !header.Response || header.ID != 42 || header.RCode != dnsmessage.RCodeServerFailure {
		t.Fatalf("unexpected failure response header: %#v", header)
	}
	question, err := parser.Question()
	if err != nil {
		t.Fatal(err)
	}
	if normalizeDNSName(question.Name.String()) != "resolver-failure.example" {
		t.Fatalf("response did not preserve question: %#v", question)
	}
}

func TestWebsiteAuditAIScopesRecordsToOperator(t *testing.T) {
	originalDB, originalAdminUsername := db, adminUsername
	database, err := OpenDatabase(DatabaseConfig{Type: "sqlite", Path: ":memory:"}, "", gormlogger.Default)
	if err != nil {
		t.Fatal(err)
	}
	db = database
	adminUsername = "administrator"
	defer func() {
		db = originalDB
		adminUsername = originalAdminUsername
		sqlDB, _ := database.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	}()

	if err := database.AutoMigrate(&User{}, &Role{}, &Permission{}, &RolePermission{}, &UserRole{}, &WebsiteAccessLog{}); err != nil {
		t.Fatal(err)
	}
	enabled := true
	operator := User{Username: "limited-operator"}
	otherUser := User{Username: "other-user"}
	if err := database.Create(&operator).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&otherUser).Error; err != nil {
		t.Fatal(err)
	}
	// A zero group ID keeps this ordinary operator scoped to their own user ID.
	if err := database.Model(&User{}).Where("id IN ?", []uint{operator.ID, otherUser.ID}).Update("gid", 0).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&Role{Name: "Website audit viewer", Code: "website_audit_viewer", IsEnable: &enabled}).Error; err != nil {
		t.Fatal(err)
	}
	var role Role
	if err := database.Where("code = ?", "website_audit_viewer").First(&role).Error; err != nil {
		t.Fatal(err)
	}
	permission := Permission{Name: "View website audit", Code: "web-audit:view", Type: "button"}
	if err := database.Create(&permission).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&RolePermission{RoleID: role.ID, PermissionID: permission.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&UserRole{UserID: operator.ID, RoleID: role.ID}).Error; err != nil {
		t.Fatal(err)
	}

	now := time.Now().Unix()
	logs := []WebsiteAccessLog{
		{UserID: operator.ID, Username: operator.Username, Domain: "operator.example", QueryType: "A", ResponseCode: "RCodeSuccess", QueriedAt: now - 10},
		{UserID: operator.ID, Username: operator.Username, Domain: "operator.example", QueryType: "AAAA", ResponseCode: "RCodeSuccess", QueriedAt: now - 20},
		// This operator-owned row must not be returned by the 24h range.
		{UserID: operator.ID, Username: operator.Username, Domain: "expired-operator.example", QueryType: "A", ResponseCode: "RCodeSuccess", QueriedAt: now - int64((25 * time.Hour).Seconds())},
		// These rows are newer than the permitted rows so the recent-record limit
		// verifies that access scope is applied before ordering and pagination.
		{UserID: otherUser.ID, Username: otherUser.Username, Domain: "other-user.example", QueryType: "A", ResponseCode: "RCodeSuccess", QueriedAt: now - 1},
		// Historical data can contain a stale username; authorization must use UserID.
		{UserID: otherUser.ID, Username: operator.Username, Domain: "stale-username.example", QueryType: "A", ResponseCode: "RCodeSuccess", QueriedAt: now - 2},
	}
	if err := database.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewAIToolService(nil)
	toolCtx := &websiteAuditToolContext{Context: context.Background()}
	result, err := svc.GetWebsiteAccessStats(toolCtx, operator.Username, ai.WebsiteAccessStatsRequest{Range: "24h", Limit: 20})
	if err != nil {
		t.Fatalf("GetWebsiteAccessStats returned an error for an authorized operator: %v", err)
	}
	if result.TotalQueries != 2 || result.ActiveUsers != 1 || result.UniqueDomains != 1 {
		t.Fatalf("unexpected scoped statistics: %#v", result)
	}
	if len(result.TopUsers) != 1 || result.TopUsers[0].Username != operator.Username || result.TopUsers[0].Queries != 2 {
		t.Fatalf("top users leaked or omitted scoped data: %#v", result.TopUsers)
	}
	if len(result.TopDomains) != 1 || result.TopDomains[0].Domain != "operator.example" || result.TopDomains[0].Queries != 2 {
		t.Fatalf("top domains leaked or omitted scoped data: %#v", result.TopDomains)
	}
	if len(result.RecentRecords) != 2 {
		t.Fatalf("recent records=%#v, want only the operator's two records", result.RecentRecords)
	}
	for _, record := range result.RecentRecords {
		if record.Username != operator.Username || record.Domain != "operator.example" {
			t.Fatalf("AI result leaked another user's record: %#v", record)
		}
	}

	filtered, err := svc.GetWebsiteAccessStats(toolCtx, operator.Username, ai.WebsiteAccessStatsRequest{
		Range: "24h", Username: otherUser.Username, Domain: "other-user.example", Limit: 20,
	})
	if err != nil {
		t.Fatalf("GetWebsiteAccessStats with an out-of-scope filter returned an error: %v", err)
	}
	if filtered.TotalQueries != 0 || filtered.ActiveUsers != 0 || filtered.UniqueDomains != 0 || len(filtered.TopUsers) != 0 || len(filtered.TopDomains) != 0 || len(filtered.RecentRecords) != 0 {
		t.Fatalf("out-of-scope filters exposed website audit data: %#v", filtered)
	}

	limited, err := svc.GetWebsiteAccessStats(toolCtx, operator.Username, ai.WebsiteAccessStatsRequest{Range: "24h", Limit: 1})
	if err != nil {
		t.Fatalf("GetWebsiteAccessStats with a limited result size returned an error: %v", err)
	}
	if len(limited.RecentRecords) != 1 || limited.RecentRecords[0].Username != operator.Username || limited.RecentRecords[0].Domain != "operator.example" {
		t.Fatalf("recent-record pagination applied before access scope: %#v", limited.RecentRecords)
	}
}

func TestWebsiteAuditUnknownOperatorUsesEmptyScope(t *testing.T) {
	originalDB, originalAdminUsername := db, adminUsername
	database, err := OpenDatabase(DatabaseConfig{Type: "sqlite", Path: ":memory:"}, "", gormlogger.Default)
	if err != nil {
		t.Fatal(err)
	}
	db = database
	adminUsername = "administrator"
	defer func() {
		db = originalDB
		adminUsername = originalAdminUsername
		sqlDB, _ := database.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	}()

	if err := database.AutoMigrate(&WebsiteAccessLog{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	if err := database.Create(&WebsiteAccessLog{Domain: "unowned.example", QueryType: "A", ResponseCode: "RCodeSuccess", QueriedAt: now}).Error; err != nil {
		t.Fatal(err)
	}

	accessible, skip := GetAccessibleUserIDs("deleted-operator")
	if skip || len(accessible) != 0 {
		t.Fatalf("unknown operator must have an empty, non-admin scope: ids=%v skip=%v", accessible, skip)
	}
	summary, err := buildWebsiteAuditSummary(context.Background(), WebsiteAuditFilter{Start: now - 60, End: now + 1}, accessible, skip, 10)
	if err != nil {
		t.Fatalf("buildWebsiteAuditSummary returned an error: %v", err)
	}
	if summary.TotalQueries != 0 || summary.ActiveUsers != 0 || summary.UniqueDomains != 0 {
		t.Fatalf("empty scope exposed an unowned DNS audit record: %#v", summary)
	}
}
func TestWebsiteAuditAIUsesGroupSubtreeScope(t *testing.T) {
	originalDB, originalAdminUsername := db, adminUsername
	database, err := OpenDatabase(DatabaseConfig{Type: "sqlite", Path: ":memory:"}, "", gormlogger.Default)
	if err != nil {
		t.Fatal(err)
	}
	db = database
	adminUsername = "administrator"
	defer func() {
		db = originalDB
		adminUsername = originalAdminUsername
		sqlDB, _ := database.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	}()

	if err := database.AutoMigrate(&User{}, &Group{}, &Role{}, &Permission{}, &RolePermission{}, &UserRole{}, &WebsiteAccessLog{}); err != nil {
		t.Fatal(err)
	}
	root := Group{Name: "Default"}
	if err := database.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	managedGroup := Group{Name: "managed-group", ParentID: &root.ID}
	childGroup := Group{Name: "managed-child", ParentID: &managedGroup.ID}
	unrelatedGroup := Group{Name: "unrelated-group", ParentID: &root.ID}
	for _, group := range []*Group{&managedGroup, &childGroup, &unrelatedGroup} {
		if err := database.Create(group).Error; err != nil {
			t.Fatal(err)
		}
	}

	enabled := true
	role := Role{Name: "Website audit viewer", Code: "website_audit_viewer", IsEnable: &enabled}
	permission := Permission{Name: "View website audit", Code: "web-audit:view", Type: "button"}
	if err := database.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&permission).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&RolePermission{RoleID: role.ID, PermissionID: permission.ID}).Error; err != nil {
		t.Fatal(err)
	}

	operator := User{Username: "group-operator", Gid: managedGroup.ID}
	member := User{Username: "group-member", Gid: childGroup.ID}
	unrelated := User{Username: "unrelated-user", Gid: unrelatedGroup.ID}
	for _, user := range []*User{&operator, &member, &unrelated} {
		if err := database.Create(user).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := database.Create(&UserRole{UserID: operator.ID, RoleID: role.ID}).Error; err != nil {
		t.Fatal(err)
	}

	now := time.Now().Unix()
	logs := []WebsiteAccessLog{
		{UserID: operator.ID, Username: operator.Username, Domain: "operator-group.example", QueryType: "A", ResponseCode: "RCodeSuccess", QueriedAt: now - 10},
		{UserID: member.ID, Username: member.Username, Domain: "child-group.example", QueryType: "AAAA", ResponseCode: "RCodeSuccess", QueriedAt: now - 5},
		{UserID: unrelated.ID, Username: unrelated.Username, Domain: "unrelated-group.example", QueryType: "A", ResponseCode: "RCodeSuccess", QueriedAt: now - 1},
	}
	if err := database.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}

	toolCtx := &websiteAuditToolContext{Context: context.Background(), userID: operator.Username}
	result, err := callWebsiteAuditTool(toolCtx, NewAIToolService(nil), map[string]any{"range": "24h", "limit": 20})
	if err != nil {
		t.Fatalf("registered get_website_access_stats tool returned an error for a grouped operator: %v", err)
	}
	if result.TotalQueries != 2 || result.ActiveUsers != 2 || result.UniqueDomains != 2 {
		t.Fatalf("unexpected group-scoped statistics: %#v", result)
	}
	users := make(map[string]int64, len(result.TopUsers))
	for _, item := range result.TopUsers {
		users[item.Username] = item.Queries
	}
	if len(users) != 2 || users[operator.Username] != 1 || users[member.Username] != 1 || users[unrelated.Username] != 0 {
		t.Fatalf("unexpected group-scoped top users: %#v", result.TopUsers)
	}
	for _, record := range result.RecentRecords {
		if record.Username == unrelated.Username || record.Domain == "unrelated-group.example" {
			t.Fatalf("AI result leaked a sibling group's record: %#v", record)
		}
	}

	filtered, err := callWebsiteAuditTool(toolCtx, NewAIToolService(nil), map[string]any{
		"range": "24h", "username": unrelated.Username, "domain": "unrelated-group.example", "limit": 20,
	})
	if err != nil {
		t.Fatalf("registered get_website_access_stats tool with a sibling-group filter returned an error: %v", err)
	}
	if filtered.TotalQueries != 0 || filtered.ActiveUsers != 0 || filtered.UniqueDomains != 0 || len(filtered.TopUsers) != 0 || len(filtered.TopDomains) != 0 || len(filtered.RecentRecords) != 0 {
		t.Fatalf("sibling-group filter exposed website audit data: %#v", filtered)
	}
}

func TestWebsiteAuditAIRejectsOperatorWithoutPermission(t *testing.T) {
	originalDB, originalAdminUsername := db, adminUsername
	database, err := OpenDatabase(DatabaseConfig{Type: "sqlite", Path: ":memory:"}, "", gormlogger.Default)
	if err != nil {
		t.Fatal(err)
	}
	db = database
	adminUsername = "administrator"
	defer func() {
		db = originalDB
		adminUsername = originalAdminUsername
		sqlDB, _ := database.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	}()
	if err := database.AutoMigrate(&User{}, &Role{}, &Permission{}, &RolePermission{}, &UserRole{}, &GroupRole{}); err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&User{Username: "limited-operator"}).Error; err != nil {
		t.Fatal(err)
	}

	_, err = NewAIToolService(nil).GetWebsiteAccessStats(&websiteAuditToolContext{Context: context.Background()}, "limited-operator", ai.WebsiteAccessStatsRequest{Range: "24h"})
	if err == nil || !strings.Contains(err.Error(), "web-audit:view") {
		t.Fatalf("expected web-audit permission error, got %v", err)
	}
}

// websiteAuditToolContext is intentionally minimal: these tests only need a
// session identity and no agent state, memory, artifacts, or confirmation data.
type websiteAuditToolContext struct {
	context.Context
	userID string
}

// runnableWebsiteAuditTool exposes the execution method implemented by ADK
// function tools without depending on its internal concrete generic type.
type runnableWebsiteAuditTool interface {
	Run(agent.ToolContext, any) (map[string]any, error)
}

func callWebsiteAuditTool(ctx agent.ToolContext, svc ai.ToolService, args map[string]any) (ai.WebsiteAccessStatsResult, error) {
	tools, err := ai.BuildBusinessTools(svc)
	if err != nil {
		return ai.WebsiteAccessStatsResult{}, err
	}
	for _, candidate := range tools {
		if candidate.Name() != "get_website_access_stats" {
			continue
		}
		runnable, ok := candidate.(runnableWebsiteAuditTool)
		if !ok {
			return ai.WebsiteAccessStatsResult{}, fmt.Errorf("get_website_access_stats is not runnable")
		}
		payload, err := runnable.Run(ctx, args)
		if err != nil {
			return ai.WebsiteAccessStatsResult{}, err
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return ai.WebsiteAccessStatsResult{}, err
		}
		var result ai.WebsiteAccessStatsResult
		if err := json.Unmarshal(encoded, &result); err != nil {
			return ai.WebsiteAccessStatsResult{}, err
		}
		return result, nil
	}
	return ai.WebsiteAccessStatsResult{}, fmt.Errorf("get_website_access_stats was not registered")
}

func (c *websiteAuditToolContext) UserContent() *genai.Content { return nil }
func (c *websiteAuditToolContext) FunctionCallID() string      { return "test" }
func (c *websiteAuditToolContext) Actions() *session.EventActions {
	return &session.EventActions{}
}
func (c *websiteAuditToolContext) SearchMemory(context.Context, string) (*memory.SearchResponse, error) {
	return nil, nil
}
func (c *websiteAuditToolContext) ToolConfirmation() *toolconfirmation.ToolConfirmation { return nil }
func (c *websiteAuditToolContext) RequestConfirmation(string, any) error                { return nil }
func (c *websiteAuditToolContext) AgentName() string                                    { return "test" }
func (c *websiteAuditToolContext) ReadonlyState() session.ReadonlyState                 { return nil }
func (c *websiteAuditToolContext) State() session.State                                 { return nil }
func (c *websiteAuditToolContext) Artifacts() agent.Artifacts                           { return nil }
func (c *websiteAuditToolContext) InvocationID() string                                 { return "test" }
func (c *websiteAuditToolContext) AppName() string                                      { return "test" }
func (c *websiteAuditToolContext) Branch() string                                       { return "test" }
func (c *websiteAuditToolContext) SessionID() string                                    { return "test" }
func (c *websiteAuditToolContext) UserID() string                                       { return c.userID }

func TestWebsiteAuditLikeEscapesWildcardsAndUnownedRowsDoNotCountAsUsers(t *testing.T) {
	originalDB := db
	database, err := OpenDatabase(DatabaseConfig{Type: "sqlite", Path: ":memory:"}, "", gormlogger.Default)
	if err != nil {
		t.Fatal(err)
	}
	db = database
	defer func() {
		db = originalDB
		sqlDB, _ := database.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	}()
	if err := database.AutoMigrate(&WebsiteAccessLog{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	logs := []WebsiteAccessLog{
		{UserID: 1, Username: "alice", Domain: "foo100%literal.example", QueriedAt: now - 5},
		{UserID: 2, Username: "bob", Domain: "foo100xliteral.example", QueriedAt: now - 6},
		{UserID: 0, Username: "", Domain: "unowned.example", QueriedAt: now - 7},
	}
	if err := database.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}
	filter := WebsiteAuditFilter{Start: now - 60, End: now, Domain: "100%literal"}
	records, err := queryWebsiteAuditRecords(context.Background(), filter, nil, true, 0, 20)
	if err != nil || records.Total != 1 || records.Data[0].Domain != "foo100%literal.example" {
		t.Fatalf("literal LIKE filtering failed: %#v, err=%v", records, err)
	}

	summary, err := buildWebsiteAuditSummary(context.Background(), WebsiteAuditFilter{Start: now - 60, End: now}, nil, true, 10)
	if err != nil {
		t.Fatal(err)
	}
	if summary.ActiveUsers != 2 {
		t.Fatalf("active users included unowned row: %#v", summary)
	}
	for _, top := range summary.TopUsers {
		if top.Username == "" {
			t.Fatalf("top users included unowned row: %#v", summary.TopUsers)
		}
	}
}

func TestWebsiteAuditExportStreamsAllRowsAndEscapesSpreadsheetFormulas(t *testing.T) {
	originalDB := db
	database, err := OpenDatabase(DatabaseConfig{Type: "sqlite", Path: ":memory:"}, "", gormlogger.Default)
	if err != nil {
		t.Fatal(err)
	}
	db = database
	defer func() {
		db = originalDB
		sqlDB, _ := database.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	}()
	if err := database.AutoMigrate(&WebsiteAccessLog{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	logs := make([]WebsiteAccessLog, 0, 202)
	for i := 0; i < 201; i++ {
		logs = append(logs, WebsiteAccessLog{UserID: 1, Username: "alice", Domain: "example.com", QueryType: "A", ResponseCode: "NOERROR", QueriedAt: now - int64(i)})
	}
	logs = append(logs, WebsiteAccessLog{UserID: 1, Username: "=formula", CommonName: "+certificate", Domain: "@evil.example", QueryType: "-A", ResponseCode: "NOERROR", QueriedAt: now})
	if err := database.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest("GET", "/ovpn/web-audit/export", nil)
	ctx.Set("isAdmin", true)
	(&ovpn{}).websiteAuditExport(ctx)
	if w.Code != 200 {
		t.Fatalf("export status=%d body=%s", w.Code, w.Body.String())
	}
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(w.Body.String(), "\ufeff")))
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 203 {
		t.Fatalf("export rows=%d, want header + 202 (must not use 200 page limit)", len(rows))
	}
	var found bool
	for _, row := range rows[1:] {
		if len(row) > 4 && row[1] == "'=formula" && row[2] == "'+certificate" && row[4] == "'@evil.example" && row[5] == "'-A" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("formula-looking CSV fields were not escaped: %v", rows[len(rows)-1])
	}
}

func TestServeTCPDNSFramesSupportsMultipleRequests(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()
	observed := 0
	done := make(chan struct{})
	go func() {
		serveTCPDNSFrames(server, func(request []byte) ([]byte, string, error) { return append([]byte("ok:"), request...), "NOERROR", nil }, func([]byte, string) { observed++ })
		close(done)
	}()
	writeFrame := func(value []byte) {
		var header [2]byte
		binary.BigEndian.PutUint16(header[:], uint16(len(value)))
		if _, err := client.Write(append(header[:], value...)); err != nil {
			t.Fatal(err)
		}
	}
	readFrame := func() []byte {
		var header [2]byte
		if _, err := io.ReadFull(client, header[:]); err != nil {
			t.Fatal(err)
		}
		value := make([]byte, binary.BigEndian.Uint16(header[:]))
		if _, err := io.ReadFull(client, value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	writeFrame([]byte("first"))
	if got := string(readFrame()); got != "ok:first" {
		t.Fatalf("first frame=%q", got)
	}
	writeFrame([]byte("second"))
	if got := string(readFrame()); got != "ok:second" {
		t.Fatalf("second frame=%q", got)
	}
	_ = client.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("TCP DNS frame loop did not finish")
	}
	if observed != 2 {
		t.Fatalf("observed=%d, want 2", observed)
	}
}

func TestAuditQueueDropsWithoutBlockingDNSPath(t *testing.T) {
	old := viper.GetBool("system.base.web_audit_enabled")
	viper.Set("system.base.web_audit_enabled", true)
	defer viper.Set("system.base.web_audit_enabled", old)
	run := &webAuditDNSRun{audits: make(chan webAuditEvent, 1)}
	run.audits <- webAuditEvent{Domain: "already.queued"}
	service := &webAuditDNSService{run: run, status: WebAuditDNSStatus{Enabled: true}}
	started := time.Now()
	service.enqueueAudit(run, webAuditEvent{Domain: "drop.example", VPNIP: "10.8.0.2", Identity: auditClientIdentity{UserID: 1, Username: "alice"}})
	if elapsed := time.Since(started); elapsed > 50*time.Millisecond {
		t.Fatalf("queue full enqueue blocked DNS path for %s", elapsed)
	}
	if service.status.DroppedAuditEvents != 1 {
		t.Fatalf("dropped=%d, want 1", service.status.DroppedAuditEvents)
	}
}

func TestAuditWorkerUsesIdentitySnapshotAfterVPNIPReuse(t *testing.T) {
	originalDB := db
	database, err := OpenDatabase(DatabaseConfig{Type: "sqlite", Path: ":memory:"}, "", gormlogger.Default)
	if err != nil {
		t.Fatal(err)
	}
	db = database
	defer func() {
		db = originalDB
		sqlDB, _ := database.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	}()
	if err := database.AutoMigrate(&WebsiteAccessLog{}); err != nil {
		t.Fatal(err)
	}

	previousEnabled := viper.GetBool("system.base.web_audit_enabled")
	viper.Set("system.base.web_audit_enabled", true)
	defer viper.Set("system.base.web_audit_enabled", previousEnabled)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	run := &webAuditDNSRun{ctx: ctx, cancel: cancel, audits: make(chan webAuditEvent, 1)}
	service := &webAuditDNSService{
		run: run,
		clients: map[string]auditClientIdentity{
			"10.8.0.2": {UserID: 2, Username: "new-owner", ConnectionID: "new"},
		},
		status: WebAuditDNSStatus{Enabled: true},
	}
	go service.auditWorker(run)

	// The packet was received while the IP belonged to the old connection. The
	// live map already contains the next owner by the time persistence occurs.
	run.audits <- webAuditEvent{
		VPNIP:        "10.8.0.2",
		Identity:     auditClientIdentity{UserID: 1, Username: "old-owner", CommonName: "old-cn", ConnectionID: "old"},
		Domain:       "snapshot.example",
		QueryType:    "A",
		ResponseCode: "NOERROR",
		QueriedAt:    time.Now().Unix(),
	}

	deadline := time.Now().Add(time.Second)
	for {
		var entry WebsiteAccessLog
		err = database.Where("domain = ?", "snapshot.example").First(&entry).Error
		if err == nil {
			if entry.UserID != 1 || entry.Username != "old-owner" || entry.ConnectionID != "old" {
				t.Fatalf("audit row used current IP owner instead of packet snapshot: %#v", entry)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("audit worker did not persist snapshot event: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestWebAuditHookMappingRejectsStaleUpsertAndDelete(t *testing.T) {
	service := &webAuditDNSService{clients: map[string]auditClientIdentity{
		"10.8.0.2": {UserID: 2, Username: "new-owner", ConnectionID: "new-connection", UpdatedAt: 200, HookSource: true},
	}}
	stale := auditClientIdentity{UserID: 1, Username: "old-owner", ConnectionID: "old-connection", UpdatedAt: 100}

	service.updateClientIdentity("upsert", stale, "10.8.0.2")
	identity, ok := service.clientIdentity("10.8.0.2")
	if !ok || identity.Username != "new-owner" || identity.ConnectionID != "new-connection" {
		t.Fatalf("stale upsert replaced newer connection: %#v, ok=%v", identity, ok)
	}

	service.updateClientIdentity("delete", stale, "10.8.0.2")
	identity, ok = service.clientIdentity("10.8.0.2")
	if !ok || identity.Username != "new-owner" || identity.ConnectionID != "new-connection" {
		t.Fatalf("stale delete removed newer connection: %#v, ok=%v", identity, ok)
	}
}

func TestWebAuditForwardRateLimitIsPerClient(t *testing.T) {
	run := &webAuditDNSRun{}
	service := &webAuditDNSService{
		run:         run,
		forwardRate: make(map[string]forwardRateBucket),
		status:      WebAuditDNSStatus{Enabled: true},
	}
	for i := 0; i < int(webAuditForwardPerClientBurst); i++ {
		if !service.allowForward(run, "10.8.0.2") {
			t.Fatalf("request %d should fit inside per-client burst", i+1)
		}
	}
	if service.allowForward(run, "10.8.0.2") {
		t.Fatal("request beyond per-client burst was unexpectedly accepted")
	}
	if !service.allowForward(run, "10.8.0.3") {
		t.Fatal("a different VPN client was blocked by another client's rate limit")
	}
	if service.status.DroppedDNSRequests != 1 {
		t.Fatalf("dropped DNS requests=%d, want 1", service.status.DroppedDNSRequests)
	}
}

func TestReserveAuditStorageDropsOnlyAuditEventsAtCapacity(t *testing.T) {
	run := &webAuditDNSRun{}
	service := &webAuditDNSService{
		run:                  run,
		storedAuditRows:      webAuditMaxStoredRows,
		storageInitialized:   true,
		storageCheckedMinute: time.Now().Unix() / 60,
		status:               WebAuditDNSStatus{Enabled: true},
	}
	if service.reserveAuditStorage(run) {
		t.Fatal("storage reservation unexpectedly succeeded at capacity")
	}
	if !service.status.StorageLimitReached || service.status.DroppedAuditEvents != 1 {
		t.Fatalf("capacity status=%#v, want limit reached with one dropped audit event", service.status)
	}
}

func TestAuditRedirectRuleArgsRecognizesOwnedAndLegacyRules(t *testing.T) {
	for _, owned := range []string{
		"-A PREROUTING -i tun0 -p udp -m comment --comment openvpn-web:web-audit:dns-redirect -d 8.8.8.8 --dport 53 -j REDIRECT --to-ports 5353",
		// A stale commented rule must be removable even if an old release used
		// a different interface/port shape. The unique comment is ownership.
		"-A PREROUTING -i eth0 -p udp -m comment --comment openvpn-web:web-audit:dns-redirect --dport 54 -j REDIRECT --to-ports 5353",
		// Pre-comment releases created exactly this broad tunnel DNS redirect.
		"-A PREROUTING -i tun0 -p tcp --dport 53 -j REDIRECT --to-ports 5353",
	} {
		rule, ok := auditRedirectRuleArgs(owned)
		if !ok {
			t.Fatalf("expected removable audit redirect rule for %q", owned)
		}
		if !strings.HasPrefix(strings.Join(rule, " "), "PREROUTING ") {
			t.Fatalf("unexpected removable rule args: %q", strings.Join(rule, " "))
		}
	}

	for _, line := range []string{
		// External redirects without our owner comment only remain removable when
		// they exactly match the old web-audit tun0/53/5353 signature above.
		"-A PREROUTING -d 8.8.8.8/32 -i tun0 -p udp -m udp --dport 53 -j REDIRECT --to-ports 5354",
		"-A PREROUTING -i eth0 -p udp --dport 53 -j REDIRECT --to-ports 5353",
		"-A PREROUTING -i tun0 -p udp -m comment --comment other-component --dport 53 -j REDIRECT --to-ports 5353",
		"-A OUTPUT -i tun0 -p udp -m comment --comment openvpn-web:web-audit:dns-redirect --dport 53 -j REDIRECT --to-ports 5353",
	} {
		if rule, ok := auditRedirectRuleArgs(line); ok {
			t.Fatalf("unexpectedly accepted non-owned rule %q as %v", line, rule)
		}
	}
}

func TestLegacyQUICBlockIsRetiredAndCannotBeEnabled(t *testing.T) {
	const key = "system.base.web_audit_block_udp_443"
	previous := viper.GetBool(key)
	t.Cleanup(func() { viper.Set(key, previous) })

	viper.Set(key, true)
	if !retireWebAuditQUICBlock() {
		t.Fatal("legacy enabled QUIC setting must be migrated")
	}
	if viper.GetBool(key) {
		t.Fatal("legacy QUIC setting remained enabled after migration")
	}
	viper.Set(key, true)
	if webAuditBlockUDP443() {
		t.Fatal("runtime must never re-enable the retired QUIC block")
	}
}

func TestHighCoverageAuditFirewallRulesStayOnTun0(t *testing.T) {
	// The DNS listener is bound to tun0 at the socket layer. Firewall rules are
	// therefore exclusively tunnel-matching and never use a broad "! -i tun0"
	// guard that could touch host or Docker bridge traffic.

	normalV4 := auditRedirectRules(false, []string{"8.8.8.8", "2001:4860:4860::8888", "not-an-ip"}, false)
	normalV6 := auditRedirectRules(true, []string{"8.8.8.8", "2001:4860:4860::8888", "not-an-ip"}, false)
	strictV4 := auditRedirectRules(false, nil, true)
	strictV6 := auditRedirectRules(true, nil, true)
	if len(normalV4) != 2 || len(normalV6) != 2 || len(strictV4) != 2 || len(strictV6) != 2 {
		t.Fatalf("unexpected DNS rule counts: normalV4=%v normalV6=%v strictV4=%v strictV6=%v", normalV4, normalV6, strictV4, strictV6)
	}
	for _, rule := range append(append([][]string{}, strictV4...), strictV6...) {
		joined := strings.Join(rule, " ")
		if !strings.Contains(joined, "-i tun0") || strings.Contains(joined, " -d ") || !strings.Contains(joined, webAuditRedirectComment) || !strings.Contains(joined, "--dport 53") {
			t.Fatalf("strict DNS rule is not a tun0-only all-resolver rule: %q", joined)
		}
	}
	for _, rule := range normalV4 {
		joined := strings.Join(rule, " ")
		if !strings.Contains(joined, "-i tun0") || !strings.Contains(joined, "-d 8.8.8.8") || strings.Contains(joined, "2001:4860") || !strings.Contains(joined, webAuditRedirectComment) {
			t.Fatalf("unexpected IPv4 resolver-scoped rule: %q", joined)
		}
	}
	for _, rule := range normalV6 {
		joined := strings.Join(rule, " ")
		if !strings.Contains(joined, "-i tun0") || !strings.Contains(joined, "-d 2001:4860:4860::8888") || strings.Contains(joined, "8.8.8.8") || !strings.Contains(joined, webAuditRedirectComment) {
			t.Fatalf("unexpected IPv6 resolver-scoped rule: %q", joined)
		}
	}

	for _, want := range []struct{ kind, proto, port string }{{webAuditDoTBlockComment, "tcp", "853"}, {webAuditQUICBlockComment, "udp", "443"}} {
		rules := auditEgressBlockRules(want.kind)
		if len(rules) != 1 {
			t.Fatalf("%s rules=%v", want.kind, rules)
		}
		joined := strings.Join(rules[0], " ")
		if !strings.Contains(joined, "FORWARD -i tun0") || !strings.Contains(joined, "-p "+want.proto) || !strings.Contains(joined, "--dport "+want.port) || !strings.Contains(joined, want.kind) || !strings.Contains(joined, "-j REJECT") {
			t.Fatalf("egress block escaped its strict tun0 boundary: %q", joined)
		}
		if want.kind == webAuditQUICBlockComment && webAuditBlockUDP443() {
			t.Fatal("legacy QUIC block must never be enabled")
		}
	}
}

func TestAuditIPTablesCandidatesCoverLegacyNftAndDefault(t *testing.T) {
	for _, test := range []struct {
		ipv6 bool
		want []string
	}{
		{want: []string{"iptables-legacy", "iptables-nft", "iptables"}},
		{ipv6: true, want: []string{"ip6tables-legacy", "ip6tables-nft", "ip6tables"}},
	} {
		got := auditIPTablesCandidates(test.ipv6)
		if strings.Join(got, ",") != strings.Join(test.want, ",") {
			t.Fatalf("ipv6=%v candidates=%v, want %v", test.ipv6, got, test.want)
		}
	}
}

func TestAuditRuleReconciliationPlanKeepsForwardChainWhenDisabling(t *testing.T) {
	dotRules := auditEgressBlockRules(webAuditDoTBlockComment)
	chain, desired := auditRuleReconciliationPlan(false, dotRules)
	if chain != "FORWARD" {
		t.Fatalf("disabled DoT cleanup chain=%q, want FORWARD", chain)
	}
	if desired != nil {
		t.Fatalf("disabled cleanup desired rules=%v, want nil", desired)
	}

	chain, desired = auditRuleReconciliationPlan(false, auditRedirectRules(false, []string{"8.8.8.8"}, false))
	if chain != "PREROUTING" || desired != nil {
		t.Fatalf("disabled DNS redirect cleanup plan=(%q, %v), want (PREROUTING, nil)", chain, desired)
	}
}

func TestAuditRuleConvergenceRemovesStrictAndDisabledRules(t *testing.T) {
	strict := auditRedirectRules(false, nil, true)
	normal := auditRedirectRules(false, []string{"1.1.1.1"}, false)
	remove, add := reconcileAuditRules(strict, normal)
	if len(remove) != len(strict) || len(add) != len(normal) {
		t.Fatalf("strict->normal must remove all broad rules and add all resolver rules: remove=%v add=%v", remove, add)
	}
	for _, rule := range remove {
		if strings.Contains(strings.Join(rule, " "), " -d ") {
			t.Fatalf("strict cleanup unexpectedly selected a resolver-scoped rule: %v", rule)
		}
	}
	current := append(append([][]string{}, normal...), auditEgressBlockRules(webAuditDoTBlockComment)...)
	remove, add = reconcileAuditRules(current, nil)
	if len(remove) != len(current) || len(add) != 0 {
		t.Fatalf("disable must remove every feature-owned desired rule: remove=%v add=%v", remove, add)
	}

	// Repeated reconciliation must also converge duplicate rules left by an
	// interrupted upgrade or a backend switch.
	duplicated := append(append([][]string{}, normal...), normal[0])
	remove, add = reconcileAuditRules(duplicated, normal)
	if len(remove) != 1 || len(add) != 0 || strings.Join(remove[0], " ") != strings.Join(normal[0], " ") {
		t.Fatalf("duplicate rule was not selected for cleanup: remove=%v add=%v", remove, add)
	}
}

func TestWebAuditCoverageDiagnosticsExplainEncryptedDNSGaps(t *testing.T) {
	service := &webAuditDNSService{status: WebAuditDNSStatus{
		Enabled: true, IngressRestricted: true, IPv4ListenerReady: true, IPv4RedirectInstalled: true,
		StrictDNSCaptureEnabled: false, DoTBlockEnabled: false, UDP443BlockEnabled: false,
	}}
	service.updateCoverageLocked()
	joined := strings.Join(service.status.DetectedGaps, "\n")
	actions := strings.Join(service.status.RecommendedActions, "\n")
	for _, want := range []string{"硬编码其他 DNS", "DoT", "HTTP/3/QUIC"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("diagnostics missing %q: %s", want, joined)
		}
	}
	for _, want := range []string{"严格普通 DNS", "TCP/853", "UDP/443", "始终放行"} {
		if !strings.Contains(actions, want) {
			t.Fatalf("recommended actions missing %q: %s", want, actions)
		}
	}
	if !strings.Contains(service.status.CoverageNote, "不会解密 HTTPS") || strings.Contains(service.status.CoverageNote, "完整网页记录") {
		t.Fatalf("coverage note overpromises privacy/coverage: %q", service.status.CoverageNote)
	}

	// QUIC remains a coverage limitation because UDP/443 is intentionally
	// permitted for client compatibility. DoH over TCP/443 is also explicit.
	if !strings.Contains(joined, "Google") || !strings.Contains(joined, "DoH（TCP/443）") || strings.Contains(joined, "完整网页") {
		t.Fatalf("QUIC compatibility diagnostics are incomplete: %s", joined)
	}
}
