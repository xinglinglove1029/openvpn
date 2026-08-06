package main

import (
	"context"
	"fmt"
	"os"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: notifycheck <path-to-db>")
		return
	}
	dbPath := os.Args[1]

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		fmt.Println("Open error:", err)
		return
	}

	ctx := context.Background()

	// 列出所有表
	var tables []string
	db.WithContext(ctx).Raw("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name").Scan(&tables)
	fmt.Println("===== 数据库中的所有表 =====")
	for _, t := range tables {
		fmt.Printf("  %s\n", t)
	}

	// 查询每个表的记录数
	fmt.Println("\n===== 各表记录数 =====")
	for _, t := range tables {
		var count int64
		db.WithContext(ctx).Raw(fmt.Sprintf("SELECT COUNT(*) FROM %s", t)).Scan(&count)
		fmt.Printf("  %s: %d 条\n", t, count)
	}

	// 如果有 user_notify_read 表，查看记录
	for _, t := range tables {
		if t == "user_notify_read" {
			type Rec struct {
				ID         uint
				UserID     uint
				Username   string
				Scope      string
				LastReadID uint
			}
			var recs []Rec
			db.WithContext(ctx).Raw("SELECT * FROM user_notify_read").Scan(&recs)
			fmt.Println("\n===== user_notify_read 记录 =====")
			for _, r := range recs {
				fmt.Printf("  id=%d user_id=%d username=%q scope=%q lastReadID=%d\n", r.ID, r.UserID, r.Username, r.Scope, r.LastReadID)
			}
		}
		if t == "notify_logs" {
			var count int64
			db.WithContext(ctx).Raw("SELECT COUNT(*) FROM notify_logs").Scan(&count)
			var maxID *uint
			db.WithContext(ctx).Raw("SELECT MAX(id) FROM notify_logs").Scan(&maxID)
			fmt.Println("\n===== notify_logs 统计 =====")
			fmt.Printf("  总数: %d\n", count)
			if maxID != nil {
				fmt.Printf("  最大 ID: %d\n", *maxID)
			}
		}
	}
}
