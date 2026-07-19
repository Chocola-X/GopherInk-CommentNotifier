package commentnotifier

import (
	"crypto/tls"
	_ "embed"
	"encoding/base64"
	"fmt"
	"html"
	"log"
	"net"
	netmail "net/mail"
	"net/smtp"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// smtpConfig holds SMTP connection parameters parsed from plugin config.
type smtpConfig struct {
	Host      string
	Port      int
	Security  string // "ssl", "tls", "none"
	Username  string
	Password  string
	FromEmail string
	FromName  string
}

// notifyContext holds data for rendering a notification email.
type notifyContext struct {
	Type            string // "owner", "guest", "pending", "test"
	ToEmail         string
	ToName          string
	PostTitle       string
	PostURL         string
	Author          string
	AuthorAvatarURL string
	Content         string
	Time            string
	ParentAuthor    string
	ParentAvatarURL string
	ParentContent   string
	SiteTitle       string
	SiteURL         string
}

type mailTask struct {
	Config  smtpConfig
	To      string
	Subject string
	Body    string
}

const (
	mailWorkerCount   = 2
	mailQueueCapacity = 64
	smtpTimeout       = 30 * time.Second
)

var mailQueue = make(chan mailTask, mailQueueCapacity)

func init() {
	for i := 0; i < mailWorkerCount; i++ {
		go func() {
			for task := range mailQueue {
				safeSendMail(task.Config, task.To, task.Subject, task.Body)
			}
		}()
	}
}

// parseSMTPConfig builds an smtpConfig from the plugin config map.
func parseSMTPConfig(cfg map[string]string) (smtpConfig, error) {
	port := 465
	if value := strings.TrimSpace(cfg["smtp_port"]); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 65535 {
			return smtpConfig{}, fmt.Errorf("comment-notifier: SMTP port must be between 1 and 65535")
		}
		port = parsed
	}
	host := strings.TrimSpace(cfg["smtp_host"])
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}
	sc := smtpConfig{
		Host:      host,
		Port:      port,
		Security:  cfg["smtp_security"],
		Username:  strings.TrimSpace(cfg["smtp_username"]),
		Password:  cfg["smtp_password"],
		FromEmail: strings.TrimSpace(cfg["from_mail"]),
		FromName:  strings.TrimSpace(cfg["from_name"]),
	}
	if sc.Host == "" || sc.Username == "" || sc.Password == "" || sc.FromEmail == "" {
		return smtpConfig{}, fmt.Errorf("comment-notifier: SMTP configuration incomplete")
	}
	if strings.ContainsAny(sc.Host, "\r\n\t /?#") || (strings.Contains(sc.Host, ":") && net.ParseIP(sc.Host) == nil) {
		return smtpConfig{}, fmt.Errorf("comment-notifier: invalid SMTP host")
	}
	if sc.Security == "" {
		sc.Security = "ssl"
	}
	switch sc.Security {
	case "ssl", "tls", "none":
	default:
		return smtpConfig{}, fmt.Errorf("comment-notifier: unsupported SMTP security mode %q", sc.Security)
	}
	from, err := netmail.ParseAddress(sc.FromEmail)
	if err != nil {
		return smtpConfig{}, fmt.Errorf("comment-notifier: invalid sender email: %w", err)
	}
	sc.FromEmail = from.Address
	if sc.FromName == "" {
		sc.FromName = "GopherInk"
	}
	return sc, nil
}

// buildSubject returns the email subject for the given notification type.
func buildSubject(nc notifyContext) string {
	switch nc.Type {
	case "owner":
		return fmt.Sprintf("你的《%s》文章有了新的评论", nc.PostTitle)
	case "guest":
		return fmt.Sprintf("你在[%s]的评论有了新的回复", nc.PostTitle)
	case "pending":
		return fmt.Sprintf("文章《%s》有条待审评论", nc.PostTitle)
	default:
		return "GopherInk 评论通知"
	}
}

type emailTemplatePlaceholder struct {
	Token       string
	Description string
}

