package commentnotifier

import (
	"context"
	"strings"

	"github.com/Chocola-X/GopherInk/core/plugin"
)

const pluginName = "comment-notifier"

type commentNotifier struct{}

func init() { plugin.Register(commentNotifier{}) }

func (commentNotifier) Name() string    { return pluginName }
func (commentNotifier) Version() string { return "0.2.0" }
func (commentNotifier) Description() string {
	return "在评论发布或审核通过后发送邮件提醒。"
}

func (commentNotifier) Info() plugin.PluginInfo {
	return plugin.PluginInfo{
		Name:             pluginName,
		Version:          "0.2.0",
		Description:      "在评论发布或审核通过后发送邮件提醒。",
		RequireGopherInk: "0.5.0",
	}
}

func (commentNotifier) Init(m *plugin.Manager) {
	m.RegisterRuntimeHook(plugin.HookCommentAfterSave, afterCommentSave)
	m.RegisterRuntimeHook(plugin.HookCommentAfterMark, afterCommentMark)
	m.RegisterAdminMenu(plugin.AdminMenuItem{
		Label: "评论邮件提醒",
		URL:   "/admin/plugins/" + pluginName + "/config",
		Icon:  "mail",
	})
}

func (commentNotifier) ConfigSchema() []plugin.FieldSchema {
	return []plugin.FieldSchema{
		{Name: "enabled", Label: "启用邮件提醒", Group: "基本设置", Type: plugin.FieldCheckbox, Default: "1"},
		{Name: "notify_owner", Label: "通知内容作者", Group: "基本设置", Type: plugin.FieldCheckbox, Default: "1"},
		{Name: "notify_parent", Label: "通知被回复者", Group: "基本设置", Type: plugin.FieldCheckbox, Default: "1"},
		{Name: "notify_pending", Label: "待审核评论通知管理员", Group: "基本设置", Type: plugin.FieldCheckbox, Default: "1"},
		{Name: "admin_email", Label: "管理员收件邮箱", Group: "基本设置", Type: plugin.FieldText, Required: true, Wide: true,
			Description: "用于接收待审核评论通知的邮箱地址。"},
		{Name: "smtp_host", Label: "SMTP 服务器", Group: "SMTP", Type: plugin.FieldText, Required: true, Wide: true, Default: "smtp.qq.com"},
		{Name: "smtp_port", Label: "SMTP 端口", Group: "SMTP", Type: plugin.FieldNumber, Default: "465", Required: true, Min: "1", Max: "65535", Step: "1"},
		{Name: "smtp_security", Label: "SMTP 加密模式", Group: "SMTP", Type: plugin.FieldSelect, Default: "ssl",
			Options: []plugin.FieldOption{
				{Label: "无安全加密", Value: "none"},
				{Label: "SSL加密", Value: "ssl"},
				{Label: "TLS加密", Value: "tls"},
			},
		},
		{Name: "smtp_username", Label: "SMTP 用户名", Group: "SMTP", Type: plugin.FieldText, Required: true, Wide: true},
		{Name: "smtp_password", Label: "SMTP 密码或授权码", Group: "SMTP", Type: plugin.FieldPassword, Required: true, Wide: true,
			Description: "通常应填写邮箱服务商生成的 SMTP 授权码，而不是网页登录密码。"},
		{Name: "from_mail", Label: "发件邮箱", Group: "发件人", Type: plugin.FieldText, Required: true, Wide: true},
		{Name: "from_name", Label: "发件人名称", Group: "发件人", Type: plugin.FieldText, Default: "GopherInk", Wide: true},
	}
}

func (commentNotifier) AdminActions() []plugin.AdminAction {
	return []plugin.AdminAction{{
		Name:        "test-email",
		Label:       "测试邮件设置",
		Icon:        "send",
		Variant:     "outlined",
		Description: "使用已经保存的 SMTP 参数发送一封测试邮件",
	}}
}

func (commentNotifier) AdminNotices(_ context.Context, _ *plugin.Runtime, values map[string]string) []plugin.AdminNotice {
	required := []string{"admin_email", "smtp_host", "smtp_username", "smtp_password", "from_mail"}
	for _, name := range required {
		if strings.TrimSpace(values[name]) == "" {
			return []plugin.AdminNotice{{
				Type:    plugin.NoticeWarning,
				Mode:    plugin.NoticeCard,
				Message: "SMTP 配置尚未填写完整，评论邮件提醒当前不会发送邮件。",
			}}
		}
	}
	return nil
}
