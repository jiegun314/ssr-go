package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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
	// 逐级缩进：嵌套层级要带深度类（CSS 用 md-d0/1/2… 控制缩进）
	nested := tabs[1].HTML // excel_import_mapping.yaml 结构最深
	for _, wanted := range []string{"md-d1", "md-d2", "md-d3"} {
		if !strings.Contains(nested, wanted) {
			t.Errorf("嵌套内容缺少深度类 %s", wanted)
		}
	}
	// 转义：渲染结果里不能出现未转义的可执行标签
	if strings.Contains(tabs[1].HTML, "<script") {
		t.Error("渲染结果里出现了未转义的 script 标签")
	}
}

// TestSettingsHTMLCarriesAnIndentForEveryDepth 固定缩进的写法：每个带层级类的
// 元素都要在 style 里带上本层的缩进基准（--md-indent = 层数 × 12px）。
// 回归点：CSS 里曾经只定义 md-d0…md-d5 六个基准，整合映射的
// merge_rules.sources.*.duplicates.compare_by（第 6 层）取不到值，列表项顶到最左边。
func TestSettingsHTMLCarriesAnIndentForEveryDepth(t *testing.T) {
	workspace := t.TempDir()
	copyDirectoryForTest(t, "config", filepath.Join(workspace, "config"))
	loader, err := config.NewLoader(filepath.Join(workspace, "config"))
	if err != nil {
		t.Fatalf("读配置失败：%v", err)
	}
	app := NewApp(nil)
	app.loader = loader

	depthClass := regexp.MustCompile(`<(h[3-6]|div|ul|li) class="([^"]*md-d(\d+)[^"]*)"([^>]*)>`)
	maxDepth := -1
	for _, tab := range app.ConfigurationDocument() {
		for _, match := range depthClass.FindAllStringSubmatch(tab.HTML, -1) {
			depth, convErr := strconv.Atoi(match[3])
			if convErr != nil {
				t.Fatalf("层数不是数字：%q", match[0])
			}
			if depth > maxDepth {
				maxDepth = depth
			}
			if match[1] == "li" {
				continue // 列表项不消费缩进变量（缩进由它所属的 ul 提供）
			}
			wanted := `style="--md-indent:` + strconv.Itoa(depth*12) + `px"`
			if !strings.Contains(match[4], wanted) {
				t.Errorf("%s: 第 %d 层元素缺少缩进基准 %s：%s", tab.Key, depth, wanted, match[0])
			}
		}
	}
	if maxDepth < 6 {
		t.Fatalf("最深层级 = %d；配置里最少有第 6 层（compare_by），用例没有覆盖到这个回归点", maxDepth)
	}
}

// TestSettingsStylesConsumeTheIndentVariable 固定样式表与渲染器的分工：
// 缩进基准由渲染器写到元素上，CSS 只负责消费 —— 不能再出现"按层级写死的
// padding-left 清单"（那正是深层内容顶头的原因）。
func TestSettingsStylesConsumeTheIndentVariable(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("frontend", "dist", "index.html"))
	if err != nil {
		t.Fatalf("读界面文件失败：%v", err)
	}
	style := string(raw)

	for _, rule := range []string{
		".settings-body .md-h{",
		".settings-body .md-kv{",
		".settings-body .md-list{",
	} {
		start := strings.Index(style, rule)
		if start < 0 {
			t.Fatalf("样式里找不到规则 %s", rule)
		}
		body := style[start:]
		if end := strings.Index(body, "}"); end >= 0 {
			body = body[:end]
		}
		if !strings.Contains(body, "var(--md-indent") {
			t.Errorf("%s 没有消费缩进变量：%s", rule, body)
		}
	}
	// 一级块之间的淡分隔线
	if !strings.Contains(style, ".settings-body h3.md-h:not(:first-child){border-top:1px solid rgba(0,0,0,.12)") {
		t.Error("样式里缺少一级块之间的分隔线")
	}
	// 按层级写死缩进的清单必须已经删掉（否则会与变量方案互相打架）
	if matched, _ := regexp.MatchString(`\.md-d\d+\{padding-left`, style); matched {
		t.Error("样式里还有按层级写死的 padding-left 清单")
	}
}
