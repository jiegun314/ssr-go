package main

// 用户反馈：先在选项（参数设定）里配置收件人。
// 收件人列表在「共享盘」页签里可增删；邮箱格式不对会被校验拦下并还原。

import (
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/config"
)

func TestRecipientsAreEditableInSettings(t *testing.T) {
	app := newShareApp(t, "")
	var workflowTab SettingsTab
	for _, tab := range app.ConfigurationDocument() {
		if tab.Key == config.WorkflowFile {
			workflowTab = tab
		}
	}
	if workflowTab.Key == "" {
		t.Fatal("应当有「共享盘」页签")
	}
	if !strings.Contains(workflowTab.Raw, "ra_receivers: []") {
		t.Fatalf("模板里应当有 recipients.ra_receivers：%q", workflowTab.Raw)
	}

	content := strings.Replace(workflowTab.Raw,
		"  ra_receivers: []",
		"  ra_receivers:\n    - \"ra.one@example.com\"\n    - \"ra.two@example.com\"", 1)
	if saved := app.SaveConfigurationFile(config.WorkflowFile, content); saved.Failed {
		t.Fatalf("保存收件人失败：%+v", saved)
	}

	// 保存后：结构化树里能看到两个收件人，且列表允许增删
	for _, tab := range app.ConfigurationDocument() {
		if tab.Key != config.WorkflowFile {
			continue
		}
		node := findSettingsNode(tab.Tree, "recipients", "ra_receivers")
		if node == nil {
			t.Fatal("保存后应当有 recipients.ra_receivers 节点")
		}
		if !node.ListEdit {
			t.Error("收件人列表应当允许增删条目")
		}
		if node.Label != "RA 数据接收人" {
			t.Errorf("收件人列表的中文标签 = %q；want RA 数据接收人", node.Label)
		}
		if len(node.Children) != 2 {
			t.Errorf("应当有两个收件人，实际 %d", len(node.Children))
		}
	}

	// 非法邮箱：拒绝并还原
	bad := strings.Replace(workflowTab.Raw,
		"  ra_receivers: []",
		"  ra_receivers:\n    - \"not-an-email\"", 1)
	if saved := app.SaveConfigurationFile(config.WorkflowFile, bad); !saved.Failed {
		t.Fatal("非法邮箱应当被拒绝")
	}
	restored := app.ConfigurationDocument()
	for _, tab := range restored {
		if tab.Key != config.WorkflowFile {
			continue
		}
		if node := findSettingsNode(tab.Tree, "recipients", "ra_receivers"); node != nil && len(node.Children) == 1 {
			t.Error("校验失败后应当还原为保存前的两个收件人")
		}
	}
}
