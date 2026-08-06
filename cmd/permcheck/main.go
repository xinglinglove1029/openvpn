package main

import (
	"context"
	"fmt"
	"os"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type PermRow struct {
	ID       uint
	ParentID *uint
	Code     string
	Name     string
	Type     string
	Sort     int
	Path     string
	Icon     string
}

func (PermRow) TableName() string { return "permission" }

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: permcheck <path-to-db>")
		return
	}
	db, err := gorm.Open(sqlite.Open(os.Args[1]), &gorm.Config{})
	if err != nil {
		fmt.Println("Open error:", err)
		return
	}
	ctx := context.Background()

	var total int64
	db.WithContext(ctx).Model(&PermRow{}).Count(&total)
	fmt.Printf("permission 表总记录: %d\n", total)

	var rows []PermRow
	db.WithContext(ctx).Order("parent_id IS NULL, sort, id").Find(&rows)
	fmt.Println("\n===== 所有记录（按 sort 排序）=====")
	for _, r := range rows {
		pid := "NULL"
		if r.ParentID != nil {
			pid = fmt.Sprintf("%d", *r.ParentID)
		}
		fmt.Printf("  id=%-4d parent=%-6s code=%-32s type=%-7s sort=%-3d path=%-14s name=%s\n",
			r.ID, pid, r.Code, r.Type, r.Sort, r.Path, r.Name)
	}

	var menuCount int64
	db.WithContext(ctx).Model(&PermRow{}).Where("type = ?", "menu").Count(&menuCount)
	var rootMenu int64
	db.WithContext(ctx).Model(&PermRow{}).Where("type = ? AND (parent_id IS NULL OR parent_id = 0)", "menu").Count(&rootMenu)
	fmt.Printf("\n菜单节点总数(menu): %d, 根菜单: %d\n", menuCount, rootMenu)
}
