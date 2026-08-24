package openvpnweb

import (
	"bytes"
	"context"
	"crypto/subtle"
	"crypto/x509"
	"embed"
	"encoding/csv"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"math/rand"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gavintan/gopkg/aes"
	"github.com/gavintan/gopkg/tools"
	"github.com/gin-contrib/sessions"
	gormsessions "github.com/gin-contrib/sessions/gorm"
	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
	"github.com/robfig/cron/v3"
	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	gLogger "gorm.io/gorm/logger"

	"openvpn-web/internal/openvpnweb/ai"
)

type ClientData struct {
	ID             string  `json:"id"`
	Rip            string  `json:"rip"`
	Vip            string  `json:"vip"`
	Vip6           string  `json:"vip6"`
	RecvBytes      float64 `json:"recvBytes"`
	SendBytes      float64 `json:"sendBytes"`
	ConnDate       string  `json:"connDate"`
	OnlineTime     string  `json:"onlineTime"`
	Username       string  `json:"username"`
	CommonName     string  `json:"commonName"`
	IsNftBlacklist bool    `json:"isNftBlacklist"`
}

type ServerData struct {
	RunDate    string
	Status     string
	StatusDesc string
	Address    string
	Nclients   string
	BytesIn    string
	BytesOut   string
	Mode       string
	Version    string
}

type ClientConfigData struct {
	Name     string `json:"name"`
	FullName string `json:"fullName"`
	File     string `json:"file"`
	Date     string `json:"date"`
}

type Params struct {
	Draw        int    `json:"draw" form:"draw"`
	Offset      int    `json:"offset" form:"offset"`
	Limit       int    `json:"limit" form:"limit"`
	OrderColumn string `json:"orderColumn" form:"orderColumn"`
	Order       string `json:"order" form:"order"`
	Search      string `json:"search" form:"search"`
	Qt          string `json:"qt" form:"qt"`
}

type CertData struct {
	Name            string `json:"name"`
	Type            string `json:"type"`
	Kind            string `json:"kind"`
	Subject         string `json:"subject"`
	Issuer          string `json:"issuer"`
	NotBefore       string `json:"notBefore"`
	NotAfter        string `json:"notAfter"`
	ExpiresIn       string `json:"expiresIn"`
	Status          string `json:"status"`
	Lifecycle       string `json:"lifecycle,omitempty"`
	Deletable       bool   `json:"deletable"`
	ProtectedReason string `json:"protectedReason,omitempty"`
	SerialNo        string `json:"serialNo"`
}

type ovpn struct {
	address string
}

type BuildInfo struct {
	Version string
	Commit  string
	Date    string
	BuiltBy string
}

var (
	version      = "1.0.0"
	commit       = "unknown"
	date         = "unknown"
	builtBy      = "source"
	assetVersion = fmt.Sprintf("v%s-%d", version, time.Now().Unix())
	//go:embed templates
	FS embed.FS

	cc     = cache.New(5*time.Minute, 10*time.Minute)
	db     *gorm.DB
	logger = gLogger.New(
		log.New(os.Stdout, "[OPENVPN-WEB] "+time.Now().Format("2006-01-02 15:04:05.000")+" MAIN ", 0),
		gLogger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  gLogger.Error,
			IgnoreRecordNotFoundError: true,
			Colorful:                  true,
		},
	)
	ovData = os.Getenv("OVPN_DATA")
	conf   config
)

// siteDownloadLandingURL 返回配置中 site_url 拼接落地页 /download 后的完整地址（供邮件按钮跳转）
// 空配置时返回空字符串，确保没有尾斜杠后拼接
func siteDownloadLandingURL() string {
	s := strings.TrimRight(viper.GetString("system.base.site_url"), "/")
	if s == "" {
		return ""
	}
	return s + "/download"
}

func (ov *ovpn) sendCommand(command string) (string, error) {
	var data string
	var sb strings.Builder

	conn, err := net.DialTimeout("tcp", ov.address, time.Second*3)
	if err != nil {
		logger.Error(context.Background(), err.Error())
		return data, err
	}

	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(time.Second * 3)); err != nil {
		return data, err
	}
	if _, err := conn.Write([]byte(fmt.Sprintf("%s\n", command))); err != nil {
		return data, err
	}

	infoLine := regexp.MustCompile(`>INFO.*\r?\n`)
	for {
		buf := make([]byte, 1024)
		n, readErr := conn.Read(buf)
		if n > 0 {
			if str := infoLine.ReplaceAllString(string(buf[:n]), ""); str != "" {
				sb.WriteString(str)
			}
		}

		response := sb.String()
		if strings.HasPrefix(response, "ERROR:") || strings.HasPrefix(response, "FAILURE:") {
			return "", fmt.Errorf("OpenVPN management command rejected: %s", strings.TrimSpace(response))
		}
		if strings.HasSuffix(response, "\r\nEND\r\n") || strings.HasSuffix(response, "\nEND\n") || strings.HasPrefix(response, "SUCCESS:") {
			break
		}
		if readErr != nil {
			return data, readErr
		}
	}

	data = strings.TrimPrefix(strings.TrimSuffix(strings.TrimSuffix(sb.String(), "\r\nEND\r\n"), "\r\n"), "SUCCESS: ")
	return data, nil
}

func (ov *ovpn) getClient() []ClientData {
	clients := make([]ClientData, 0)

	data, err := ov.sendCommand("status 3")
	if err != nil {
		return clients
	}

	for _, v := range strings.Split(data, "\r\n") {
		cdSlice := strings.Split(v, "\t")

		if cdSlice[0] == "CLIENT_LIST" {
			recv, _ := strconv.ParseFloat(cdSlice[5], 64)
			send, _ := strconv.ParseFloat(cdSlice[6], 64)
			connDate, _ := time.ParseInLocation("2006-01-02 15:04:05", cdSlice[7], time.Local)

			rip := cdSlice[2]
			if strings.Count(cdSlice[2], ":") == 1 {
				rip = cdSlice[2][:strings.IndexByte(cdSlice[2], ':')]
			}

			cd := ClientData{
				Rip:            rip,
				Vip:            cdSlice[3],
				Vip6:           cdSlice[4],
				RecvBytes:      recv,
				SendBytes:      send,
				ConnDate:       cdSlice[7],
				Username:       cdSlice[9],
				CommonName:     cdSlice[1],
				ID:             cdSlice[10],
				OnlineTime:     (time.Duration(time.Now().Unix()-connDate.Unix()) * time.Second).String(),
				IsNftBlacklist: getNftTableSetElement("blacklist", cdSlice[3]) || getNftTableSetElement("blacklist", cdSlice[4]),
			}

			clients = append(clients, cd)
		}
	}

	return clients

}

func (ov *ovpn) getServer() ServerData {
	var sd ServerData

	data, err := ov.sendCommand("state")
	if err != nil {
		return sd
	}

	sateSlice := strings.Split(data, ",")
	if len(sateSlice) >= 3 {
		runDate, _ := strconv.ParseInt(sateSlice[0], 10, 64)
		sd.RunDate = time.Unix(runDate, 0).Format("2006-01-02 15:04:05")
		sd.Status = sateSlice[1]
		sd.StatusDesc = sateSlice[2]
		sd.Address = sateSlice[3]
	}

	data, err = ov.sendCommand("load-stats")
	if err != nil {
		return sd
	}

	statsSlice := strings.Split(data, ",")
	for _, v := range statsSlice {
		statsKeySlice := strings.Split(v, "=")

		switch statsKeySlice[0] {
		case "nclients":
			sd.Nclients = statsKeySlice[1]
		case "bytesin":
			in, _ := strconv.ParseFloat(statsKeySlice[1], 64)
			sd.BytesIn = tools.FormatBytes(in)
		case "bytesout":
			out, _ := strconv.ParseFloat(statsKeySlice[1], 64)
			sd.BytesOut = tools.FormatBytes(out)
		}
	}

	data, err = ov.sendCommand("version")
	if err != nil {
		return sd
	}

	for _, v := range strings.Split(data, "\n") {
		if strings.HasPrefix(v, "OpenVPN Version: ") {
			sd.Version = strings.TrimPrefix(v, "OpenVPN Version: ")
		}
	}

	return sd

}

func (ov *ovpn) killClient(cid string) {
	ov.sendCommand(fmt.Sprintf("client-kill %s HALT", cid))
}

func parseCrl(crlPath string) (*CertData, error) {
	crlData, err := os.ReadFile(crlPath)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(crlData)
	if block == nil {
		return nil, fmt.Errorf("无法解析证书文件")
	}

	crl, err := x509.ParseRevocationList(block.Bytes)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	expiresIn := crl.NextUpdate.Sub(now)

	var status string
	var expiresInStr string

	if now.After(crl.NextUpdate) {
		status = "已过期"
		expiresInStr = fmt.Sprintf("已过期 %d 天", int(now.Sub(crl.NextUpdate).Hours()/24))
	} else if expiresIn < 30*24*time.Hour {
		status = "即将过期"
		expiresInStr = fmt.Sprintf("%d 天后过期", int(expiresIn.Hours()/24))
	} else {
		status = "正常"
		expiresInStr = fmt.Sprintf("%d 天后过期", int(expiresIn.Hours()/24))
	}

	return &CertData{
		Name:      strings.TrimSuffix(filepath.Base(crlPath), filepath.Ext(crlPath)),
		Type:      "CRL证书",
		Subject:   "",
		Issuer:    crl.Issuer.String(),
		NotBefore: crl.ThisUpdate.Local().Format("2006-01-02 15:04:05"),
		NotAfter:  crl.NextUpdate.Local().Format("2006-01-02 15:04:05"),
		ExpiresIn: expiresInStr,
		Status:    status,
		SerialNo:  "",
	}, nil
}

func parseCert(certPath string) (*CertData, error) {
	certData, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(certData)
	if block == nil {
		return nil, fmt.Errorf("无法解析证书文件")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	expiresIn := cert.NotAfter.Sub(now)

	var status string
	var expiresInStr string

	if now.After(cert.NotAfter) {
		status = "已过期"
		expiresInStr = fmt.Sprintf("已过期 %d 天", int(now.Sub(cert.NotAfter).Hours()/24))
	} else if expiresIn < 30*24*time.Hour {
		status = "即将过期"
		expiresInStr = fmt.Sprintf("%d 天后过期", int(expiresIn.Hours()/24))
	} else {
		status = "正常"
		expiresInStr = fmt.Sprintf("%d 天后过期", int(expiresIn.Hours()/24))
	}

	certType := "客户端证书"
	if cert.IsCA {
		certType = "CA证书"
	} else if strings.Contains(cert.Subject.CommonName, "server") {
		certType = "服务端证书"
	}

	return &CertData{
		Name:      strings.TrimSuffix(filepath.Base(certPath), filepath.Ext(certPath)),
		Type:      certType,
		Subject:   cert.Subject.String(),
		Issuer:    cert.Issuer.String(),
		NotBefore: cert.NotBefore.Local().Format("2006-01-02 15:04:05"),
		NotAfter:  cert.NotAfter.Local().Format("2006-01-02 15:04:05"),
		ExpiresIn: expiresInStr,
		Status:    status,
		SerialNo:  cert.SerialNumber.String(),
	}, nil
}

func enrichCertificateData(cert *CertData) {
	if cert == nil {
		return
	}
	serverName := viperGetString("system.base.server_name", "server")
	isServerCertificate := cert.Name == "server" || cert.Name == serverName
	isCACertificate := cert.Name == "ca"
	if parsed, err := parseCertificateFile(filepath.Join(pkiIssuedDir(), cert.Name+".crt")); err == nil {
		isCACertificate = isCACertificate || parsed.IsCA
		for _, usage := range parsed.ExtKeyUsage {
			if usage == x509.ExtKeyUsageServerAuth {
				isServerCertificate = true
				break
			}
		}
	}
	switch {
	case isCACertificate:
		cert.Kind = "ca"
		cert.Deletable = false
		cert.ProtectedReason = "CA certificate is protected and cannot be deleted"
	case cert.Name == "crl":
		cert.Kind = "crl"
		cert.Deletable = false
		cert.ProtectedReason = "CRL is required for OpenVPN and cannot be deleted"
	case isServerCertificate:
		cert.Kind = "server"
		cert.Deletable = false
		cert.ProtectedReason = "OpenVPN Server certificate is protected and cannot be deleted"
	default:
		cert.Kind = "client"
		cert.Deletable = true
		cert.Lifecycle = "active"
		if parsed, err := parseCertificateFile(clientCertPath(cert.Name)); err == nil {
			if revoked, err := isCertificateRevoked(parsed); err == nil && revoked {
				cert.Status = "Revoked"
				cert.Lifecycle = "revoked"
			}
		}
		if cert.Lifecycle == "active" && !fileExists(filepath.Join(ovData, "clients", cert.Name+".ovpn")) {
			var count int64
			if db != nil {
				db.Model(&User{}).Where("username = ? OR ovpn_config = ?", cert.Name, cert.Name+".ovpn").Count(&count)
			}
			if count == 0 {
				cert.Lifecycle = "orphaned"
			}
		}
	}
}

func getCerts(ovData string) []CertData {
	cers := make([]CertData, 0)
	pkiDir := filepath.Join(ovData, "pki")

	caPath := filepath.Join(pkiDir, "ca.crt")
	if cert, err := parseCert(caPath); err == nil {
		enrichCertificateData(cert)
		cers = append(cers, *cert)
	} else {
		logger.Error(context.Background(), err.Error())
	}

	crlPath := filepath.Join(pkiDir, "crl.pem")
	if cert, err := parseCrl(crlPath); err == nil {
		enrichCertificateData(cert)
		cers = append(cers, *cert)
	} else {
		logger.Error(context.Background(), err.Error())
	}

	issuedDir := filepath.Join(pkiDir, "issued")
	if files, err := os.ReadDir(issuedDir); err == nil {
		for _, file := range files {
			if strings.HasSuffix(file.Name(), ".crt") {
				certPath := filepath.Join(issuedDir, file.Name())
				if cert, err := parseCert(certPath); err == nil {
					enrichCertificateData(cert)
					cers = append(cers, *cert)
				} else {
					logger.Error(context.Background(), err.Error())
				}
			}
		}
	} else {
		logger.Error(context.Background(), err.Error())
	}

	return cers
}

func isValidPassword(pw string) bool {
	lower := regexp.MustCompile(`[a-z]`)
	upper := regexp.MustCompile(`[A-Z]`)
	digit := regexp.MustCompile(`[0-9]`)
	special := regexp.MustCompile(`[!@#\$%\^&\*()_+\-=\[\]{};':"\\|,.<>\/?]`)

	count := 0
	if len(pw) >= 12 {
		count++
	}
	if lower.MatchString(pw) {
		count++
	}
	if upper.MatchString(pw) {
		count++
	}
	if digit.MatchString(pw) {
		count++
	}
	if special.MatchString(pw) {
		count++
	}

	return count == 5
}

func genRandomString(length int) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[rand.Intn(len(charset))]
	}
	return string(result)
}

func IsLocalRequest(c *gin.Context) bool {
	ip, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return false
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	return parsedIP.IsLoopback()
}

func reactRuntime(page string, clientUrls ClientUrlConfig) gin.H {
	clientUrlsJSON, _ := json.Marshal(clientUrls)

	return gin.H{
		"page":           page,
		"sysUser":        adminUsername,
		"version":        "v" + version,
		"assetVersion":   assetVersion,
		"clientUrlsJson": template.JS(clientUrlsJSON),
	}
}

