package openvpnweb

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

type AuditLog struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	Operator  string    `json:"operator"`
	Module    string    `json:"module"`
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	Success   bool      `json:"success"`
	Message   string    `json:"message"`
	IP        string    `json:"ip"`
	CreatedAt time.Time `json:"createdAt"`
}

var auditTargets = []struct {
	Method string
	Re     *regexp.Regexp
	Module string
	Action string
}{
	{http.MethodPost, regexp.MustCompile(`^/login$`), "auth", "login"},
	{http.MethodPost, regexp.MustCompile(`^/settings$`), "settings", "update"},
	{http.MethodPost, regexp.MustCompile(`^/email/send$`), "email", "test"},
	{http.MethodPost, regexp.MustCompile(`^/ovpn/server$`), "server", "operate"},
	{http.MethodPost, regexp.MustCompile(`^/ovpn/kill$`), "online", "disconnect"},
	{http.MethodPost, regexp.MustCompile(`^/ovpn/group$`), "group", "create"},
	{http.MethodPatch, regexp.MustCompile(`^/ovpn/group$`), "group", "update"},
	{http.MethodDelete, regexp.MustCompile(`^/ovpn/group/[^/]+$`), "group", "delete"},
	{http.MethodPost, regexp.MustCompile(`^/ovpn/user$`), "user", "create"},
	{http.MethodPatch, regexp.MustCompile(`^/ovpn/user$`), "user", "update"},
	{http.MethodDelete, regexp.MustCompile(`^/ovpn/user/[^/]+$`), "user", "delete"},
	{http.MethodPost, regexp.MustCompile(`^/ovpn/client$`), "client", "create"},
	{http.MethodPut, regexp.MustCompile(`^/ovpn/client/[^/]+/(ccd|config)$`), "client", "update-config"},
	{http.MethodDelete, regexp.MustCompile(`^/ovpn/client/[^/]+$`), "client", "delete"},
	{http.MethodPost, regexp.MustCompile(`^/ovpn/firewall$`), "firewall", "create"},
	{http.MethodPatch, regexp.MustCompile(`^/ovpn/firewall$`), "firewall", "update"},
	{http.MethodDelete, regexp.MustCompile(`^/ovpn/firewall/[^/]+$`), "firewall", "delete"},
	{http.MethodPost, regexp.MustCompile(`^/ovpn/notify$`), "notify", "test"},
	// 注意：角色管理（POST/PATCH/DELETE /ovpn/role*、PUT /ovpn/role/:id/permissions）
	// 的审计日志已由 handler 内主动调用 recordAudit 记录，此处不再注册兜底规则，
	// 否则 AuditMiddleware 会再次记录一次，导致审计日志出现重复条目。
}

func AuditMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		module, action, ok := auditModuleAction(c.Request.Method, c.Request.URL.Path)
		if !ok {
			c.Next()
			return
		}

		c.Next()

		target := auditTarget(c)
		message := "ok"
		if len(c.Errors) > 0 {
			message = c.Errors.String()
		}
		if c.Writer.Status() >= http.StatusBadRequest {
			message = http.StatusText(c.Writer.Status())
		}
		recordAudit(c, module, action, target, c.Writer.Status() < http.StatusBadRequest, message)
	}
}

func auditModuleAction(method, path string) (string, string, bool) {
	for _, target := range auditTargets {
		if target.Method == method && target.Re.MatchString(path) {
			return target.Module, target.Action, true
		}
	}
	return "", "", false
}

func auditTarget(c *gin.Context) string {
	for _, key := range []string{"username", "name", "id", "action", "key", "email", "vip", "cid"} {
		if value := strings.TrimSpace(c.PostForm(key)); value != "" {
			return value
		}
	}
	if value := strings.TrimSpace(c.Param("id")); value != "" {
		return value
	}
	if value := strings.TrimSpace(c.Param("name")); value != "" {
		return value
	}
	return c.Request.URL.Path
}

