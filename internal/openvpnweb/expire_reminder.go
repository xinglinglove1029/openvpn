package openvpnweb

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"time"

	"github.com/spf13/viper"
)

func checkAndSendExpireReminders() {
	if db == nil {
		return
	}

	now := time.Now()
	reminderDays := viper.GetInt("notify.expire_reminder_days")
	if reminderDays <= 0 {
		reminderDays = 7
	}

	var users []User
	if err := db.WithContext(context.Background()).
		Where("expire_date IS NOT NULL AND expire_date != '' AND is_enable = ?", true).
		Find(&users).Error; err != nil {
		logger.Error(context.Background(), "查询到期用户失败: %s", err.Error())
		return
	}

	for _, u := range users {
		if u.Email == "" {
			continue
		}

		expireTime, err := time.Parse("2006-01-02", u.ExpireDate)
		if err != nil {
			continue
		}

		daysLeft := int(expireTime.Sub(now).Hours() / 24)

		if daysLeft >= 0 && daysLeft <= reminderDays {
			go sendExpireReminderEmail(u, daysLeft)
		}
	}
}

func sendExpireReminderEmail(u User, daysLeft int) {
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
			ExpireDate    string
			DaysLeft      int
			LocalPackages []LocalPackageInfo
		}{
			Type:          "expire",
			Name:          u.Name,
			Username:      u.Username,
			Password:      "",
			SiteUrl:       viper.GetString("system.base.site_url"),
			ExpireDate:    u.ExpireDate,
			DaysLeft:      daysLeft,
			LocalPackages: nil,
		})
	}

	if err != nil {
		logger.Error(context.Background(), "渲染到期提醒邮件模板失败: %s", err.Error())
		return
	}

	subject := fmt.Sprintf("【到期提醒】您的 OpenVPN 账号将在 %d 天后到期", daysLeft)
	if err := sendUserEmail(u.Email, subject, buf.String(), nil, u.Username, "expire_reminder"); err != nil {
		logger.Error(context.Background(), "发送到期提醒邮件失败: %s", err.Error())
	}
}
