package openvpnweb

import (
	"strings"
	"testing"

	gormlogger "gorm.io/gorm/logger"
)

// 注意：本包测试与存量测试一致，需要 OVPN_DATA 指向含 config.json 的目录（如仓库根 data/）。
// 运行方式：OVPN_DATA=<仓库>/data go test ./internal/openvpnweb/

func TestDialectAndDSNDefaultSQLite(t *testing.T) {
	dialect, dsn, err := dialectAndDSN(DatabaseConfig{}, "F:\\data")
	if err != nil {
		t.Fatal(err)
	}
	if dialect != "sqlite" {
		t.Fatalf("dialect = %q, want sqlite", dialect)
	}
	want := "F:\\data\\ovpn.db?_pragma=foreign_keys(1)"
	if dsn != want {
		t.Fatalf("dsn = %q, want %q", dsn, want)
	}
}

func TestDialectAndDSNSQLiteAbsolutePath(t *testing.T) {
	dialect, dsn, err := dialectAndDSN(DatabaseConfig{Type: "sqlite", Path: "C:\\tmp\\my.db"}, "F:\\data")
	if err != nil {
		t.Fatal(err)
	}
	if dialect != "sqlite" {
		t.Fatalf("dialect = %q, want sqlite", dialect)
	}
	if !strings.HasPrefix(dsn, "C:\\tmp\\my.db") {
		t.Fatalf("absolute path not preserved: %q", dsn)
	}
}

func TestDialectAndDSNMySQL(t *testing.T) {
	dialect, dsn, err := dialectAndDSN(DatabaseConfig{
		Type:     "mysql",
		Host:     "db.internal",
		User:     "admin",
		Password: "p@ss",
		Name:     "openvpn",
	}, "F:\\data")
	if err != nil {
		t.Fatal(err)
	}
	if dialect != "mysql" {
		t.Fatalf("dialect = %q, want mysql", dialect)
	}
	// 默认端口 3306、默认字符集 utf8mb4、密码特殊字符被转义
	if !strings.Contains(dsn, "@tcp(db.internal:3306)/openvpn?") {
		t.Fatalf("unexpected mysql dsn: %q", dsn)
	}
	if !strings.Contains(dsn, "charset=utf8mb4") || !strings.Contains(dsn, "parseTime=True") {
		t.Fatalf("unexpected mysql dsn params: %q", dsn)
	}
	// go-sql-driver 的 ParseDSN 按最后一个 @ 切分密码，密码含 @ 仍可正确解析
	if !strings.HasPrefix(dsn, "admin:p@ss@tcp(db.internal:3306)/openvpn?") {
		t.Fatalf("password with @ not preserved: %q", dsn)
	}
}

func TestDialectAndDSNMySQLCustomPort(t *testing.T) {
	_, dsn, err := dialectAndDSN(DatabaseConfig{
		Type: "mysql", Host: "127.0.0.1", Port: 3307, User: "u", Password: "p", Name: "n",
	}, "F:\\data")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dsn, "@tcp(127.0.0.1:3307)/n?") {
		t.Fatalf("custom port not applied: %q", dsn)
	}
}

func TestDialectAndDSNPostgres(t *testing.T) {
	dialect, dsn, err := dialectAndDSN(DatabaseConfig{
		Type:     "postgres",
		Host:     "127.0.0.1",
		Port:     0,
		User:     "postgres",
		Password: "p@ss",
		Name:     "openvpn",
		SSLMode:  "require",
	}, "F:\\data")
	if err != nil {
		t.Fatal(err)
	}
	if dialect != "postgres" {
		t.Fatalf("dialect = %q, want postgres", dialect)
	}
	// 默认端口 5432、sslmode=require、密码 URL 编码
	if !strings.Contains(dsn, "postgres://postgres:p%40ss@127.0.0.1:5432/openvpn") {
		t.Fatalf("unexpected postgres dsn: %q", dsn)
	}
	if !strings.Contains(dsn, "sslmode=require") {
		t.Fatalf("sslmode not applied: %q", dsn)
	}
}

func TestDialectAndDSNUnknown(t *testing.T) {
	if _, _, err := dialectAndDSN(DatabaseConfig{Type: "oracle"}, "F:\\data"); err == nil {
		t.Fatal("expected error for unsupported dialect")
	}
}

func TestDialectAndDSNTypeTrimmed(t *testing.T) {
	dialect, _, err := dialectAndDSN(DatabaseConfig{Type: " mysql "}, "F:\\data")
	if err != nil {
		t.Fatal(err)
	}
	if dialect != "mysql" {
		t.Fatalf("dialect = %q, want mysql (type should be trimmed)", dialect)
	}
}

