package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/config"
)

// TestConfigurationDocumentRendersFourTabs 固定「参数设定」窗口的数据来源：
// 四个页签对应 config/ 下的四份 YAML，都要渲染出非空的 Markdown 风格内容，
// 并且内容经过转义（配置里出现 < > 也不会变成标签）。
func TestConfigurationDocumentRendersFourTabs(t *testing.T) {
	workspace := t.TempDir()
	copyDirectoryForTest(t, "config", filepath.Join(workspace, "config"))
	loader, err := config.NewLoader(filepath.Join(workspace, "config"))
	if err != nil {
		t.Fatalf("读配置失败：%v", err)
	}
	app := NewApp(nil)
	app.loader = loader

	tabs := app.ConfigurationDocument()

	if len(tabs) != 4 {
		t.Fatalf("页签数量 = %d; want 4", len(tabs))
	}
	expectedTitles := []string{"基础设置", "导入映射", "整合映射", "日志列"}
	expectedKeys := []string{
		config.SettingFile, config.ImportMappingFile,
		config.ConsolidationMappingFile, config.LogColumnsFile,
	}
	for index, tab := range tabs {
		if tab.Title != expectedTitles[index] || tab.Key != expectedKeys[index] {
			t.Errorf("第 %d 个页签 = %q/%q; want %q/%q",
				index, tab.Title, tab.Key, expectedTitles[index], expectedKeys[index])
		}
		if tab.Path == "" || !strings.HasSuffix(tab.Path, tab.Key) {
			t.Errorf("%s 的路径不对：%q", tab.Key, tab.Path)
		}
		if len(tab.HTML) < 200 {
			t.Errorf("%s 渲染内容过短（%d 字节）", tab.Key, len(tab.HTML))
		}
		if !strings.Contains(tab.HTML, "md-") {
			t.Errorf("%s 没有渲染成 Markdown 风格结构", tab.Key)
		}
	}
	// 抽查每份配置里的代表性键都出现在对应页签
	expectations := map[string][]string{
		config.SettingFile:              {"export_template", "startup"},
		config.ImportMappingFile:        {"global_settings", "mandatory_status"},
		config.ConsolidationMappingFile: {"merge_rules", "duplicate_check"},
		config.LogColumnsFile:           {"columns", "material_code"},
	}
	for _, tab := range tabs {
		for _, wanted := range expectations[tab.Key] {
			if !strings.Contains(tab.HTML, wanted) {
				t.Errorf("%s 页签里缺少 %q", tab.Key, wanted)
			}
		}
	}
	// 转义：渲染结果里不能出现未转义的可执行标签
	if strings.Contains(tabs[1].HTML, "<script") {
		t.Error("渲染结果里出现了未转义的 script 标签")
	}
}
