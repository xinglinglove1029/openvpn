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
	ID         uint      `gorm:"primarykey" json:"id"`
	OperatorID uint      `gorm:"index;default:0" json:"operatorId"`
	Operator   string    `json:"operator"`
	Module     string    `json:"module"`
	Action     string    `json:"action"`
	Target     string    `json:"target"`
	Success    bool      `json:"success"`
	Message    string    `json:"message"`
	IP         string    `json:"ip"`
	IPRegion   string    `gorm:"-" json:"ipRegion"` // 非数据库字段，运行时计算
	CreatedAt  time.Time `json:"createdAt"`
}

// AdminAuditOperatorID / SystemAuditOperatorID 为历史版本使用的保留 operator_id
// admin/system 现已纳入 user 表，新写入的审计日志使用真实 user.id；
// 这两个常量仅用于 RepairAuditLogOperatorIDs 迁移历史保留 ID 记录
const AdminAuditOperatorID uint = 0xFFFFFFFF
const SystemAuditOperatorID uint = 0xFFFFFFFE

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
	{http.MethodDelete, regexp.MustCompile(`^/ovpn/certs$`), "cert", "delete"},
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
	if hasInternalFirewallHookIdentity(c) {
		return internalFirewallHookAuditActor
	}
	if user, ok := sessions.Default(c).Get("user").(string); ok && user != "" {
		return user
	}
	// 兜底使用 admin（system 账号已移除，只保留 admin）
	if adminUsername != "" {
		return adminUsername
	}
	return "admin"
}

func recordAudit(c *gin.Context, module, action, target string, success bool, message string) {
	if db == nil {
		return
	}

	operator := auditOperator(c)
	operatorID := uint(0)
	if !hasInternalFirewallHookIdentity(c) {
		operatorID = GetUserIDByUsername(operator)
	}

	// admin/system 已纳入 user 表，统一使用真实 user.id；
	// 若解析失败（理论上不会发生，除非 user 表未初始化），记录告警
	if !hasInternalFirewallHookIdentity(c) && operator != "" && operatorID == 0 {
		logger.Error(context.Background(), "[recordAudit] 无法解析操作人ID operator=%q module=%s action=%s target=%s", operator, module, action, target)
	}

	logItem := AuditLog{
		OperatorID: operatorID,
		Operator:   operator,
		Module:     module,
		Action:     action,
		Target:     target,
		Success:    success,
		Message:    message,
		IP:         c.ClientIP(),
	}
	if err := db.WithContext(context.Background()).Create(&logItem).Error; err != nil {
		logger.Error(context.Background(), "record audit log failed: %s", err)
	}
}

// RepairAuditLogOperatorIDs 修复历史 audit_logs 中 operator_id=0 或使用保留 ID 的记录
// admin/system 现已纳入 user 表，统一迁移到真实 user.id
func RepairAuditLogOperatorIDs() {
	if db == nil {
		return
	}

	// 收集需要修复的 operator 列表（operator_id=0 或为历史保留 ID）
	var operators []string
	if err := db.WithContext(context.Background()).
		Model(&AuditLog{}).
		Distinct("operator").
		Where("operator != ? AND (operator_id = ? OR operator_id = ? OR operator_id = ?)", "", 0, AdminAuditOperatorID, SystemAuditOperatorID).
		Pluck("operator", &operators).Error; err != nil {
		logger.Error(context.Background(), "查询待修复 audit_log operator 失败: %s", err.Error())
		return
	}

	var totalFixed int64
	for _, op := range operators {
		userID := GetUserIDByUsername(op)
		if userID == 0 {
			continue
		}

		// 修复该 operator 的 operator_id=0 或保留 ID 的记录
		result := db.WithContext(context.Background()).
			Model(&AuditLog{}).
			Where("operator = ? AND (operator_id = ? OR operator_id = ? OR operator_id = ?)", op, 0, AdminAuditOperatorID, SystemAuditOperatorID).
			Update("operator_id", userID)
		if result.Error != nil {
			logger.Error(context.Background(), "修复 operator=%s 的 audit_log 失败: %s", op, result.Error.Error())
			continue
		}
		totalFixed += result.RowsAffected
	}

	if totalFixed > 0 {
		logger.Error(context.Background(), "已修复 %d 条 audit_log 的 operator_id（保留 ID → 真实 user.id）", totalFixed)
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
		accessibleUserIDs, skipFilter := GetAccessibleUserIDs(currentUsername)
		if !skipFilter {
			query = query.Where("operator_id IN ?", accessibleUserIDs)
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

	// 解析 IP 归属地
	for i := range logs {
		logs[i].IPRegion = GetIPRegion(logs[i].IP)
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
	c.Writer.WriteString("ID,Operator,Module,Action,Target,Success,Message,IP,IPRegion,CreatedAt\n")
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
			csvCell(item.IPRegion),
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