var emailTemplatePlaceholders = []emailTemplatePlaceholder{
	{Token: "{notification_type}", Description: "通知类型：owner、guest、pending 或 test"},
	{Token: "{headline}", Description: "通知标题"},
	{Token: "{intro}", Description: "根据通知类型生成的摘要"},
	{Token: "{site_title}", Description: "站点名称"},
	{Token: "{site_url}", Description: "站点地址"},
	{Token: "{post_title}", Description: "文章或页面标题"},
	{Token: "{post_url}", Description: "评论所在位置的链接"},
	{Token: "{recipient_name}", Description: "收件人名称"},
	{Token: "{comment_author}", Description: "评论者名称"},
	{Token: "{comment_avatar_url}", Description: "评论者邮箱对应的头像地址"},
	{Token: "{comment_label}", Description: "评论、回复或测试内容标签"},
	{Token: "{comment_content}", Description: "评论正文"},
	{Token: "{comment_time}", Description: "评论时间"},
	{Token: "{parent_author}", Description: "被回复者名称"},
	{Token: "{parent_avatar_url}", Description: "被回复者邮箱对应的头像地址"},
	{Token: "{parent_content}", Description: "被回复的评论正文"},
	{Token: "{parent_comment_block}", Description: "回复通知中的原评论区块，其他通知为空"},
	{Token: "{action_url}", Description: "查看评论或前往审核的地址"},
	{Token: "{action_label}", Description: "操作按钮文本"},
}

//go:embed email_style.html
var defaultEmailTemplate string

var emailPlaceholderPattern = regexp.MustCompile(`\{[a-z][a-z0-9_]*\}`)

func configuredEmailTemplate(value string) string {
	if strings.TrimSpace(value) == "" {
		return defaultEmailTemplate
	}
	return value
}

func emailTemplateValues(nc notifyContext) (map[string]string, error) {
	escape := html.EscapeString
	escapeLines := func(value string) string {
		return strings.ReplaceAll(escape(value), "\n", "<br>")
	}
	headline := ""
	intro := ""
	actionLabel := "查看评论"
	actionURL := nc.PostURL
	commentLabel := "评论"
	switch nc.Type {
	case "owner":
		headline = "你的文章收到了新评论"
		intro = fmt.Sprintf("%s 在《%s》发表了新评论。", escape(nc.Author), escape(nc.PostTitle))
	case "guest":
		headline = "你的评论收到了新回复"
		intro = fmt.Sprintf("%s 回复了你在《%s》下的评论。", escape(nc.Author), escape(nc.PostTitle))
		actionLabel = "查看回复"
		commentLabel = "回复"
	case "pending":
		headline = "有待审核的评论"
		intro = fmt.Sprintf("《%s》收到一条等待审核的评论。", escape(nc.PostTitle))
		actionLabel = "前往审核"
		actionURL = strings.TrimRight(nc.SiteURL, "/") + "/admin/comments"
	case "test":
		headline = "邮件发送测试"
		intro = "这是一封用于验证 SMTP 配置和邮件外观的测试邮件。"
		actionLabel = "访问站点"
		actionURL = nc.SiteURL
		commentLabel = "测试内容"
	default:
		return nil, fmt.Errorf("comment-notifier: unknown notification type %q", nc.Type)
	}
	parentBlock := ""
	if nc.Type == "guest" {
		parentBlock = `<p style="margin:0 0 8px;color:#777;">你此前的评论：</p><div style="margin:0 0 18px;padding:14px 16px;border-left:3px solid #70c9d4;border-radius:4px;color:#777;background:#f5f7f8;word-break:break-word;">` + escapeLines(nc.ParentContent) + `</div>`
	}
	return map[string]string{
		"{notification_type}":    escape(nc.Type),
		"{headline}":             headline,
		"{intro}":                intro,
		"{site_title}":           escape(nc.SiteTitle),
		"{site_url}":             escape(nc.SiteURL),
		"{post_title}":           escape(nc.PostTitle),
		"{post_url}":             escape(nc.PostURL),
		"{recipient_name}":       escape(nc.ToName),
		"{comment_author}":       escape(nc.Author),
		"{comment_avatar_url}":   escape(nc.AuthorAvatarURL),
		"{comment_label}":        commentLabel,
		"{comment_content}":      escapeLines(nc.Content),
		"{comment_time}":         escape(nc.Time),
		"{parent_author}":        escape(nc.ParentAuthor),
		"{parent_avatar_url}":    escape(nc.ParentAvatarURL),
		"{parent_content}":       escapeLines(nc.ParentContent),
		"{parent_comment_block}": parentBlock,
		"{action_url}":           escape(actionURL),
		"{action_label}":         actionLabel,
	}, nil
}

func buildHTMLBody(nc notifyContext, source string) (string, error) {
	values, err := emailTemplateValues(nc)
	if err != nil {
		return "", err
	}
	return emailPlaceholderPattern.ReplaceAllStringFunc(configuredEmailTemplate(source), func(token string) string {
		if value, ok := values[token]; ok {
			return value
		}
		return token
	}), nil
}

