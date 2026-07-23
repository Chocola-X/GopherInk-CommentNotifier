package commentnotifier

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/Chocola-X/GopherInk/core/plugin"
)

var errNotifierRecordNotFound = errors.New("record not found")

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

	comment, err := notifierCommentByID(ctx, rt, payload.ID)
	if err != nil {
		log.Printf("[comment-notifier] query comment %d: %v", payload.ID, err)
		return value, nil
	}

	content, err := notifierContentByID(ctx, rt, comment.CID)
	if err != nil {
		log.Printf("[comment-notifier] query content %d: %v", comment.CID, err)
		return value, nil
	}

	authorUser, _ := notifierUserByID(ctx, rt, content.AuthorID)

	siteTitle, _ := rt.Option(ctx, "site_title")
	if siteTitle == "" {
		siteTitle = "GopherInk"
	}
	siteURL, _ := rt.Option(ctx, "base_url")
	siteURL = strings.TrimRight(siteURL, "/")

	lang := rt.Language(ctx)
	commentURL, _ := rt.CommentURL(ctx, comment.COID)
	commentAvatarURL := pluginAvatarURL(ctx, rt, comment.Mail, 72)

	recipients := make(map[string]notifyContext)

	switch comment.Status {
	case "approved":
		if cfg["notify_owner"] == "1" && comment.AuthorID != content.AuthorID && authorUser.Mail != "" && !mailEqual(authorUser.Mail, comment.Mail) {
			recipients[strings.ToLower(authorUser.Mail)] = notifyContext{
				Type:            "owner",
				Lang:            lang,
				ToEmail:         authorUser.Mail,
				ToName:          authorUser.ScreenName,
				PostTitle:       content.Title,
				PostURL:         commentURL,
				Author:          comment.Author,
				AuthorAvatarURL: commentAvatarURL,
				Content:         comment.Text,
				Time:            formatTime(comment.Created),
				SiteTitle:       siteTitle,
				SiteURL:         siteURL,
			}
		}

		// Notify parent commenter if this is a reply.
		if comment.Parent > 0 && cfg["notify_parent"] == "1" {
			parent, err := notifierCommentByID(ctx, rt, comment.Parent)
			if err == nil && parent.Mail != "" && !mailEqual(parent.Mail, comment.Mail) {
				recipients[strings.ToLower(parent.Mail)] = notifyContext{
					Type:            "guest",
					Lang:            lang,
					ToEmail:         parent.Mail,
					ToName:          parent.Author,
					PostTitle:       content.Title,
					PostURL:         commentURL,
					Author:          comment.Author,
					AuthorAvatarURL: commentAvatarURL,
					Content:         comment.Text,
					Time:            formatTime(comment.Created),
					ParentAuthor:    parent.Author,
					ParentAvatarURL: pluginAvatarURL(ctx, rt, parent.Mail, 72),
					ParentContent:   parent.Text,
					SiteTitle:       siteTitle,
					SiteURL:         siteURL,
				}
			}
		}

	case "waiting":
		// Notify admin about pending comment.
		if cfg["notify_pending"] == "1" {
			adminMail := strings.TrimSpace(cfg["admin_email"])
			if adminMail != "" && !mailEqual(adminMail, comment.Mail) {
				recipients[strings.ToLower(adminMail)] = notifyContext{
					Type:            "pending",
					Lang:            lang,
					ToEmail:         adminMail,
					PostTitle:       content.Title,
					PostURL:         commentURL,
					Author:          comment.Author,
					AuthorAvatarURL: commentAvatarURL,
					Content:         comment.Text,
					Time:            formatTime(comment.Created),
					SiteTitle:       siteTitle,
					SiteURL:         siteURL,
				}
			}
		}
	}

	for _, nc := range recipients {
		if err := queueNotification(sc, nc, cfg["email_template"]); err != nil {
			log.Printf("[comment-notifier] queue mail to %s: %v", nc.ToEmail, err)
		}
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

	comment, err := notifierCommentByID(ctx, rt, payload.ID)
	if err != nil {
		log.Printf("[comment-notifier] query comment %d: %v", payload.ID, err)
		return value, nil
	}

	content, err := notifierContentByID(ctx, rt, comment.CID)
	if err != nil {
		log.Printf("[comment-notifier] query content %d: %v", comment.CID, err)
		return value, nil
	}

	authorUser, _ := notifierUserByID(ctx, rt, content.AuthorID)

	siteTitle, _ := rt.Option(ctx, "site_title")
	if siteTitle == "" {
		siteTitle = "GopherInk"
	}
	siteURL, _ := rt.Option(ctx, "base_url")
	siteURL = strings.TrimRight(siteURL, "/")

	lang := rt.Language(ctx)
	commentURL, _ := rt.CommentURL(ctx, comment.COID)
	commentAvatarURL := pluginAvatarURL(ctx, rt, comment.Mail, 72)

	adminMail := strings.TrimSpace(cfg["admin_email"])
	recipients := make(map[string]notifyContext)

	if comment.Parent > 0 && cfg["notify_parent"] == "1" {
		parent, err := notifierCommentByID(ctx, rt, comment.Parent)
		if err == nil && parent.Mail != "" && !mailEqual(parent.Mail, comment.Mail) && !mailEqual(parent.Mail, adminMail) {
			recipients[strings.ToLower(parent.Mail)] = notifyContext{
				Type:            "guest",
				Lang:            lang,
				ToEmail:         parent.Mail,
				ToName:          parent.Author,
				PostTitle:       content.Title,
				PostURL:         commentURL,
				Author:          comment.Author,
				AuthorAvatarURL: commentAvatarURL,
				Content:         comment.Text,
				Time:            formatTime(comment.Created),
				ParentAuthor:    parent.Author,
				ParentAvatarURL: pluginAvatarURL(ctx, rt, parent.Mail, 72),
				ParentContent:   parent.Text,
				SiteTitle:       siteTitle,
				SiteURL:         siteURL,
			}
		}
	} else if cfg["notify_owner"] == "1" {
		if authorUser.Mail != "" && !mailEqual(authorUser.Mail, comment.Mail) && !mailEqual(authorUser.Mail, adminMail) {
			recipients[strings.ToLower(authorUser.Mail)] = notifyContext{
				Type:            "owner",
				Lang:            lang,
				ToEmail:         authorUser.Mail,
				ToName:          authorUser.ScreenName,
				PostTitle:       content.Title,
				PostURL:         commentURL,
				Author:          comment.Author,
				AuthorAvatarURL: commentAvatarURL,
				Content:         comment.Text,
				Time:            formatTime(comment.Created),
				SiteTitle:       siteTitle,
				SiteURL:         siteURL,
			}
		}
	}

	for _, nc := range recipients {
		if err := queueNotification(sc, nc, cfg["email_template"]); err != nil {
			log.Printf("[comment-notifier] queue mail to %s: %v", nc.ToEmail, err)
		}
	}

	return value, nil
}

