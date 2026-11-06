package mail

import "testing"

func TestNormalizeModeAndRecipients(t *testing.T) {
	if NormalizeMode("draft") != "draft" || NormalizeMode(" DRAFT ") != "draft" {
		t.Error("draft 应当归一化为草稿")
	}
	if NormalizeMode("send") != "send" || NormalizeMode("") != "send" || NormalizeMode("x") != "send" {
		t.Error("其它值应当按 send 处理")
	}
	if got := recipientsLine([]string{" a@x.com ", "A@X.COM", "", "b@y.com"}); got != "a@x.com; b@y.com" {
		t.Errorf("收件人去重/去空后应当是 a@x.com; b@y.com，实际 %q", got)
	}
}