// initBuiltinUsers 启动时初始化内置 admin 用户到 user 表
//   - admin：FirstOrCreate，username 来自 config.json admin_username，密码来自 admin_password（bcrypt 哈希），
//     name/email 来自 config.json admin_name/admin_email，isEnable=true，绑定 administrator 角色
//   - admin 已存在：检查 config.json admin_password/name/email 是否变化，变化则同步到 user 表
//
// 密码同步策略：config.json admin_password 是 bcrypt 哈希，直接写入 user.Password（BeforeSave 钩子会 AES 加密入库，
// AfterFind 解密回 bcrypt 哈希，登录时用 bcrypt.CompareHashAndPassword 校验）
func initBuiltinUsers(db *gorm.DB) {
	if db == nil {
		return
	}

	// 清理历史遗留的 system 账号（只保留 admin）
	systemUsername := viper.GetString("system.base.system_username")
	if systemUsername == "" {
		systemUsername = "system"
	}
	// 找到 system 用户并删除（先删 user_role 关联，再删 user）
	var systemUser User
	if err := db.Where("username = ?", systemUsername).First(&systemUser).Error; err == nil && systemUser.ID > 0 {
		if e := db.Where("user_id = ?", systemUser.ID).Delete(&UserRole{}).Error; e != nil {
			logger.Error(context.Background(), "initBuiltinUsers 删除 system 用户 user_role 关联失败: %s", e.Error())
		}
		if e := db.Delete(&systemUser).Error; e != nil {
			logger.Error(context.Background(), "initBuiltinUsers 删除 system 用户失败: %s", e.Error())
		} else {
			logger.Error(context.Background(), "initBuiltinUsers 已删除历史遗留的 system 用户 (id=%d)", systemUser.ID)
		}
	}

	// admin 用户
	if adminUsername != "" {
		var admin User
		err := db.Where("username = ?", adminUsername).First(&admin).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 全新部署：创建 admin 用户
			enable := true
			notFirst := false
			name := viper.GetString("system.base.admin_name")
			if name == "" {
				name = "超级管理员"
			}
			admin = User{
				Username:     adminUsername,
				Name:         name,
				Email:        viper.GetString("system.base.admin_email"),
				IsEnable:     &enable,
				IsFirstLogin: &notFirst,
				Password:     adminPassword, // bcrypt 哈希，BeforeSave 会 AES 加密入库
			}
			if err := db.Create(&admin).Error; err != nil {
				logger.Error(context.Background(), "initBuiltinUsers 创建 admin 用户失败: %s", err.Error())
			} else {
				// 绑定 administrator 角色
				var adminRole Role
				if e := db.Where("code = ?", BuiltinRoleAdministrator).First(&adminRole).Error; e == nil && adminRole.ID > 0 {
					if e := db.Create(&UserRole{UserID: admin.ID, RoleID: adminRole.ID}).Error; e != nil {
						logger.Error(context.Background(), "initBuiltinUsers 绑定 administrator 角色失败: %s", e.Error())
					}
				} else {
					logger.Error(context.Background(), "initBuiltinUsers 未找到 administrator 角色，跳过角色绑定")
				}
				logger.Error(context.Background(), "initBuiltinUsers 已创建 admin 用户 (id=%d) 并绑定 administrator 角色", admin.ID)
			}
		} else if err != nil {
			logger.Error(context.Background(), "initBuiltinUsers 查询 admin 用户失败: %s", err.Error())
		} else {
			// admin 已存在：检查 config.json 密码/name/email 是否变化并同步
			syncAdminFromConfig(db, &admin)
		}
	}
}

// syncAdminFromConfig 检查 config.json 中 admin 的密码/name/email 是否变化，变化则同步到 user 表
//   - 密码：adminPassword 全局变量来自 config.json（bcrypt 哈希），与 user 表解密后的值比对；
//     用 struct Updates 触发 BeforeSave 钩子（AES 加密），与 Create 路径保持一致
//   - name/email：config.json 值与 user 表值比对，用 map Updates 避免 struct 零值忽略
func syncAdminFromConfig(db *gorm.DB, admin *User) {
	// 密码同步：admin.Password 经 AfterFind 解密后为 bcrypt 哈希，与 adminPassword 比对
	if adminPassword != "" && admin.Password != adminPassword {
		if err := db.Model(&User{}).Where("id = ?", admin.ID).Updates(User{Password: adminPassword}).Error; err != nil {
			logger.Error(context.Background(), "syncAdminFromConfig 同步 admin 密码到 user 表失败: %s", err.Error())
		} else {
			logger.Error(context.Background(), "syncAdminFromConfig 已同步 admin 密码变更到 user 表")
		}
	}

	// name/email 同步
	updates := map[string]interface{}{}
	if name := viper.GetString("system.base.admin_name"); name != "" && name != admin.Name {
		updates["name"] = name
	}
	if email := viper.GetString("system.base.admin_email"); email != "" && email != admin.Email {
		updates["email"] = email
	}
	if len(updates) > 0 {
		if err := db.Model(&User{}).Where("id = ?", admin.ID).Updates(updates).Error; err != nil {
			logger.Error(context.Background(), "syncAdminFromConfig 同步 admin name/email 到 user 表失败: %s", err.Error())
		} else {
			logger.Error(context.Background(), "syncAdminFromConfig 已同步 admin name/email 变更到 user 表: %v", updates)
		}
	}
}

const (
	internalFirewallHookContextKey      = "internalFirewallHook"
	internalFirewallHookAuditActor      = "openvpn-hook"
	internalWebAuditClientMapContextKey = "internalWebAuditClientMapHook"
)

// hasMatchingLocalServiceToken verifies the configured internal service token without
// allowing an unset token to authenticate a request.
func hasMatchingLocalServiceToken(c *gin.Context) bool {
	expected := viper.GetString("system.base.token")
	actual := c.GetHeader("O-Token")
	return expected != "" && actual != "" &&
		subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}

// isOpenVPNFirewallHookRequest recognizes only the two firewall operations invoked by
// OpenVPN's local lifecycle hooks. It intentionally does not grant a user or admin role.
func isOpenVPNFirewallHookRequest(c *gin.Context) bool {
	if !IsLocalRequest(c) || !hasMatchingLocalServiceToken(c) ||
		c.Request.Method != http.MethodPost || c.Request.URL.Path != "/ovpn/firewall" {
		return false
	}

	switch c.Query("a") {
	case "add_ovips", "delete_ovips":
		return true
	default:
		return false
	}
}

func hasInternalFirewallHookIdentity(c *gin.Context) bool {
	internal, ok := c.Get(internalFirewallHookContextKey)
	return ok && internal == true
}

// isWebAuditClientMapHookRequest only permits the local OpenVPN lifecycle script
// to update the transient VPN-IP-to-user mapping used by DNS audit attribution.
func isWebAuditClientMapHookRequest(c *gin.Context) bool {
	return IsLocalRequest(c) && hasMatchingLocalServiceToken(c) &&
		c.Request.Method == http.MethodPost && c.Request.URL.Path == "/ovpn/web-audit/client-map"
}

func AuthMiddleWare() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		user := session.Get("user")

		if isOpenVPNFirewallHookRequest(c) {
			c.Set(internalFirewallHookContextKey, true)
			c.Next()
			return
		}
		if isWebAuditClientMapHookRequest(c) {
			c.Set(internalWebAuditClientMapContextKey, true)
			c.Next()
			return
		}

		if hasMatchingLocalServiceToken(c) && IsLocalRequest(c) {
			if c.Request.URL.Path == "/ovpn/login" || c.Request.URL.Path == "/ovpn/history" || c.Request.URL.Path == "/ovpn/notify" {
				c.Next()
				return
			}
		}

		if user == nil {
			c.Redirect(302, "/login")
			c.Abort()
			return
		}

		if username, ok := user.(string); ok {
			c.Set("user", username)

			// admin 用户已纳入 user 表，走标准 RBAC 加载权限码
			// 保留 isAdmin 标志用于数据权限旁路（GetAccessible*/queryAuditLogs 等基于 username 判断）
			isAdmin := adminUsername != "" && username == adminUsername

			// 统一加载权限 code 列表（admin 走 user_role 加载 administrator 角色权限码）
			u := User{Username: username}.Info()
			// 用户已被删除：session 仍有效但 DB 无记录，清除 session 强制登出（与角色禁用处理一致）
			if u.ID == 0 {
				session.Clear()
				session.Options(sessions.Options{Path: "/", MaxAge: -1})
				session.Save()
				c.Redirect(302, "/login")
				c.Abort()
				return
			}
			codes, err := u.LoadPermissionCodes(db)
			if err != nil {
				// 角色被禁用或不存在：清除 session 强制登出
				if errors.Is(err, ErrRoleDisabled) || errors.Is(err, ErrRoleNotFound) {
					session.Clear()
					session.Options(sessions.Options{Path: "/", MaxAge: -1})
					session.Save()
					c.Redirect(302, "/login")
					c.Abort()
					return
				}
				// 其他错误（如 DB 连接断开）：拒绝访问，避免静默赋空权限让用户陷入"突然无权限"的困惑
				c.JSON(http.StatusInternalServerError, gin.H{"message": "权限加载失败，请稍后重试"})
				c.Abort()
				return
			}
			c.Set("permissions", codes)
			c.Set("isAdmin", isAdmin)
		}

		c.Next()
	}
}

func init() {
	if ovData == "" {
		ovData = "data"
	}
	initConfig()
	loadConfig()
}

