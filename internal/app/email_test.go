package app

// 数据更新通知：导入后把推送时间/数据量发给 Supply Chain 与 Admin（本机 Outlook）。
// 测试注入假 mailer，不真的调用 Outlook。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/mail"
)

type fakeMailer struct {
	messages []mail.Message
	err      error
}

func (fake *fakeMailer) Send(message mail.Message) error {
	fake.messages = append(fake.messages, message)
	return fake.err
}

func prepareNotificationApp(t *testing.T) *App {
	t.Helper()
	shareRoot := filepath.Join(t.TempDir(), "share")
	if err := os.MkdirAll(shareRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	app := newUsersShareApp(t, shareRoot)
	if login := app.Login("ra", ""); login.Failed {
		t.Fatalf("RA 登录失败：%+v", login)
	}
	if result := app.ImportSource("ra_input", filepath.Join("testdata", "sample-valid", "ra_input.xlsx")); result.Failed {
		t.Fatalf("导入失败：%+v", result)
	}
	app.mailer = &fakeMailer{}
	return app
}

func TestSendImportNotificationToSupplyChainAndAdmin(t *testing.T) {
	app := prepareNotificationApp(t)
	app.shareConfig.EmailTransport = "outlook"
	app.shareConfig.EmailMode = "send"
	app.shareConfig.Recipients = map[string][]string{
		"supply_chain": {"sc@example.com"},
		"admins":       {"admin@example.com"},
	}
	fake := app.mailer.(*fakeMailer)

	preview := app.PreviewImportNotification("ra_input")
	if !preview.Ready {
		t.Fatalf("导入后应当可以预览通知：%+v", preview)
	}
	if len(preview.To) != 2 || preview.To[0] != "sc@example.com" || preview.To[1] != "admin@example.com" {
		t.Errorf("收件人应当是 Supply Chain + Admin：%v", preview.To)
	}
	if !strings.Contains(preview.Subject, "RA信息") {
		t.Errorf("邮件主题缺少来源：%s", preview.Subject)
	}
	if !strings.Contains(preview.Body, "数据量：6 行") || !strings.Contains(preview.Body, "导入时间：") {
		t.Errorf("邮件正文缺少时间/数据量：\n%s", preview.Body)
	}

	result := app.SendImportNotification("ra_input")
	if result.Failed {
		t.Fatalf("发送失败：%+v", result)
	}
	if len(fake.messages) != 1 {
		t.Fatalf("应当调用一次 Outlook 发送器，实际 %d", len(fake.messages))
	}
	message := fake.messages[0]
	if message.Mode != "send" {
		t.Errorf("发送方式应当是 send：%s", message.Mode)
	}
	if strings.Join(message.To, ",") != "sc@example.com,admin@example.com" {
		t.Errorf("收件人不对：%v", message.To)
	}
	if !strings.Contains(message.Body, "RA信息") {
		t.Errorf("正文缺少来源：\n%s", message.Body)
	}
}

func TestNotificationFallsBackToUsersEmails(t *testing.T) {
	app := prepareNotificationApp(t)
	app.shareConfig.EmailTransport = "outlook"
	app.shareConfig.Recipients = nil

	preview := app.PreviewImportNotification("ra_input")
	if !preview.Ready {
		t.Fatalf("用户名单里有邮箱时应当可以发送：%+v", preview)
	}
	joined := strings.Join(preview.To, ",")
	if !strings.Contains(joined, "zhaoliu@example.com") || !strings.Contains(joined, "admin@example.com") {
		t.Errorf("没有收件人配置时应当回退到 users.yaml 的 Supply Chain / Admin 邮箱：%v", preview.To)
	}
}

func TestNotificationRequiresImportedDataAndRecipients(t *testing.T) {
	shareRoot := filepath.Join(t.TempDir(), "share")
	if err := os.MkdirAll(shareRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	app := newUsersShareApp(t, shareRoot)
	if login := app.Login("ra", ""); login.Failed {
		t.Fatalf("登录失败：%+v", login)
	}
	preview := app.PreviewImportNotification("ra_input")
	if preview.Ready || !strings.Contains(preview.Reason, "还没有导入") {
		t.Errorf("未导入时应当说明原因：%+v", preview)
	}
	if result := app.SendImportNotification("ra_input"); !result.Failed {
		t.Error("未导入时发送应当失败")
	}

	// 导入后但收件人为空（用户邮箱也清空）
	if result := app.ImportSource("ra_input", filepath.Join("testdata", "sample-valid", "ra_input.xlsx")); result.Failed {
		t.Fatalf("导入失败：%+v", result)
	}
	app.users = nil
	preview = app.PreviewImportNotification("ra_input")
	if preview.Ready || !strings.Contains(preview.Reason, "没有收件人") {
		t.Errorf("没有收件人时应当说明原因：%+v", preview)
	}
}

func TestNotificationTransportAndPermission(t *testing.T) {
	app := prepareNotificationApp(t)
	// 通道关闭
	app.shareConfig.EmailTransport = "disabled"
	app.shareConfig.Recipients = map[string][]string{"supply_chain": {"sc@example.com"}}
	if result := app.SendImportNotification("ra_input"); !result.Failed ||
		!strings.Contains(result.Title, "邮件通道未启用") {
		t.Errorf("通道关闭时应当拒绝发送：%+v", result)
	}

	// 非导入角色没有权限
	app.shareConfig.EmailTransport = "outlook"
	app.Login("supply_chain", "")
	if result := app.SendImportNotification("ra_input"); !result.Failed ||
		!strings.Contains(result.Title, "无权限") {
		t.Errorf("Supply Chain 不应能代表 RA 发通知：%+v", result)
	}
}

func TestNotificationDraftMode(t *testing.T) {
	app := prepareNotificationApp(t)
	app.shareConfig.EmailTransport = "outlook"
	app.shareConfig.EmailMode = "draft"
	app.shareConfig.Recipients = map[string][]string{"admins": {"admin@example.com"}}
	fake := app.mailer.(*fakeMailer)

	result := app.SendImportNotification("ra_input")
	if result.Failed {
		t.Fatalf("草稿模式发送失败：%+v", result)
	}
	if result.Title != "已打开邮件草稿" {
		t.Errorf("草稿模式的标题不对：%s", result.Title)
	}
	if len(fake.messages) != 1 || fake.messages[0].Mode != "draft" {
		t.Errorf("应当以 draft 模式交给 Outlook：%+v", fake.messages)
	}
}
