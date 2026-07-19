package commentnotifier

import (
	"context"
	"html"
	"log"
	"strings"
	"time"

	"github.com/Chocola-X/GopherInk/core/plugin"
)

// afterCommentSave handles the comment.after_save hook.
// It covers both new comments and backend replies (operation "comment" or "reply").
func afterCommentSave(ctx context.Context, rt *plugin.Runtime, value any) (any, error) {
	payload, ok := value.(plugin.CommentSavePayload)
	if !ok {
		return value, nil
	}
	// Only handle new comments and replies, not edits.
	if payload.Operation != "comment" && payload.Operation != "reply" {
		return value, nil
	}

	cfg, err := rt.Config(ctx, pluginName)
	if err != nil || cfg["enabled"] != "1" {
		return value, nil
	}

	sc, err := parseSMTPConfig(cfg)
	if err != nil {
		log.Printf("[comment-notifier] %v", err)
		return value, nil
	}

	comment, err := rt.CommentByID(ctx, payload.ID)
	if err != nil {
		log.Printf("[comment-notifier] query comment %d: %v", payload.ID, err)
		return value, nil
	}

	// Get content info via Runtime API.
	content, err := rt.ContentByID(ctx, comment.CID)
	if err != nil {
		log.Printf("[comment-notifier] query content %d: %v", comment.CID, err)
		return value, nil
	}

	authorUser, _ := rt.UserByID(ctx, content.AuthorID)

	siteTitle, _ := rt.Option(ctx, "title")
	siteURL, _ := rt.Option(ctx, "base_url")
	siteURL = strings.TrimRight(siteURL, "/")

	commentURL, _ := rt.CommentURL(ctx, comment.COID)

	// Deduplicate recipients by normalized email.
	recipients := make(map[string]notifyContext)

	switch comment.Status {
	case "approved":
		// Notify content author if the commenter is not the author.
		if cfg["notify_owner"] == "1" && authorUser.Mail != "" && !mailEqual(authorUser.Mail, comment.Mail) {
			recipients[strings.ToLower(authorUser.Mail)] = notifyContext{
				Type:      "owner",
				ToEmail:   authorUser.Mail,
				ToName:    authorUser.ScreenName,
				PostTitle: content.Title,
				PostURL:   commentURL,
				Author:    comment.Author,
				Content:   comment.Text,
				Time:      formatTime(comment.Created),
				SiteTitle: siteTitle,
				SiteURL:   siteURL,
			}
		}

		// Notify parent commenter if this is a reply.
		if comment.Parent > 0 && cfg["notify_parent"] == "1" {
			parent, err := rt.CommentByID(ctx, comment.Parent)
			if err == nil && parent.Mail != "" && !mailEqual(parent.Mail, comment.Mail) && !mailEqual(parent.Mail, authorUser.Mail) {
				recipients[strings.ToLower(parent.Mail)] = notifyContext{
					Type:          "guest",
					ToEmail:       parent.Mail,
					ToName:        parent.Author,
					PostTitle:     content.Title,
					PostURL:       commentURL,
					Author:        comment.Author,
					Content:       comment.Text,
					Time:          formatTime(comment.Created),
					ParentAuthor:  parent.Author,
					ParentContent: parent.Text,
					SiteTitle:     siteTitle,
					SiteURL:       siteURL,
				}
			}
		}

	case "waiting":
		// Notify admin about pending comment.
		if cfg["notify_pending"] == "1" {
			adminMail := strings.TrimSpace(cfg["admin_email"])
			if adminMail != "" && !mailEqual(adminMail, comment.Mail) {
				recipients[strings.ToLower(adminMail)] = notifyContext{
					Type:      "pending",
					ToEmail:   adminMail,
					PostTitle: content.Title,
					PostURL:   commentURL,
					Author:    comment.Author,
					Content:   comment.Text,
					Time:      formatTime(comment.Created),
					SiteTitle: siteTitle,
					SiteURL:   siteURL,
				}
			}
		}
	}

	// Send emails to all recipients.
	for _, nc := range recipients {
		nc := nc
		go func() {
			subject := buildSubject(nc)
			// Escape user input in HTML context.
			nc.Author = html.EscapeString(nc.Author)
			nc.Content = html.EscapeString(nc.Content)
			nc.ParentAuthor = html.EscapeString(nc.ParentAuthor)
			nc.ParentContent = html.EscapeString(nc.ParentContent)
			nc.PostTitle = html.EscapeString(nc.PostTitle)
			nc.SiteTitle = html.EscapeString(nc.SiteTitle)
			body, err := buildHTMLBody(nc)
			if err != nil {
				log.Printf("[comment-notifier] build body: %v", err)
				return
			}
			safeSendMail(sc, nc.ToEmail, subject, body)
		}()
	}

	return value, nil
}

