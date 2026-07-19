package commentnotifier

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"html/template"
	"log"
	"net"
	netmail "net/mail"
	"net/smtp"
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
	Type          string // "owner", "guest", "pending"
	ToEmail       string
	ToName        string
	PostTitle     string
	PostURL       string
	Author        string
	Content       string
	Time          string
	ParentAuthor  string
	ParentContent string
	SiteTitle     string
	SiteURL       string
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

// HTML email templates for each notification type.
var emailTemplates = map[string]*template.Template{}

func init() {
	parse := func(name, text string) {
		t := template.Must(template.New(name).Parse(text))
		emailTemplates[name] = t
	}

	parse("owner", `<!DOCTYPE html>
<html><head><meta charset="utf-8"></head>
<body style="margin:0;padding:0;background:#f6f6f6;font-family:-apple-system,PingFang SC,Microsoft YaHei,sans-serif;">
<div style="max-width:560px;margin:24px auto;background:#fff;border-radius:12px;padding:40px;box-shadow:0 1px 4px rgba(0,0,0,0.08);">
<h2 style="font-size:20px;margin:0 0 20px;color:#333;">你的文章收到了新评论</h2>
<p style="color:#666;margin:0 0 8px;">文章：<a href="{{.PostURL}}" style="color:#12ADDB;text-decoration:none;">{{.PostTitle}}</a></p>
<p style="color:#666;margin:0 0 8px;">评论者：<strong>{{.Author}}</strong></p>
<p style="color:#666;margin:0 0 8px;">时间：{{.Time}}</p>
<div style="background:#f5f5f5;border-radius:8px;padding:16px;margin:16px 0;color:#333;line-height:1.6;">{{.Content}}</div>
<p style="margin:16px 0 0;"><a href="{{.PostURL}}" style="display:inline-block;padding:10px 24px;background:#12ADDB;color:#fff;border-radius:6px;text-decoration:none;">查看评论</a></p>
<p style="color:#999;margin:24px 0 0;font-size:13px;">此邮件由 <a href="{{.SiteURL}}" style="color:#999;">{{.SiteTitle}}</a> 自动发送</p>
</div></body></html>`)

	parse("guest", `<!DOCTYPE html>
<html><head><meta charset="utf-8"></head>
<body style="margin:0;padding:0;background:#f6f6f6;font-family:-apple-system,PingFang SC,Microsoft YaHei,sans-serif;">
<div style="max-width:560px;margin:24px auto;background:#fff;border-radius:12px;padding:40px;box-shadow:0 1px 4px rgba(0,0,0,0.08);">
<h2 style="font-size:20px;margin:0 0 20px;color:#333;">你的评论收到了新回复</h2>
<p style="color:#666;margin:0 0 8px;">文章：<a href="{{.PostURL}}" style="color:#12ADDB;text-decoration:none;">{{.PostTitle}}</a></p>
<div style="background:#f0f0f0;border-radius:8px;padding:12px 16px;margin:12px 0;color:#888;line-height:1.6;">
<p style="margin:0 0 4px;font-size:13px;color:#999;">{{.ParentAuthor}} 的评论：</p>
{{.ParentContent}}
</div>
<p style="color:#666;margin:0 0 8px;"><strong>{{.Author}}</strong> 回复：</p>
<div style="background:#f5f5f5;border-radius:8px;padding:16px;margin:12px 0;color:#333;line-height:1.6;">{{.Content}}</div>
<p style="color:#666;margin:0 0 8px;">时间：{{.Time}}</p>
<p style="margin:16px 0 0;"><a href="{{.PostURL}}" style="display:inline-block;padding:10px 24px;background:#12ADDB;color:#fff;border-radius:6px;text-decoration:none;">查看回复</a></p>
<p style="color:#999;margin:24px 0 0;font-size:13px;">此邮件由 <a href="{{.SiteURL}}" style="color:#999;">{{.SiteTitle}}</a> 自动发送</p>
</div></body></html>`)

	parse("pending", `<!DOCTYPE html>
<html><head><meta charset="utf-8"></head>
<body style="margin:0;padding:0;background:#f6f6f6;font-family:-apple-system,PingFang SC,Microsoft YaHei,sans-serif;">
<div style="max-width:560px;margin:24px auto;background:#fff;border-radius:12px;padding:40px;box-shadow:0 1px 4px rgba(0,0,0,0.08);">
<h2 style="font-size:20px;margin:0 0 20px;color:#333;">有待审核的评论</h2>
<p style="color:#666;margin:0 0 8px;">文章：<a href="{{.PostURL}}" style="color:#12ADDB;text-decoration:none;">{{.PostTitle}}</a></p>
<p style="color:#666;margin:0 0 8px;">评论者：<strong>{{.Author}}</strong></p>
<p style="color:#666;margin:0 0 8px;">时间：{{.Time}}</p>
<div style="background:#f5f5f5;border-radius:8px;padding:16px;margin:16px 0;color:#333;line-height:1.6;">{{.Content}}</div>
<p style="margin:16px 0 0;"><a href="{{.SiteURL}}/admin/comments" style="display:inline-block;padding:10px 24px;background:#e67e22;color:#fff;border-radius:6px;text-decoration:none;">前往审核</a></p>
<p style="color:#999;margin:24px 0 0;font-size:13px;">此邮件由 <a href="{{.SiteURL}}" style="color:#999;">{{.SiteTitle}}</a> 自动发送</p>
</div></body></html>`)
}

// buildHTMLBody renders the email HTML body for the given notification context.
func buildHTMLBody(nc notifyContext) (string, error) {
	tmpl, ok := emailTemplates[nc.Type]
	if !ok {
		return "", fmt.Errorf("comment-notifier: unknown notification type %q", nc.Type)
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, nc); err != nil {
		return "", fmt.Errorf("comment-notifier: render template: %w", err)
	}
	return buf.String(), nil
}

func queueNotification(sc smtpConfig, nc notifyContext) error {
	body, err := buildHTMLBody(nc)
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
