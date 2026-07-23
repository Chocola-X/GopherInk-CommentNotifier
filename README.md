# GopherInk-CommentNotifier

[GopherInk](https://github.com/Chocola-X/GopherInk) 博客系统的评论邮件通知插件。当新评论发布、回复或审核通过时，自动向相关用户发送邮件通知。

## 功能特性

- **评论通知**：文章收到新评论时通知作者
- **回复通知**：评论被回复时通知原评论者
- **待审核通知**：待审核评论通知管理员
- **审核通过通知**：评论审核通过后通知相关用户
- **SMTP 支持**：支持 SSL / STARTTLS / 无加密三种模式
- **异步发送**：基于 Go channel 的邮件队列，2 个 worker 并发消费
- **模板自定义**：可视化 HTML 邮件模板编辑器，支持实时预览
- **测试邮件**：后台一键发送测试邮件验证配置
- **国际化**：内置中文（zh-CN）支持

## 要求

- GopherInk >= 0.5.0
- Go >= 1.25.0

## 安装

将本目录放置于 GopherInk 的 `plugins/GopherInk-CommentNotifier` 下，重新编译即可。

## 配置

在后台 **插件 → Comment Notifier → 配置** 页面进行设置。

### 通用设置

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| 启用邮件提醒 | 总开关 | 开启 |
| 通知内容作者 | 文章收到新评论时通知作者 | 开启 |
| 通知被回复者 | 评论被回复时通知原评论者 | 开启 |
| 待审核评论通知管理员 | 待审核评论通知管理员 | 开启 |
| 管理员收件邮箱 | 接收待审核评论通知的邮箱 | — |

### SMTP 设置

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| SMTP 服务器 | SMTP 服务器地址 | `smtp.qq.com` |
| SMTP 端口 | 端口号（1-65535） | `465` |
| SMTP 加密模式 | `none` / `ssl` / `tls` | `ssl` |
| SMTP 用户名 | 登录用户名 | — |
| SMTP 密码或授权码 | 通常为邮箱授权码 | — |

### 发件人设置

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| 发件邮箱 | 发件人邮箱地址 | — |
| 发件人名称 | 发件人显示名称 | `GopherInk` |

## 邮件模板

在后台 **插件 → Comment Notifier → 外观** 页面可编辑 HTML 邮件模板并实时预览。

### 可用占位符

| 占位符 | 说明 |
|--------|------|
| `{notification_type}` | 通知类型：`owner` / `guest` / `pending` / `test` |
| `{headline}` | 通知标题 |
| `{intro}` | 通知摘要 |
| `{site_title}` | 站点名称 |
| `{site_url}` | 站点地址 |
| `{post_title}` | 文章标题 |
| `{post_url}` | 评论所在位置的链接 |
| `{recipient_name}` | 收件人名称 |
| `{comment_author}` | 评论者名称 |
| `{comment_avatar_url}` | 评论者头像 URL |
| `{comment_label}` | 评论/回复/测试内容标签 |
| `{comment_content}` | 评论正文 |
| `{comment_time}` | 评论时间 |
| `{parent_author}` | 被回复者名称 |
| `{parent_avatar_url}` | 被回复者头像 URL |
| `{parent_content}` | 被回复的评论正文 |
| `{parent_comment_block}` | 原评论区块（仅回复通知） |
| `{action_url}` | 操作按钮链接 |
| `{action_label}` | 操作按钮文本 |

## 通知触发逻辑

### 评论发布时

1. 已审核评论 → 通知文章作者 + 通知被回复者（如有）
2. 待审核评论 → 通知管理员

### 评论审核通过时

1. 回复评论 → 通知被回复者
2. 普通评论 → 通知文章作者
3. 自动排除管理员邮箱，避免重复通知

## 许可证

[GNU AGPL-3.0](LICENSE)
