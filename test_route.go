package commentnotifier

import (
	"context"
	"fmt"
	"html"
	"strings"

	"github.com/Chocola-X/GopherInk/core/plugin"
)

func (commentNotifier) HandleAdminAction(ctx context.Context, rt *plugin.Runtime, action string) (plugin.AdminNotice, error) {
	if action != "test-email" {
		return plugin.AdminNotice{}, fmt.Errorf("未知的插件操作：%s", action)
	}
	cfg, err := rt.Config(ctx, pluginName)
	if err != nil {
		return plugin.AdminNotice{}, fmt.Errorf("无法读取插件配置：%w", err)
	}
	sc, err := parseSMTPConfig(cfg)
	if err != nil {
		return plugin.AdminNotice{}, fmt.Errorf("SMTP 配置无效：%w", err)
	}
	to := strings.TrimSpace(cfg["admin_email"])
	if to == "" {
		to = sc.FromEmail
	}
	siteTitle, _ := rt.Option(ctx, "site_title")
	if siteTitle == "" {
		siteTitle = "GopherInk"
	}
	siteURL, _ := rt.Option(ctx, "base_url")
	siteURL = strings.TrimRight(siteURL, "/")
	subject := "[GopherInk] 邮件发送测试"
	body := `<!DOCTYPE html>
<html><head><meta charset="utf-8"></head>
<body style="margin:0;padding:0;background:#f6f6f6;font-family:-apple-system,PingFang SC,Microsoft YaHei,sans-serif;">
<div style="max-width:560px;margin:24px auto;background:#fff;border-radius:12px;padding:40px;box-shadow:0 1px 4px rgba(0,0,0,0.08);">
<h2 style="font-size:20px;margin:0 0 20px;color:#333;">邮件发送测试</h2>
<p style="color:#666;margin:0 0 8px;">这是一封来自 <strong>` + html.EscapeString(siteTitle) + `</strong> 的测试邮件。</p>
<p style="color:#666;margin:0 0 8px;">如果你收到了这封邮件，说明 SMTP 配置正确。</p>
<p style="color:#999;margin:24px 0 0;font-size:13px;">此邮件由 <a href="` + html.EscapeString(siteURL) + `" style="color:#999;">` + html.EscapeString(siteTitle) + `</a> 自动发送</p>
</div></body></html>`
	if err := sendMail(sc, to, subject, body); err != nil {
		return plugin.AdminNotice{}, fmt.Errorf("测试邮件发送失败：%w", err)
	}
	return plugin.AdminNotice{
		Type:    plugin.NoticeSuccess,
		Mode:    plugin.NoticeSnackbar,
		Message: fmt.Sprintf("测试邮件已发送至 %s。", to),
	}, nil
}
