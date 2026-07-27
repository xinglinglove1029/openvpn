package openvpnweb

type LocalPackageInfo struct {
	Platform      string
	PlatformLabel string
	Version       string
	DownloadURL   string
}

const accountEmailTemplate = `<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>OpenVPN 账号通知</title>
  </head>
  <body style="margin:0;padding:0;background:#f5f7fb;font-family:Arial,'Microsoft YaHei',sans-serif;color:#1f2937;">
    <table width="100%" cellpadding="0" cellspacing="0" style="background:#f5f7fb;padding:32px 0;">
      <tr>
        <td align="center">
          <table width="620" cellpadding="0" cellspacing="0" style="background:#ffffff;border-radius:18px;overflow:hidden;border:1px solid #e5e7eb;box-shadow:0 18px 48px rgba(15,23,42,.08);">
            <tr>
              <td style="background:linear-gradient(135deg,#0f172a,#2563eb);color:#ffffff;padding:26px 30px;">
                <div style="font-size:13px;letter-spacing:.18em;text-transform:uppercase;opacity:.72;">OpenVPN Secure Console</div>
                <div style="font-size:24px;font-weight:700;margin-top:8px;">账号{{ if eq .Type "resetPass" }}密码重置{{ else if eq .Type "resetMfa" }}MFA 重置{{ else if eq .Type "mfaEnabled" }}MFA 已启用{{ else if eq .Type "expire" }}到期提醒{{ else }}开通{{ end }}通知</div>
              </td>
            </tr>
            <tr>
              <td style="padding:30px;line-height:1.8;font-size:14px;">
                <p style="margin:0 0 14px;">您好 <strong style="color:#2563eb;">{{.Name}}</strong>：</p>
                <p style="margin:0 0 18px;">您的 OpenVPN {{ if eq .Type "resetPass" }}账号密码已重置{{ else if eq .Type "resetMfa" }}账号 MFA 二次验证已重置{{ else if eq .Type "mfaEnabled" }}账号 MFA 二次验证已启用{{ else if eq .Type "expire" }}账号即将到期{{ else }}服务已开通{{ end }}，请妥善保管以下信息。</p>
                <table width="100%" cellpadding="0" cellspacing="0" style="border:1px solid #e5e7eb;border-radius:12px;overflow:hidden;background:#f8fafc;">
                  <tr>
                    <td width="120" style="padding:13px 16px;font-weight:700;border-bottom:1px solid #e5e7eb;color:#475569;">账号</td>
                    <td style="padding:13px 16px;border-bottom:1px solid #e5e7eb;">{{.Username}}</td>
                  </tr>
                  {{ if and (ne .Type "resetMfa") (ne .Type "mfaEnabled") (ne .Type "expire") }}
                  <tr>
                    <td style="padding:13px 16px;font-weight:700;color:#475569;">{{ if eq .Type "resetPass" }}新密码{{ else }}初始密码{{ end }}</td>
                    <td style="padding:13px 16px;font-family:Consolas,monospace;">{{.Password}}</td>
                  </tr>
                  {{ end }}
                  {{ if eq .Type "resetMfa" }}
                  <tr>
                    <td style="padding:13px 16px;font-weight:700;color:#475569;">说明</td>
                    <td style="padding:13px 16px;">MFA 二次验证已被管理员停用，您可以重新登录后在个人设置中再次开启。</td>
                  </tr>
                  {{ end }}
                  {{ if eq .Type "mfaEnabled" }}
                  <tr>
                    <td style="padding:13px 16px;font-weight:700;color:#475569;">说明</td>
                    <td style="padding:13px 16px;">MFA 二次验证已启用，后续登录管理后台和连接 OpenVPN 均需输入动态验证码。最新的客户端配置文件已随邮件附件发送，请使用新配置连接 VPN。</td>
                  </tr>
                  {{ end }}
                  {{ if eq .Type "expire" }}
                  <tr>
                    <td style="padding:13px 16px;font-weight:700;color:#475569;">到期时间</td>
                    <td style="padding:13px 16px;font-family:Consolas,monospace;color:#dc2626;font-weight:700;">{{.ExpireDate}}</td>
                  </tr>
                  <tr>
                    <td style="padding:13px 16px;font-weight:700;color:#475569;">剩余天数</td>
                    <td style="padding:13px 16px;color:#dc2626;font-weight:700;">还剩 {{.DaysLeft}} 天</td>
                  </tr>
                  {{ end }}
                </table>
                {{ if eq .Type "expire" }}
                <p style="margin:16px 0 0;color:#dc2626;font-size:13px;">为避免影响您的正常使用，请及时联系管理员续期。</p>
                {{ end }}
                {{ if and (ne .Type "resetPass") (ne .Type "resetMfa") (ne .Type "mfaEnabled") (ne .Type "expire") }}
                <p style="margin:16px 0 0;color:#dc2626;font-size:13px;">为保障账号安全，首次登录后请尽快修改密码。</p>
                {{ end }}
                {{ if .LocalPackages }}
                <div style="margin:20px 0;padding:16px;background:#f0f9ff;border:1px solid #bae6fd;border-radius:12px;">
                  <div style="font-weight:700;color:#0369a1;margin-bottom:10px;">📦 客户端安装包下载（国内加速）</div>
                  <p style="margin:0 0 12px;color:#475569;font-size:13px;">请根据您的设备选择对应的客户端安装包：</p>
                  {{ range .LocalPackages }}
                  <a href="{{.DownloadURL}}" style="display:inline-block;margin:4px 10px 4px 0;padding:8px 16px;border-radius:8px;background:#0284c7;color:#ffffff;text-decoration:none;font-weight:600;font-size:13px;">{{.PlatformLabel}} v{{.Version}}</a>
                  {{ end }}
                </div>
                {{ end }}
                <p style="margin:28px 0 0;text-align:center;">
                  <a href="{{.SiteUrl}}" target="_blank" style="display:inline-block;padding:12px 20px;border-radius:999px;background:#2563eb;color:#ffffff;text-decoration:none;font-weight:700;">进入 OpenVPN 自助门户</a>
                </p>
              </td>
            </tr>
            <tr>
              <td style="background:#f8fafc;color:#94a3b8;font-size:12px;padding:14px 30px;text-align:center;">本邮件由系统自动发送，请勿直接回复。</td>
            </tr>
          </table>
        </td>
      </tr>
    </table>
  </body>
</html>`
