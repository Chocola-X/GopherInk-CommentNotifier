package commentnotifier

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	body, err := buildHTMLBody(notifyContext{
		Type:            "test",
		ToEmail:         to,
		ToName:          "管理员",
		PostTitle:       "邮件外观预览",
		PostURL:         siteURL,
		Author:          "GopherInk",
		AuthorAvatarURL: pluginAvatarURL(ctx, rt, to, 72),
		Content:         "如果你收到了这封邮件，说明 SMTP 配置和当前邮件模板可以正常工作。",
		Time:            time.Now().Format("2006-01-02 15:04:05"),
		SiteTitle:       siteTitle,
		SiteURL:         siteURL,
	}, cfg["email_template"])
	if err != nil {
		return plugin.AdminNotice{}, fmt.Errorf("无法渲染测试邮件：%w", err)
	}
	if err := sendMail(sc, to, subject, body); err != nil {
		return plugin.AdminNotice{}, fmt.Errorf("测试邮件发送失败：%w", err)
	}
	return plugin.AdminNotice{
		Type:    plugin.NoticeSuccess,
		Mode:    plugin.NoticeSnackbar,
		Message: fmt.Sprintf("测试邮件已发送至 %s。", to),
	}, nil
}
