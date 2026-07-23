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

type smtpConfig struct {
	Host      string
	Port      int
	Security  string
	Username  string
	Password  string
	FromEmail string
	FromName  string
}

type notifyContext struct {
	Type            string
	Lang            string
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

func buildSubject(nc notifyContext) string {
	switch nc.Type {
	case "owner":
		return fmt.Sprintf(T(nc.Lang, "Your post \"%s\" has a new comment"), nc.PostTitle)
	case "guest":
		return fmt.Sprintf(T(nc.Lang, "Your comment on [%s] has a new reply"), nc.PostTitle)
	case "pending":
		return fmt.Sprintf(T(nc.Lang, "Post \"%s\" has a pending comment"), nc.PostTitle)
	default:
		return T(nc.Lang, "GopherInk Comment Notification")
	}
}

type emailTemplatePlaceholder struct {
	Token       string
	Description string
}

var emailTemplatePlaceholders = []emailTemplatePlaceholder{
	{Token: "{notification_type}", Description: "Notification type: owner, guest, pending, or test"},
	{Token: "{headline}", Description: "Notification headline"},
	{Token: "{intro}", Description: "Summary generated based on notification type"},
	{Token: "{site_title}", Description: "Site name"},
	{Token: "{site_url}", Description: "Site URL"},
	{Token: "{post_title}", Description: "Post or page title"},
	{Token: "{post_url}", Description: "Link to the comment location"},
	{Token: "{recipient_name}", Description: "Recipient name"},
	{Token: "{comment_author}", Description: "Commenter name"},
	{Token: "{comment_avatar_url}", Description: "Avatar URL for commenter email"},
	{Token: "{comment_label}", Description: "Comment, reply, or test content label"},
	{Token: "{comment_content}", Description: "Comment body"},
	{Token: "{comment_time}", Description: "Comment time"},
	{Token: "{parent_author}", Description: "Parent commenter name"},
	{Token: "{parent_avatar_url}", Description: "Avatar URL for parent commenter email"},
	{Token: "{parent_content}", Description: "Parent comment body"},
	{Token: "{parent_comment_block}", Description: "Original comment block in reply notifications; empty for other types"},
	{Token: "{action_url}", Description: "URL to view the comment or go to review"},
	{Token: "{action_label}", Description: "Action button text"},
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
	actionLabel := T(nc.Lang, "View comment")
	actionURL := nc.PostURL
	commentLabel := T(nc.Lang, "Comment")
	switch nc.Type {
	case "owner":
		headline = T(nc.Lang, "Your post received a new comment")
		intro = fmt.Sprintf(T(nc.Lang, "%s commented on your post \"%s\"."), escape(nc.Author), escape(nc.PostTitle))
	case "guest":
		headline = T(nc.Lang, "Your comment received a new reply")
		intro = fmt.Sprintf(T(nc.Lang, "%s replied to your comment on \"%s\"."), escape(nc.Author), escape(nc.PostTitle))
		actionLabel = T(nc.Lang, "View reply")
		commentLabel = T(nc.Lang, "Reply")
	case "pending":
		headline = T(nc.Lang, "There is a comment pending review")
		intro = fmt.Sprintf(T(nc.Lang, "\"%s\" received a comment awaiting review."), escape(nc.PostTitle))
		actionLabel = T(nc.Lang, "Go to review")
		actionURL = strings.TrimRight(nc.SiteURL, "/") + "/admin/comments"
	case "test":
		headline = T(nc.Lang, "Email send test")
		intro = T(nc.Lang, "This is a test email to verify SMTP configuration and email appearance.")
		actionLabel = T(nc.Lang, "Visit site")
		actionURL = nc.SiteURL
		commentLabel = T(nc.Lang, "Test content")
	default:
		return nil, fmt.Errorf("comment-notifier: unknown notification type %q", nc.Type)
	}
	parentBlock := ""
	if nc.Type == "guest" {
		parentBlock = `<p style="margin:0 0 8px;color:#777;">` + html.EscapeString(T(nc.Lang, "Your previous comment:")) + `</p><div style="margin:0 0 18px;padding:14px 16px;border-left:3px solid #70c9d4;border-radius:4px;color:#777;background:#f5f7f8;word-break:break-word;">` + escapeLines(nc.ParentContent) + `</div>`
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

func sendMail(sc smtpConfig, to, subject, htmlBody string) error {
	recipient, err := netmail.ParseAddress(strings.TrimSpace(to))
	if err != nil {
		return fmt.Errorf("invalid recipient email: %w", err)
	}
	to = recipient.Address
	addr := net.JoinHostPort(sc.Host, strconv.Itoa(sc.Port))
	from := sc.FromEmail

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

	switch sc.Security {
	case "ssl":
		return sendMailTLS(addr, sc.Host, sc.Username, sc.Password, from, to, []byte(msg.String()))
	case "tls":
		return sendMailSTARTTLS(addr, sc.Host, sc.Username, sc.Password, from, to, []byte(msg.String()))
	default:
		return sendMailPlain(addr, sc.Host, sc.Username, sc.Password, from, to, []byte(msg.String()))
	}
}

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

func formatAddress(name, email string) string {
	if name == "" {
		return email
	}
	return fmt.Sprintf("%s <%s>", mimeEncodeText(name), email)
}

func mimeEncodeSubject(s string) string {
	return "=?UTF-8?B?" + encodeBase64(s) + "?="
}

func mimeEncodeText(s string) string {
	return "=?UTF-8?B?" + encodeBase64(s) + "?="
}

func encodeBase64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}
