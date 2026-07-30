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
		fmt.Println("Usage: dbcheck <path-to-db>")
		return
	}
	dbPath := os.Args[1]

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		fmt.Println("Open error:", err)
		return
	}

	var auditZeroCount int64
	db.WithContext(context.Background()).Table("audit_logs").Where("operator_id = ?", 0).Count(&auditZeroCount)

	var historyZeroCount int64
	db.WithContext(context.Background()).Table("history").Where("user_id = ?", 0).Count(&historyZeroCount)

	fmt.Printf("audit_logs operator_id=0: %d\n", auditZeroCount)
	fmt.Printf("history user_id=0: %d\n", historyZeroCount)
}
