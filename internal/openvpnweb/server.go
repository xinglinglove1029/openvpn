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
	"github.com/glebarez/sqlite"
	"github.com/patrickmn/go-cache"
	"github.com/robfig/cron/v3"
	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	gLogger "gorm.io/gorm/logger"
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
	Name      string `json:"name"`
	Type      string `json:"type"`
	Subject   string `json:"subject"`
	Issuer    string `json:"issuer"`
	NotBefore string `json:"notBefore"`
	NotAfter  string `json:"notAfter"`
	ExpiresIn string `json:"expiresIn"`
	Status    string `json:"status"`
	SerialNo  string `json:"serialNo"`
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

func (ov *ovpn) sendCommand(command string) (string, error) {
	var data string
	var sb strings.Builder

	conn, err := net.DialTimeout("tcp", ov.address, time.Second*3)
	if err != nil {
		logger.Error(context.Background(), err.Error())
		return data, err
	}

	defer conn.Close()

	conn.SetDeadline(time.Now().Add(time.Second * 3))
	conn.Write([]byte(fmt.Sprintf("%s\n", command)))

	for {
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)

		re := regexp.MustCompile(">INFO(.)*\r\n")
		if str := re.ReplaceAllString(string(buf[:n]), ""); str != "" {
			sb.Write([]byte(str))
		}

		if err != nil || strings.HasSuffix(sb.String(), "\r\nEND\r\n") || strings.HasPrefix(sb.String(), "SUCCESS:") {
			break
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

func getCerts(ovData string) []CertData {
	cers := make([]CertData, 0)
	pkiDir := filepath.Join(ovData, "pki")

	caPath := filepath.Join(pkiDir, "ca.crt")
	if cert, err := parseCert(caPath); err == nil {
		cers = append(cers, *cert)
	} else {
		logger.Error(context.Background(), err.Error())
	}

	crlPath := filepath.Join(pkiDir, "crl.pem")
	if cert, err := parseCrl(crlPath); err == nil {
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

func AuthMiddleWare() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		user := session.Get("user")

		if c.GetHeader("O-Token") == viper.GetString("system.base.token") {
			if c.Request.URL.Path == "/ovpn/login" || c.Request.URL.Path == "/ovpn/history" || c.Request.URL.Path == "/ovpn/firewall" || c.Request.URL.Path == "/ovpn/notify" {
				if IsLocalRequest(c) {
					c.Next()
					return
				}
			}
		}

		if user == nil {
			c.Redirect(302, "/login")
			c.Abort()
			return
		}

		if user, ok := user.(string); ok {
			c.Set("user", user)

			isPublicPath := func(path string) bool {
				if path == "/" ||
					strings.HasPrefix(path, "/client") ||
					strings.HasPrefix(path, "/ovpn/ws") ||
					strings.HasPrefix(path, "/ovpn/notify") ||
					strings.HasPrefix(path, "/ovpn/dashboard") ||
					strings.HasPrefix(path, "/ovpn/audit") ||
					strings.HasPrefix(path, "/ovpn/client") ||
					strings.HasPrefix(path, "/ovpn/user") ||
					strings.HasPrefix(path, "/ovpn/history") ||
					strings.HasPrefix(path, "/ovpn/online-client") ||
					strings.HasPrefix(path, "/ovpn/certs") {
					return true
				}
				if path == "/ovpn/settings" && c.Request.Method == "GET" {
					return true
				}
				return false
			}

			if !isPublicPath(c.Request.URL.Path) && user != adminUsername {
				c.Redirect(302, "/")
				c.Abort()
				return
			}
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
	db, err = gorm.Open(sqlite.Open(filepath.Join(ovData, "ovpn.db")+"?_pragma=foreign_keys(1)"), &gorm.Config{
		Logger: logger,
	})

	if err != nil {
		panic(err)
	}

	c := cron.New()
	c.AddFunc("@daily", func() {
		err := History{}.Clear()
		if err != nil {
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
	db.AutoMigrate(&User{}, &History{}, &Firewall{}, &NotifyLog{}, &AuditLog{}, &NotificationChannel{}, &UserNotifyRead{}, &ClientPackage{})

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
			if dp, err := aes.AesDecrypt(adminPassword, secretKey); err == nil {
				if subtle.ConstantTimeCompare([]byte(dp), []byte(u.Password)) == 1 {
					passwd, _ := bcrypt.GenerateFromPassword([]byte("admin"), 12)
					viper.Set("system.base.admin_password", string(passwd))
					viper.WriteConfig()

					c.JSON(401, gin.H{"message": "检测到旧的密码加密格式，已重置为默认密码，请使用默认密码 admin 登录后修改"})
					return
				}
			}

			if bcrypt.CompareHashAndPassword([]byte(adminPassword), []byte(u.Password)) == nil {
				session.Set("user", u.Username)
				session.Save()

				resetLoginFail(cip)
				adminUser := User{Username: u.Username}.Info()
				adminID := adminUser.ID
				if adminID == 0 {
					// admin 用户在 user 表中无记录，使用 0 作为占位 ID（前端按 username 查询资料）
					adminID = 0
				}
				c.JSON(200, gin.H{
					"message":  "登录成功",
					"redirect": "/admin",
					"user": gin.H{
						"id":           adminID,
						"username":     adminUser.Username,
						"name":         adminUser.Name,
						"email":        adminUser.Email,
						"isFirstLogin": false,
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
						session.Set("user", u.Username)
						session.Save()
						resetLoginFail(cip)
						c.JSON(200, gin.H{
							"message":  "登录成功",
							"redirect": "/",
							"user": gin.H{
								"id":           userInfo.ID,
								"username":     userInfo.Username,
								"name":         userInfo.Name,
								"email":        userInfo.Email,
								"isFirstLogin": *userInfo.IsFirstLogin,
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
							cc.Delete("valid_user")
							session.Set("user", u.Username)
							session.Save()
							resetLoginFail(cip)
							c.JSON(200, gin.H{
								"message":  "登录成功",
								"redirect": "/",
								"user": gin.H{
									"id":           userInfo.ID,
									"username":     userInfo.Username,
									"name":         userInfo.Name,
									"email":        userInfo.Email,
									"isFirstLogin": *userInfo.IsFirstLogin,
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

				session.Set("user", u.Username)
				session.Save()

				resetLoginFail(cip)

				c.JSON(200, gin.H{
					"message":  "登录成功",
					"redirect": "/",
					"user": gin.H{
						"id":           user.ID,
						"username":     user.Username,
						"name":         user.Name,
						"email":        user.Email,
						"isFirstLogin": *user.IsFirstLogin,
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
		ovpn.GET("/dashboard/summary", ov.dashboardSummary)
		ovpn.GET("/audit/logs", auditLogsHandler)
		ovpn.GET("/audit/export", auditExportHandler)

		ovpn.GET("/settings", func(c *gin.Context) {
			var conf config
			viper.Unmarshal(&conf)

			c.JSON(http.StatusOK, conf)
		})

		ovpn.POST("/settings", func(c *gin.Context) {
			c.Request.ParseForm()
			for k, vs := range c.Request.PostForm {
				val := vs[0]

				switch k {
				case "system.base.admin_password":
					ep, _ := bcrypt.GenerateFromPassword([]byte(val), 12)
					val = string(ep)
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
				case "openvpn.ovpn_subnet", "openvpn.ovpn_subnet6":
					_, _, err := net.ParseCIDR(val)
					if err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
						return
					}
				case "openvpn.ovpn_push_dns1", "openvpn.ovpn_push_dns2":
					if net.ParseIP(val) == nil {
						c.JSON(http.StatusInternalServerError, gin.H{"message": "invalid IP address: " + val})
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
			if err := viper.WriteConfig(); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
		})

		// 客户端安装包管理
		ovpn.GET("/client-packages", func(c *gin.Context) {
			pkg := &ClientPackage{}
			packages := pkg.All()
			type PackageWithURL struct {
				ClientPackage
				DownloadURL string `json:"downloadUrl"`
			}
			result := make([]PackageWithURL, 0, len(packages))
			for _, p := range packages {
				pw := PackageWithURL{ClientPackage: p}
				pw.DownloadURL = p.PublicDownloadURL()
				result = append(result, pw)
			}
			c.JSON(http.StatusOK, result)
		})

		ovpn.POST("/client-packages", func(c *gin.Context) {
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

		ovpn.DELETE("/client-packages/:id", func(c *gin.Context) {
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

		ovpn.POST("/client-packages/:id/enable", func(c *gin.Context) {
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

		ovpn.GET("/client-packages/:id/download", func(c *gin.Context) {
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

			if !result.IsActive {
				c.JSON(http.StatusNotFound, gin.H{"message": "该安装包未启用"})
				return
			}

			filePath := result.FullPath()
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				c.JSON(http.StatusNotFound, gin.H{"message": "文件不存在"})
				return
			}

			c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", result.Filename))
			c.Header("Content-Type", "application/octet-stream")
			c.File(filePath)
		})

		ovpn.POST("/server", func(c *gin.Context) {
			a := c.PostForm("action")

			switch a {
			case "settings":
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
				day := c.PostForm("day")
				serverName := viper.GetString("system.base.server_name")

				var out []byte
				var err error

				if _, statErr := os.Stat("docker-entrypoint.sh"); statErr == nil {
					cmd := exec.Command("docker-entrypoint.sh", "renewcert", day)
					cmd.Dir = ovData
					out, err = cmd.CombinedOutput()
				} else {
					easyrsaPath := "easyrsa"
					if _, statErr2 := os.Stat("/usr/share/easy-rsa/easyrsa"); statErr2 == nil {
						easyrsaPath = "/usr/share/easy-rsa/easyrsa"
					}
					if _, statErr3 := os.Stat("easyrsa"); statErr3 != nil {
						if _, lookErr := exec.LookPath("easyrsa"); lookErr == nil {
							easyrsaPath = "easyrsa"
						}
					}

					if serverName != "" {
						cmds := [][]string{
							{easyrsaPath, "--batch", fmt.Sprintf("--days=%s", day), "renew-ca"},
							{easyrsaPath, "--batch", fmt.Sprintf("--days=%s", day), "renew", serverName},
							{easyrsaPath, "--batch", "revoke-renewed", serverName},
							{easyrsaPath, "--batch", fmt.Sprintf("--days=%s", day), "gen-crl"},
						}
						for _, args := range cmds {
							cmd := exec.Command(args[0], args[1:]...)
							cmd.Dir = ovData
							cmd.Env = append(os.Environ(), "EASYRSA="+ovData)
							var stepOut []byte
							stepOut, err = cmd.CombinedOutput()
							out = append(out, stepOut...)
							if err != nil {
								break
							}
						}
					} else {
						err = fmt.Errorf("server_name 未配置")
					}
				}

				if err != nil {
					msg := strings.TrimSpace(string(out))
					if msg == "" {
						msg = err.Error()
					} else {
						msg = msg + ": " + err.Error()
					}
					logger.Error(context.Background(), "更新证书失败: %s", msg)
					c.JSON(http.StatusInternalServerError, gin.H{"message": fmt.Sprintf("更新证书失败: %s", msg)})
					return
				}

				ov.sendCommand("signal SIGHUP")
				c.JSON(http.StatusOK, gin.H{"message": "更新证书成功"})
			case "restartSrv":
				_, err := ov.sendCommand("signal SIGHUP")
				if err != nil {
					logger.Error(context.Background(), err.Error())
					c.JSON(http.StatusInternalServerError, gin.H{"message": "重启服务失败"})
					return
				}

				c.JSON(http.StatusOK, gin.H{"message": "重启服务成功"})
			case "getConfig":
				data, err := os.ReadFile(filepath.Join(ovData, "server.conf"))
				if err != nil {
					logger.Error(context.Background(), err.Error())
					c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
					return
				}

				c.JSON(http.StatusOK, gin.H{"content": string(data)})
			case "updateConfig":
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

		ovpn.POST("/kill", func(c *gin.Context) {
			cid := c.PostForm("cid")
			ov.killClient(cid)
			c.JSON(http.StatusOK, gin.H{"code": http.StatusOK})
		})

		ovpn.GET("/firewall", FirewallHandler)
		ovpn.POST("/firewall", FirewallHandler)
		ovpn.PATCH("/firewall", FirewallHandler)
		ovpn.DELETE("/firewall/:id", FirewallHandler)

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

		ovpn.GET("/online-client", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"server": ov.getServer(), "clients": ov.getClient()})
		})

		ovpn.GET("/group", func(c *gin.Context) {
			var g Group
			c.JSON(http.StatusOK, g.All())
		})

		ovpn.GET("/group/:id", func(c *gin.Context) {
			var g Group
			c.JSON(http.StatusOK, g.Get(c.Param("id")))
		})

		ovpn.GET("/group/:id/users", func(c *gin.Context) {
			var auth bool
			var g Group

			gid := c.Param("id")

			cmd := exec.Command("egrep", "^auth-user-pass-verify", filepath.Join(ovData, "server.conf"))
			if err := cmd.Run(); err != nil {
				auth = false
			} else {
				auth = true
			}

			c.JSON(http.StatusOK, gin.H{"users": g.GetUsers(gid), "authUser": auth})
		})

		ovpn.POST("/group", func(c *gin.Context) {
			var g Group
			c.ShouldBind(&g)

			err := g.Create()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": "添加成功"})
		})

		ovpn.PATCH("/group", func(c *gin.Context) {
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

		ovpn.DELETE("/group/:id", func(c *gin.Context) {
			var g Group
			c.ShouldBind(&g)

			err := g.Delete(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusConflict, gin.H{"message": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
		})

		ovpn.GET("/user", func(c *gin.Context) {
			var u User

			username := c.Query("username")
			if username != "" {
				u.Username = username
			}

			c.JSON(http.StatusOK, u.Info())
		})

		ovpn.GET("/user/:id", func(c *gin.Context) {
			var u User
			c.JSON(http.StatusOK, u.Get(c.Param("id")))
		})

		r.GET("/user/template", func(c *gin.Context) {
			c.Header("Content-Type", "text/csv")
			c.Header("Content-Disposition", "attachment; filename=user_template.csv")

			c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})

			writer := csv.NewWriter(c.Writer)
			defer writer.Flush()

			writer.Write([]string{"username", "password", "name", "email", "is_enable", "expire_date", "ip_addr", "ovpn_config"})
			writer.Write([]string{"zhangsan", "123456", "寮犱笁", "zhangsan@example.com", "1", "2025-12-01/00:00:00", "10.8.0.222", "tt-gz.ovpn"})
			writer.Write([]string{"lisi", "123456", "鏉庡洓", "lisi@example.com", "0", "", "", "tt-sh.ovpn"})
		})

		ovpn.GET("/user/export", func(c *gin.Context) {
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

		ovpn.POST("/user", func(c *gin.Context) {
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
					u := User{
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

					err = u.Create()
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
							labels := map[string]string{
								"windows": "Windows",
								"macos":   "macOS",
								"linux":   "Linux",
								"android": "Android",
								"ios":     "iOS",
							}
							localPackages = append(localPackages, LocalPackageInfo{
								Platform:      platform,
								PlatformLabel: labels[platform],
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
								SiteUrl:       viper.GetString("system.base.site_url"),
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

		ovpn.PATCH("/user", func(c *gin.Context) {
			c.Request.ParseForm()
			formData := make(map[string]string)
			for k, v := range c.Request.PostForm {
				if len(v) > 0 {
					formData[k] = v[0]
				}
			}
			os.WriteFile("data/debug_patch_user.log", []byte(fmt.Sprintf("form data: %v\nu.Password: %q\nsendNotifyEmail: %q", formData, c.PostForm("password"), c.PostForm("sendNotifyEmail"))), 0644)

			var u User
			c.ShouldBind(&u)

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
			logger.Error(context.Background(), "PATCH user debug before update - sendNotifyEmail=%s, rawPwdLen=%d", sendNotifyEmail, len(rawPassword))

			err := u.Update()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			} else {
				if sendNotifyEmail == "true" && rawPassword != "" {
					os.WriteFile("data/debug_patch_user.log", []byte(fmt.Sprintf("form data: %v\nu.Password: %q\nsendNotifyEmail: %q\n进入goroutine前，u.ID=%d, rawPwdLen=%d", formData, c.PostForm("password"), c.PostForm("sendNotifyEmail"), u.ID, len(rawPassword))), 0644)
					go func(userID uint, password string) {
						os.WriteFile("data/debug_goroutine.log", []byte(fmt.Sprintf("goroutine开始执行，userID=%d, pwdLen=%d\n", userID, len(password))), 0644)
						var cu User
						result := db.First(&cu, userID)
						os.WriteFile("data/debug_goroutine.log", []byte(fmt.Sprintf("goroutine开始执行，userID=%d, pwdLen=%d\n查询用户结果: error=%v, username=%s, email=%s\n", userID, len(password), result.Error, cu.Username, cu.Email)), 0644)

						if cu.Email != "" {
							var tpl *template.Template
							var buf bytes.Buffer
							var tplErr error

							tpl, tplErr = template.New("account-email").Parse(accountEmailTemplate)
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
									SiteUrl:       viper.GetString("system.base.site_url"),
									LocalPackages: nil,
								})
							}

							if tplErr != nil {
								os.WriteFile("data/debug_goroutine.log", []byte(fmt.Sprintf("goroutine开始执行，userID=%d, pwdLen=%d\n查询用户结果: error=%v, username=%s, email=%s\n模板渲染失败: %v\n", userID, len(password), result.Error, cu.Username, cu.Email, tplErr)), 0644)
								logger.Error(context.Background(), "渲染邮件模板失败: %s", tplErr.Error())
								return
							}

							os.WriteFile("data/debug_goroutine.log", []byte(fmt.Sprintf("goroutine开始执行，userID=%d, pwdLen=%d\n查询用户结果: error=%v, username=%s, email=%s\n模板渲染成功，准备调用sendUserEmail\n", userID, len(password), result.Error, cu.Username, cu.Email)), 0644)
							if sendErr := sendUserEmail(cu.Email, "用户密码配置通知", buf.String(), nil, cu.Username, "password_reset"); sendErr != nil {
								os.WriteFile("data/debug_goroutine.log", []byte(fmt.Sprintf("goroutine开始执行，userID=%d, pwdLen=%d\n查询用户结果: error=%v, username=%s, email=%s\n模板渲染成功，准备调用sendUserEmail\nsendUserEmail失败: %v\n", userID, len(password), result.Error, cu.Username, cu.Email, sendErr)), 0644)
								logger.Error(context.Background(), "发送用户邮件失败: %s", sendErr.Error())
							} else {
								os.WriteFile("data/debug_goroutine.log", []byte(fmt.Sprintf("goroutine开始执行，userID=%d, pwdLen=%d\n查询用户结果: error=%v, username=%s, email=%s\n模板渲染成功，准备调用sendUserEmail\nsendUserEmail成功\n", userID, len(password), result.Error, cu.Username, cu.Email)), 0644)
							}
						} else {
							os.WriteFile("data/debug_goroutine.log", []byte(fmt.Sprintf("goroutine开始执行，userID=%d, pwdLen=%d\n查询用户结果: error=%v, username=%s, email=%s\n用户没有邮箱，跳过发送\n", userID, len(password), result.Error, cu.Username, cu.Email)), 0644)
							logger.Error(context.Background(), "发送邮件通知失败，用户没有配置邮箱地址")
						}
					}(u.ID, rawPassword)
				}

				c.JSON(http.StatusOK, gin.H{"message": "用户更新成功"})
			}
		})

		ovpn.DELETE("/user/:id", func(c *gin.Context) {
			var u User
			id := c.Param("id")

			err := u.Delete(id)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			} else {
				c.JSON(http.StatusOK, gin.H{"message": "删除用户成功"})
			}
		})

		ovpn.GET("/client", func(c *gin.Context) {
			clients := make([]ClientConfigData, 0)

			files, _ := os.ReadDir(filepath.Join(ovData, "clients"))
			for _, file := range files {
				finfo, _ := file.Info()

				f := ClientConfigData{
					Name:     strings.TrimSuffix(file.Name(), filepath.Ext(file.Name())),
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

		ovpn.GET("/client/:name/ccd", func(c *gin.Context) {
			name := c.Param("name")
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

		ovpn.GET("/client/:name/config", func(c *gin.Context) {
			name := c.Param("name")
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

		ovpn.PUT("/client/:name/ccd", func(c *gin.Context) {
			name := c.Param("name")
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

		ovpn.PUT("/client/:name/config", func(c *gin.Context) {
			name := c.Param("name")
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

		ovpn.POST("/client", func(c *gin.Context) {
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

		ovpn.DELETE("/client/:name", func(c *gin.Context) {
			name := c.Param("name")

			cmd := exec.Command("easyrsa", "--batch", "revoke", name)
			out, err := cmd.CombinedOutput()
			if err == nil {
				cmd = exec.Command("easyrsa", "gen-crl")
				if out, err = cmd.CombinedOutput(); err != nil {
					logger.Error(context.Background(), string(out))
					c.JSON(http.StatusInternalServerError, gin.H{"message": "更新 CRL 证书失败"})
				}
			} else {
				if len(out) == 0 {
					out = []byte(err.Error())
				}
				logger.Error(context.Background(), string(out))
				c.JSON(http.StatusInternalServerError, gin.H{"message": "删除客户端失败"})
				return
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

		ovpn.GET("/history", func(c *gin.Context) {
			var h History
			var p Params

			c.ShouldBindQuery(&p)

			c.JSON(http.StatusOK, h.Query(p))
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
			LogNotifyError(event, NotifyClientEvent(event))

			c.JSON(http.StatusOK, gin.H{"message": "添加记录成功"})
		})

		ovpn.GET("/notify/logs", func(c *gin.Context) {
			limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
			c.JSON(http.StatusOK, gin.H{"data": queryNotifyLogs(limit)})
		})

		// 站内信：未读数 + 标记已读
		ovpn.GET("/notify/unread-count", func(c *gin.Context) {
			username, _ := c.Get("user")
			uname, _ := username.(string)
			rec := getUserNotifyRead(uname)
			total := countUnreadNotifyLogs(rec.LastReadID)
			c.JSON(http.StatusOK, gin.H{
				"unread":     total,
				"lastReadId": rec.LastReadID,
				"maxId":      maxNotifyLogID(),
			})
		})

		ovpn.POST("/notify/mark-read", func(c *gin.Context) {
			username, _ := c.Get("user")
			uname, _ := username.(string)
			if uname == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
				return
			}
			// 可选：客户端传入 lastReadId；未传则推进到当前最大 id
			var body struct {
				LastReadID uint `json:"lastReadId"`
			}
			_ = c.ShouldBind(&body)
			target := body.LastReadID
			if target == 0 {
				target = maxNotifyLogID()
			}
			rec, err := markUserNotifyRead(uname, target)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"message":    "marked",
				"lastReadId": rec.LastReadID,
				"unread":     countUnreadNotifyLogs(rec.LastReadID),
			})
		})

		// WebSocket：站内信实时推送
		ovpn.GET("/ws/notifications", func(c *gin.Context) {
			username, _ := c.Get("user")
			if uname, ok := username.(string); !ok || uname == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
				return
			}
			WsHubInstance().ServeWs(c.Writer, c.Request)
		})

		// 通知渠道维护：CRUD + Test
		ovpn.GET("/channel-types", channelTypesHandler)
		ovpn.GET("/channel", channelHandler)
		ovpn.GET("/channel/:id", channelHandler)
		ovpn.POST("/channel", channelHandler)
		ovpn.PUT("/channel/:id", channelHandler)
		ovpn.PATCH("/channel/:id", channelHandler)
		ovpn.DELETE("/channel/:id", channelHandler)
		ovpn.POST("/channel/:id/test", channelTestHandler)

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

			if err := NotifyClientEvent(event); err != nil {
				logger.Error(context.Background(), err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": "notify sent"})
		})

		ovpn.GET("/history/export", func(c *gin.Context) {
			var p Params
			c.ShouldBindQuery(&p)

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

		ovpn.GET("/certs", func(c *gin.Context) {
			c.JSON(http.StatusOK, getCerts(ovData))
		})
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

				c.JSON(http.StatusOK, lu)
				return
			}

			if u.Username == adminUsername {
				u.Name = viper.GetString("system.base.admin_name")
				u.Email = viper.GetString("system.base.admin_email")
				c.JSON(http.StatusOK, u)
				return
			}

			c.JSON(http.StatusOK, u.Info())
		})

		client.PUT("/userinfo", func(c *gin.Context) {
			session := sessions.Default(c)
			currentUsername := ""
			if user, ok := session.Get("user").(string); ok {
				currentUsername = user
			}

			if currentUsername != adminUsername {
				c.JSON(http.StatusForbidden, gin.H{"message": "仅系统管理员可修改个人资料"})
				return
			}

			name := c.PostForm("name")
			email := c.PostForm("email")

			viper.Set("system.base.admin_name", name)
			viper.Set("system.base.admin_email", email)
			err := viper.WriteConfig()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}

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

			// 校验请求中的 id 与当前登录用户一致
			if currentUsername == adminUsername {
				// admin 用户：忽略 u.ID 校验，密码存放在配置中
			} else {
				cu := User{Username: currentUsername}.Info()
				if u.ID != cu.ID {
					c.JSON(http.StatusInternalServerError, gin.H{"message": "非法请求"})
					return
				}
			}

			if !isValidPassword(u.Password) {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "密码不满足要求（长度12位，包含大小写字母、数字、特殊字符）"})
				return
			}

			if currentPass, ok := c.Request.PostForm["currentPass"]; ok {
				if currentUsername == adminUsername {
					if bcrypt.CompareHashAndPassword([]byte(adminPassword), []byte(currentPass[0])) != nil {
						c.JSON(http.StatusUnauthorized, gin.H{"message": "当前密码错误"})
						return
					}

					passwd, err := bcrypt.GenerateFromPassword([]byte(u.Password), 12)
					if err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
						return
					}

					viper.Set("system.base.admin_password", string(passwd))
					viper.WriteConfig()
					adminPassword = string(passwd)

					c.JSON(http.StatusOK, gin.H{"message": "密码修改成功"})
					return
				}

				if u.Info().Password != currentPass[0] {
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

				// 异步重新生成客户端配置文件（含 static-challenge）并发送邮件通知
				go func(userID uint) {
					cu := User{ID: userID}.Info()
					if cu.Email == "" {
						return
					}

					// 重新生成客户端配置文件（先删除旧的，再生成含 MFA 的新配置）
					if cu.OvpnConfig != "" {
						clientFilePath := filepath.Join(ovData, "clients", cu.OvpnConfig)
						os.Remove(clientFilePath)
						if err := generateClientConfig(strings.TrimSuffix(cu.OvpnConfig, ".ovpn"), true); err != nil {
							logger.Error(context.Background(), "重新生成客户端配置失败: %s", err.Error())
						}
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
						SiteUrl:  viper.GetString("system.base.site_url"),
					})
					if err != nil {
						logger.Error(context.Background(), "渲染邮件模板失败: %s", err.Error())
						return
					}

					var attachments []string
					if cu.OvpnConfig != "" {
						clientFilePath := filepath.Join(ovData, "clients", cu.OvpnConfig)
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
					c.JSON(http.StatusInternalServerError, gin.H{"message": "非法请求"})
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
					var tpl *template.Template
					var buf bytes.Buffer

					tpl, err := template.New("account-email").Parse(accountEmailTemplate)
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
							SiteUrl:       viper.GetString("system.base.site_url"),
							LocalPackages: nil,
						})
					}

					if err != nil {
						logger.Error(context.Background(), "渲染邮件模板失败: %s", err.Error())
						return
					}

					if err := sendUserEmail(targetUser.Email, "用户 MFA 重置通知", buf.String(), nil, targetUser.Username, "mfa_reset"); err != nil {
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
