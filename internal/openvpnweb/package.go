package openvpnweb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
	"gorm.io/gorm"
)

type ClientPackage struct {
	ID         uint      `gorm:"primarykey" json:"id"`
	Platform   string    `gorm:"size:32;not null;index" json:"platform"`
	Version    string    `gorm:"size:32;not null" json:"version"`
	Filename   string    `gorm:"size:255;not null" json:"filename"`
	StoredName string    `gorm:"size:255;not null" json:"storedName"`
	FileSize   int64     `json:"fileSize"`
	IsActive   bool      `gorm:"default:false" json:"isActive"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

var platformLabelMap = map[string]string{
	"windows": "Windows",
	"macos":   "macOS",
	"linux":   "Linux",
	"android": "Android",
	"ios":     "iOS",
}

// PlatformLabel 返回平台的友好中文/英文展示名，未知平台返回原值
func PlatformLabel(platform string) string {
	if v, ok := platformLabelMap[platform]; ok {
		return v
	}
	return platform
}

func (ClientPackage) TableName() string { return "client_packages" }

func (p *ClientPackage) BeforeSave(tx *gorm.DB) error {
	p.Platform = strings.TrimSpace(p.Platform)
	p.Version = strings.TrimSpace(p.Version)
	p.Filename = strings.TrimSpace(p.Filename)
	return nil
}

func (p *ClientPackage) Validate() error {
	if p.Platform == "" {
		return errors.New("平台不能为空")
	}
	validPlatforms := map[string]bool{"windows": true, "macos": true, "linux": true, "android": true, "ios": true}
	if !validPlatforms[p.Platform] {
		return fmt.Errorf("不支持的平台类型: %s", p.Platform)
	}
	if p.Version == "" {
		return errors.New("版本号不能为空")
	}
	if p.Filename == "" {
		return errors.New("文件名不能为空")
	}
	return nil
}

func (p *ClientPackage) PackagesDir() string {
	return filepath.Join(ovData, "client-packages")
}

func (p *ClientPackage) PlatformDir() string {
	return filepath.Join(p.PackagesDir(), p.Platform)
}

func (p *ClientPackage) FullPath() string {
	return filepath.Join(p.PlatformDir(), p.StoredName)
}

func (p *ClientPackage) PublicDownloadURL() string {
	siteURL := viper.GetString("system.base.site_url")
	if siteURL == "" {
		return ""
	}
	return strings.TrimRight(siteURL, "/") + "/ovpn/public/packages/" + fmt.Sprintf("%d", p.ID) + "/download"
}

// AdminDownloadURL 管理员后台使用的需要鉴权的下载地址（保留以兼容现有下载按钮）
func (p *ClientPackage) AdminDownloadURL() string {
	siteURL := viper.GetString("system.base.site_url")
	if siteURL == "" {
		return ""
	}
	return strings.TrimRight(siteURL, "/") + "/ovpn/client-packages/" + fmt.Sprintf("%d", p.ID) + "/download"
}

func (p *ClientPackage) All() []ClientPackage {
	out := make([]ClientPackage, 0)
	if db == nil {
		return out
	}
	if err := db.WithContext(context.Background()).Order("platform ASC, is_active DESC, id DESC").Find(&out).Error; err != nil {
		logger.Error(context.Background(), "query client packages failed: "+err.Error())
		return []ClientPackage{}
	}
	return out
}

func (p *ClientPackage) ActiveByPlatform(platform string) (ClientPackage, error) {
	var pkg ClientPackage
	if db == nil {
		return pkg, errors.New("数据库未初始化")
	}
	err := db.WithContext(context.Background()).Where("platform = ? AND is_active = ?", platform, true).First(&pkg).Error
	if err != nil {
		return pkg, err
	}
	return pkg, nil
}

func (p *ClientPackage) ActivesByPlatforms() map[string]ClientPackage {
	result := make(map[string]ClientPackage)
	if db == nil {
		return result
	}
	var packages []ClientPackage
	if err := db.WithContext(context.Background()).Where("is_active = ?", true).Find(&packages).Error; err != nil {
		logger.Error(context.Background(), "query active client packages failed: "+err.Error())
		return result
	}
	for _, pkg := range packages {
		result[pkg.Platform] = pkg
	}
	return result
}

func (p *ClientPackage) Get(id uint) (ClientPackage, error) {
	var pkg ClientPackage
	if db == nil {
		return pkg, errors.New("数据库未初始化")
	}
	if err := db.WithContext(context.Background()).First(&pkg, id).Error; err != nil {
		return pkg, err
	}
	return pkg, nil
}

func (p *ClientPackage) Create(filePath string) error {
	if err := p.Validate(); err != nil {
		return err
	}

	if db == nil {
		return errors.New("数据库未初始化")
	}

	dir := p.PlatformDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建存储目录失败: %w", err)
	}

	ext := filepath.Ext(p.Filename)
	uuid := generateUUID()
	p.StoredName = uuid + ext

	destPath := filepath.Join(dir, p.StoredName)
	if err := copyFile(filePath, destPath); err != nil {
		return fmt.Errorf("保存文件失败: %w", err)
	}

	info, err := os.Stat(destPath)
	if err == nil {
		p.FileSize = info.Size()
	}

	return db.WithContext(context.Background()).Create(p).Error
}

func (p *ClientPackage) Activate(id uint) error {
	if db == nil {
		return errors.New("数据库未初始化")
	}

	var pkg ClientPackage
	if err := db.WithContext(context.Background()).First(&pkg, id).Error; err != nil {
		return err
	}

	tx := db.Begin()
	if err := tx.Model(&ClientPackage{}).Where("platform = ?", pkg.Platform).Update("is_active", false).Error; err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Model(&pkg).Update("is_active", true).Error; err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}

func (p *ClientPackage) Delete() error {
	if p.ID == 0 {
		return errors.New("缺少安装包 ID")
	}

	pkg, err := p.Get(p.ID)
	if err != nil {
		return err
	}

	if err := db.WithContext(context.Background()).Delete(&pkg).Error; err != nil {
		return err
	}

	fullPath := pkg.FullPath()
	if fullPath != "" {
		os.Remove(fullPath)
	}

	return nil
}

func (p *ClientPackage) DeleteByID(id uint) error {
	pkg := &ClientPackage{}
	*pkg = ClientPackage{ID: id}
	return pkg.Delete()
}

func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func InitClientPackagesDir() {
	dir := filepath.Join(ovData, "client-packages")
	platforms := []string{"windows", "macos", "linux", "android", "ios"}
	for _, p := range platforms {
		os.MkdirAll(filepath.Join(dir, p), 0755)
	}
}

func GetActivePackagesByPlatform() map[string]ClientPackage {
	pkg := &ClientPackage{}
	return pkg.ActivesByPlatforms()
}