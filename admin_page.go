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

var appearancePageTemplate = htmltemplate.Must(htmltemplate.New("appearance").Funcs(htmltemplate.FuncMap{
	"T": T,
}).Parse(`
<form method="post" class="form-panel plugin-template-panel" data-plugin-template-editor>
  <input type="hidden" name="_csrf" value="{{.CSRF}}">
  <input type="hidden" name="action" value="save-template" data-template-action>
  <fieldset class="fieldset plugin-template-help">
    <legend>{{T .Lang "Available Placeholders"}}</legend>
    <div class="plugin-template-placeholders">
      {{range .Placeholders}}<div><code>{{.Token}}</code><span>{{T $.Lang .Description}}</span></div>{{end}}
    </div>
    <p class="muted plugin-template-note">{{T .Lang "Comment content and other external data are automatically HTML-escaped. Inline CSS and JavaScript in the template are written as-is into the email; actual support depends on the recipient's email client."}}</p>
  </fieldset>
  <div class="plugin-template-toolbar">
    <mdui-segmented-button-group selects="single" value="edit" data-template-mode aria-label="{{T .Lang "Email template view mode"}}">
      <mdui-segmented-button value="edit" selected><mdui-icon slot="icon" name="code"></mdui-icon>{{T .Lang "Edit"}}</mdui-segmented-button>
      <mdui-segmented-button value="preview"><mdui-icon slot="icon" name="preview"></mdui-icon>{{T .Lang "Preview"}}</mdui-segmented-button>
    </mdui-segmented-button-group>
  </div>
  <div class="plugin-template-editor-pane" data-template-editor-pane>
    <label for="comment-notifier-template">{{T .Lang "HTML email template"}}</label>
    <textarea id="comment-notifier-template" name="email_template" class="plugin-template-code" spellcheck="false" maxlength="49152" data-template-source>{{.Template}}</textarea>
  </div>
  <div class="plugin-template-preview-pane" data-template-preview-pane hidden>
    <iframe title="{{T .Lang "Email template preview"}}" sandbox="allow-scripts" referrerpolicy="no-referrer" data-template-preview></iframe>
  </div>
  <textarea hidden data-template-preview-values>{{.PreviewValues}}</textarea>
  <div class="form-actions plugin-template-actions">
    <mdui-button type="submit"><mdui-icon slot="icon" name="save"></mdui-icon>{{T .Lang "Save settings"}}</mdui-button>
    <mdui-button type="button" variant="outlined" data-template-reset><mdui-icon slot="icon" name="restart_alt"></mdui-icon>{{T .Lang "Restore default"}}</mdui-button>
  </div>
</form>`))

type appearancePageData struct {
	CSRF          string
	Template      string
	PreviewValues string
	Placeholders  []emailTemplatePlaceholder
	Lang          string
}

func (commentNotifier) AdminPages() []plugin.AdminPage {
	return []plugin.AdminPage{{
		Name:        appearancePageName,
		Label:       "Customize email notification appearance",
		Icon:        "palette",
		Title:       "Comment email appearance",
		Description: "Adjust the HTML template for comment notification emails and preview the effect in real time.",
	}}
}

func (commentNotifier) RenderAdminPage(ctx context.Context, rt *plugin.Runtime, page string, renderContext plugin.AdminPageRenderContext) (htmltemplate.HTML, error) {
	if page != appearancePageName {
		return "", fmt.Errorf(T(rt.Language(ctx), "unknown plugin page: %s"), page)
	}
	lang := rt.Language(ctx)
	siteTitle, _ := rt.Option(ctx, "site_title")
	if strings.TrimSpace(siteTitle) == "" {
		siteTitle = "GopherInk"
	}
	siteURL, _ := rt.Option(ctx, "base_url")
	siteURL = strings.TrimRight(siteURL, "/")
	previewValues, err := emailTemplateValues(notifyContext{
		Type:            "guest",
		Lang:            lang,
		ToName:          T(lang, "Visitor"),
		PostTitle:       T(lang, "A post for previewing email styles"),
		PostURL:         siteURL + "/post/1.html#comment-2",
		Author:          T(lang, "New commenter"),
		AuthorAvatarURL: pluginAvatarURL(ctx, rt, "preview-author@example.com", 72),
		Content:         T(lang, "This is the reply content.\nPlaceholders will be rendered with mock data in real time."),
		Time:            time.Now().Format("2006-01-02 15:04:05"),
		ParentAuthor:    T(lang, "Visitor"),
		ParentAvatarURL: pluginAvatarURL(ctx, rt, "preview-parent@example.com", 72),
		ParentContent:   T(lang, "This is the original comment content."),
		SiteTitle:       siteTitle,
		SiteURL:         siteURL,
	})
	if err != nil {
		return "", err
	}
	previewJSON, err := json.Marshal(previewValues)
	if err != nil {
		return "", fmt.Errorf(T(lang, "generate email preview data: %w"), err)
	}
	var output bytes.Buffer
	if err := appearancePageTemplate.Execute(&output, appearancePageData{
		CSRF:          renderContext.CSRF,
		Template:      configuredEmailTemplate(renderContext.Config["email_template"]),
		PreviewValues: string(previewJSON),
		Placeholders:  emailTemplatePlaceholders,
		Lang:          lang,
	}); err != nil {
		return "", fmt.Errorf(T(lang, "render email appearance settings: %w"), err)
	}
	return htmltemplate.HTML(output.String()), nil
}

func (commentNotifier) HandleAdminPageAction(ctx context.Context, rt *plugin.Runtime, page string, form map[string][]string) (plugin.AdminPageActionResult, error) {
	if page != appearancePageName {
		return plugin.AdminPageActionResult{}, fmt.Errorf(T(rt.Language(ctx), "unknown plugin page: %s"), page)
	}
	lang := rt.Language(ctx)
	action := firstFormValue(form, "action")
	switch action {
	case "save-template":
		source := firstFormValue(form, "email_template")
		if strings.TrimSpace(source) == "" {
			return plugin.AdminPageActionResult{}, fmt.Errorf(T(lang, "Email template cannot be empty; to restore the built-in template, use \"Restore default\""))
		}
		if len([]byte(source)) > maxTemplateBytes {
			return plugin.AdminPageActionResult{}, fmt.Errorf(T(lang, "Email template cannot exceed 48 KiB"))
		}
		return plugin.AdminPageActionResult{
			ConfigPatch: map[string]string{"email_template": source},
			Notice:      plugin.AdminNotice{Type: plugin.NoticeSuccess, Mode: plugin.NoticeSnackbar, Message: T(lang, "Email appearance settings saved.")},
		}, nil
	case "reset-template":
		return plugin.AdminPageActionResult{
			ConfigPatch: map[string]string{"email_template": ""},
			Notice:      plugin.AdminNotice{Type: plugin.NoticeSuccess, Mode: plugin.NoticeSnackbar, Message: T(lang, "Email appearance has been restored to the default template.")},
		}, nil
	default:
		return plugin.AdminPageActionResult{}, fmt.Errorf(T(lang, "Unsupported email appearance action"))
	}
}

func firstFormValue(form map[string][]string, name string) string {
	values := form[name]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
