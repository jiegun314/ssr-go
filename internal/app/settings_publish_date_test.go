package app

// 「DI Record Publish Date 往后几天」这个固定参数在参数设定里的口径：
// 与统一社会信用代码同一处（setting.yaml 的 user_defined_data），有中文标签、数字控件，
// 改它只动一行；重新加载后，整合出来的日期列跟着走。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/config"
)

// TestPublishDateOffsetIsATunableSetting 固定"参数设定里能看到、能改"。
func TestPublishDateOffsetIsATunableSetting(t *testing.T) {
	workspace := t.TempDir()
	copyDirectoryForTest(t, "config", filepath.Join(workspace, "config"))
	loader, err := config.NewLoader(filepath.Join(workspace, "config"))
	if err != nil {
		t.Fatalf("读配置失败：%v", err)
	}
	app := NewApp(nil)
	app.loader = loader

	var node *SettingsNode
	for _, tab := range app.ConfigurationDocument() {
		if tab.Key != config.SettingFile {
			continue
		}
		node = findSettingsNode(tab.Tree, "user_defined_data", "di_record_publish_date_offset_days")
	}
	if node == nil {
		t.Fatal("参数设定里没有 di_record_publish_date_offset_days（应当与统一社会信用代码同一处）")
	}
	if node.Label != "DI 记录发布日期偏移天数" {
		t.Errorf("标签 = %q；want 中文标签", node.Label)
	}
	if node.Control != "number" || !node.Editable {
		t.Errorf("应当是数字控件且可编辑：control=%q editable=%v", node.Control, node.Editable)
	}
	if node.Min == nil || node.Max == nil || *node.Min != 0 || *node.Max != 3650 {
		t.Errorf("范围 = %v..%v；want 0..3650", node.Min, node.Max)
	}
	if node.Value != "5" {
		t.Errorf("默认值 = %q；want 5", node.Value)
	}
	if !strings.Contains(node.Note, "运行日") {
		t.Errorf("改动提示应当说明日期怎么算：%q", node.Note)
	}
}

// TestPublishDateOffsetCanBeChangedAndTakesEffect 固定"改了只动一行，且新值生效"。
func TestPublishDateOffsetCanBeChangedAndTakesEffect(t *testing.T) {
	app, configDir := newSettingsApp(t)
	path := filepath.Join(configDir, config.SettingFile)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读原文件失败：%v", err)
	}

	result := app.SaveSettingsValues(config.SettingFile, []SettingsChange{
		{Path: []string{"user_defined_data", "di_record_publish_date_offset_days"}, Value: "9"},
	})
	if result.Failed {
		t.Fatalf("保存失败：%s", result.Message)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读回失败：%v", err)
	}
	changed := diffLines(string(before), string(after))
	if len(changed) != 1 || !strings.Contains(changed[0], "di_record_publish_date_offset_days: 9") {
		t.Fatalf("应当只改动这一行，实际 %v", changed)
	}

	// 重新加载（参数在 setting.yaml 里，改完要重新读）后，参数就是新值
	fresh, err := config.NewLoader(configDir)
	if err != nil {
		t.Fatalf("重读配置失败：%v", err)
	}
	setting, err := fresh.LoadSetting()
	if err != nil {
		t.Fatalf("读 setting.yaml 失败：%v", err)
	}
	userDefined, _ := setting["user_defined_data"].(map[string]any)
	if got := userDefined["di_record_publish_date_offset_days"]; got != 9 {
		t.Fatalf("参数 = %#v；want 9", got)
	}
}
