package commentnotifier

import (
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/Chocola-X/GopherInk/core/plugin"
)

// handleTestEmail handles GET /plugins/comment-notifier/test
// It sends a test email using the current saved SMTP configuration
// and displays the result as an HTML page in the browser.
func handleTestEmail(rt *plugin.Runtime, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfg, err := rt.Config(r.Context(), pluginName)
	if err != nil {
		renderTestResult(w, false, "无法读取插件配置", "", "")
		return
	}

	if cfg["smtp_host"] == "" || cfg["smtp_username"] == "" || cfg["from_mail"] == "" {
		renderTestResult(w, false, "请先保存 SMTP 配置后再测试", "", "")
		return
	}

	sc, err := parseSMTPConfig(cfg)
	if err != nil {
		renderTestResult(w, false, err.Error(), "", "")
		return
	}

	to := strings.TrimSpace(cfg["admin_email"])
	if to == "" {
		to = sc.FromEmail
	}

	siteTitle, _ := rt.Option(r.Context(), "title")
	siteURL, _ := rt.Option(r.Context(), "base_url")
	siteURL = strings.TrimRight(siteURL, "/")

	subject := "[GopherInk] 邮件发送测试"
	body := `<!DOCTYPE html>
<html><head><meta charset="utf-8"></head>
<body style="margin:0;padding:0;background:#f6f6f6;font-family:-apple-system,PingFang SC,Microsoft YaHei,sans-serif;">
<div style="max-width:560px;margin:24px auto;background:#fff;border-radius:12px;padding:40px;box-shadow:0 1px 4px rgba(0,0,0,0.08);">
<h2 style="font-size:20px;margin:0 0 20px;color:#333;">邮件发送测试</h2>
<p style="color:#666;margin:0 0 8px;">这是一封来自 <strong>` + html.EscapeString(siteTitle) + `</strong> 的测试邮件。</p>
<p style="color:#666;margin:0 0 8px;">如果你收到了这封邮件，说明 SMTP 配置正确。</p>
<p style="color:#999;margin:24px 0 0;font-size:13px;">此邮件由 <a href="` + siteURL + `" style="color:#999;">` + html.EscapeString(siteTitle) + `</a> 自动发送</p>
</div></body></html>`

	if err := sendMail(sc, to, subject, body); err != nil {
		renderTestResult(w, false, "发送失败："+err.Error(), sc.FromEmail, to)
		return
	}

	renderTestResult(w, true, "", sc.FromEmail, to)
}

// renderTestResult renders an HTML page showing the test result.
func renderTestResult(w http.ResponseWriter, ok bool, errMsg, from, to string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	var statusClass, statusText, detail string
	if ok {
		statusClass = "color:#27ae60"
		statusText = "发送成功"
		detail = fmt.Sprintf(
			`<p style="color:#666;margin:8px 0;">发件邮箱：%s</p>
			<p style="color:#666;margin:8px 0;">收件邮箱：%s</p>
			<p style="color:#666;margin:8px 0;">发送时间：%s</p>`,
			html.EscapeString(from),
			html.EscapeString(to),
			time.Now().Format("2006-01-02 15:04:05"),
		)
	} else {
		statusClass = "color:#e74c3c"
		statusText = "发送失败"
		detail = fmt.Sprintf(
			`<p style="color:#e74c3c;margin:8px 0;">%s</p>`,
			html.EscapeString(errMsg),
		)
	}

	fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>邮件发送测试</title></head>
<body style="margin:0;padding:0;background:#f6f6f6;font-family:-apple-system,PingFang SC,Microsoft YaHei,sans-serif;">
<div style="max-width:560px;margin:40px auto;background:#fff;border-radius:12px;padding:40px;box-shadow:0 1px 4px rgba(0,0,0,0.08);">
<h2 style="font-size:20px;margin:0 0 20px;color:#333;">邮件发送测试</h2>
<p style="font-size:18px;font-weight:bold;margin:0 0 16px;%s;">%s</p>
%s
<p style="color:#999;margin:24px 0 0;font-size:13px;">可关闭此页面</p>
</div></body></html>`, statusClass, statusText, detail)
}