// mailEqual compares two email addresses case-insensitively.
func mailEqual(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func pluginAvatarURL(ctx context.Context, rt *plugin.Runtime, mail string, size int) string {
	if rt == nil || rt.AvatarURL == nil {
		return ""
	}
	return rt.AvatarURL(ctx, mail, size)
}

func notifierCommentByID(ctx context.Context, rt *plugin.Runtime, id int64) (plugin.PublicComment, error) {
	comments, _, err := rt.ListComments(ctx, plugin.PublicCommentQuery{COID: id, Status: "all", Limit: 1})
	if err != nil {
		return plugin.PublicComment{}, err
	}
	if len(comments) == 0 {
		return plugin.PublicComment{}, errNotifierRecordNotFound
	}
	return comments[0], nil
}

func notifierContentByID(ctx context.Context, rt *plugin.Runtime, id int64) (plugin.PublicContent, error) {
	contents, _, err := rt.ListContents(ctx, plugin.PublicContentQuery{CID: id, Type: "all", Status: "all", IncludeDrafts: true, Limit: 1})
	if err != nil {
		return plugin.PublicContent{}, err
	}
	if len(contents) == 0 {
		return plugin.PublicContent{}, errNotifierRecordNotFound
	}
	return contents[0], nil
}

func notifierUserByID(ctx context.Context, rt *plugin.Runtime, id int64) (plugin.PublicUser, error) {
	users, _, err := rt.ListUsers(ctx, plugin.PublicUserQuery{UID: id, Limit: 1})
	if err != nil {
		return plugin.PublicUser{}, err
	}
	if len(users) == 0 {
		return plugin.PublicUser{}, errNotifierRecordNotFound
	}
	return users[0], nil
}

// formatTime formats a Unix timestamp as a readable date-time string.
func formatTime(unix int64) string {
	if unix <= 0 {
		return ""
	}
	return time.Unix(unix, 0).Format("2006-01-02 15:04:05")
}