func TestDialectAndDSNNegativePort(t *testing.T) {
	if _, _, err := dialectAndDSN(DatabaseConfig{Type: "mysql", Port: -1}, "F:\\data"); err == nil {
		t.Fatal("expected error for negative port")
	}
}

func TestDialectAndDSNMySQLIPv6(t *testing.T) {
	_, dsn, err := dialectAndDSN(DatabaseConfig{
		Type: "mysql", Host: "::1", User: "u", Password: "p", Name: "n",
	}, "F:\\data")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dsn, "@tcp([::1]:3306)/n?") {
		t.Fatalf("ipv6 host not bracketed: %q", dsn)
	}
}

func TestDialectAndDSNPostgresIPv6(t *testing.T) {
	_, dsn, err := dialectAndDSN(DatabaseConfig{
		Type: "postgres", Host: "::1", User: "u", Password: "p", Name: "n",
	}, "F:\\data")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dsn, "postgres://u:p@[::1]:5432/n") {
		t.Fatalf("ipv6 host not bracketed: %q", dsn)
	}
}

func TestDialectAndDSNPostgresSpecialCharsInName(t *testing.T) {
	_, dsn, err := dialectAndDSN(DatabaseConfig{
		Type: "postgres", Host: "127.0.0.1", User: "u", Password: "p", Name: "my db/1",
	}, "F:\\data")
	if err != nil {
		t.Fatal(err)
	}
	// url.URL.String() 会对 Path 中的保留字符做 percent-encode
	if !strings.Contains(dsn, "/my%20db%2F1") {
		t.Fatalf("special chars in db name not escaped: %q", dsn)
	}
}

func TestDialectAndDSNSQLitePathWithQuestionMark(t *testing.T) {
	if _, _, err := dialectAndDSN(DatabaseConfig{Type: "sqlite", Path: "C:\\tmp\\my?db.db"}, "F:\\data"); err == nil {
		t.Fatal("expected error for sqlite path containing '?'")
	}
}

func TestOpenDatabaseSQLiteInMemory(t *testing.T) {
	db, err := OpenDatabase(DatabaseConfig{Type: "sqlite", Path: ":memory:"}, "", gormlogger.Default)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("ping failed: %v", err)
	}
	if db.Dialector.Name() != "sqlite" {
		t.Fatalf("dialect = %q, want sqlite", db.Dialector.Name())
	}
}

func TestQuoteIdentSQLite(t *testing.T) {
	db, err := OpenDatabase(DatabaseConfig{Type: "sqlite", Path: ":memory:"}, "", gormlogger.Default)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"group", "user", "history"} {
		got := quoteIdent(db, name)
		if got == name {
			t.Fatalf("quoteIdent(%q) = %q, expected quoting", name, got)
		}
		if !strings.Contains(got, name) {
			t.Fatalf("quoteIdent(%q) = %q, expected to contain %q", name, got, name)
		}
	}
}

func TestColumnExistsSQLite(t *testing.T) {
	db, err := OpenDatabase(DatabaseConfig{Type: "sqlite", Path: ":memory:"}, "", gormlogger.Default)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)").Error; err != nil {
		t.Fatal(err)
	}
	ok, err := columnExists(db, "users", "name")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("column name should exist")
	}
	ok, err = columnExists(db, "users", "missing")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("column missing should not exist")
	}
}

func TestInsertIgnoreSQLite(t *testing.T) {
	db, err := OpenDatabase(DatabaseConfig{Type: "sqlite", Path: ":memory:"}, "", gormlogger.Default)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT UNIQUE)").Error; err != nil {
		t.Fatal(err)
	}
	insertIgnore(db, "INTO t (id, v) VALUES (1, 'a')")
	res := insertIgnore(db, "INTO t (id, v) VALUES (1, 'a')")
	if res.Error != nil {
		t.Fatal(res.Error)
	}
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM t").Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("insert ignore should not duplicate rows, count = %d", count)
	}
}

func TestIndexExistsSQLite(t *testing.T) {
	db, err := OpenDatabase(DatabaseConfig{Type: "sqlite", Path: ":memory:"}, "", gormlogger.Default)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE t (a TEXT, b TEXT)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE INDEX idx_t_ab ON t (a, b)").Error; err != nil {
		t.Fatal(err)
	}
	ok, err := indexExists(db, "t", "idx_t_ab")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("index idx_t_ab should exist")
	}
	ok, err = indexExists(db, "t", "idx_missing")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("index idx_missing should not exist")
	}
}
