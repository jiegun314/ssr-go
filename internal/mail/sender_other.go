//go:build !windows && !darwin

package mail

import "fmt"

type outlookSender struct{}

func newSender(string) Sender { return outlookSender{} }

func (outlookSender) Send(Message) error {
	return fmt.Errorf("邮件推送只支持 Windows / macOS 上的本机 Outlook")
}
