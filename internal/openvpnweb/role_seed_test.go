package openvpnweb

import (
	"testing"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func newRoleSeedTestDatabase(t *testing.T) *gorm.DB {
	t.Helper()

	database, err := OpenDatabase(DatabaseConfig{Type: "sqlite", Path: ":memory:"}, "", gormlogger.Default)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&Role{}, &Permission{}, &RolePermission{}); err != nil {
		t.Fatal(err)
	}

	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return database
}

func roleHasPermissionCode(t *testing.T, database *gorm.DB, roleID uint, code string) bool {
	t.Helper()

	return rolePermissionCodeCount(t, database, roleID, code) > 0
}

func rolePermissionCodeCount(t *testing.T, database *gorm.DB, roleID uint, code string) int64 {
	t.Helper()

	var count int64
	if err := database.Table(RolePermission{}.TableName()+" AS rp").
		Joins("JOIN "+Permission{}.TableName()+" AS p ON p.id = rp.permission_id").
		Where("rp.role_id = ? AND p.code = ?", roleID, code).
		Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func rolePermissionSeedStateCount(t *testing.T, database *gorm.DB, roleID uint, seedCode string) int64 {
	t.Helper()

	var count int64
	if err := database.Model(&RolePermissionSeedState{}).
		Where("role_id = ? AND seed_code = ?", roleID, seedCode).
		Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func TestSeedPermissionsAndRolesAssignsWebAuditPermissionsToNewBuiltinUser(t *testing.T) {
	database := newRoleSeedTestDatabase(t)

	if err := SeedPermissionsAndRoles(database); err != nil {
		t.Fatal(err)
	}

	var userRole Role
	if err := database.Where("code = ?", BuiltinRoleUser).First(&userRole).Error; err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"menu:web-audit", "web-audit:view"} {
		if !roleHasPermissionCode(t, database, userRole.ID, code) {
			t.Fatalf("new built-in user role is missing %s", code)
		}
	}
}

func TestSeedPermissionsAndRolesBackfillsWebAuditPermissionsForExistingBuiltinUserIdempotently(t *testing.T) {
	database := newRoleSeedTestDatabase(t)
	enabled := true
	userRole := Role{
		Name:      "旧普通用户",
		Code:      BuiltinRoleUser,
		IsBuiltin: true,
		IsEnable:  &enabled,
	}
	if err := database.Create(&userRole).Error; err != nil {
		t.Fatal(err)
	}
	oldPermission := Permission{Name: "查看连接历史", Code: "history:view", Type: "button", IsBuiltin: true}
	if err := database.Create(&oldPermission).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&RolePermission{RoleID: userRole.ID, PermissionID: oldPermission.ID}).Error; err != nil {
		t.Fatal(err)
	}

	if err := SeedPermissionsAndRoles(database); err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"history:view", "menu:web-audit", "web-audit:view"} {
		if !roleHasPermissionCode(t, database, userRole.ID, code) {
			t.Fatalf("existing built-in user role is missing %s after backfill", code)
		}
	}

	if err := SeedPermissionsAndRoles(database); err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"menu:web-audit", "web-audit:view"} {
		if count := rolePermissionCodeCount(t, database, userRole.ID, code); count != 1 {
			t.Fatalf("%s association count = %d, want 1 after repeated seed", code, count)
		}
	}
	if count := rolePermissionSeedStateCount(t, database, userRole.ID, userWebAuditPermissionSeedCode); count != 1 {
		t.Fatalf("web audit seed state count = %d, want 1 after repeated seed", count)
	}

	var viewPermission Permission
	if err := database.Where("code = ?", "web-audit:view").First(&viewPermission).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Where("role_id = ? AND permission_id = ?", userRole.ID, viewPermission.ID).Delete(&RolePermission{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := SeedPermissionsAndRoles(database); err != nil {
		t.Fatal(err)
	}
	if roleHasPermissionCode(t, database, userRole.ID, "web-audit:view") {
		t.Fatal("explicitly revoked web audit permission was restored after seed")
	}
}

func TestSeedPermissionsAndRolesDoesNotGrantWebAuditPermissionsToCustomRoles(t *testing.T) {
	database := newRoleSeedTestDatabase(t)
	enabled := true
	customRole := Role{
		Name:      "自定义角色",
		Code:      "custom_operator",
		IsBuiltin: false,
		IsEnable:  &enabled,
	}
	if err := database.Create(&customRole).Error; err != nil {
		t.Fatal(err)
	}
	oldPermission := Permission{Name: "查看连接历史", Code: "history:view", Type: "button", IsBuiltin: true}
	if err := database.Create(&oldPermission).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&RolePermission{RoleID: customRole.ID, PermissionID: oldPermission.ID}).Error; err != nil {
		t.Fatal(err)
	}

	if err := SeedPermissionsAndRoles(database); err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"menu:web-audit", "web-audit:view"} {
		if roleHasPermissionCode(t, database, customRole.ID, code) {
			t.Fatalf("custom role unexpectedly received %s", code)
		}
	}
}
