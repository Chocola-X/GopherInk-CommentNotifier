package commentnotifier

import (
	"encoding/json"
	"html"
	"net/http"
	"strings"

	"github.com/Chocola-X/GopherInk/core/plugin"
)

// handleTestEmail handles POST /plugins/comment-notifier/test
// It sends a test email using the current saved SMTP configuration.
func handleTestEmail(rt *plugin.Runtime, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfg, err := rt.Config(r.Context(), pluginName)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "无法读取插件配置"})
		return
	}

	if cfg["smtp_host"] == "" || cfg["smtp_username"] == "" || cfg["from_mail"] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "请先填写并保存 SMTP 配置"})
		return
	}

	sc, err := parseSMTPConfig(cfg)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
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
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "发送失败：" + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
