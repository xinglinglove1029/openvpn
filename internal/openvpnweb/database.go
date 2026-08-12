package openvpnweb

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// dialectAndDSN 根据配置返回数据库方言与连接 DSN（纯函数，便于单测）。
func dialectAndDSN(cfg DatabaseConfig, dataDir string) (dialect string, dsn string, err error) {
	if cfg.Port < 0 {
		return "", "", fmt.Errorf("数据库端口不能为负数: %d", cfg.Port)
	}

	switch strings.ToLower(strings.TrimSpace(cfg.Type)) {
	case "", "sqlite":
		p := cfg.Path
		if p == "" {
			p = "ovpn.db"
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(dataDir, p)
		}
		if strings.ContainsAny(p, "?#") {
			return "", "", fmt.Errorf("sqlite 文件路径不能包含 '?' 或 '#' 字符: %q", p)
		}
		return "sqlite", p + "?_pragma=foreign_keys(1)", nil
	case "mysql":
		port := cfg.Port
		if port == 0 {
			port = 3306
		}
		charset := cfg.Charset
		if charset == "" {
			charset = "utf8mb4"
		}
		mc := mysqlDriver.Config{
			User:   cfg.User,
			Passwd: cfg.Password,
			Net:    "tcp",
			Addr:   net.JoinHostPort(cfg.Host, strconv.Itoa(port)), // 兼容 IPv6（[::1]:3306）
			DBName: cfg.Name,
			// go-sql-driver v1.8+ 默认拒绝 mysql_native_password；老账号（如 MySQL 5.7 / 8.0 默认插件）
			// 仍需该插件认证，显式开启以兼容。
			AllowNativePasswords: true,
			Params: map[string]string{
				"charset":   charset,
				"parseTime": "True",
				"loc":       "Local",
			},
		}
		return "mysql", mc.FormatDSN(), nil
	case "postgres":
		port := cfg.Port
		if port == 0 {
			port = 5432
		}
		sslmode := cfg.SSLMode
		if sslmode == "" {
			sslmode = "disable"
		}
		// 显式拼接并转义：用户名/密码用 UserPassword 转义，库名用 PathEscape（含 / ? 等字符）
		userinfo := url.UserPassword(cfg.User, cfg.Password).String()
		dsn := "postgres://" + userinfo + "@" + net.JoinHostPort(cfg.Host, strconv.Itoa(port)) + "/" + url.PathEscape(cfg.Name)
		q := url.Values{}
		q.Set("sslmode", sslmode)
		return "postgres", dsn + "?" + q.Encode(), nil
	default:
		return "", "", fmt.Errorf("不支持的数据库类型: %q（可选：sqlite / mysql / postgres）", cfg.Type)
	}
}

// OpenDatabase 按配置创建 GORM 数据库连接。
func OpenDatabase(cfg DatabaseConfig, dataDir string, log gormlogger.Interface) (*gorm.DB, error) {
	dialect, dsn, err := dialectAndDSN(cfg, dataDir)
	if err != nil {
		return nil, err
	}

	var d gorm.Dialector
	switch dialect {
	case "sqlite":
		d = sqlite.Open(dsn)
	case "mysql":
		d = mysql.Open(dsn)
	case "postgres":
		d = postgres.Open(dsn)
	}

	db, err := gorm.Open(d, &gorm.Config{Logger: log})
	if err != nil {
		return nil, err
	}

	if sqlDB, err := db.DB(); err == nil {
		if cfg.MaxIdleConns > 0 {
			sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
		}
		if cfg.MaxOpenConns > 0 {
			sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
		}
		if cfg.ConnMaxLifetime > 0 {
			sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Second)
		}
	}

	return db, nil
}

// sqlWriter 实现 gorm.io/gorm/clause.Writer，用于收集 Dialector.QuoteTo 输出的标识符。
type sqlWriter struct {
	strings.Builder
}

func (w *sqlWriter) WriteStringQuoted(s string) (int, error) {
	return w.WriteString(s)
}

// quoteIdent 按当前数据库方言为标识符（表名/列名）添加引号。
func quoteIdent(db *gorm.DB, name string) string {
	w := &sqlWriter{}
	db.Dialector.QuoteTo(w, name)
	return w.String()
}

// groupIdent / userIdent 返回按方言引号包裹的保留字表名（group、user）。
func groupIdent(db *gorm.DB) string { return quoteIdent(db, "group") }
func userIdent(db *gorm.DB) string  { return quoteIdent(db, "user") }

// columnExists 跨方言判断表列是否存在。
func columnExists(db *gorm.DB, table, column string) (bool, error) {
	var n int64
	switch db.Dialector.Name() {
	case "sqlite":
		// pragma_table_info 的表名参数须使用字符串字面量（反引号标识符在此处不被识别），单引号转义防止拼入
		escaped := strings.ReplaceAll(table, "'", "''")
		err := db.Raw("SELECT COUNT(*) FROM pragma_table_info('"+escaped+"') WHERE name = ?", column).Scan(&n).Error
		return n > 0, err
	case "mysql":
		err := db.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?", table, column).Scan(&n).Error
		return n > 0, err
	case "postgres":
		err := db.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?", table, column).Scan(&n).Error
		return n > 0, err
	default:
		return false, fmt.Errorf("不支持的数据库方言: %s", db.Dialector.Name())
	}
}

// indexExists 跨方言判断索引是否存在。
func indexExists(db *gorm.DB, table, index string) (bool, error) {
	var n int64
	switch db.Dialector.Name() {
	case "sqlite":
		err := db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND tbl_name = ? AND name = ?", table, index).Scan(&n).Error
		return n > 0, err
	case "mysql":
		err := db.Raw("SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?", table, index).Scan(&n).Error
		return n > 0, err
	case "postgres":
		err := db.Raw("SELECT COUNT(*) FROM pg_indexes WHERE schemaname = current_schema() AND tablename = ? AND indexname = ?", table, index).Scan(&n).Error
		return n > 0, err
	default:
		return false, fmt.Errorf("不支持的数据库方言: %s", db.Dialector.Name())
	}
}

// insertIgnore 按方言生成 INSERT ... IGNORE / OR IGNORE / ON CONFLICT DO NOTHING。
// intoSQL 形如 "INTO user_role (user_id, role_id, created_at) SELECT ..."。
func insertIgnore(db *gorm.DB, intoSQL string, values ...interface{}) *gorm.DB {
	switch db.Dialector.Name() {
	case "mysql":
		return db.Exec("INSERT IGNORE "+intoSQL, values...)
	case "postgres":
		return db.Exec("INSERT "+intoSQL+" ON CONFLICT DO NOTHING", values...)
	default: // sqlite
		return db.Exec("INSERT OR IGNORE "+intoSQL, values...)
	}
}
