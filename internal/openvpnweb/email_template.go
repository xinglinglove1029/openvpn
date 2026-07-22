package openvpnweb

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
                <div style="font-size:24px;font-weight:700;margin-top:8px;">账号{{ if eq .Type "resetPass" }}密码重置{{ else }}开通{{ end }}通知</div>
              </td>
            </tr>
            <tr>
              <td style="padding:30px;line-height:1.8;font-size:14px;">
                <p style="margin:0 0 14px;">您好 <strong style="color:#2563eb;">{{.Name}}</strong>：</p>
                <p style="margin:0 0 18px;">您的 OpenVPN {{ if eq .Type "resetPass" }}账号密码已重置{{ else }}服务已开通{{ end }}，请妥善保管以下信息。</p>
                <table width="100%" cellpadding="0" cellspacing="0" style="border:1px solid #e5e7eb;border-radius:12px;overflow:hidden;background:#f8fafc;">
                  <tr>
                    <td width="120" style="padding:13px 16px;font-weight:700;border-bottom:1px solid #e5e7eb;color:#475569;">账号</td>
                    <td style="padding:13px 16px;border-bottom:1px solid #e5e7eb;">{{.Username}}</td>
                  </tr>
                  <tr>
                    <td style="padding:13px 16px;font-weight:700;color:#475569;">{{ if eq .Type "resetPass" }}新密码{{ else }}初始密码{{ end }}</td>
                    <td style="padding:13px 16px;font-family:Consolas,monospace;">{{.Password}}</td>
                  </tr>
                </table>
                {{ if ne .Type "resetPass" }}
                <p style="margin:16px 0 0;color:#dc2626;font-size:13px;">为保障账号安全，首次登录后请尽快修改密码。</p>
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
