package commentnotifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	htmltemplate "html/template"
	"strings"
	"time"

	"github.com/Chocola-X/GopherInk/core/plugin"
)

const (
	appearancePageName = "appearance"
	maxTemplateBytes   = 48 * 1024
)

var appearancePageTemplate = htmltemplate.Must(htmltemplate.New("appearance").Parse(`
<form method="post" class="form-panel plugin-template-panel" data-plugin-template-editor>
  <input type="hidden" name="_csrf" value="{{.CSRF}}">
  <input type="hidden" name="action" value="save-template" data-template-action>
  <fieldset class="fieldset plugin-template-help">
    <legend>可用占位符</legend>
    <div class="plugin-template-placeholders">
      {{range .Placeholders}}<div><code>{{.Token}}</code><span>{{.Description}}</span></div>{{end}}
    </div>
    <p class="muted plugin-template-note">评论内容等外部数据会自动进行 HTML 转义。模板中的内嵌 CSS 和 JavaScript 会原样写入邮件，实际支持情况由收件客户端决定。</p>
  </fieldset>
  <div class="plugin-template-toolbar">
    <mdui-segmented-button-group selects="single" value="edit" data-template-mode aria-label="邮件模板查看模式">
      <mdui-segmented-button value="edit" selected><mdui-icon slot="icon" name="code"></mdui-icon>编辑</mdui-segmented-button>
      <mdui-segmented-button value="preview"><mdui-icon slot="icon" name="preview"></mdui-icon>预览</mdui-segmented-button>
    </mdui-segmented-button-group>
  </div>
  <div class="plugin-template-editor-pane" data-template-editor-pane>
    <label for="comment-notifier-template">HTML 邮件模板</label>
    <textarea id="comment-notifier-template" name="email_template" class="plugin-template-code" spellcheck="false" maxlength="49152" data-template-source>{{.Template}}</textarea>
  </div>
  <div class="plugin-template-preview-pane" data-template-preview-pane hidden>
    <iframe title="邮件模板预览" sandbox="allow-scripts" referrerpolicy="no-referrer" data-template-preview></iframe>
  </div>
  <textarea hidden data-template-preview-values>{{.PreviewValues}}</textarea>
  <div class="form-actions plugin-template-actions">
    <mdui-button type="submit"><mdui-icon slot="icon" name="save"></mdui-icon>保存设置</mdui-button>
    <mdui-button type="button" variant="outlined" data-template-reset><mdui-icon slot="icon" name="restart_alt"></mdui-icon>恢复默认</mdui-button>
  </div>
</form>`))

type appearancePageData struct {
	CSRF          string
	Template      string
	PreviewValues string
	Placeholders  []emailTemplatePlaceholder
}

func (commentNotifier) AdminPages() []plugin.AdminPage {
	return []plugin.AdminPage{{
		Name:        appearancePageName,
		Label:       "自定义邮件提醒外观",
		Icon:        "palette",
		Title:       "评论邮件外观",
		Description: "调整评论通知邮件的 HTML 模板并即时预览效果。",
	}}
}

func (commentNotifier) RenderAdminPage(ctx context.Context, rt *plugin.Runtime, page string, renderContext plugin.AdminPageRenderContext) (htmltemplate.HTML, error) {
	if page != appearancePageName {
		return "", fmt.Errorf("未知的插件页面：%s", page)
	}
	siteTitle, _ := rt.Option(ctx, "site_title")
	if strings.TrimSpace(siteTitle) == "" {
		siteTitle = "GopherInk"
	}
	siteURL, _ := rt.Option(ctx, "base_url")
	siteURL = strings.TrimRight(siteURL, "/")
	previewValues, err := emailTemplateValues(notifyContext{
		Type:            "guest",
		ToName:          "访客",
		PostTitle:       "一篇用于预览邮件样式的文章",
		PostURL:         siteURL + "/post/1.html#comment-2",
		Author:          "新评论者",
		AuthorAvatarURL: pluginAvatarURL(ctx, rt, "preview-author@example.com", 72),
		Content:         "这是回复内容。\n占位符会使用模拟数据即时渲染。",
		Time:            time.Now().Format("2006-01-02 15:04:05"),
		ParentAuthor:    "访客",
		ParentAvatarURL: pluginAvatarURL(ctx, rt, "preview-parent@example.com", 72),
		ParentContent:   "这是原评论内容。",
		SiteTitle:       siteTitle,
		SiteURL:         siteURL,
	})
	if err != nil {
		return "", err
	}
	previewJSON, err := json.Marshal(previewValues)
	if err != nil {
		return "", fmt.Errorf("生成邮件预览数据：%w", err)
	}
	var output bytes.Buffer
	if err := appearancePageTemplate.Execute(&output, appearancePageData{
		CSRF:          renderContext.CSRF,
		Template:      configuredEmailTemplate(renderContext.Config["email_template"]),
		PreviewValues: string(previewJSON),
		Placeholders:  emailTemplatePlaceholders,
	}); err != nil {
		return "", fmt.Errorf("渲染邮件外观设置：%w", err)
	}
	return htmltemplate.HTML(output.String()), nil
}

func (commentNotifier) HandleAdminPageAction(_ context.Context, _ *plugin.Runtime, page string, form map[string][]string) (plugin.AdminPageActionResult, error) {
	if page != appearancePageName {
		return plugin.AdminPageActionResult{}, fmt.Errorf("未知的插件页面：%s", page)
	}
	action := firstFormValue(form, "action")
	switch action {
	case "save-template":
		source := firstFormValue(form, "email_template")
		if strings.TrimSpace(source) == "" {
			return plugin.AdminPageActionResult{}, fmt.Errorf("邮件模板不能为空；如需恢复内置模板，请使用“恢复默认”")
		}
		if len([]byte(source)) > maxTemplateBytes {
			return plugin.AdminPageActionResult{}, fmt.Errorf("邮件模板不能超过 48 KiB")
		}
		return plugin.AdminPageActionResult{
			ConfigPatch: map[string]string{"email_template": source},
			Notice:      plugin.AdminNotice{Type: plugin.NoticeSuccess, Mode: plugin.NoticeSnackbar, Message: "邮件外观设置已保存。"},
		}, nil
	case "reset-template":
		return plugin.AdminPageActionResult{
			ConfigPatch: map[string]string{"email_template": ""},
			Notice:      plugin.AdminNotice{Type: plugin.NoticeSuccess, Mode: plugin.NoticeSnackbar, Message: "邮件外观已恢复为默认模板。"},
		}, nil
	default:
		return plugin.AdminPageActionResult{}, fmt.Errorf("不支持的邮件外观操作")
	}
}

func firstFormValue(form map[string][]string, name string) string {
	values := form[name]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