func auditOperator(c *gin.Context) string {
	if user, ok := sessions.Default(c).Get("user").(string); ok && user != "" {
		return user
	}
	return "system"
}

func recordAudit(c *gin.Context, module, action, target string, success bool, message string) {
	if db == nil {
		return
	}

	logItem := AuditLog{
		Operator: auditOperator(c),
		Module:   module,
		Action:   action,
		Target:   target,
		Success:  success,
		Message:  message,
		IP:       c.ClientIP(),
	}
	if err := db.WithContext(context.Background()).Create(&logItem).Error; err != nil {
		logger.Error(context.Background(), "record audit log failed: %s", err)
	}
}

func queryAuditLogs(c *gin.Context) ([]AuditLog, int64, error) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	query := db.WithContext(context.Background()).Model(&AuditLog{})

	// 数据权限过滤：普通用户只能看到自己所在分组及下级分组用户的审计日志
	currentUsername := ""
	if user, ok := c.Get("user"); ok {
		if s, ok := user.(string); ok {
			currentUsername = s
		}
	}
	if isAdmin, _ := c.Get("isAdmin"); isAdmin != true && currentUsername != "" {
		accessibleUsers, skipFilter := GetAccessibleUsernames(currentUsername)
		if !skipFilter {
			query = query.Where("operator IN ?", accessibleUsers)
		}
	}

	if operator := strings.TrimSpace(c.Query("operator")); operator != "" {
		query = query.Where("operator LIKE ?", "%"+operator+"%")
	}
	if module := strings.TrimSpace(c.Query("module")); module != "" {
		query = query.Where("module = ?", module)
	}
	if action := strings.TrimSpace(c.Query("action")); action != "" {
		query = query.Where("action = ?", action)
	}
	if start := strings.TrimSpace(c.Query("start")); start != "" {
		if startTime, err := time.ParseInLocation("2006-01-02", start, time.Local); err == nil {
			query = query.Where("created_at >= ?", startTime)
		}
	}
	if end := strings.TrimSpace(c.Query("end")); end != "" {
		if endTime, err := time.ParseInLocation("2006-01-02", end, time.Local); err == nil {
			query = query.Where("created_at <= ?", endTime.Add(24*time.Hour-time.Nanosecond))
		}
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	logs := make([]AuditLog, 0)
	if err := query.Order("created_at DESC").Limit(limit).Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

func auditLogsHandler(c *gin.Context) {
	logs, total, err := queryAuditLogs(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": logs, "total": total})
}

func auditExportHandler(c *gin.Context) {
	logs, _, err := queryAuditLogs(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=audit_%s.csv", time.Now().Format("20060102150405")))
	c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	c.Writer.WriteString("ID,Operator,Module,Action,Target,Success,Message,IP,CreatedAt\n")
	for _, item := range logs {
		line := []string{
			strconv.Itoa(int(item.ID)),
			csvCell(item.Operator),
			csvCell(item.Module),
			csvCell(item.Action),
			csvCell(item.Target),
			strconv.FormatBool(item.Success),
			csvCell(item.Message),
			csvCell(item.IP),
			item.CreatedAt.Format("2006-01-02 15:04:05"),
		}
		c.Writer.WriteString(strings.Join(line, ",") + "\n")
	}
}

func csvCell(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

type auditUserOption struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
}

func auditUserOptionsHandler(c *gin.Context) {
	isAdmin, _ := c.Get("isAdmin")
	currentUsername, _ := c.Get("user")
	currentUserStr, _ := currentUsername.(string)

	var users []auditUserOption

	if isAdmin == true {
		if err := db.WithContext(context.Background()).Model(&User{}).Select("id", "username").Find(&users).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "查询用户列表失败"})
			return
		}
	} else {
		if currentUserStr == "" {
			c.JSON(http.StatusOK, []auditUserOption{})
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
}