// afterCommentMark handles the comment.after_mark hook.
// It sends notifications when a comment is first approved.
func afterCommentMark(ctx context.Context, rt *plugin.Runtime, value any) (any, error) {
	payload, ok := value.(plugin.CommentActionPayload)
	if !ok {
		return value, nil
	}
	// Only handle transition to approved, and skip if it was already approved.
	if payload.Status != "approved" || payload.PreviousStatus == "approved" {
		return value, nil
	}

	cfg, err := rt.Config(ctx, pluginName)
	if err != nil || cfg["enabled"] != "1" {
		return value, nil
	}

	sc, err := parseSMTPConfig(cfg)
	if err != nil {
		log.Printf("[comment-notifier] %v", err)
		return value, nil
	}

	comment, err := rt.CommentByID(ctx, payload.ID)
	if err != nil {
		log.Printf("[comment-notifier] query comment %d: %v", payload.ID, err)
		return value, nil
	}

	content, err := rt.ContentByID(ctx, comment.CID)
	if err != nil {
		log.Printf("[comment-notifier] query content %d: %v", comment.CID, err)
		return value, nil
	}

	authorUser, _ := rt.UserByID(ctx, content.AuthorID)

	siteTitle, _ := rt.Option(ctx, "title")
	siteURL, _ := rt.Option(ctx, "base_url")
	siteURL = strings.TrimRight(siteURL, "/")

	commentURL, _ := rt.CommentURL(ctx, comment.COID)

	adminMail := strings.TrimSpace(cfg["admin_email"])
	recipients := make(map[string]notifyContext)

	if comment.Parent > 0 && cfg["notify_parent"] == "1" {
		// Notify parent commenter.
		parent, err := rt.CommentByID(ctx, comment.Parent)
		if err == nil && parent.Mail != "" && !mailEqual(parent.Mail, comment.Mail) && !mailEqual(parent.Mail, adminMail) {
			recipients[strings.ToLower(parent.Mail)] = notifyContext{
				Type:          "guest",
				ToEmail:       parent.Mail,
				ToName:        parent.Author,
				PostTitle:     content.Title,
				PostURL:       commentURL,
				Author:        comment.Author,
				Content:       comment.Text,
				Time:          formatTime(comment.Created),
				ParentAuthor:  parent.Author,
				ParentContent: parent.Text,
				SiteTitle:     siteTitle,
				SiteURL:       siteURL,
			}
		}
	} else if cfg["notify_owner"] == "1" {
		// Notify content author.
		if authorUser.Mail != "" && !mailEqual(authorUser.Mail, comment.Mail) && !mailEqual(authorUser.Mail, adminMail) {
			recipients[strings.ToLower(authorUser.Mail)] = notifyContext{
				Type:      "owner",
				ToEmail:   authorUser.Mail,
				ToName:    authorUser.ScreenName,
				PostTitle: content.Title,
				PostURL:   commentURL,
				Author:    comment.Author,
				Content:   comment.Text,
				Time:      formatTime(comment.Created),
				SiteTitle: siteTitle,
				SiteURL:   siteURL,
			}
		}
	}

	for _, nc := range recipients {
		nc := nc
		go func() {
			subject := buildSubject(nc)
			nc.Author = html.EscapeString(nc.Author)
			nc.Content = html.EscapeString(nc.Content)
			nc.ParentAuthor = html.EscapeString(nc.ParentAuthor)
			nc.ParentContent = html.EscapeString(nc.ParentContent)
			nc.PostTitle = html.EscapeString(nc.PostTitle)
			nc.SiteTitle = html.EscapeString(nc.SiteTitle)
			body, err := buildHTMLBody(nc)
			if err != nil {
				log.Printf("[comment-notifier] build body: %v", err)
				return
			}
			safeSendMail(sc, nc.ToEmail, subject, body)
		}()
	}

	return value, nil
}

// mailEqual compares two email addresses case-insensitively.
func mailEqual(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// formatTime formats a Unix timestamp as a readable date-time string.
func formatTime(unix int64) string {
	if unix <= 0 {
		return ""
	}
	return time.Unix(unix, 0).Format("2006-01-02 15:04:05")
}
