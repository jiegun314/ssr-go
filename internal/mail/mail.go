// Package mail 通过本机 Outlook 发送/起草邮件（不依赖 SMTP 服务器）。
//
// 设计口径：
//   - Windows：PowerShell + Outlook COM（New-Object -ComObject Outlook.Application）；
//   - macOS：AppleScript 调 Microsoft Outlook；
//   - 其它平台：明确报错（本产品不做 Linux 桌面版）。
//
// 「发送方式」由 config 决定：send = 直接发送；draft = 打开草稿，由用户点发送。
// 测试通过 App 注入 Sender，不会真的调用 Outlook。
package mail

import "strings"

// Message 是一封要交给本机 Outlook 的邮件。
type Message struct {
	To      []string
	Subject string
	Body    string
	// Mode 是 send（默认）或 draft。
	Mode string
}

// Sender 把邮件交给本机邮件客户端。
type Sender interface {
	Send(message Message) error
}

// Mode 归一化发送方式：只有 draft 是草稿，其余按 send。
func NormalizeMode(mode string) string {
	if strings.TrimSpace(strings.ToLower(mode)) == "draft" {
		return "draft"
	}
	return "send"
}

// NewOutlookSender 按平台创建本机 Outlook 发送器。
func NewOutlookSender(mode string) Sender {
	return newSender(NormalizeMode(mode))
}

// To 把收件人拼成 Outlook 认的分号分隔格式（去空、去重）。
func recipientsLine(list []string) string {
	seen := map[string]bool{}
	parts := []string{}
	for _, item := range list {
		address := strings.TrimSpace(item)
		if address == "" || seen[strings.ToLower(address)] {
			continue
		}
		seen[strings.ToLower(address)] = true
		parts = append(parts, address)
	}
	return strings.Join(parts, "; ")
}
