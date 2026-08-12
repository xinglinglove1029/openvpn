package main

import (
	"fmt"
	"os"

	gormlogger "gorm.io/gorm/logger"
	"openvpn-web/internal/openvpnweb"
)

func main() {
	os.Setenv("OVPN_DATA", "F:/develop/openvpn/data")

	check("mysql", openvpnweb.DatabaseConfig{
		Type: "mysql", Host: "10.100.100.168", Port: 3306,
		User: "ymeet", Password: "xiaobingbing123456", Name: "yb_meeting",
		Charset: "utf8mb4",
	})
	check("postgres", openvpnweb.DatabaseConfig{
		Type: "postgres", Host: "10.50.16.133", Port: 30432,
		User: "postgres", Password: "LrUoXQCJAdFkbtrG5DaGFztz", Name: "medipaas",
		SSLMode: "disable",
	})
}

func check(label string, cfg openvpnweb.DatabaseConfig) {
	fmt.Printf("=== %s (%s@%s:%d/%s) ===\n", label, cfg.User, cfg.Host, cfg.Port, cfg.Name)
	db, err := openvpnweb.OpenDatabase(cfg, "", gormlogger.Default)
	if err != nil {
		fmt.Printf("OpenDatabase FAIL: %v\n", err)
		return
	}
	sqlDB, err := db.DB()
	if err != nil {
		fmt.Printf("db.DB FAIL: %v\n", err)
		return
	}
	defer sqlDB.Close()
	if err := sqlDB.Ping(); err != nil {
		fmt.Printf("PING FAIL: %v\n", err)
		return
	}
	fmt.Println("PING OK, dialect =", db.Dialector.Name())

	var version string
	if db.Dialector.Name() == "mysql" {
		db.Raw("SELECT VERSION()").Scan(&version)
		// 查看目标库中已存在的表，评估 AutoMigrate 冲突风险
		type tname struct{ Tables string }
		var tables []tname
		if err := db.Raw("SELECT table_name AS tables FROM information_schema.tables WHERE table_schema = DATABASE() ORDER BY table_name").Scan(&tables).Error; err != nil {
			fmt.Println("list tables FAIL:", err)
		} else {
			fmt.Printf("existing tables (%d): ", len(tables))
			for i, t := range tables {
				fmt.Print(t.Tables)
				if i != len(tables)-1 {
					fmt.Print(", ")
				}
			}
			fmt.Println()
		}
		var grants string
		db.Raw("SELECT GROUP_CONCAT(privilege_type) FROM information_schema.user_privileges WHERE grantee LIKE '%ymeeet%'").Scan(&grants)
		fmt.Println("user privileges:", grants)
	} else {
		db.Raw("SELECT version()").Scan(&version)
		type tname struct{ Tables string }
		var tables []tname
		if err := db.Raw("SELECT tablename AS tables FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename").Scan(&tables).Error; err != nil {
			fmt.Println("list tables FAIL:", err)
		} else {
			fmt.Printf("existing tables (%d): ", len(tables))
			for i, t := range tables {
				fmt.Print(t.Tables)
				if i != len(tables)-1 {
					fmt.Print(", ")
				}
			}
			fmt.Println()
		}
	}
	fmt.Println("server version:", version)
	fmt.Println()
}
