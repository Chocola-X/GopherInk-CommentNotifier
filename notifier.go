package commentnotifier

import "github.com/Chocola-X/GopherInk/core/plugin"

const pluginName = "comment-notifier"

type commentNotifier struct{}

func init() { plugin.Register(commentNotifier{}) }

func (commentNotifier) Name() string    { return pluginName }
func (commentNotifier) Version() string { return "0.1.0" }
func (commentNotifier) Description() string {
	return "在评论发布或审核通过后发送邮件提醒。"
}

func (commentNotifier) Info() plugin.PluginInfo {
	return plugin.PluginInfo{
		Name:             pluginName,
		Version:          "0.1.0",
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
		{Name: "smtp_port", Label: "SMTP 端口", Group: "SMTP", Type: plugin.FieldNumber, Default: "587", Required: true, Min: "1", Max: "65535", Step: "1"},
		{Name: "smtp_security", Label: "连接加密", Group: "SMTP", Type: plugin.FieldSelect, Default: "starttls",
			Options: []plugin.FieldOption{
				{Label: "STARTTLS", Value: "starttls"},
				{Label: "TLS 直连", Value: "tls"},
				{Label: "不加密", Value: "none"},
			},
		},
		{Name: "smtp_username", Label: "SMTP 用户名", Group: "SMTP", Type: plugin.FieldText, Required: true, Wide: true},
		{Name: "smtp_password", Label: "SMTP 密码或授权码", Group: "SMTP", Type: plugin.FieldPassword, Required: true, Wide: true},
		{Name: "from_mail", Label: "发件邮箱", Group: "发件人", Type: plugin.FieldText, Required: true, Wide: true},
		{Name: "from_name", Label: "发件人名称", Group: "发件人", Type: plugin.FieldText, Default: "GopherInk", Wide: true},
	}
}