func Run(info BuildInfo) {
	if info.Version != "" {
		version = info.Version
	}
	if info.Commit != "" {
		commit = info.Commit
	}
	if info.Date != "" {
		date = info.Date
	}
	if info.BuiltBy != "" {
		builtBy = info.BuiltBy
	}
	assetVersion = fmt.Sprintf("v%s-%s-%d", version, commit, time.Now().Unix())

	ov := ovpn{
		address: ovManage,
	}

	var err error
	db, err = OpenDatabase(conf.Database, ovData, logger)

	if err != nil {
		panic(err)
	}

	c := cron.New()
	c.AddFunc("@daily", func() {
		err := History{}.Clear()
		if err != nil {
			logger.Error(context.Background(), err.Error())
		}
		if err := (ClientTrafficSample{}).Clear(); err != nil {
			logger.Error(context.Background(), err.Error())
		}
		if err := (WebsiteAccessLog{}).Clear(); err != nil {
			logger.Error(context.Background(), err.Error())
		}
		if err := (SuricataNetworkEvent{}).Clear(); err != nil {
			logger.Error(context.Background(), err.Error())
		}
	})
	c.AddFunc("@daily", func() {
		checkAndSendExpireReminders()
	})
	c.Start()

	store := gormsessions.NewStore(db, true, []byte(secretKey))

	db.AutoMigrate(&Group{})
	db.FirstOrCreate(&Group{Name: "Default", ParentID: nil})
	db.AutoMigrate(&User{}, &History{}, &ClientTrafficSample{}, &WebsiteAccessLog{}, &SuricataNetworkEvent{}, &SuricataEVEOffset{}, &Firewall{}, &NotifyLog{}, &AuditLog{}, &NotificationChannel{}, &UserNotifyRead{}, &ClientPackage{})
	db.AutoMigrate(&Role{}, &Permission{}, &RolePermission{}, &UserRole{}, &GroupRole{})
	if err := MigrateAISettings(db); err != nil {
		panic(fmt.Errorf("initialize AI provider settings: %w", err))
	}
	if err := MigrateAIChatHistory(db); err != nil {
		panic(fmt.Errorf("initialize AI chat history: %w", err))
	}

	// 初始化 IP 归属地解析器
	if err := InitIPRegion(""); err != nil {
		logger.Error(context.Background(), "初始化 IP 解析器失败: %v", err)
	}

	// 初始化权限定义与内置角色（administrator / user）
	if err := SeedPermissionsAndRoles(db); err != nil {
		logger.Error(context.Background(), "SeedPermissionsAndRoles 失败: %s", err.Error())
	}

	// 初始化内置 admin 用户到 user 表
	// admin 绑定 administrator 角色；config.json 配置变更同步到 user 表
	// 必须在 RepairAuditLogOperatorIDs/RepairNotifyReadUserIDs 之前执行，以便保留 ID 迁移到真实 user.id
	initBuiltinUsers(db)

	// 历史升级兼容：将 user.role_id（旧模型）数据回填并迁移到 user_role 表
	// 全新部署时 user 表无 role_id 列，跳过迁移；仅在列存在时执行（避免 SQL 报错）
	defaultRoleID := GetDefaultRoleID(db)
	hasRoleIDColumn, err := columnExists(db, "user", "role_id")
	if err != nil {
		logger.Error(context.Background(), "检查 user.role_id 列存在性失败: %s", err.Error())
	}
	if hasRoleIDColumn {
		// 历史用户 role_id 为 NULL 时回填到普通用户角色 ID
		if defaultRoleID > 0 {
			result := db.Exec("UPDATE "+userIdent(db)+" SET role_id = ? WHERE role_id IS NULL", defaultRoleID)
			if result.Error != nil {
				logger.Error(context.Background(), "回填历史用户 role_id 失败: %s", result.Error.Error())
			} else if result.RowsAffected > 0 {
				logger.Error(context.Background(), "已回填 %d 个历史用户到普通用户角色 (role_id=%d)", result.RowsAffected, defaultRoleID)
			}
		} else {
			logger.Error(context.Background(), "未找到普通用户角色，跳过历史用户 role_id 回填")
		}

		// 迁移历史 user.role_id 到 user_role 表
		if err := insertIgnore(db, "INTO user_role (user_id, role_id, created_at) SELECT id, role_id, CURRENT_TIMESTAMP FROM "+userIdent(db)+" WHERE role_id IS NOT NULL AND role_id > 0").Error; err != nil {
			logger.Error(context.Background(), "迁移历史 user.role_id 到 user_role 表失败: %s", err.Error())
		}
	}

	// 历史升级兼容：将 group.role_id（旧单角色模型）迁移到 group_role 多对多关联表
	// group.role_id 字段保留但不再使用，新代码通过 group_role 表管理组-角色关联
	hasGroupRoleIDColumn, err := columnExists(db, "group", "role_id")
	if err != nil {
		logger.Error(context.Background(), "检查 group.role_id 列存在性失败: %s", err.Error())
	}
	if hasGroupRoleIDColumn {
		result := insertIgnore(db, "INTO group_role (group_id, role_id, created_at) SELECT id, role_id, CURRENT_TIMESTAMP FROM "+groupIdent(db)+" WHERE role_id IS NOT NULL AND role_id > 0")
		if result.Error != nil {
			logger.Error(context.Background(), "迁移历史 group.role_id 到 group_role 表失败: %s", result.Error.Error())
		} else if result.RowsAffected > 0 {
			logger.Error(context.Background(), "已迁移 %d 条 group.role_id 到 group_role 表", result.RowsAffected)
		}
	}

	// 修复历史 audit_logs 中 operator_id=0 的记录（根据 operator 反向查找 user.id）
	RepairAuditLogOperatorIDs()

	// 修复历史 history 中 user_id=0 的记录（根据 username/common_name 反向查找 user.id）
	RepairHistoryUserIDs()

	// 修复历史 user_notify_read 中 user_id=0 的记录（admin 内置账号改用真实 ID，历史 system 记录统一迁到 admin）
	RepairNotifyReadUserIDs()

	// 旧表 channel_type 列数据迁移到新 channel_name 列
	migrateNotifyLogChannelName()

	// 初始化客户端安装包存储目录
	InitClientPackagesDir()

	// 注册所有通知渠道实现（webhook/email/dingtalk/feishu/wecom/discord/slack/telegram/mattermost）
	registerNotifiers()

	// 启动 WebSocket Hub（用于站内信实时推送）
	WsHubInstance().Run()

	// 启动系统监控采集器：周期采集 CPU/内存/磁盘/网络，通过 WebSocket 推送到首页
	StartSystemStatsCollector(5 * time.Second)

	// 启动概览数据采集器：周期采集 dashboard summary / 在线客户端 / 服务状态，通过 WebSocket 推送到首页
	// 概览页所有卡片均通过 dashboard:stats topic 实时更新，无需前端定时器或手动刷新
	StartDashboardStatsCollector(&ov, 5*time.Second)

	// DNS 审计先启动本地 UDP/TCP 转发监听，再由服务自身安全安装 tun0:53 重定向。
	// 监听或规则安装失败只会降级审计，绝不会阻断 OpenVPN。
	startWebAuditDNS(context.Background(), &ov)
	// The EVE tailer is optional and failure-open; it only reads a deployment
	// supplied file and never changes VPN or packet-capture configuration.
	suricataEVE.reconcile()

	// 初始化 AI 助手模块（可选，通过 ai.enabled 控制）
	// 注意：chatMgr 始终初始化，确保 AI 路由可注册，即使 LLM 客户端暂未就绪
	// 架构：chatMgr（会话ID管理）+ aiClient（LLM 原子引用）+ agentRunner（ADK 推理循环）+ healthChecker（后台自检）
	chatHistoryStore := NewSQLiteAIChatHistoryStore(db)
	var chatMgr *ai.ChatSessionManager = ai.NewChatSessionManager(nil, chatHistoryStore)
	aiClient := ai.NewAtomicClient(nil)
	// healthChecker 始终创建，确保 /ovpn/ai/health 路由可读缓存（即使 LLM 未配置也返回 unavailable）
	healthChecker := ai.NewHealthChecker(aiClient,
		ai.WithHealthChangeHandler(func(status ai.HealthStatus) {
			// 状态变更时通过 WebSocket 推送 ai:health 事件
			Bus().Publish("ai:health", map[string]any{
				"available": status.Available,
				"model":     status.Model,
				"provider":  status.Provider,
				"error":     status.Error,
				"checkedAt": status.CheckedAt,
			})
		}),
	)
	// 注册 ai:health / ai:session_reset 主题到 WebSocket 桥接，前端可通过 notificationHub 订阅
	WsHubInstance().SubscribeTopic("ai:health")
	WsHubInstance().SubscribeTopic("ai:session_reset")

	// 构建 AI 业务工具服务（实现 ai.ToolService 接口，注入到 Agent 工具中）
	aiToolSvc := NewAIToolService(&ov)

	startupAICfg, startupAIConfigErr := activeAIConfig(db)
	if startupAIConfigErr != nil {
		log.Printf("AI provider configuration unavailable: %v", startupAIConfigErr)
	} else if startupAICfg.Enabled {
		var initErr error
		llmCfg := ai.LLMConfig{
			Provider:    startupAICfg.Provider,
			BaseURL:     startupAICfg.BaseURL,
			APIKey:      startupAICfg.APIKey,
			Model:       startupAICfg.Model,
			MaxTokens:   startupAICfg.MaxTokens,
			Temperature: startupAICfg.Temperature,
		}
		client, initErr := ai.NewLLMClient(llmCfg)
		if initErr != nil {
			log.Printf("⚠ AI 助手初始化失败: %v（将禁用 AI 功能）", initErr)
		} else {
			aiClient.Set(client)
			// 构建 ADK Agent + Runner（含业务工具）
			// operator 直接通过 ADK ToolContext.UserID() 获取（即 AgentRunner.Run 调用时传入的 usernameStr）
			tools, toolErr := ai.BuildBusinessTools(aiToolSvc)
			if toolErr != nil {
				log.Printf("⚠ AI 业务工具构建失败: %v（将仅支持对话模式）", toolErr)
				tools = nil
			}
			agentRunner, agentErr := ai.NewAgentRunner(client, tools, ai.AgentConfig{
				SystemPrompt: startupAICfg.SystemPrompt,
				MaxTokens:    startupAICfg.MaxTokens,
				Temperature:  startupAICfg.Temperature,
			})
			if agentErr != nil {
				log.Printf("⚠ AI AgentRunner 创建失败: %v（将禁用 Agent 能力）", agentErr)
			} else {
				chatMgr.SetAgentRunner(agentRunner)
				log.Printf("✅ AI AgentRunner 已就绪（工具数: %d）", len(tools))
			}
			// Cache the configured state; provider calls happen only during explicit connection tests.
			healthChecker.SetConfiguredStatus()
			// 启动会话清理定时器（每 10 分钟清理空闲超过 30 分钟的会话）
			go func() {
				ticker := time.NewTicker(10 * time.Minute)
				defer ticker.Stop()
				for range ticker.C {
					n := chatMgr.CleanupIdle(context.Background(), 30*time.Minute)
					if n > 0 {
						log.Printf("AI 会话清理: 已清理 %d 个空闲用户记录", n)
					}
				}
			}()
			log.Printf("✅ AI 助手已就绪（Provider: %s, 模型: %s, 端点: %s）",
				startupAICfg.Provider, startupAICfg.Model, startupAICfg.BaseURL)
		}
	}

	r := gin.New()
	r.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {

		var statusColor, methodColor, resetColor string
		if param.IsOutputColor() {
			statusColor = param.StatusCodeColor()
			methodColor = param.MethodColor()
			resetColor = param.ResetColor()
		}

		if param.Latency > time.Minute {
			param.Latency = param.Latency.Truncate(time.Second)
		}
		return fmt.Sprintf("[OPENVPN-WEB] %v GIN |%s %3d %s| %13v | %15s |%s %-7s %s %#v\n%s",
			param.TimeStamp.Format("2006-01-02 15:04:05.000"),
			statusColor, param.StatusCode, resetColor,
			param.Latency,
			param.ClientIP,
			methodColor, param.Method, resetColor,
			param.Path,
			param.ErrorMessage,
		)
	}))

	r.Use(sessions.Sessions("user_session", store))
	r.Use(AuditMiddleware())

	// r.Use(gin.Recovery())

	templ := template.Must(template.New("").ParseFS(FS, "templates/*.html"))
	r.SetHTMLTemplate(templ)
	f, _ := fs.Sub(FS, "templates/static")
	r.StaticFS("/static", http.FS(f))

	r.GET("/login", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", reactRuntime("login", conf.Client.ClientUrl))
	})

	r.POST("/login", func(c *gin.Context) {
		var err error

		cip := c.ClientIP()
		passcode := c.PostForm("passcode")

		session := sessions.Default(c)
		remember7d := c.PostForm("remember7d")

		if remember7d == "on" {
			session.Options(sessions.Options{
				HttpOnly: true,
				MaxAge:   3600 * 24 * 7,
			})
		} else {
			session.Options(sessions.Options{
				HttpOnly: true,
				MaxAge:   3600 * 1,
			})
		}

		var u User
		c.ShouldBind(&u)

		if u.Username == adminUsername {
			// admin 登录：bcrypt 校验密码（user 表 admin.Password 解密后为 bcrypt 哈希）
			adminUser := User{Username: u.Username}.Info()
			if adminUser.ID == 0 {
				setLoginFail(cip)
				c.JSON(401, gin.H{"message": "管理员账户未初始化，请重启服务"})
				return
			}
			// 兼容历史：检测旧的 AES 加密密码格式（config.json 迁移场景）
			if dp, e := aes.AesDecrypt(adminPassword, secretKey); e == nil {
				if subtle.ConstantTimeCompare([]byte(dp), []byte(u.Password)) == 1 {
					passwd, _ := bcrypt.GenerateFromPassword([]byte("admin"), 12)
					viper.Set("system.base.admin_password", string(passwd))
					viper.WriteConfig()
					adminPassword = string(passwd)
					// 同步到 user 表（触发 BeforeSave AES 加密）
					_ = db.Model(&User{}).Where("id = ?", adminUser.ID).Updates(User{Password: string(passwd)})
					c.JSON(401, gin.H{"message": "检测到旧的密码加密格式，已重置为默认密码，请使用默认密码 admin 登录后修改"})
					return
				}
			}

			if bcrypt.CompareHashAndPassword([]byte(adminPassword), []byte(u.Password)) == nil {
				session.Set("user", u.Username)
				session.Save()

				resetLoginFail(cip)
				// 加载 administrator 角色权限码（走标准 RBAC）
				permCodes, perr := adminUser.LoadPermissionCodes(db)
				if perr != nil || permCodes == nil {
					permCodes = []string{}
				}
				roleIDs, roleNames := adminUser.LoadRoleIDsAndNames(db)
				c.JSON(200, gin.H{
					"message":  "登录成功",
					"redirect": "/admin",
					"user": gin.H{
						"id":           adminUser.ID,
						"username":     adminUser.Username,
						"name":         adminUser.Name,
						"email":        adminUser.Email,
						"isFirstLogin": false,
						"isAdmin":      true,
						"permissions":  permCodes,
						"roleIds":      roleIDs,
						"roleNames":    roleNames,
					},
				})
				return
			} else {
				err = fmt.Errorf("密码错误")
			}
		} else {
			if passcode != "" {
				// 一步式 MFA 验证（适用于 OpenVPN 客户端认证）
				if err = u.Login(false); err == nil {
					userInfo := u.Info()
					if ValidateMfa(passcode, userInfo.MfaSecret) {
						// 校验角色是否启用
						permCodes, perr := userInfo.LoadPermissionCodes(db)
						if errors.Is(perr, ErrRoleDisabled) || errors.Is(perr, ErrRoleNotFound) {
							c.JSON(403, gin.H{"message": "角色已禁用或不存在，请联系管理员"})
							return
						}
						if permCodes == nil {
							permCodes = []string{}
						}
						session.Set("user", u.Username)
						session.Save()
						resetLoginFail(cip)
						userRoleIDs, userRoleNames := userInfo.LoadRoleIDsAndNames(db)
						c.JSON(200, gin.H{
							"message":  "登录成功",
							"redirect": "/",
							"user": gin.H{
								"id":           userInfo.ID,
								"username":     userInfo.Username,
								"name":         userInfo.Name,
								"email":        userInfo.Email,
								"isFirstLogin": *userInfo.IsFirstLogin,
								"isAdmin":      false,
								"permissions":  permCodes,
								"roleIds":      userRoleIDs,
								"roleNames":    userRoleNames,
							},
						})
						return
					}
					c.JSON(401, gin.H{"message": "MFA 验证失败"})
					return
				}

				// Web 登录两步验证（依赖 valid_user 缓存）
				if validUser, ok := cc.Get("valid_user"); ok {
					if u.Username == validUser.(string) {
						userInfo := u.Info()
						if ValidateMfa(passcode, userInfo.MfaSecret) {
							permCodes, perr := userInfo.LoadPermissionCodes(db)
							if errors.Is(perr, ErrRoleDisabled) || errors.Is(perr, ErrRoleNotFound) {
								c.JSON(403, gin.H{"message": "角色已禁用或不存在，请联系管理员"})
								return
							}
							if permCodes == nil {
								permCodes = []string{}
							}
							cc.Delete("valid_user")
							session.Set("user", u.Username)
							session.Save()
							resetLoginFail(cip)
							userRoleIDs, userRoleNames := userInfo.LoadRoleIDsAndNames(db)
							c.JSON(200, gin.H{
								"message":  "登录成功",
								"redirect": "/",
								"user": gin.H{
									"id":           userInfo.ID,
									"username":     userInfo.Username,
									"name":         userInfo.Name,
									"email":        userInfo.Email,
									"isFirstLogin": *userInfo.IsFirstLogin,
									"isAdmin":      false,
									"permissions":  permCodes,
									"roleIds":      userRoleIDs,
									"roleNames":    userRoleNames,
								},
							})
						} else {
							c.JSON(401, gin.H{"message": "MFA 验证失败"})
						}

						return
					}
				}

				c.JSON(401, gin.H{"message": "登录超时", "redirect": "/login"})
				return
			}

			if err = u.Login(false); err == nil {
				user := u.Info()
				if user.MfaSecret != "" {
					cc.Set("valid_user", u.Username, 1*time.Minute)
					c.JSON(200, gin.H{"message": "需要 MFA 验证", "mfaRequired": true})
					return
				}

				// 校验角色是否启用
				permCodes, perr := user.LoadPermissionCodes(db)
				if errors.Is(perr, ErrRoleDisabled) || errors.Is(perr, ErrRoleNotFound) {
					c.JSON(403, gin.H{"message": "角色已禁用或不存在，请联系管理员"})
					return
				}
				if permCodes == nil {
					permCodes = []string{}
				}

				session.Set("user", u.Username)
				session.Save()

				resetLoginFail(cip)

				userRoleIDs, userRoleNames := user.LoadRoleIDsAndNames(db)
				c.JSON(200, gin.H{
					"message":  "登录成功",
					"redirect": "/",
					"user": gin.H{
						"id":           user.ID,
						"username":     user.Username,
						"name":         user.Name,
						"email":        user.Email,
						"isFirstLogin": *user.IsFirstLogin,
						"isAdmin":      false,
						"permissions":  permCodes,
						"roleIds":      userRoleIDs,
						"roleNames":    userRoleNames,
					},
				})
				return
			}
		}

		setLoginFail(cip)

		c.JSON(401, gin.H{"message": err.Error()})
	})

	r.GET("/logout", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Clear()
		session.Options(sessions.Options{MaxAge: -1})
		session.Save()
		c.Redirect(302, "/login")
	})

	// 公开客户端下载落地页：未登录可访问
	r.GET("/download", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", reactRuntime("client", conf.Client.ClientUrl))
	})

	// 公开客户端安装包相关 API（未登录可访问，仅返回 is_active=true 的包）
	public := r.Group("/ovpn/public")
	{
		// 公开列表：返回当前所有已启用的客户端安装包
		public.GET("/packages", func(c *gin.Context) {
			pkg := ClientPackage{}
			actives := pkg.ActivesByPlatforms()
			result := make([]gin.H, 0, len(actives))
			for _, p := range actives {
				result = append(result, gin.H{
					"id":            p.ID,
					"platform":      p.Platform,
					"platformLabel": PlatformLabel(p.Platform),
					"version":       p.Version,
					"filename":      p.Filename,
					"fileSize":      p.FileSize,
					"downloadUrl":   p.PublicDownloadURL(),
				})
			}
			c.JSON(http.StatusOK, result)
		})

		// 公开下载路由：只允许下载 is_active=true 的包，其余 404
		public.GET("/packages/:id/download", func(c *gin.Context) {
			idStr := c.Param("id")
			id, err := strconv.ParseUint(idStr, 10, 64)
			if err != nil || id == 0 {
				c.JSON(http.StatusNotFound, gin.H{"message": "安装包不存在或已停用"})
				return
			}
			var p ClientPackage
			if err := db.Where("id = ? AND is_active = ?", id, true).First(&p).Error; err != nil {
				c.JSON(http.StatusNotFound, gin.H{"message": "安装包不存在或已停用"})
				return
			}
			fullPath := p.FullPath()
			if _, statErr := os.Stat(fullPath); statErr != nil {
				logger.Error(context.Background(), "公开下载包磁盘文件缺失: id="+idStr+" err="+statErr.Error())
				c.JSON(http.StatusNotFound, gin.H{"message": "安装包文件不存在"})
				return
			}
			c.FileAttachment(fullPath, p.Filename)
		})

		// 公开接口：返回 AI 助手启用状态（无需鉴权，供前端登录前/登录后控制菜单显示）
		public.GET("/ai-status", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"enabled": isAIEnabled(db)})
		})
	}

	r.Use(AuthMiddleWare())

	r.GET("/", func(c *gin.Context) {
		session := sessions.Default(c)
		if user, ok := session.Get("user").(string); ok {
			if user == adminUsername {
				c.Redirect(302, "/admin")
				return
			}

			u := User{Username: user}.Info()
			if *u.IsFirstLogin {
				c.Redirect(302, "/login")
				return
			}
		}

		c.HTML(http.StatusOK, "index.html", reactRuntime("client", conf.Client.ClientUrl))
	})

	r.GET("/admin", func(c *gin.Context) {
		session := sessions.Default(c)
		if user, ok := session.Get("user").(string); ok {
			if user != adminUsername {
				c.Redirect(302, "/")
				return
			}
		}

		c.HTML(http.StatusOK, "index.html", reactRuntime("admin", conf.Client.ClientUrl))
	})

	r.POST("/email/send", func(c *gin.Context) {
		email := c.PostForm("email")
		subject := c.PostForm("subject")
		content := c.PostForm("content")

		err := sendEmail(email, subject, content)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		} else {
			c.JSON(http.StatusOK, gin.H{"message": "发送成功"})
		}
	})

	ovpn := r.Group("/ovpn")
	{
		ovpn.StaticFS("/download", http.Dir(filepath.Join(ovData, "clients")))
		ovpn.GET("/dashboard/summary", RequirePermission("menu:overview"), ov.dashboardSummary)
		ovpn.GET("/dashboard/traffic-users", RequirePermission("menu:overview"), ov.dashboardTrafficUsers)
		ovpn.GET("/dashboard/geo-map", RequirePermission("menu:overview"), ov.dashboardGeo)
		ovpn.GET("/dashboard/geo-map/ips", RequirePermission("menu:overview"), ov.dashboardGeoIPs)
		ovpn.GET("/dashboard/geo-boundary/:iso3/:level", RequirePermission("menu:overview"), ov.dashboardGeoBoundary)
		ovpn.GET("/dashboard/china-boundary/:adcode", RequirePermission("menu:overview"), ov.dashboardChinaBoundary)
		ovpn.GET("/system-stats/history", RequirePermission("menu:overview"), func(c *gin.Context) {
			history, latest := GetSystemStatsHistory()
			c.JSON(http.StatusOK, gin.H{
				"history": history,
				"latest":  latest,
			})
		})
		ovpn.GET("/audit/logs", RequirePermission("audit:view"), auditLogsHandler)
		ovpn.GET("/audit/export", RequirePermission("audit:view"), auditExportHandler)
		ovpn.GET("/audit/user-options", RequirePermission("audit:view"), auditUserOptionsHandler)
		ovpn.GET("/auth-status", func(c *gin.Context) {
			cmd := exec.Command("egrep", "^auth-user-pass-verify", filepath.Join(ovData, "server.conf"))
			auth := cmd.Run() == nil
			c.JSON(http.StatusOK, gin.H{"authUser": auth})
		})

		ovpn.GET("/settings", func(c *gin.Context) {
			var conf config
			if err := viper.Unmarshal(&conf); err != nil {
				// 配置解析失败，返回500错误而不是泄露零值配置
				c.JSON(http.StatusInternalServerError, gin.H{"message": "配置解析失败"})
				return
			}

			// 根据用户 Tab 权限过滤返回数据
			// admin 用户直接返回全量数据
			// AI configuration is stored in SQLite only, never in the legacy JSON file.
			aiSettings, err := aiSettingsAPIResponse(db, "", "")
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "load AI settings failed"})
				return
			}
			conf.AI = aiSettings.Config

			if isAdmin, _ := c.Get("isAdmin"); isAdmin == true {
				c.JSON(http.StatusOK, conf)
				return
			}

			// 非 admin 用户：检查是否有 settings 相关权限
			perms, _ := c.Get("permissions")
			permList, _ := perms.([]string)

			// 检查是否有任何 settings 权限
			hasSettingsAccess := false
			for _, p := range permList {
				if p == "*" || strings.HasPrefix(p, "settings:") {
					hasSettingsAccess = true
					break
				}
			}
			if !hasSettingsAccess {
				c.JSON(http.StatusForbidden, gin.H{"message": "无权限访问系统设置"})
				return
			}

			// 辅助函数：检查是否拥有某个权限
			hasPerm := func(code string) bool {
				// 空权限数组时，默认无任何Tab权限
				if len(permList) == 0 {
					return false
				}
				for _, p := range permList {
					if p == "*" || p == code {
						return true
					}
				}
				return false
			}

			// 过滤数据：无对应 Tab 权限时返回空对象
			// 注意：settings:service 和 settings:packages 权限不在此处过滤，
			// 因为服务管理和客户端包数据不在配置文件中，而是通过独立API提供
			filtered := conf
			if !hasPerm("settings:base") {
				filtered.System.Base = SysBeseConfig{}
			}
			if !hasPerm("settings:ldap") {
				filtered.System.Ldap = SysLdapConfig{}
			}
			if !hasPerm("settings:openvpn") {
				filtered.Openvpn = OvpnConfig{}
			}
			if !hasPerm("settings:ai") {
				filtered.AI = AIConfig{}
			} else {
				// 脱敏 API Key（只显示前后各4位）
				if len(filtered.AI.APIKey) > 8 {
					filtered.AI.APIKey = filtered.AI.APIKey[:4] + "****" + filtered.AI.APIKey[len(filtered.AI.APIKey)-4:]
				} else if filtered.AI.APIKey != "" {
					filtered.AI.APIKey = "****"
				}
			}

			c.JSON(http.StatusOK, filtered)
		})

		ovpn.POST("/settings", func(c *gin.Context) {
			c.Request.ParseForm()

			// 按Tab计算保存权限
			canSaveBase := hasPermissionCode(c, "settings:base:update")
			canSaveLdap := hasPermissionCode(c, "settings:ldap:update")
			canSaveOvpn := hasPermissionCode(c, "settings:openvpn:update")

			// 非 admin 用户：如果没有任何Tab保存权限，返回 403
			if !canSaveBase && !canSaveLdap && !canSaveOvpn {
				recordAudit(c, "rbac", "deny", "settings:update", false, "无权限")
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "无权限"})
				return
			}

			savedCount := 0 // 记录实际保存的字段数
			webAuditConfigChanged := false
			suricataEVEConfigChanged := false
			if c.PostForm("system.base.suricata_eve_enabled") == "true" {
				path := c.PostForm("system.base.suricata_eve_path")
				if path == "" {
					path = viper.GetString("system.base.suricata_eve_path")
				}
				if _, err := validateSuricataEVEPath(path); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
					return
				}
			}
			for k, vs := range c.Request.PostForm {
				// 权限过滤：跳过用户无保存权限的Tab字段
				if strings.HasPrefix(k, "system.base.") && !canSaveBase {
					continue
				}
				if strings.HasPrefix(k, "system.ldap.") && !canSaveLdap {
					continue
				}
				if strings.HasPrefix(k, "openvpn.") && !canSaveOvpn {
					continue
				}
				// 其他字段跳过（不属于任何Tab的配置不在此接口保存）
				if !strings.HasPrefix(k, "system.base.") && !strings.HasPrefix(k, "system.ldap.") && !strings.HasPrefix(k, "openvpn.") {
					continue
				}

				savedCount++
				if strings.HasPrefix(k, "system.base.web_audit_") {
					// All domain-audit policies share one lifecycle so a live change
					// always converges rules and never leaves a stale strict/block rule.
					webAuditConfigChanged = true
				}
				if strings.HasPrefix(k, "system.base.suricata_eve_") {
					suricataEVEConfigChanged = true
				}
				val := vs[0]

				switch k {
				case "system.base.admin_password":
					ep, _ := bcrypt.GenerateFromPassword([]byte(val), 12)
					val = string(ep)
					// 同步到 user 表 admin 用户的 password（struct Updates 触发 BeforeSave AES 加密）
					if adminUsername != "" {
						if e := db.Model(&User{}).Where("username = ?", adminUsername).Updates(User{Password: val}).Error; e != nil {
							logger.Error(context.Background(), "同步 admin 密码到 user 表失败: %s", e.Error())
						}
						adminPassword = val
					}
				case "system.email.password":
					val, _ = aes.AesEncrypt(val, secretKey)
				case "system.base.max_duplicate_login":
					n, err := strconv.Atoi(val)
					if err != nil {
						n = 0
					}

					if n > 0 {
						cfg, err := initOvpnConfig()
						if err != nil {
							logger.Error(context.Background(), err.Error())
							return
						}

						statusLogPath := filepath.Join(ovData, "openvpn-status.log")
						if cfg.Get("status-version") != "3" || cfg.Get("status") != statusLogPath+" 1" {
							cfg.Set("status", statusLogPath+" 1")
							cfg.Set("status-version", "3")
							cfg.Save()

							ov.sendCommand("signal SIGHUP")
						}
					}
				case "system.base.renew_days":
					n, err := strconv.Atoi(val)
					if err != nil || n <= 0 {
						c.JSON(http.StatusBadRequest, gin.H{"message": "续签天数必须是大于 0 的整数"})
						return
					}
				case "system.base.history_max_days", "system.base.suricata_eve_max_days":
					n, err := strconv.Atoi(val)
					if err != nil || n < 0 {
						c.JSON(http.StatusBadRequest, gin.H{"message": "历史保留天数必须是非负整数"})
						return
					}
				case "system.base.suricata_eve_poll_seconds":
					n, err := strconv.Atoi(val)
					if err != nil || n < 1 || n > suricataEVEMaxPollSecs {
						c.JSON(http.StatusBadRequest, gin.H{"message": fmt.Sprintf("Suricata EVE 轮询秒数必须在 1 到 %d 之间", suricataEVEMaxPollSecs)})
						return
					}
				case "system.base.suricata_eve_path":
					if strings.TrimSpace(val) != "" {
						if _, err := validateSuricataEVEPath(val); err != nil {
							c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
							return
						}
					}
				case "openvpn.ovpn_subnet", "openvpn.ovpn_subnet6":
					_, _, err := net.ParseCIDR(val)
					if err != nil {
						c.JSON(http.StatusBadRequest, gin.H{"message": "无效的CIDR格式: " + val})
						return
					}
				case "openvpn.ovpn_push_dns1", "openvpn.ovpn_push_dns2":
					if net.ParseIP(val) == nil {
						c.JSON(http.StatusBadRequest, gin.H{"message": "无效的IP地址: " + val})
						return
					}
				}

				switch val {
				case "true":
					viper.Set(k, true)
				case "false":
					viper.Set(k, false)
				default:
					viper.Set(k, val)
				}
			}

			// 所有字段都被权限过滤掉，返回403
			if savedCount == 0 {
				recordAudit(c, "rbac", "deny", "settings:update", false, "无权限保存任何字段")
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "无权限保存任何字段"})
				return
			}

			if err := viper.WriteConfig(); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}

			// Domain-audit settings are hot-reloaded after persistence succeeds. Disabling
			// removes every feature-owned DNS/DoT/QUIC rule before returning; enabling
			// binds DNS listeners to tun0 before any DNS redirect is installed.
			if webAuditConfigChanged {
				syncWebAuditDNS(context.Background(), &ov)
			}
			if suricataEVEConfigChanged {
				suricataEVE.reconcile()
			}

			c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
		})

		// AI 设置接口
		// GET  /ovpn/settings/ai - 获取 AI 配置（需要 settings:ai 权限，仅授权配置界面使用）
		ovpn.GET("/settings/ai", RequirePermission("settings:ai"), func(c *gin.Context) {
			response, err := aiSettingsAPIResponse(db, aiClient.Provider(), aiClient.Model())
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "load AI settings failed: " + err.Error()})
				return
			}
			c.JSON(http.StatusOK, response)
		})

		ovpn.PUT("/settings/ai", RequirePermission("settings:ai:update"), func(c *gin.Context) {
			var req AISettingsRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"message": "invalid AI settings request", "detail": err.Error()})
				return
			}
			saved, err := saveAISettings(db, req)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"message": "save AI settings failed: " + err.Error()})
				return
			}

			if saved.Enabled {
				newClient, err := ai.NewLLMClient(ai.LLMConfig{Provider: saved.Provider, BaseURL: saved.BaseURL, APIKey: saved.APIKey, Model: saved.Model, MaxTokens: saved.MaxTokens, Temperature: saved.Temperature})
				if err != nil {
					log.Printf("AI settings saved but client initialization failed: %v", err)
					c.JSON(http.StatusOK, gin.H{"message": "AI settings saved, but service initialization failed: " + err.Error()})
					return
				}
				aiClient.Set(newClient)
				tools, toolErr := ai.BuildBusinessTools(aiToolSvc)
				if toolErr != nil {
					log.Printf("AI business tools rebuild failed: %v", toolErr)
					tools = nil
				}
				newRunner, agentErr := ai.NewAgentRunner(newClient, tools, ai.AgentConfig{SystemPrompt: saved.SystemPrompt, MaxTokens: saved.MaxTokens, Temperature: saved.Temperature})
				if agentErr != nil {
					log.Printf("AI runner rebuild failed: %v", agentErr)
					c.JSON(http.StatusOK, gin.H{"message": "AI settings saved, but agent initialization failed: " + agentErr.Error()})
					return
				}
				chatMgr.SetAgentRunner(newRunner)
				// Saving configuration must not consume model tokens; use the explicit connection test to probe.
				healthChecker.SetConfiguredStatus()
				Bus().Publish("ai:session_reset", map[string]any{"reason": "config_changed", "message": "AI configuration changed; start a new chat"})
				log.Printf("AI settings hot-switched (provider=%s, model=%s)", saved.Provider, saved.Model)
			} else {
				aiClient.Set(nil)
				chatMgr.SetAgentRunner(nil)
				healthChecker.SetConfiguredStatus()
				Bus().Publish("ai:session_reset", map[string]any{"reason": "ai_disabled", "message": "AI assistant disabled"})
			}
			recordAudit(c, "config", "save", "settings:ai", true, fmt.Sprintf("Provider=%s, Model=%s", saved.Provider, saved.Model))
			c.JSON(http.StatusOK, gin.H{"message": "AI settings updated"})
		})

		// 客户端安装包管理
		ovpn.GET("/client-packages", RequirePermission("settings:packages"), func(c *gin.Context) {
			pkg := &ClientPackage{}
			packages := pkg.All()
			type PackageWithURL struct {
				ClientPackage
				DownloadURL string `json:"downloadUrl"`
			}
			result := make([]PackageWithURL, 0, len(packages))
			for _, p := range packages {
				pw := PackageWithURL{ClientPackage: p}
				pw.DownloadURL = p.AdminDownloadURL()
				result = append(result, pw)
			}
			c.JSON(http.StatusOK, result)
		})

		ovpn.POST("/client-packages", RequirePermission("settings:packages:upload"), func(c *gin.Context) {
			file, err := c.FormFile("file")
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"message": "请上传安装包文件"})
				return
			}

			if file.Size > 500*1024*1024 {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{"message": "文件大小不能超过 500MB"})
				return
			}

			platform := c.PostForm("platform")
			version := c.PostForm("version")

			validPlatforms := map[string]bool{"windows": true, "macos": true, "linux": true, "android": true, "ios": true}
			if !validPlatforms[platform] {
				c.JSON(http.StatusBadRequest, gin.H{"message": "无效的平台类型，支持: windows, macos, linux, android, ios"})
				return
			}
			if version == "" {
				c.JSON(http.StatusBadRequest, gin.H{"message": "版本号不能为空"})
				return
			}

			tmpPath := filepath.Join(os.TempDir(), "pkg-"+fmt.Sprintf("%d", time.Now().UnixNano())+filepath.Ext(file.Filename))
			if err := c.SaveUploadedFile(file, tmpPath); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "保存上传文件失败"})
				return
			}
			defer os.Remove(tmpPath)

			pkg := &ClientPackage{
				Platform: platform,
				Version:  version,
				Filename: file.Filename,
			}

			if err := pkg.Create(tmpPath); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"message":  "上传成功",
				"id":       pkg.ID,
				"platform": pkg.Platform,
				"version":  pkg.Version,
			})
		})

		ovpn.DELETE("/client-packages/:id", RequirePermission("settings:packages:delete"), func(c *gin.Context) {
			id, err := strconv.ParseUint(c.Param("id"), 10, 32)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"message": "无效的 ID"})
				return
			}

			pkg := &ClientPackage{}
			pkg.ID = uint(id)
			if err := pkg.Delete(); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
		})

		ovpn.POST("/client-packages/:id/enable", RequirePermission("settings:packages:enable"), func(c *gin.Context) {
			id, err := strconv.ParseUint(c.Param("id"), 10, 32)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"message": "无效的 ID"})
				return
			}

			pkg := &ClientPackage{}
			if err := pkg.Activate(uint(id)); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": "已启用"})
		})

		ovpn.GET("/client-packages/:id/download", RequirePermission("settings:packages"), func(c *gin.Context) {
			id, err := strconv.ParseUint(c.Param("id"), 10, 32)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"message": "无效的 ID"})
				return
			}

			pkg := &ClientPackage{}
			result, err := pkg.Get(uint(id))
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"message": "安装包不存在"})
				return
			}

			// 管理员接口允许下载已停用的包（停用也可以用来下载验证或调试）
			filePath := result.FullPath()
			if _, statErr := os.Stat(filePath); statErr != nil {
				logger.Error(context.Background(), "管理员下载包磁盘文件缺失: id="+c.Param("id")+" err="+statErr.Error())
				c.JSON(http.StatusNotFound, gin.H{"message": "安装包文件不存在"})
				return
			}
			c.FileAttachment(filePath, result.Filename)
		})

		ovpn.POST("/server", func(c *gin.Context) {
			a := c.PostForm("action")

			switch a {
			case "settings":
				// auth-user 开关等常规服务管理操作，沿用 server:manage 权限
				if !requirePermissionCode(c, "server:manage") {
					return
				}
				k := c.PostForm("key")
				v := c.PostForm("value")

				if k == "auth-user" {
					msg := "禁用"
					if v == "true" {
						msg = "启用"
					}
					cmd := exec.Command("docker-entrypoint.sh", "auth", v)
					if out, err := cmd.CombinedOutput(); err != nil {
						if len(out) == 0 {
							out = []byte(err.Error())
						}
						logger.Error(context.Background(), string(out))
						c.JSON(http.StatusInternalServerError, gin.H{"message": fmt.Sprintf("%s用户验证失败", msg)})
					} else {
						ov.sendCommand("signal SIGHUP")
						c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("%s用户验证成功", msg)})
					}
				}
			case "renewCert":
				// 证书续签沿用 server:manage 权限
				if !requirePermissionCode(c, "server:manage") {
					return
				}
				day := strings.TrimSpace(c.PostForm("day"))
				if day == "" {
					// 优先使用系统配置的默认续签天数，兜底 365（1 年）
					if cfgDays := viper.GetInt("system.base.renew_days"); cfgDays > 0 {
						day = strconv.Itoa(cfgDays)
					} else {
						day = "365"
					}
				}
				n, err := strconv.Atoi(day)
				if err != nil || n <= 0 {
					c.JSON(http.StatusBadRequest, gin.H{"message": fmt.Sprintf("续签天数必须是大于 0 的整数: %s", day)})
					return
				}
				serverName := viper.GetString("system.base.server_name")

				var message string
				if _, statErr := os.Stat("docker-entrypoint.sh"); statErr == nil {
					out, runErr := exec.Command("docker-entrypoint.sh", "renewcert", day).CombinedOutput()
					if runErr != nil {
						msg := strings.TrimSpace(string(out))
						if msg == "" {
							msg = runErr.Error()
						} else {
							msg = msg + ": " + runErr.Error()
						}
						logger.Error(context.Background(), "更新证书失败: %s", msg)
						c.JSON(http.StatusInternalServerError, gin.H{"message": fmt.Sprintf("更新证书失败: %s", msg)})
						return
					}
					message = "更新证书成功"
				} else {
					msgs, runErr := RenewCA(n)
					if runErr != nil {
						logger.Error(context.Background(), "更新证书失败: %s", runErr.Error())
						c.JSON(http.StatusInternalServerError, gin.H{"message": fmt.Sprintf("更新证书失败: %s", runErr.Error())})
						return
					}
					message = strings.Join(msgs, "；")
					_ = serverName
				}

				ov.sendCommand("signal SIGHUP")
				c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("%s（续签 %s 天）", message, day)})

			case "renewCertByName":
				// 单证书续签：按证书名称（CN）执行 renew，需要 cert:renew 权限
				if !requirePermissionCode(c, "cert:renew") {
					return
				}
				name := strings.TrimSpace(c.PostForm("name"))
				dayStr := strings.TrimSpace(c.PostForm("day"))
				if name == "" {
					c.JSON(http.StatusBadRequest, gin.H{"message": "证书名称不能为空"})
					return
				}
				// 名称/CN 安全校验：避免路径穿越与 shell 异常字符
				if strings.ContainsAny(name, "/\\ \t\n\r\"';$`|&<>()[]{}\x00") {
					c.JSON(http.StatusBadRequest, gin.H{"message": "证书名称包含非法字符"})
					return
				}
				n, perr := strconv.Atoi(dayStr)
				if perr != nil || n <= 0 {
					c.JSON(http.StatusBadRequest, gin.H{"message": "续签天数必须是大于 0 的整数"})
					return
				}
				day := strconv.Itoa(n)

				// CRL 特殊处理：刷新 CRL
				if strings.EqualFold(name, "crl") {
					if err := generateCRL(); err != nil {
						logger.Error(context.Background(), "刷新 CRL 失败: %s", err.Error())
						c.JSON(http.StatusInternalServerError, gin.H{"message": fmt.Sprintf("刷新 CRL 失败: %s", err.Error())})
						return
					}
					ov.sendCommand("signal SIGHUP")
					c.JSON(http.StatusOK, gin.H{"message": "CRL 刷新成功"})
					return
				}

				summary, runErr := RenewByName(name, n)
				if runErr != nil {
					logger.Error(context.Background(), "续签证书[%s]失败: %s", name, runErr.Error())
					c.JSON(http.StatusInternalServerError, gin.H{"message": fmt.Sprintf("续签证书失败: %s", runErr.Error())})
					return
				}
				_ = day
				ov.sendCommand("signal SIGHUP")
				c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("续签证书「%s」成功（%s 天）：%s", name, day, summary)})
			case "restartSrv":
				// 重启 OpenVPN 服务：需要 settings:service:restart 权限
				if !requirePermissionCode(c, "settings:service:restart") {
					return
				}
				_, err := ov.sendCommand("signal SIGHUP")
				if err != nil {
					logger.Error(context.Background(), err.Error())
					c.JSON(http.StatusInternalServerError, gin.H{"message": "重启服务失败"})
					return
				}

				c.JSON(http.StatusOK, gin.H{"message": "重启服务成功"})
			case "getConfig":
				// 读取 server.conf：需要 settings:service:config 权限
				if !requirePermissionCode(c, "settings:service:config") {
					return
				}
				data, err := os.ReadFile(filepath.Join(ovData, "server.conf"))
				if err != nil {
					logger.Error(context.Background(), err.Error())
					c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
					return
				}

				c.JSON(http.StatusOK, gin.H{"content": string(data)})
			case "updateConfig":
				// 编辑 server.conf：需要 settings:service:config 权限
				if !requirePermissionCode(c, "settings:service:config") {
					return
				}
				content := c.PostForm("content")

				file, err := os.OpenFile(filepath.Join(ovData, "server.conf"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
				if err != nil {
					logger.Error(context.Background(), err.Error())
					c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
					return
				}
				defer file.Close()

				_, err = file.WriteString(content)
				if err != nil {
					logger.Error(context.Background(), err.Error())
					c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
					return
				}

				c.JSON(http.StatusOK, gin.H{"message": "配置更新成功"})
			default:
				c.JSON(http.StatusUnprocessableEntity, gin.H{"message": "未知操作"})
			}

		})

		ovpn.POST("/kill", RequirePermission("client:kill"), func(c *gin.Context) {
			cid := c.PostForm("cid")
			ov.killClient(cid)
			c.JSON(http.StatusOK, gin.H{"code": http.StatusOK})
		})

		ovpn.GET("/firewall", RequirePermission("firewall:view"), FirewallHandler)
		ovpn.POST("/firewall", RequirePermission("firewall:create"), FirewallHandler)
		ovpn.PATCH("/firewall", RequirePermission("firewall:update"), FirewallHandler)
		ovpn.DELETE("/firewall/:id", RequirePermission("firewall:delete"), FirewallHandler)

		ovpn.POST("/login", func(c *gin.Context) {
			var u User
			c.ShouldBind(&u)
			u.OvpnConfig = c.PostForm("common_name")

			err := u.Login(true)
			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"message": err.Error()})
			} else {
				c.JSON(http.StatusOK, gin.H{"message": "登录成功"})
			}
		})

		ovpn.GET("/online-client", RequirePermission("client:view_online"), func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"server": ov.getServer(), "clients": ov.getClient()})
		})

		ovpn.GET("/group", RequirePermission("group:view"), func(c *gin.Context) {
			var g Group
			allGroups := g.All()

			// admin 直接返回全部分组
			if isAdmin, _ := c.Get("isAdmin"); isAdmin == true {
				c.JSON(http.StatusOK, allGroups)
				return
			}

			// 普通用户：只返回自己所在分组及其所有下级分组
			currentUsername := ""
			if user, ok := c.Get("user"); ok {
				if s, ok := user.(string); ok {
					currentUsername = s
				}
			}
			if currentUsername == "" {
				c.JSON(http.StatusOK, []Group{})
				return
			}

			currentUser := User{Username: currentUsername}.Info()
			if currentUser.ID == 0 || currentUser.Gid == 0 {
				c.JSON(http.StatusOK, []Group{})
				return
			}

			accessibleIDs := GetSubtreeIDs(currentUser.Gid)
			idSet := make(map[uint]bool, len(accessibleIDs))
			for _, id := range accessibleIDs {
				idSet[id] = true
			}

			filtered := make([]Group, 0, len(allGroups))
			for _, group := range allGroups {
				if idSet[group.ID] {
					filtered = append(filtered, group)
				}
			}
			c.JSON(http.StatusOK, filtered)
		})

		ovpn.GET("/group/:id", RequirePermission("group:view"), func(c *gin.Context) {
			var g Group
			c.JSON(http.StatusOK, g.Get(c.Param("id")))
		})

		ovpn.GET("/group/:id/users", RequirePermission("group:view"), func(c *gin.Context) {
			var auth bool
			var g Group

			gid := c.Param("id")

			cmd := exec.Command("egrep", "^auth-user-pass-verify", filepath.Join(ovData, "server.conf"))
			if err := cmd.Run(); err != nil {
				auth = false
			} else {
				auth = true
			}

			// 数据权限过滤：普通用户只能查看自己分组及下级分组的数据
			session := sessions.Default(c)
			currentUsername := ""
			if user, ok := session.Get("user").(string); ok {
				currentUsername = user
			}

			if currentUsername != adminUsername {
				currentUser := User{Username: currentUsername}.Info()
				accessibleGroupIDs := GetSubtreeIDs(currentUser.Gid)
				requestedGid, _ := strconv.ParseUint(gid, 10, 64)
				found := false
				for _, id := range accessibleGroupIDs {
					if id == uint(requestedGid) {
						found = true
						break
					}
				}
				if !found {
					c.JSON(http.StatusForbidden, gin.H{"message": "无权限查看该分组数据"})
					return
				}
			}

			c.JSON(http.StatusOK, gin.H{"users": g.GetUsers(gid), "authUser": auth})
		})

		ovpn.POST("/group", RequirePermission("group:create"), func(c *gin.Context) {
			var g Group
			c.ShouldBind(&g)

			err := g.Create()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": "添加成功"})
		})

		ovpn.PATCH("/group", RequirePermission("group:update"), func(c *gin.Context) {
			var g Group
			c.ShouldBind(&g)

			if config, ok := c.Request.PostForm["config"]; ok {
				if config[0] == "" {
					db.Model(&g).Update("config", nil)
				}
			}

			err := g.Update()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
		})

		ovpn.DELETE("/group/:id", RequirePermission("group:delete"), func(c *gin.Context) {
			var g Group
			c.ShouldBind(&g)

			err := g.Delete(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusConflict, gin.H{"message": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
		})

		ovpn.GET("/user", RequirePermission("user:view"), func(c *gin.Context) {
			var u User

			username := c.Query("username")
			if username != "" {
				u.Username = username
				c.JSON(http.StatusOK, u.Info())
				return
			}

			// 无 username 参数时返回当前用户可见的用户列表（轻量，仅 id 和 username）
			type SimpleUser struct {
				ID       uint   `json:"id"`
				Username string `json:"username"`
			}

			isAdmin, _ := c.Get("isAdmin")
			currentUsername, _ := c.Get("user")
			currentUserStr, _ := currentUsername.(string)

			var users []SimpleUser

			if isAdmin == true {
				if err := db.WithContext(context.Background()).Model(&User{}).Select("id", "username").Find(&users).Error; err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"message": "查询用户列表失败"})
					return
				}
			} else {
				// 普通用户：只返回自己所在分组及下级分组的用户
				if currentUserStr == "" {
					c.JSON(http.StatusOK, []SimpleUser{})
					return
				}
				accessibleUsers, skipFilter := GetAccessibleUsernames(currentUserStr)
				if skipFilter {
					if err := db.WithContext(context.Background()).Model(&User{}).Select("id", "username").Find(&users).Error; err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"message": "查询用户列表失败"})
						return
					}
				} else {
					if err := db.WithContext(context.Background()).Model(&User{}).Select("id", "username").Where("username IN ?", accessibleUsers).Find(&users).Error; err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"message": "查询用户列表失败"})
						return
					}
				}
			}

			c.JSON(http.StatusOK, gin.H{"data": users})
		})

		ovpn.GET("/user/:id", RequirePermission("user:view"), func(c *gin.Context) {
			var u User
			c.JSON(http.StatusOK, u.Get(c.Param("id")))
		})

		ovpn.GET("/user/me", func(c *gin.Context) {
			session := sessions.Default(c)
			currentUsername := ""
			if user, ok := session.Get("user").(string); ok {
				currentUsername = user
			}
			if currentUsername == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
				return
			}
			u := User{Username: currentUsername}.Info()
			if u.ID == 0 {
				c.JSON(http.StatusNotFound, gin.H{"message": "用户不存在"})
				return
			}
			// 填充 RoleIDs/RoleNames（RoleIDs 为 gorm:"-" 临时字段，需显式加载）
			roleIDs, roleNames := u.LoadRoleIDsAndNames(db)
			u.RoleIDs = roleIDs
			c.JSON(http.StatusOK, gin.H{
				"id":           u.ID,
				"username":     u.Username,
				"name":         u.Name,
				"email":        u.Email,
				"gid":          u.Gid,
				"isEnable":     u.IsEnable,
				"expireDate":   u.ExpireDate,
				"ipAddr":       u.IpAddr,
				"ipRegion":     u.IpRegion,
				"ovpnConfig":   u.OvpnConfig,
				"mfaEnabled":   u.MfaEnabled,
				"isFirstLogin": u.IsFirstLogin,
				"roleIds":      roleIDs,
				"roleNames":    roleNames,
				"lastLoginAt":  u.LastLoginAt,
				"createdAt":    u.CreatedAt,
				"updatedAt":    u.UpdatedAt,
				"aiEnabled":    isAIEnabled(db),
			})
		})

		// 普通用户更新个人信息（不需要 user:update 权限，只允许改自己的 name/email）
		ovpn.PATCH("/user/profile", func(c *gin.Context) {
			session := sessions.Default(c)
			currentUsername := ""
			if user, ok := session.Get("user").(string); ok {
				currentUsername = user
			}
			if currentUsername == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
				return
			}

			// 统一更新 user 表中当前用户的 name/email（admin 已纳入 user 表）
			u := User{Username: currentUsername}.Info()
			if u.ID == 0 {
				c.JSON(http.StatusNotFound, gin.H{"message": "用户不存在"})
				return
			}

			name := strings.TrimSpace(c.Request.FormValue("name"))
			email := strings.TrimSpace(c.Request.FormValue("email"))
			updates := map[string]interface{}{}
			if name != "" && name != u.Name {
				updates["name"] = name
			}
			if email != "" && email != u.Email {
				updates["email"] = email
			}
			if len(updates) > 0 {
				if err := db.Model(&User{}).Where("id = ?", u.ID).Updates(updates).Error; err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"message": fmt.Sprintf("更新失败：%v", err)})
					return
				}
			}
			c.JSON(http.StatusOK, gin.H{"message": "个人资料已更新"})
		})

		r.GET("/user/template", RequirePermission("user:export"), func(c *gin.Context) {
			c.Header("Content-Type", "text/csv")
			c.Header("Content-Disposition", "attachment; filename=user_template.csv")

			c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})

			writer := csv.NewWriter(c.Writer)
			defer writer.Flush()

			writer.Write([]string{"username", "password", "name", "email", "is_enable", "expire_date", "ip_addr", "ovpn_config"})
			writer.Write([]string{"zhangsan", "123456", "寮犱笁", "zhangsan@example.com", "1", "2025-12-01/00:00:00", "10.8.0.222", "tt-gz.ovpn"})
			writer.Write([]string{"lisi", "123456", "鏉庡洓", "lisi@example.com", "0", "", "", "tt-sh.ovpn"})
		})

		ovpn.GET("/user/export", RequirePermission("user:export"), func(c *gin.Context) {
			gid := c.Query("gid")

			fileName := fmt.Sprintf("user_%s.csv", time.Now().Format("20060102150405"))

			c.Header("Content-Type", "text/csv; charset=utf-8")
			c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))
			c.Header("Cache-Control", "no-cache")

			c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})

			writer := csv.NewWriter(c.Writer)
			header := []string{"ID", "用户名", "密码", "姓名", "邮箱", "启用", "过期时间", "IP地址", "配置文件", "MFA", "创建时间"}
			if err := writer.Write(header); err != nil {
				logger.Error(context.Background(), err.Error())
				return
			}
			writer.Flush()

			gQuery := db.Model(&Group{}).
				Select("id").
				Where(`
				parent_id = ?
				OR EXISTS (
					SELECT 1 FROM `+"`group`"+`
					WHERE id = ? AND parent_id IS NULL
				)
				`, gid, gid)

			rows, err := db.Model(&User{}).Where("gid = ? OR gid IN (?)", gid, gQuery).Rows()
			if err != nil {
				return
			}
			defer rows.Close()

			for rows.Next() {
				var u User
				var g Group

				db.ScanRows(rows, &u)

				enable := "0"
				if *u.IsEnable {
					enable = "1"
				}

				dp, _ := aes.AesDecrypt(u.Password, secretKey)
				record := []string{
					strconv.Itoa(int(u.ID)),
					u.Username,
					dp,
					u.Name,
					g.Get(strconv.Itoa(int(u.Gid))).Name,
					enable,
					u.ExpireDate,
					u.IpAddr,
					u.OvpnConfig,
					u.MfaSecret,
					u.CreatedAt.Format("2006-01-02 15:04:05"),
				}

				if err := writer.Write(record); err != nil {
					logger.Error(context.Background(), err.Error())
					return
				}
			}
			writer.Flush()

			if err := writer.Error(); err != nil {
				logger.Error(context.Background(), err.Error())
			}
		})

		ovpn.POST("/user", RequirePermission("user:create"), func(c *gin.Context) {
			var u User
			c.ShouldBind(&u)

			file, err := c.FormFile("file")
			if err != nil {
				if strings.Contains(err.Error(), "no such file") {
					c.JSON(http.StatusInternalServerError, gin.H{"message": "没有上传文件"})
					return
				}
			} else {
				gid := c.PostForm("gid")
				f, _ := file.Open()

				defer f.Close()

				reader := csv.NewReader(f)

				header, err := reader.Read()
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
					return
				}

				if len(header) != 8 {
					c.JSON(http.StatusInternalServerError, gin.H{"message": "导入文件格式错误"})
					return
				}

				// 导入用户：角色继承与单用户创建一致
				// 不再自动继承组角色到 user_role，组角色权限在 LoadPermissionCodes 中动态合并
				// 此处仅设置默认角色，确保用户有基本权限
				importDefaultRoleID := GetDefaultRoleID(db)

				for {
					record, err := reader.Read()
					if err == io.EOF {
						break
					}

					if err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
						return
					}

					enable := record[4] == "1"
					gid64, err := strconv.ParseUint(gid, 10, 64)
					newUser := User{
						Username:   record[0],
						Password:   record[1],
						Name:       record[2],
						Email:      record[3],
						IsEnable:   &enable,
						ExpireDate: strings.Replace(record[5], "/", " ", 1),
						IpAddr:     record[6],
						OvpnConfig: record[7],
						Gid:        uint(gid64),
					}
					// 角色设置：仅设置默认角色（与单用户创建逻辑一致）
					if importDefaultRoleID > 0 {
						newUser.RoleIDs = []uint{importDefaultRoleID}
					}

					err = newUser.Create()
					if err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
						return
					}
				}

				c.JSON(http.StatusOK, gin.H{"message": "导入用户成功"})
				return
			}

			if isFirstLogin, ok := c.Request.PostForm["isFirstLogin"]; ok {
				val := isFirstLogin[0] == "true"
				u.IsFirstLogin = &val
			}

			if mfaEnabled, ok := c.Request.PostForm["mfaEnabled"]; ok {
				u.MfaEnabled = mfaEnabled[0] == "true"
			}

			// 解析 roleIds（前端可显式指定多角色）
			if roleIdsStr, ok := c.Request.PostForm["roleIds"]; ok {
				for _, s := range roleIdsStr {
					s = strings.TrimSpace(s)
					if s == "" {
						continue
					}
					rid, convErr := strconv.ParseUint(s, 10, 64)
					if convErr != nil || rid == 0 {
						continue
					}
					u.RoleIDs = append(u.RoleIDs, uint(rid))
				}
				if len(u.RoleIDs) > 0 {
					if err := validateRoleIDs(db, u.RoleIDs); err != nil {
						c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
						return
					}
				}
			}

			// 未指定角色时：不再自动继承组角色到 user_role
			// 组角色权限在 LoadPermissionCodes 中动态合并（用户直接角色 ∪ 组角色权限）
			// 此处仅设置默认角色，确保用户有基本权限
			if len(u.RoleIDs) == 0 {
				if defaultRoleID := GetDefaultRoleID(db); defaultRoleID > 0 {
					u.RoleIDs = []uint{defaultRoleID}
				}
			}

			err = u.Create()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			} else {
				// 自动创建客户端配置：当 autoCreateClient=true 时，
				// 基于用户名自动生成 <username>.ovpn 客户端配置文件
				autoCreateClient := c.PostForm("autoCreateClient") == "true"
				createdClientName := ""
				if autoCreateClient {
					clientName := u.Username
					if u.OvpnConfig == "" {
						u.OvpnConfig = clientName
						_ = db.Model(&User{}).Where("id = ?", u.ID).Update("ovpn_config", clientName).Error
					} else {
						clientName = u.OvpnConfig
					}
					if err := generateClientConfig(clientName, u.IsMFAEnabled()); err != nil {
						logger.Error(context.Background(), "auto create client for %s failed: %s", u.Username, err)
					} else {
						createdClientName = clientName
					}
				}

				sendNotifyEmail := c.PostForm("sendNotifyEmail")
				if sendNotifyEmail == "true" && strings.TrimSpace(u.Email) != "" {
					go func() {
						var tpl *template.Template
						var buf bytes.Buffer

						activePackages := GetActivePackagesByPlatform()
						var localPackages []LocalPackageInfo
						for platform, pkg := range activePackages {
							localPackages = append(localPackages, LocalPackageInfo{
								Platform:      platform,
								PlatformLabel: PlatformLabel(platform),
								Version:       pkg.Version,
								DownloadURL:   pkg.PublicDownloadURL(),
							})
						}

						tpl, err = template.New("account-email").Parse(accountEmailTemplate)
						if err == nil {
							err = tpl.Execute(&buf, struct {
								Type          string
								Name          string
								Username      string
								Password      string
								SiteUrl       string
								LocalPackages []LocalPackageInfo
							}{
								Type:          "addUser",
								Name:          u.Name,
								Username:      u.Username,
								Password:      c.PostForm("password"),
								SiteUrl:       siteDownloadLandingURL(),
								LocalPackages: localPackages,
							})
						}

						if err != nil {
							logger.Error(context.Background(), "渲染邮件模板失败: %s", err.Error())
							return
						}

						var attachments []string
						if createdClientName != "" {
							clientFilePath := filepath.Join(ovData, "clients", createdClientName+".ovpn")
							attachments = append(attachments, clientFilePath)
						}

						if err := sendUserEmail(u.Email, "用户开通通知", buf.String(), attachments, u.Username, "user_register"); err != nil {
							logger.Error(context.Background(), "发送用户邮件失败: %s", err.Error())
						}
					}()
				}

				message := "添加用户成功"
				if createdClientName != "" {
					message = fmt.Sprintf("添加用户成功，已自动创建客户端配置 %s.ovpn", createdClientName)
				}
				c.JSON(http.StatusOK, gin.H{"message": message})
			}
		})

		ovpn.PATCH("/user", RequirePermission("user:update"), func(c *gin.Context) {
			c.Request.ParseForm()

			var u User
			c.ShouldBind(&u)

			// 解析 roleIds（前端可显式指定多角色）
			// form tag "roleIds" 仅在 PostForm 显式传入时设置；未传入时 u.RoleIDs 保持 nil（不修改）
			if roleIdsStr, ok := c.Request.PostForm["roleIds"]; ok {
				for _, s := range roleIdsStr {
					s = strings.TrimSpace(s)
					if s == "" {
						continue
					}
					rid, convErr := strconv.ParseUint(s, 10, 64)
					if convErr != nil || rid == 0 {
						continue
					}
					u.RoleIDs = append(u.RoleIDs, uint(rid))
				}
				if err := validateRoleIDs(db, u.RoleIDs); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
					return
				}
			}

			// 数据权限校验：普通用户修改目标 gid 时，仅允许迁移到自己所在分组及其下级分组
			// admin 用户跳过此校验
			session := sessions.Default(c)
			currentUsername := ""
			if user, ok := session.Get("user").(string); ok {
				currentUsername = user
			}
			if adminUsername != "" && currentUsername == adminUsername {
				// admin 不做数据权限校验
			} else if currentUsername != "" {
				currentUser := User{Username: currentUsername}.Info()
				if currentUser.ID == 0 {
					c.JSON(http.StatusUnauthorized, gin.H{"message": "用户不存在，请重新登录"})
					return
				}
				// 目标分组必须为当前用户可访问的分组（自己及其下级）
				if u.Gid != 0 && u.Gid != currentUser.Gid {
					accessibleGroupIDs := GetSubtreeIDs(currentUser.Gid)
					found := false
					for _, id := range accessibleGroupIDs {
						if id == u.Gid {
							found = true
							break
						}
					}
					if !found {
						c.JSON(http.StatusForbidden, gin.H{"message": "无权限将用户迁移到该分组"})
						return
					}
				}
			}

			if ipAddr, ok := c.Request.PostForm["ipAddr"]; ok {
				if ipAddr[0] == "" {
					db.Model(&u).Update("ip_addr", nil)
				}
			}

			if expireDate, ok := c.Request.PostForm["expireDate"]; ok {
				if expireDate[0] == "" {
					db.Model(&u).Update("expire_date", nil)
				}
			}

			sendNotifyEmail := c.PostForm("sendNotifyEmail")
			rawPassword := u.Password

			err := u.Update()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			} else {
				if sendNotifyEmail == "true" && rawPassword != "" {
					go func(userID uint, password string) {
						var cu User
						if err := db.First(&cu, userID).Error; err != nil {
							logger.Error(context.Background(), "发送邮件通知失败，查询用户出错: %s", err.Error())
							return
						}

						if cu.Email == "" {
							logger.Error(context.Background(), "发送邮件通知失败，用户没有配置邮箱地址")
							return
						}

						var buf bytes.Buffer
						tpl, tplErr := template.New("account-email").Parse(accountEmailTemplate)
						if tplErr == nil {
							tplErr = tpl.Execute(&buf, struct {
								Type          string
								Name          string
								Username      string
								Password      string
								SiteUrl       string
								LocalPackages []LocalPackageInfo
							}{
								Type:          "resetPass",
								Name:          cu.Name,
								Username:      cu.Username,
								Password:      password,
								SiteUrl:       siteDownloadLandingURL(),
								LocalPackages: nil,
							})
						}
						if tplErr != nil {
							logger.Error(context.Background(), "渲染邮件模板失败: %s", tplErr.Error())
							return
						}

						if sendErr := sendUserEmail(cu.Email, "用户密码配置通知", buf.String(), nil, cu.Username, "password_reset"); sendErr != nil {
							logger.Error(context.Background(), "发送用户邮件失败: %s", sendErr.Error())
						}
					}(u.ID, rawPassword)
				}

				c.JSON(http.StatusOK, gin.H{"message": "用户更新成功"})
			}
		})

		ovpn.DELETE("/user/:id", RequirePermission("user:delete"), func(c *gin.Context) {
			id := c.Param("id")

			// 先获取用户信息，用 username 删除关联的客户端配置
			var u User
			user := u.Get(id)
			if user.ID == 0 {
				c.JSON(http.StatusNotFound, gin.H{"message": "用户不存在"})
				return
			}

			// 内置 admin 用户不允许删除
			if user.Username == adminUsername {
				c.JSON(http.StatusBadRequest, gin.H{"message": "内置 admin 用户不允许删除"})
				return
			}

			username := user.Username
			clientName := strings.TrimSuffix(strings.TrimSpace(user.OvpnConfig), ".ovpn")
			if clientName == "" {
				clientName = username
			}
			if _, nameErr := validateCertificateName(clientName); nameErr != nil {
				// Historical database data can be malformed. Never derive a filesystem
				// path from it; delete only the account record and leave PKI untouched.
				logger.Warn(context.Background(), "skip client artifact cleanup for invalid user VPN config: "+nameErr.Error())
				clientName = ""
			}

			var deleteErr error
			if clientName != "" {
				// Revoke and remove client artifacts through the shared safe deletion path.
				// Missing certificates are allowed for legacy users, but protected PKI material is never touched.
				_, deleteErr = DeleteClientCertificate(clientName)
				if hasCRLReloadPending() {
					if reloadErr := synchronizePendingCRL(&ov); reloadErr != nil {
						logger.Warn(context.Background(), "reload OpenVPN CRL after user deletion failed: "+reloadErr.Error())
						c.JSON(http.StatusServiceUnavailable, gin.H{"message": reloadErr.Error()})
						return
					}
				}
			}
			if deleteErr != nil {
				// A legacy fallback is safe only when the certificate is truly absent.
				// Never erase same-named artifacts after a valid certificate (including
				// a non-default ServerAuth certificate) was deliberately protected.
				if certificateProtectionReason(clientName, nil) != "" {
					c.JSON(http.StatusBadRequest, gin.H{"message": deleteErr.Error()})
					return
				}
				if _, certErr := parseCertificateFile(clientCertPath(clientName)); certErr == nil || !os.IsNotExist(certErr) {
					c.JSON(http.StatusBadRequest, gin.H{"message": deleteErr.Error()})
					return
				}
				logger.Warn(context.Background(), "remove legacy user client certificate artifacts failed: "+deleteErr.Error())
				if _, nameErr := validateCertificateName(clientName); nameErr == nil {
					if cleanupErr := cleanupClientArtifacts(clientName); cleanupErr != nil {
						logger.Warn(context.Background(), "remove legacy user client artifacts failed: "+cleanupErr.Error())
					}
				}
			}

			// 3. 删除用户
			err = u.Delete(id)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			} else {
				c.JSON(http.StatusOK, gin.H{"message": "删除用户成功"})
			}
		})

		ovpn.GET("/client", RequirePermission("client:view"), func(c *gin.Context) {
			clients := make([]ClientConfigData, 0)

			files, _ := os.ReadDir(filepath.Join(ovData, "clients"))

			// 数据权限过滤：普通用户只能看到自己分组及下级分组用户关联的客户端
			isAdmin, _ := c.Get("isAdmin")
			currentUsername, _ := c.Get("user")
			currentUserStr, _ := currentUsername.(string)

			var accessibleConfigs []string
			skipFilter := false

			if isAdmin == true {
				skipFilter = true
			} else if currentUserStr != "" {
				accessibleConfigs, skipFilter = GetAccessibleClientConfigs(currentUserStr)
			}

			configSet := make(map[string]bool)
			if !skipFilter {
				for _, name := range accessibleConfigs {
					configSet[name] = true
				}
			}

			for _, file := range files {
				finfo, _ := file.Info()
				name := strings.TrimSuffix(file.Name(), filepath.Ext(file.Name()))

				if !skipFilter && !configSet[name] {
					continue
				}

				f := ClientConfigData{
					Name:     name,
					FullName: file.Name(),
					File:     fmt.Sprintf("/ovpn/download/%s", file.Name()),
					Date:     finfo.ModTime().Local().Format("2006-01-02 15:04:05"),
				}
				clients = append(clients, f)
			}

			sort.Slice(clients, func(i, j int) bool {
				return clients[i].Date < clients[j].Date
			})

			c.JSON(http.StatusOK, clients)

		})

		ovpn.GET("/client/:name/ccd", RequirePermission("client:view"), func(c *gin.Context) {
			name := c.Param("name")

			isAdmin, _ := c.Get("isAdmin")
			currentUsername, _ := c.Get("user")
			currentUserStr, _ := currentUsername.(string)
			if isAdmin != true && !CanAccessClientConfig(currentUserStr, name) {
				c.JSON(http.StatusForbidden, gin.H{"message": "无权限访问该客户端配置"})
				return
			}

			ccdDir := filepath.Join(ovData, "ccd")

			os.MkdirAll(ccdDir, 0755)

			ccdRoot, err := os.OpenRoot(ccdDir)
			if err != nil {
				logger.Error(context.Background(), err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}
			defer ccdRoot.Close()

			data, err := ccdRoot.ReadFile(name)
			if err != nil {
				if os.IsNotExist(err) {
					c.JSON(http.StatusOK, gin.H{"content": ""})
				} else {
					c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				}
				return
			}

			c.JSON(http.StatusOK, gin.H{"content": string(data)})
		})

		ovpn.GET("/client/:name/config", RequirePermission("client:download"), func(c *gin.Context) {
			name := c.Param("name")

			isAdmin, _ := c.Get("isAdmin")
			currentUsername, _ := c.Get("user")
			currentUserStr, _ := currentUsername.(string)
			if isAdmin != true && !CanAccessClientConfig(currentUserStr, name) {
				c.JSON(http.StatusForbidden, gin.H{"message": "无权限访问该客户端配置"})
				return
			}

			clientsDir := filepath.Join(ovData, "clients")

			clientsRoot, err := os.OpenRoot(clientsDir)
			if err != nil {
				logger.Error(context.Background(), err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}
			defer clientsRoot.Close()

			data, err := clientsRoot.ReadFile(name + ".ovpn")
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"content": string(data)})
		})

		ovpn.PUT("/client/:name/ccd", RequirePermission("client:create"), func(c *gin.Context) {
			name := c.Param("name")

			isAdmin, _ := c.Get("isAdmin")
			currentUsername, _ := c.Get("user")
			currentUserStr, _ := currentUsername.(string)
			if isAdmin != true && !CanAccessClientConfig(currentUserStr, name) {
				c.JSON(http.StatusForbidden, gin.H{"message": "无权限操作该客户端配置"})
				return
			}

			content := c.PostForm("content")
			msg := "客户端更新成功"
			ccdDir := filepath.Join(ovData, "ccd")

			os.MkdirAll(ccdDir, 0755)

			cfg, err := initOvpnConfig()
			if err != nil {
				logger.Error(context.Background(), err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}

			if cfg.Get("client-config-dir") == "" {
				cfg.Set("client-config-dir", ccdDir)
				cfg.Save()

				msg += "（未启用 CCD，需要重启服务生效）"
			}

			ccdRoot, err := os.OpenRoot(ccdDir)
			if err != nil {
				logger.Error(context.Background(), err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}
			defer ccdRoot.Close()

			err = ccdRoot.WriteFile(name, []byte(content), 0644)
			if err != nil {
				logger.Error(context.Background(), err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": msg})
		})

		ovpn.PUT("/client/:name/config", RequirePermission("client:regenerate"), func(c *gin.Context) {
			name := c.Param("name")

			isAdmin, _ := c.Get("isAdmin")
			currentUsername, _ := c.Get("user")
			currentUserStr, _ := currentUsername.(string)
			if isAdmin != true && !CanAccessClientConfig(currentUserStr, name) {
				c.JSON(http.StatusForbidden, gin.H{"message": "无权限操作该客户端配置"})
				return
			}

			content := c.PostForm("content")
			clientsDir := filepath.Join(ovData, "clients")

			clientsRoot, err := os.OpenRoot(clientsDir)
			if err != nil {
				logger.Error(context.Background(), err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}
			defer clientsRoot.Close()

			err = clientsRoot.WriteFile(name+".ovpn", []byte(content), 0644)
			if err != nil {
				logger.Error(context.Background(), err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			} else {
				c.JSON(http.StatusOK, gin.H{"message": "客户端配置更新成功"})
			}
		})

		ovpn.POST("/client", RequirePermission("client:create"), func(c *gin.Context) {
			name := c.PostForm("name")
			serverAddr := strings.TrimSpace(c.PostForm("serverAddr"))
			serverPort := strings.TrimSpace(c.PostForm("serverPort"))
			config := c.PostForm("config")
			ccdConfig := c.PostForm("ccdConfig")

			// 动态查询该客户端名称对应的用户是否已启用 MFA
			mfaEnabled := false
			var queryUser User
			if err := db.Where("username = ? OR ovpn_config = ?", name, name+".ovpn").First(&queryUser).Error; err == nil {
				mfaEnabled = queryUser.IsMFAEnabled()
			}

			clientsDir := filepath.Join(ovData, "clients")
			os.MkdirAll(clientsDir, 0755)

			clientsRoot, err := os.OpenRoot(clientsDir)
			if err != nil {
				logger.Error(context.Background(), err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}
			defer clientsRoot.Close()

			_, err = clientsRoot.Stat(name + ".ovpn")
			if err != nil {
				if os.IsNotExist(err) {
					// 服务器地址：优先用用户填写的；否则从系统配置获取
					if serverAddr == "" {
						serverAddr = strings.TrimSpace(viper.GetString("system.base.server_addr"))
						if serverAddr == "" {
							siteURL := viper.GetString("system.base.site_url")
							if siteURL != "" {
								if u, err := neturl.Parse(siteURL); err == nil && u.Hostname() != "" {
									serverAddr = u.Hostname()
								}
							}
						}
						if serverAddr == "" {
							if v, err := readServerConfKey("local"); err == nil && strings.TrimSpace(v) != "" {
								serverAddr = strings.TrimSpace(v)
							}
						}
						if serverAddr == "" {
							serverAddr = "127.0.0.1"
						}
					}

					// 端口：优先用用户填写的；否则从系统配置获取
					if serverPort == "" {
						serverPort = strings.TrimSpace(viper.GetString("openvpn.ovpn_port"))
						if serverPort == "" {
							serverPort = "1194"
						}
					}

					// 协议和 IPv6
					proto := strings.TrimSpace(viper.GetString("openvpn.ovpn_proto"))
					if proto == "" {
						proto = "udp"
					}
					ipv6 := viper.GetBool("openvpn.ovpn_ipv6")

					// 写入 CCD 配置
					if ccdConfig != "" {
						ccdDir := filepath.Join(ovData, "ccd")
						os.MkdirAll(ccdDir, 0755)
						if err := os.WriteFile(filepath.Join(ccdDir, name), []byte(ccdConfig), 0644); err != nil {
							logger.Error(context.Background(), "写入 CCD 配置失败: %s", err.Error())
						}
					}

					// 使用 Go 版本生成客户端配置
					if err := generateClientConfigGo(name, serverAddr, serverPort, proto, ipv6, config, mfaEnabled); err != nil {
						logger.Error(context.Background(), "客户端添加失败: %s", err.Error())
						c.JSON(http.StatusInternalServerError, gin.H{"message": "客户端添加失败: " + err.Error()})
						return
					}

					c.JSON(http.StatusOK, gin.H{"message": "客户端添加成功"})
					return
				}

				logger.Error(context.Background(), err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"message": "非法客户端名称"})
				return
			}

			c.JSON(http.StatusUnprocessableEntity, gin.H{"message": "客户端已存在"})
		})

		ovpn.DELETE("/client/:name", RequirePermission("client:delete"), func(c *gin.Context) {
			name := c.Param("name")

			isAdmin, _ := c.Get("isAdmin")
			currentUsername, _ := c.Get("user")
			currentUserStr, _ := currentUsername.(string)
			if isAdmin != true && !CanAccessClientConfig(currentUserStr, name) {
				c.JSON(http.StatusForbidden, gin.H{"message": "无权限删除该客户端配置"})
				return
			}

			if revErr := RevokeByName(name); revErr != nil {
				// 如果找不到证书文件，但有 ovpn 文件存在，仍允许删除；证书吊销步骤独立失败
				if fileExists(filepath.Join(ovData, "clients", fmt.Sprintf("%s.ovpn", name))) || fileExists(clientCertPath(name)) {
					logger.Error(context.Background(), "吊销客户端证书失败: %s", revErr.Error())
					c.JSON(http.StatusInternalServerError, gin.H{"message": fmt.Sprintf("吊销客户端证书失败: %s", revErr.Error())})
					return
				}
				logger.Warn(context.Background(), "吊销证书失败，继续清理客户端文件: %s", revErr.Error())
			} else {
				if reloadErr := synchronizePendingCRL(&ov); reloadErr != nil {
					logger.Error(context.Background(), "reload OpenVPN CRL after client deletion failed: "+reloadErr.Error())
					c.JSON(http.StatusServiceUnavailable, gin.H{"message": reloadErr.Error()})
					return
				}
				if err := removeClientCredentials(name); err != nil {
					logger.Warn(context.Background(), "remove revoked client credentials failed: "+err.Error())
				}
			}

			dataRoot, err := os.OpenRoot(ovData)
			if err != nil {
				logger.Error(context.Background(), err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}
			defer dataRoot.Close()

			dataRoot.Remove(filepath.Join("clients", fmt.Sprintf("%s.ovpn", name)))
			dataRoot.Remove(filepath.Join("ccd", name))

			c.JSON(http.StatusOK, gin.H{"message": "删除客户端成功"})
		})

		// 网站访问 DNS 审计：仅普通 DNS 域名元数据，按数据范围授权。
		ovpn.GET("/web-audit/status", RequirePermission("web-audit:view"), ov.websiteAuditStatus)
		ovpn.GET("/web-audit/suricata/status", RequirePermission("web-audit:view"), ov.suricataEVEStatus)
		ovpn.GET("/web-audit/suricata/records", RequirePermission("web-audit:view"), ov.suricataNetworkAuditRecords)
		ovpn.GET("/web-audit/suricata/export", RequirePermission("web-audit:view"), ov.suricataNetworkAuditExport)
		ovpn.GET("/web-audit/summary", RequirePermission("web-audit:view"), ov.websiteAuditSummary)
		ovpn.GET("/web-audit/users", RequirePermission("web-audit:view"), ov.websiteAuditUsers)
		ovpn.GET("/web-audit/records", RequirePermission("web-audit:view"), ov.websiteAuditRecords)
		ovpn.GET("/web-audit/export", RequirePermission("web-audit:view"), ov.websiteAuditExport)
		// OpenVPN local hooks keep DNS audit attribution accurate during VPN IP reuse.
		ovpn.POST("/web-audit/client-map", ov.websiteAuditClientMap)

		// AI 助手路由（需要 ai:chat 权限）
		// 始终注册路由，由 handler 内部判断 LLM 客户端是否就绪
		ai.RegisterAIRoutes(ovpn.Group("/ai", RequirePermission("ai:chat")), chatMgr, aiClient, healthChecker)

		ovpn.GET("/history/:id/web-audit", RequirePermission("history:view"), ov.historyWebsiteAudit)

		ovpn.GET("/history", RequirePermission("history:view"), func(c *gin.Context) {
			var h History
			var p Params

			c.ShouldBindQuery(&p)

			// 数据权限过滤：普通用户只能看到自己所在分组及下级分组用户的连接历史
			isAdmin, _ := c.Get("isAdmin")
			currentUsername, _ := c.Get("user")
			currentUserStr, _ := currentUsername.(string)

			var accessibleUserIDs []uint
			skipFilter := false

			if isAdmin == true {
				skipFilter = true
			} else if currentUserStr != "" {
				accessibleUserIDs, skipFilter = GetAccessibleUserIDs(currentUserStr)
			}

			c.JSON(http.StatusOK, h.Query(p, accessibleUserIDs, skipFilter))
		})

		ovpn.POST("/history", func(c *gin.Context) {
			var h History
			c.ShouldBind(&h)

			err := h.Create()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}

			event := NotifyEvent{
				Event:         "disconnect",
				Vip:           h.Vip,
				Vip6:          h.Vip6,
				Rip:           h.Rip,
				Rip6:          h.Rip6,
				CommonName:    h.CommonName,
				Username:      h.Username,
				BytesReceived: h.BytesReceived,
				BytesSent:     h.BytesSent,
				TimeUnix:      h.TimeUnix,
				TimeDuration:  h.TimeDuration,
			}
			// A disconnect must be recorded before returning, but delivery to SMTP
			// or webhooks is best-effort and must not hold OpenVPN's lifecycle hook.
			enqueueLifecycleNotification(event)

			c.JSON(http.StatusOK, gin.H{"message": "添加记录成功"})
		})

		ovpn.GET("/notify/logs", func(c *gin.Context) {
			limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
			uname, _ := c.Get("user")
			unameStr, _ := uname.(string)
			isAdmin, _ := c.Get("isAdmin")
			c.JSON(http.StatusOK, gin.H{"data": queryNotifyLogs(limit, unameStr, isAdmin == true)})
		})

		// 站内信：未读数 + 标记已读
		ovpn.GET("/notify/unread-count", func(c *gin.Context) {
			username, _ := c.Get("user")
			uname, _ := username.(string)
			isAdmin, _ := c.Get("isAdmin")
			rec := getUserNotifyRead(uname)
			total := countUnreadNotifyLogs(rec.LastReadID, uname, isAdmin == true)
			c.JSON(http.StatusOK, gin.H{
				"unread":     total,
				"lastReadId": rec.LastReadID,
				"maxId":      maxNotifyLogID(uname, isAdmin == true),
			})
		})

		ovpn.POST("/notify/mark-read", func(c *gin.Context) {
			username, _ := c.Get("user")
			uname, _ := username.(string)
			if uname == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
				return
			}
			isAdmin, _ := c.Get("isAdmin")
			// 可选：客户端传入 lastReadId；未传则推进到当前最大 id
			var body struct {
				LastReadID uint `json:"lastReadId"`
			}
			_ = c.ShouldBind(&body)
			target := body.LastReadID
			if target == 0 {
				target = maxNotifyLogID(uname, isAdmin == true)
			}
			rec, err := markUserNotifyRead(uname, target)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"message":    "marked",
				"lastReadId": rec.LastReadID,
				"unread":     countUnreadNotifyLogs(rec.LastReadID, uname, isAdmin == true),
			})
		})

		// WebSocket：站内信实时推送
		ovpn.GET("/ws/notifications", func(c *gin.Context) {
			username, _ := c.Get("user")
			if uname, ok := username.(string); !ok || uname == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
				return
			}
			permissions := make(map[string]bool)
			if isAdmin, ok := c.Get("isAdmin"); ok {
				if admin, _ := isAdmin.(bool); admin {
					permissions["*"] = true
				}
			}
			if raw, ok := c.Get("permissions"); ok {
				if codes, ok := raw.([]string); ok {
					for _, code := range codes {
						permissions[code] = true
					}
				}
			}
			WsHubInstance().ServeWs(c.Writer, c.Request, permissions)
		})

		// 通知渠道维护：CRUD + Test
		ovpn.GET("/channel-types", RequirePermission("channel:view"), channelTypesHandler)
		ovpn.GET("/channel", RequirePermission("channel:view"), channelHandler)
		ovpn.GET("/channel/:id", RequirePermission("channel:view"), channelHandler)
		ovpn.POST("/channel", RequirePermission("channel:create"), channelHandler)
		ovpn.PUT("/channel/:id", RequirePermission("channel:update"), channelHandler)
		ovpn.PATCH("/channel/:id", RequirePermission("channel:update"), channelHandler)
		ovpn.DELETE("/channel/:id", RequirePermission("channel:delete"), channelHandler)
		ovpn.POST("/channel/:id/test", RequirePermission("channel:test"), channelTestHandler)

		ovpn.POST("/notify", func(c *gin.Context) {
			bytesReceived, _ := strconv.ParseFloat(c.PostForm("bytes_received"), 64)
			bytesSent, _ := strconv.ParseFloat(c.PostForm("bytes_sent"), 64)
			timeUnix, _ := strconv.ParseInt(c.PostForm("time_unix"), 10, 64)
			timeDuration, _ := strconv.ParseInt(c.PostForm("time_duration"), 10, 64)

			event := NotifyEvent{
				Event:         c.PostForm("event"),
				Vip:           c.PostForm("vip"),
				Vip6:          c.PostForm("vip6"),
				Rip:           c.PostForm("rip"),
				Rip6:          c.PostForm("rip6"),
				CommonName:    c.PostForm("common_name"),
				Username:      c.PostForm("username"),
				BytesReceived: bytesReceived,
				BytesSent:     bytesSent,
				TimeUnix:      timeUnix,
				TimeDuration:  timeDuration,
			}

			if event.Event == "" {
				event.Event = "connect"
			}
			if event.Username == "" {
				event.Username = "openvpn-test"
			}
			if event.CommonName == "" {
				event.CommonName = event.Username + "-client"
			}
			if event.Vip == "" {
				event.Vip = "10.8.0.100"
			}
			if event.Rip == "" {
				event.Rip = "127.0.0.1"
			}
			if event.TimeUnix == 0 {
				event.TimeUnix = time.Now().Unix()
			}

			// This endpoint is invoked by the OpenVPN client-connect hook. Queue the
			// potentially slow fan-out instead of blocking TLS handshakes on SMTP or
			// webhook latency.
			enqueueLifecycleNotification(event)

			c.JSON(http.StatusAccepted, gin.H{"message": "notify queued"})
		})

		ovpn.GET("/history/export", RequirePermission("history:view"), func(c *gin.Context) {
			var p Params
			c.ShouldBindQuery(&p)

			// 数据权限过滤：普通用户只能看到自己所在分组及下级分组用户的连接历史
			currentUsername, _ := c.Get("user")
			currentUserStr, _ := currentUsername.(string)
			isAdmin, _ := c.Get("isAdmin")

			var accessibleUserIDs []uint
			skipFilter := false
			if isAdmin == true {
				skipFilter = true
			} else {
				accessibleUserIDs, skipFilter = GetAccessibleUserIDs(currentUserStr)
			}

			fileName := fmt.Sprintf("history_%s.csv", time.Now().Format("20060102150405"))

			c.Header("Content-Type", "text/csv; charset=utf-8")
			c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))
			c.Header("Cache-Control", "no-cache")

			c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})

			writer := csv.NewWriter(c.Writer)
			header := []string{"ID", "用户名", "客户端", "VPN IP", "用户 IP", "下行流量", "上行流量", "上线时间", "在线时长", "创建时间"}
			if err := writer.Write(header); err != nil {
				logger.Error(context.Background(), err.Error())
				return
			}
			writer.Flush()

			query := db.Model(&History{})
			if p.Qt != "" {
				qt := strings.Split(p.Qt, ",")
				if len(qt) == 2 {
					query = query.Where("time_unix BETWEEN ? AND ?", qt[0], qt[1])
				}
			}

			// 数据权限过滤
			if !skipFilter && len(accessibleUserIDs) > 0 {
				query = query.Where("user_id IN ?", accessibleUserIDs)
			}

			rows, err := query.Rows()
			if err != nil {
				return
			}
			defer rows.Close()

			for rows.Next() {
				var h History

				db.ScanRows(rows, &h)
				record := []string{
					strconv.Itoa(int(h.ID)),
					h.Username,
					h.CommonName,
					h.Vip,
					h.Rip,
					tools.FormatBytes(h.BytesReceived),
					tools.FormatBytes(h.BytesSent),
					time.Unix(h.TimeUnix, 0).Format("2006-01-02 15:04:05"),
					(time.Duration(h.TimeDuration) * time.Second).String(),
					h.CreatedAt.Format("2006-01-02 15:04:05"),
				}

				if err := writer.Write(record); err != nil {
					logger.Error(context.Background(), err.Error())
					return
				}
			}
			writer.Flush()

			if err := writer.Error(); err != nil {
				logger.Error(context.Background(), err.Error())
			}
		})

		ovpn.GET("/certs", RequirePermission("cert:view"), func(c *gin.Context) {
			c.JSON(http.StatusOK, getCerts(ovData))
		})

		ovpn.DELETE("/certs", RequirePermission("cert:delete"), func(c *gin.Context) {
			var request struct {
				Names []string `json:"names"`
			}
			if err := c.ShouldBindJSON(&request); err != nil || len(request.Names) == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"message": "names must contain at least one client certificate"})
				return
			}
			if len(request.Names) > 50 {
				c.JSON(http.StatusBadRequest, gin.H{"message": "at most 50 certificates may be deleted at once"})
				return
			}
			results := DeleteClientCertificates(request.Names)
			if hasCRLReloadPending() {
				if reloadErr := synchronizePendingCRL(&ov); reloadErr != nil {
					logger.Warn(context.Background(), "reload OpenVPN CRL after certificate deletion failed: "+reloadErr.Error())
					successCount := markCertificateReloadPending(results, reloadErr)
					c.JSON(http.StatusServiceUnavailable, gin.H{
						"message":      reloadErr.Error(),
						"results":      results,
						"successCount": successCount,
						"total":        len(results),
					})
					return
				}
			}
			successCount := 0
			for _, result := range results {
				if result.Success {
					successCount++
				}
			}
			c.JSON(http.StatusOK, gin.H{"results": results, "successCount": successCount, "total": len(results)})
		})

		// 角色管理：CRUD + 分配权限
		ovpn.GET("/role", RequirePermission("role:view"), roleListHandler)
		ovpn.GET("/role/:id", RequirePermission("role:view"), roleDetailHandler)
		ovpn.POST("/role", RequirePermission("role:create"), roleCreateHandler)
		ovpn.PATCH("/role/:id", RequirePermission("role:update"), roleUpdateHandler)
		ovpn.DELETE("/role/:id", RequirePermission("role:delete"), roleDeleteHandler)
		ovpn.PUT("/role/:id/permissions", RequirePermission("role:assign_permissions"), roleAssignPermissionsHandler)
		ovpn.GET("/role/:id/users", RequirePermission("role:assign_users"), roleUsersHandler)
		ovpn.PUT("/role/:id/users", RequirePermission("role:assign_users"), roleAssignUsersHandler)
		ovpn.GET("/role/:id/groups", RequirePermission("role:assign_groups"), roleGroupsHandler)
		ovpn.PUT("/role/:id/groups", RequirePermission("role:assign_groups"), roleAssignGroupsHandler)

		// 权限定义查询：返回权限树供 Sidebar 动态渲染菜单和角色编辑页使用
		// 所有已登录用户均可访问（菜单渲染是基本功能，不涉及敏感操作）
		ovpn.GET("/permission/tree", permissionTreeHandler)

		// 权限管理：CRUD + 批量排序（需 permission:manage 权限）
		ovpn.POST("/permission", RequirePermission("permission:manage"), permissionCreateHandler)
		ovpn.PUT("/permission/:id", RequirePermission("permission:manage"), permissionUpdateHandler)
		ovpn.DELETE("/permission/:id", RequirePermission("permission:manage"), permissionDeleteHandler)
		ovpn.PUT("/permission/sort", RequirePermission("permission:manage"), permissionSortHandler)
	}

	client := r.Group("/client")
	{
		client.GET("/userinfo", func(c *gin.Context) {
			var u User

			session := sessions.Default(c)
			if user, ok := session.Get("user").(string); ok {
				u.Username = user
			}

			if ldapAuth {
				l, err := InitLdap()
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
					return
				}

				lu, err := l.Get(u.Username)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
					return
				}

				// LDAP 用户也补充权限信息
				// 角色被禁用或不存在时返回 403，与登录时行为一致，避免静默返回空权限让前端陷入空白页
				permCodes, perr := u.LoadPermissionCodes(db)
				if errors.Is(perr, ErrRoleDisabled) || errors.Is(perr, ErrRoleNotFound) {
					c.JSON(http.StatusForbidden, gin.H{"message": "角色已禁用或不存在，请联系管理员"})
					return
				}
				if perr != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"message": "权限加载失败，请稍后重试"})
					return
				}
				if permCodes == nil {
					permCodes = []string{}
				}
				c.JSON(http.StatusOK, gin.H{
					"id":           0,
					"username":     lu.Username,
					"name":         "",
					"email":        "",
					"ldapAuth":     true,
					"isFirstLogin": false,
					"isAdmin":      adminUsername != "" && u.Username == adminUsername,
					"permissions":  permCodes,
					"roleIds":      []uint{},
					"roleNames":    []string{},
				})
				return
			}

			userInfo := u.Info()
			// 用户已被删除：session 仍有效但 DB 无记录，返回 401 让前端登出
			if userInfo.ID == 0 {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "用户不存在，请重新登录"})
				return
			}
			permCodes, perr := userInfo.LoadPermissionCodes(db)
			if errors.Is(perr, ErrRoleDisabled) || errors.Is(perr, ErrRoleNotFound) {
				c.JSON(http.StatusForbidden, gin.H{"message": "角色已禁用或不存在，请联系管理员"})
				return
			}
			if perr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "权限加载失败，请稍后重试"})
				return
			}
			if permCodes == nil {
				permCodes = []string{}
			}
			userRoleIDs, userRoleNames := userInfo.LoadRoleIDsAndNames(db)
			c.JSON(http.StatusOK, gin.H{
				"id":           userInfo.ID,
				"username":     userInfo.Username,
				"name":         userInfo.Name,
				"email":        userInfo.Email,
				"isFirstLogin": userInfo.IsFirstLogin,
				"isAdmin":      adminUsername != "" && userInfo.Username == adminUsername,
				"permissions":  permCodes,
				"roleIds":      userRoleIDs,
				"roleNames":    userRoleNames,
			})
		})

		client.PUT("/userinfo", func(c *gin.Context) {
			session := sessions.Default(c)
			currentUsername := ""
			if user, ok := session.Get("user").(string); ok {
				currentUsername = user
			}

			// 统一走 user 表更新（admin 已纳入 user 表，与普通用户一致）
			cu := User{Username: currentUsername}.Info()
			// 用户已被删除：session 仍有效但 DB 无记录，返回 401 让前端登出
			if cu.ID == 0 {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "用户不存在，请重新登录"})
				return
			}
			formID := c.PostForm("id")
			formIDUint, _ := strconv.ParseUint(formID, 10, 64)
			if formID == "" || cu.ID != uint(formIDUint) {
				c.JSON(http.StatusForbidden, gin.H{"message": "仅可修改自己的资料"})
				return
			}
			// 更新 user 表的 name/email
			// 使用 struct Updates 触发 BeforeSave 钩子（bluemonday XSS 净化）
			name := c.PostForm("name")
			email := c.PostForm("email")
			if err := db.Model(&cu).Updates(User{Name: name, Email: email}).Error; err != nil {
				recordAudit(c, "user", "update_own", cu.Username, false, err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}
			recordAudit(c, "user", "update_own", cu.Username, true, "更新个人资料")
			c.JSON(http.StatusOK, gin.H{"message": "个人资料已更新"})
		})

		client.POST("/modifyPass", func(c *gin.Context) {
			var u User
			c.ShouldBind(&u)

			session := sessions.Default(c)
			currentUsername := ""
			if user, ok := session.Get("user").(string); ok {
				currentUsername = user
			}

			// 当前登录用户（来自 session，不可被请求体篡改）
			var cu User
			// 校验请求中的 id 与当前登录用户一致
			if currentUsername == adminUsername {
				// admin 用户：忽略 u.ID 校验，密码存放在配置中
			} else {
				cu = User{Username: currentUsername}.Info()
				if u.ID != cu.ID {
					c.JSON(http.StatusInternalServerError, gin.H{"message": "非法请求"})
					return
				}
			}

			if !isValidPassword(u.Password) {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "密码不满足要求（长度12位，包含大小写字母、数字、特殊字符）"})
				return
			}

			currentPass := c.Request.PostFormValue("currentPass")

			if currentUsername == adminUsername {
				if currentPass == "" {
					c.JSON(http.StatusBadRequest, gin.H{"message": "管理员修改密码需要输入当前密码"})
					return
				}
				if bcrypt.CompareHashAndPassword([]byte(adminPassword), []byte(currentPass)) != nil {
					c.JSON(http.StatusUnauthorized, gin.H{"message": "当前密码错误"})
					return
				}

				passwd, err := bcrypt.GenerateFromPassword([]byte(u.Password), 12)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
					return
				}

				// 同步到 user 表 admin 用户的 password（struct Updates 触发 BeforeSave AES 加密）
				adminUser := User{Username: adminUsername}.Info()
				if adminUser.ID > 0 {
					if e := db.Model(&User{}).Where("id = ?", adminUser.ID).Updates(User{Password: string(passwd)}).Error; e != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"message": "同步密码到 user 表失败: " + e.Error()})
						return
					}
				}

				viper.Set("system.base.admin_password", string(passwd))
				viper.WriteConfig()
				adminPassword = string(passwd)

				c.JSON(http.StatusOK, gin.H{"message": "密码修改成功"})
				return
			}

			if currentPass != "" {
				if cu.Info().Password != currentPass {
					c.JSON(http.StatusUnauthorized, gin.H{"message": "当前密码错误"})
					return
				}
			}

			err := db.Transaction(func(tx *gorm.DB) error {
				data := User{
					Password: u.Password,
				}

				if isFirstLogin, ok := c.Request.PostForm["isFirstLogin"]; ok {
					val := isFirstLogin[0] == "true"
					data.IsFirstLogin = &val
				}

				if err := tx.Model(&u).Updates(data).Error; err != nil {
					return err
				}

				return nil
			})

			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			} else {
				c.JSON(http.StatusOK, gin.H{"message": "密码修改成功"})
			}
		})

		client.GET("/userConfig", func(c *gin.Context) {
			var u User
			session := sessions.Default(c)
			if user, ok := session.Get("user").(string); ok {
				u.Username = user
			}

			u = u.Info()
			configName := u.OvpnConfig

			if ldapAuth {
				l, err := InitLdap()
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
					return
				}

				lu, err := l.Get(u.Username)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
					return
				}

				configName = lu.OvpnConfig
			}

			if configName == "" {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "该账号未指定配置文件，请联系管理员"})
				return
			}

			clientsDir := filepath.Join(ovData, "clients")

			clientsRoot, err := os.OpenRoot(clientsDir)
			if err != nil {
				logger.Error(context.Background(), err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}
			defer clientsRoot.Close()

			data, err := clientsRoot.ReadFile(configName)
			if err != nil {
				logger.Error(context.Background(), err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"message": "读取配置文件失败"})
				return
			}

			challengeLine := `static-challenge "Enter MFA code" 1`
			content := string(data)

			if u.IsMFAEnabled() {
				if !strings.Contains(content, challengeLine) {
					if !strings.HasSuffix(content, "\n") {
						content += "\n"
					}
					content += challengeLine + "\n"
				}
			} else {
				content = strings.ReplaceAll(content, challengeLine+"\n", "")
			}

			cfg, err := initOvpnConfig()
			if err != nil {
				logger.Error(context.Background(), err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}

			if cfg.Get("auth-user-pass-verify") != "" {
				if strings.Contains(content, "#auth-user-pass") {
					content = strings.ReplaceAll(content, "#auth-user-pass", "auth-user-pass")
				}
			} else {
				if !strings.Contains(content, "#auth-user-pass") {
					content = strings.ReplaceAll(content, "auth-user-pass", "#auth-user-pass")
				}
			}

			c.JSON(http.StatusOK, gin.H{"filename": configName, "content": content})
		})

		client.GET("/mfa", func(c *gin.Context) {
			if ldapAuth {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "LDAP 用户不支持设置 MFA"})
				return
			}

			var u User

			session := sessions.Default(c)
			if user, ok := session.Get("user").(string); ok {
				u.Username = user
			}

			u = u.Info()
			if u.MfaSecret == "" {
				secret, otpauthUrl, err := GenMfa(u.Username)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"message": fmt.Errorf("MFA: %w", err).Error()})
				} else {
					u.MfaSecret = secret
					c.JSON(http.StatusOK, gin.H{"mfaEnable": false, "user": u, "otpauthUrl": otpauthUrl})
				}
			} else {
				c.JSON(http.StatusOK, gin.H{"mfaEnable": true, "user": u})
			}
		})

		client.POST("/mfa", func(c *gin.Context) {
			var u User
			c.ShouldBind(&u)

			session := sessions.Default(c)
			if user, ok := session.Get("user").(string); ok {
				cu := User{Username: user}.Info()
				if u.ID != cu.ID {
					c.JSON(http.StatusInternalServerError, gin.H{"message": "非法请求"})
					return
				}
			}

			passcode := c.PostForm("passcode")

			vaild := ValidateMfa(passcode, u.MfaSecret)
			if !vaild {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "验证码错误"})
			} else {
				db.Model(&User{}).Where("id = ?", u.ID).Updates(map[string]interface{}{
					"mfa_secret":  u.MfaSecret,
					"mfa_enabled": true,
				})

				// 异步更新客户端配置文件（添加 static-challenge）并发送邮件通知
				go func(userID uint) {
					cu := User{ID: userID}.Info()
					if cu.Email == "" {
						return
					}

					// 更新该用户的所有客户端配置文件（添加 MFA 验证）
					updatedConfigs, err := RegenerateUserClientConfigs(cu.Username, true)
					if err != nil {
						logger.Error(context.Background(), "更新客户端配置失败: %s", err.Error())
					}

					var buf bytes.Buffer
					tpl, err := template.New("account-email").Parse(accountEmailTemplate)
					if err != nil {
						logger.Error(context.Background(), "渲染邮件模板失败: %s", err.Error())
						return
					}
					err = tpl.Execute(&buf, struct {
						Type          string
						Name          string
						Username      string
						Password      string
						SiteUrl       string
						LocalPackages []LocalPackageInfo
					}{
						Type:     "mfaEnabled",
						Name:     cu.Name,
						Username: cu.Username,
						SiteUrl:  siteDownloadLandingURL(),
					})
					if err != nil {
						logger.Error(context.Background(), "渲染邮件模板失败: %s", err.Error())
						return
					}

					var attachments []string
					for _, configName := range updatedConfigs {
						clientFilePath := filepath.Join(ovData, "clients", configName+".ovpn")
						if _, err := os.Stat(clientFilePath); err == nil {
							attachments = append(attachments, clientFilePath)
						}
					}

					if err := sendUserEmail(cu.Email, "MFA 已启用通知", buf.String(), attachments, cu.Username, "mfa_enabled"); err != nil {
						logger.Error(context.Background(), "发送 MFA 启用邮件失败: %s", err.Error())
					}
				}(u.ID)

				c.JSON(http.StatusOK, gin.H{"message": "MFA 已启用，最新客户端配置文件已发送至您的邮箱"})
			}
		})

		client.DELETE("/mfa/:id", func(c *gin.Context) {
			var u User
			c.ShouldBindUri(&u)

			session := sessions.Default(c)
			if user, ok := session.Get("user").(string); ok {
				cu := User{Username: user}.Info()
				if !(u.ID == cu.ID || cu.Username == adminUsername) {
					c.JSON(http.StatusForbidden, gin.H{"message": "非法请求"})
					return
				}
			}

			targetUser := User{ID: u.ID}.Info()
			db.Model(&User{}).Where("id = ?", u.ID).Updates(map[string]interface{}{
				"mfa_secret":  nil,
				"mfa_enabled": false,
			})

			go func() {
				if targetUser.Email != "" {
					// 更新该用户的所有客户端配置文件（移除 MFA 验证）
					updatedConfigs, err := RegenerateUserClientConfigs(targetUser.Username, false)
					if err != nil {
						logger.Error(context.Background(), "更新客户端配置失败: %s", err.Error())
					}

					var tpl *template.Template
					var buf bytes.Buffer

					tpl, err = template.New("account-email").Parse(accountEmailTemplate)
					if err == nil {
						err = tpl.Execute(&buf, struct {
							Type          string
							Name          string
							Username      string
							Password      string
							SiteUrl       string
							LocalPackages []LocalPackageInfo
						}{
							Type:          "resetMfa",
							Name:          targetUser.Name,
							Username:      targetUser.Username,
							Password:      "",
							SiteUrl:       siteDownloadLandingURL(),
							LocalPackages: nil,
						})
					}

					if err != nil {
						logger.Error(context.Background(), "渲染邮件模板失败: %s", err.Error())
						return
					}

					var attachments []string
					for _, configName := range updatedConfigs {
						clientFilePath := filepath.Join(ovData, "clients", configName+".ovpn")
						if _, err := os.Stat(clientFilePath); err == nil {
							attachments = append(attachments, clientFilePath)
						}
					}

					if err := sendUserEmail(targetUser.Email, "用户 MFA 重置通知", buf.String(), attachments, targetUser.Username, "mfa_reset"); err != nil {
						logger.Error(context.Background(), "发送用户邮件失败: %s", err.Error())
					}
				}
			}()

			c.JSON(http.StatusOK, gin.H{"message": "MFA 已停用"})
		})
	}

	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/ovpn") ||
			strings.HasPrefix(path, "/client/") ||
			path == "/client" ||
			strings.HasPrefix(path, "/static") ||
			strings.HasPrefix(path, "/download") ||
			strings.HasPrefix(path, "/ws") ||
			strings.HasPrefix(path, "/email") {
			c.JSON(http.StatusNotFound, gin.H{"message": "not found"})
			return
		}
		c.HTML(http.StatusOK, "index.html", reactRuntime("admin", conf.Client.ClientUrl))
	})

	r.Run(fmt.Sprintf(":%s", webPort))
}