func queueNotification(sc smtpConfig, nc notifyContext, templateSource string) error {
	body, err := buildHTMLBody(nc, templateSource)
	if err != nil {
		return err
	}
	task := mailTask{Config: sc, To: nc.ToEmail, Subject: buildSubject(nc), Body: body}
	select {
	case mailQueue <- task:
		return nil
	default:
		return fmt.Errorf("comment-notifier: mail queue is full")
	}
}

// sendMail sends an HTML email via SMTP.
func sendMail(sc smtpConfig, to, subject, htmlBody string) error {
	recipient, err := netmail.ParseAddress(strings.TrimSpace(to))
	if err != nil {
		return fmt.Errorf("invalid recipient email: %w", err)
	}
	to = recipient.Address
	addr := net.JoinHostPort(sc.Host, strconv.Itoa(sc.Port))
	from := sc.FromEmail

	// Build the email message.
	var msg strings.Builder
	msg.WriteString("From: " + formatAddress(sc.FromName, from) + "\r\n")
	msg.WriteString("To: " + formatAddress("", to) + "\r\n")
	msg.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	msg.WriteString("Subject: " + mimeEncodeSubject(subject) + "\r\n")
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	msg.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(htmlBody)

	// Connect and send based on security mode.
	switch sc.Security {
	case "ssl":
		return sendMailTLS(addr, sc.Host, sc.Username, sc.Password, from, to, []byte(msg.String()))
	case "tls":
		return sendMailSTARTTLS(addr, sc.Host, sc.Username, sc.Password, from, to, []byte(msg.String()))
	default:
		return sendMailPlain(addr, sc.Host, sc.Username, sc.Password, from, to, []byte(msg.String()))
	}
}

// safeSendMail sends an email, logging errors instead of propagating them.
func safeSendMail(sc smtpConfig, to, subject, htmlBody string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[comment-notifier] panic sending mail to %s: %v", to, r)
		}
	}()
	if err := sendMail(sc, to, subject, htmlBody); err != nil {
		log.Printf("[comment-notifier] failed to send mail to %s: %v", to, err)
	}
}

// sendMailTLS connects via TLS (port 465) and sends the email.
func sendMailTLS(addr, host, username, password, from, to string, msg []byte) error {
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: smtpTimeout}, "tcp", addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	if err != nil {
		return fmt.Errorf("tls dial %s: %w", addr, err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(smtpTimeout)); err != nil {
		return fmt.Errorf("set SMTP deadline: %w", err)
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp new client: %w", err)
	}
	defer client.Close()

	return smtpSend(client, host, username, password, from, to, msg)
}

// sendMailSTARTTLS connects plain, upgrades to TLS, and sends the email.
func sendMailSTARTTLS(addr, host, username, password, from, to string, msg []byte) error {
	conn, err := net.DialTimeout("tcp", addr, smtpTimeout)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(smtpTimeout)); err != nil {
		return fmt.Errorf("set SMTP deadline: %w", err)
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp new client: %w", err)
	}
	defer client.Close()

	if err := client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
		return fmt.Errorf("starttls: %w", err)
	}

	return smtpSend(client, host, username, password, from, to, msg)
}

// sendMailPlain connects without encryption and sends the email.
func sendMailPlain(addr, host, username, password, from, to string, msg []byte) error {
	conn, err := net.DialTimeout("tcp", addr, smtpTimeout)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(smtpTimeout)); err != nil {
		return fmt.Errorf("set SMTP deadline: %w", err)
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp new client: %w", err)
	}
	defer client.Close()

	return smtpSend(client, host, username, password, from, to, msg)
}

// smtpSend authenticates and sends the message via an smtp.Client.
func smtpSend(client *smtp.Client, host, username, password, from, to string, msg []byte) error {
	if username != "" && password != "" {
		auth := smtp.PlainAuth("", username, password, host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close data: %w", err)
	}
	return client.Quit()
}

// formatAddress formats an email address with an optional display name.
func formatAddress(name, email string) string {
	if name == "" {
		return email
	}
	return fmt.Sprintf("%s <%s>", mimeEncodeText(name), email)
}

// mimeEncodeSubject encodes a subject line as UTF-8 encoded-words.
func mimeEncodeSubject(s string) string {
	return "=?UTF-8?B?" + encodeBase64(s) + "?="
}

// mimeEncodeText encodes text as UTF-8 encoded-words for display names.
func mimeEncodeText(s string) string {
	return "=?UTF-8?B?" + encodeBase64(s) + "?="
}

// encodeBase64 returns a base64 encoding of the string.
func encodeBase64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}
