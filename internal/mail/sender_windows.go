//go:build windows

package mail

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"unicode/utf16"
)

// outlookScript 通过环境变量取值，避免中文/引号/换行进命令行转义。
const outlookScript = `$ErrorActionPreference = 'Stop'
$outlook = New-Object -ComObject Outlook.Application
$mail = $outlook.CreateItem(0)
$mail.To = $env:SSR_MAIL_TO
$mail.Subject = $env:SSR_MAIL_SUBJECT
$mail.Body = $env:SSR_MAIL_BODY
if ($env:SSR_MAIL_MODE -eq 'draft') {
    $mail.Display()
} else {
    $mail.Send()
}`

type outlookSender struct {
	mode string
}

func newSender(mode string) Sender {
	return outlookSender{mode: mode}
}

func (sender outlookSender) Send(message Message) error {
	to := recipientsLine(message.To)
	if to == "" {
		return fmt.Errorf("没有收件人")
	}
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive",
		"-ExecutionPolicy", "Bypass", "-EncodedCommand", encodePowerShell(outlookScript))
	cmd.Env = append(os.Environ(),
		"SSR_MAIL_TO="+to,
		"SSR_MAIL_SUBJECT="+message.Subject,
		"SSR_MAIL_BODY="+message.Body,
		"SSR_MAIL_MODE="+sender.mode,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("Outlook 发送失败：%s", detail)
	}
	return nil
}

// encodePowerShell 把脚本编成 UTF-16LE + base64（powershell -EncodedCommand 的格式）。
func encodePowerShell(script string) string {
	units := utf16.Encode([]rune(script))
	raw := make([]byte, 0, len(units)*2)
	for _, unit := range units {
		raw = append(raw, byte(unit), byte(unit>>8))
	}
	return base64.StdEncoding.EncodeToString(raw)
}
