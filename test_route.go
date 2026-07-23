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
		return plugin.AdminNotice{}, fmt.Errorf(T(rt.Language(ctx), "unknown plugin action: %s"), action)
	}
	lang := rt.Language(ctx)
	cfg, err := rt.Config(ctx, pluginName)
	if err != nil {
		return plugin.AdminNotice{}, fmt.Errorf(T(lang, "unable to read plugin config: %w"), err)
	}
	sc, err := parseSMTPConfig(cfg)
	if err != nil {
		return plugin.AdminNotice{}, fmt.Errorf(T(lang, "invalid SMTP configuration: %w"), err)
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
	subject := T(lang, "[GopherInk] Email send test")
	body, err := buildHTMLBody(notifyContext{
		Type:            "test",
		Lang:            lang,
		ToEmail:         to,
		ToName:          T(lang, "Admin"),
		PostTitle:       T(lang, "Email appearance preview"),
		PostURL:         siteURL,
		Author:          "GopherInk",
		AuthorAvatarURL: pluginAvatarURL(ctx, rt, to, 72),
		Content:         T(lang, "If you received this email, the SMTP configuration and current email template are working correctly."),
		Time:            time.Now().Format("2006-01-02 15:04:05"),
		SiteTitle:       siteTitle,
		SiteURL:         siteURL,
	}, cfg["email_template"])
	if err != nil {
		return plugin.AdminNotice{}, fmt.Errorf(T(lang, "unable to render test email: %w"), err)
	}
	if err := sendMail(sc, to, subject, body); err != nil {
		return plugin.AdminNotice{}, fmt.Errorf(T(lang, "test email send failed: %w"), err)
	}
	return plugin.AdminNotice{
		Type:    plugin.NoticeSuccess,
		Mode:    plugin.NoticeSnackbar,
		Message: fmt.Sprintf(T(lang, "Test email has been sent to %s."), to),
	}, nil
}
