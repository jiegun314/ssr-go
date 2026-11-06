//go:build darwin

package mail

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type outlookSender struct {
	mode string
}

func newSender(mode string) Sender {
	return outlookSender{mode: mode}
}

func (sender outlookSender) Send(message Message) error {
	if len(message.To) == 0 {
		return fmt.Errorf("没有收件人")
	}
	if sender.mode == "draft" {
		// 草稿：交给默认邮件客户端（Outlook）打开，收件人/主题/正文都预填好
		return openDraft(message)
	}
	return sendWithOutlook(message)
}

// openDraft 用 mailto: 打开本机默认邮件客户端（通常是 Outlook）的草稿窗口。
func openDraft(message Message) error {
	query := url.Values{}
	query.Set("subject", message.Subject)
	query.Set("body", message.Body)
	mailto := "mailto:" + strings.Join(message.To, ",") + "?" + query.Encode()
	if err := exec.Command("open", mailto).Run(); err != nil {
		return fmt.Errorf("打开 Outlook 草稿失败：%v", err)
	}
	return nil
}

// sendWithOutlook 用 AppleScript 让 Microsoft Outlook 直接发送。
func sendWithOutlook(message Message) error {
	directory, err := os.MkdirTemp("", "ssr-mail-")
	if err != nil {
		return fmt.Errorf("准备邮件内容失败：%v", err)
	}
	defer os.RemoveAll(directory)

	bodyPath := filepath.Join(directory, "body.txt")
	subjectPath := filepath.Join(directory, "subject.txt")
	if err := os.WriteFile(bodyPath, []byte(message.Body), 0o600); err != nil {
		return fmt.Errorf("准备邮件内容失败：%v", err)
	}
	if err := os.WriteFile(subjectPath, []byte(message.Subject), 0o600); err != nil {
		return fmt.Errorf("准备邮件内容失败：%v", err)
	}

	var lines []string
	lines = append(lines,
		"set bodyText to (read POSIX file \""+escapeAppleScript(bodyPath)+"\" as «class utf8»)",
		"set subjectText to (read POSIX file \""+escapeAppleScript(subjectPath)+"\" as «class utf8»)",
		"tell application \"Microsoft Outlook\"",
		"  set msg to make new outgoing message with properties {subject:subjectText, content:bodyText}",
	)
	for _, address := range message.To {
		address = strings.TrimSpace(address)
		if address == "" {
			continue
		}
		lines = append(lines,
			"  make new recipient at msg with properties {email address:{address:\""+escapeAppleScript(address)+"\"}}")
	}
	lines = append(lines, "  send msg", "end tell")
	scriptPath := filepath.Join(directory, "send.applescript")
	if err := os.WriteFile(scriptPath, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		return fmt.Errorf("准备邮件脚本失败：%v", err)
	}
	output, err := exec.Command("osascript", scriptPath).CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("Outlook 发送失败：%s", detail)
	}
	return nil
}

func escapeAppleScript(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	return strings.ReplaceAll(value, "\"", "\\\"")
}
