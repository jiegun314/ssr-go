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
	style := readFrontendStyles(t)

	for _, rule := range []string{
		".settings-body .md-h{",
		".settings-body .md-kv{",
		".settings-body .md-list{",
	} {
		body := styleRule(t, style, rule)
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

// TestSettingsColorsStayThreeToned 固定颜色层级：参数设定里只允许三种颜色 ——
// 一级标题＝品牌红、二级标题＝正文黑、其余内容（键名、键值对的值、列表项）
// ＝一种正文灰。回归点：键值对的值 .md-v 与列表项 li 曾经没有跟随灰色，
// 结果同一份配置里"键是灰的、值/列表是黑的"。
func TestSettingsColorsStayThreeToned(t *testing.T) {
	style := readFrontendStyles(t)

	gray := "color:var(--md-on-surface-medium)"
	black := "color:var(--md-on-surface)"

	if body := styleRule(t, style, ".settings-body{"); !strings.Contains(body, gray) {
		t.Errorf("参数设定的基准色不是正文灰：%s", body)
	}
	// 一级、二级标题各自覆盖基准色，保留红色与黑色
	if body := styleRule(t, style, ".settings-body h3.md-h{"); !strings.Contains(body, "color:var(--md-primary)") {
		t.Errorf("一级标题不是品牌红：%s", body)
	}
	if body := styleRule(t, style, ".settings-body h4.md-h{"); !strings.Contains(body, black) {
		t.Errorf("二级标题不是正文黑：%s", body)
	}
	// 三级及更深的标题、键值对的值都不能再自己指定黑色（否则会脱离正文灰）
	for _, rule := range []string{".settings-body .md-h{", ".settings-body .md-v{"} {
		body := styleRule(t, style, rule)
		if strings.Contains(body, black) {
			t.Errorf("%s 不应指定正文黑，应跟随基准灰：%s", rule, body)
		}
	}
	// 列表项自己不设颜色，跟随 .settings-body 的基准灰
	if body := styleRule(t, style, ".settings-body .md-list{"); strings.Contains(body, "color:") {
		t.Errorf("列表不应单独指定颜色，应跟随基准灰：%s", body)
	}
}

// readFrontendStyles 取出界面文件里所有 <style> 块的内容。
// 界面有两块样式表（组件规则 + 颜色变量），少了任何一块都量不出真实颜色。
func readFrontendStyles(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("frontend", "dist", "index.html"))
	if err != nil {
		t.Fatalf("读界面文件失败：%v", err)
	}
	blocks := regexp.MustCompile(`(?s)<style>(.*?)</style>`).FindAllStringSubmatch(string(raw), -1)
	if len(blocks) == 0 {
		t.Fatal("界面文件里找不到 <style> 块")
	}
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		parts = append(parts, block[1])
	}
	return strings.Join(parts, "\n")
}

// styleRule 返回某条规则的声明体（选择器到第一个 } 之间的内容）。
func styleRule(t *testing.T, style, selector string) string {
	t.Helper()
	start := strings.Index(style, selector)
	if start < 0 {
		t.Fatalf("样式里找不到规则 %s", selector)
	}
	body := style[start+len(selector):]
	if end := strings.Index(body, "}"); end >= 0 {
		body = body[:end]
	}
	return body
}
