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
		if !strings.Contains(tab.HTML, "tree-") {
			t.Errorf("%s 没有渲染成树状结构", tab.Key)
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
	// 树状结构：嵌套节点要带深度标记（data-depth），标量按类型着色
	nested := tabs[1].HTML // excel_import_mapping.yaml 结构最深
	for _, wanted := range []string{
		`<details class="tree-node" data-depth="0"`,
		`data-depth="4"`,
		`class="tree-value type-string"`,
	} {
		if !strings.Contains(nested, wanted) {
			t.Errorf("树状结构里缺少 %s", wanted)
		}
	}
	// 转义：渲染结果里不能出现未转义的可执行标签
	if strings.Contains(tabs[1].HTML, "<script") {
		t.Error("渲染结果里出现了未转义的 script 标签")
	}
}

// TestSettingsHTMLCarriesTheTreeStructure 固定树状渲染的契约：
// 容器节点用 <details class="tree-node" data-depth="N">，标量行用
// <div class="tree-row tree-leaf" data-depth="N">，值按 YAML 类型着色。
// 回归点：以前是 Markdown 风格的扁平排版（md-* 类），深层内容缩进曾经顶头；
// 现在缩进靠嵌套的 <details> 累加，深度没有上限。
func TestSettingsHTMLCarriesTheTreeStructure(t *testing.T) {
	workspace := t.TempDir()
	copyDirectoryForTest(t, "config", filepath.Join(workspace, "config"))
	loader, err := config.NewLoader(filepath.Join(workspace, "config"))
	if err != nil {
		t.Fatalf("读配置失败：%v", err)
	}
	app := NewApp(nil)
	app.loader = loader

	tabs := app.ConfigurationDocument()
	depthAttribute := regexp.MustCompile(`data-depth="(\d+)"`)
	maxDepth := -1
	for _, tab := range tabs {
		for _, match := range depthAttribute.FindAllStringSubmatch(tab.HTML, -1) {
			depth, convErr := strconv.Atoi(match[1])
			if convErr != nil {
				t.Fatalf("深度不是数字：%q", match[0])
			}
			if depth > maxDepth {
				maxDepth = depth
			}
		}
		if strings.Contains(tab.HTML, "md-h") || strings.Contains(tab.HTML, "md-kv") {
			t.Errorf("%s 里还有 Markdown 风格的旧排版残留", tab.Key)
		}
	}
	if maxDepth < 6 {
		t.Fatalf("最深层级 = %d；配置里最少有第 6 层（compare_by），用例没有覆盖到这个回归点", maxDepth)
	}
	// 前两层默认展开，再深的默认折叠：一千多行的配置不至于一上来铺满整屏
	importTab := tabs[1].HTML
	if !strings.Contains(importTab, `<details class="tree-node" data-depth="0" open>`) {
		t.Error("顶层节点应该默认展开")
	}
	if !strings.Contains(importTab, `<details class="tree-node" data-depth="2">`) {
		t.Error("第 2 层及更深的节点应该默认折叠")
	}
	// 容器节点带子项数量标注：映射 {n}、列表 [n]
	if !strings.Contains(importTab, `class="tree-meta">{`) || !strings.Contains(importTab, `class="tree-meta">[`) {
		t.Error("容器节点缺少 {n} / [n] 数量标注")
	}
	// 空集合要有占位说明，而不是一片空白（用一份临时配置验证，不动真实配置）
	emptyDir := t.TempDir()
	copyDirectoryForTest(t, "config", emptyDir)
	emptyPath := filepath.Join(emptyDir, config.ImportMappingFile)
	if err := os.WriteFile(emptyPath, []byte("version: \"1.0\"\nglobal_settings: {}\nsources: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	emptyLoader, err := config.NewLoader(emptyDir)
	if err != nil {
		t.Fatal(err)
	}
	emptyApp := NewApp(nil)
	emptyApp.loader = emptyLoader
	emptyTab := emptyApp.ConfigurationDocument()[1]
	if !strings.Contains(emptyTab.HTML, "（空 object）") {
		t.Errorf("空映射没有占位说明：%s", emptyTab.HTML)
	}
}

// TestTheTreeShowsACollapsedAndExpandedTriangle 固定展开/折叠指示：
// summary 是 flex 容器，会吃掉浏览器原生的三角标记 —— 所以自己画一个：
// 折叠 ▸、展开 ▾，并且要隐藏原生标记（否则会出现两个三角）。
// 回归点：一开始只给原生 ::marker 上色，结果是"没有三角可看"。
func TestTheTreeShowsACollapsedAndExpandedTriangle(t *testing.T) {
	style := readFrontendStyles(t)

	collapsed := styleRule(t, style, ".tree-node>summary::before{")
	if !strings.Contains(collapsed, `content:"▸"`) {
		t.Errorf("折叠状态的三角指示不对：%s", collapsed)
	}
	expanded := styleRule(t, style, ".tree-node[open]>summary::before{")
	if !strings.Contains(expanded, `content:"▾"`) {
		t.Errorf("展开状态的三角指示不对：%s", expanded)
	}
	// 两个状态必须真的不同（免得改成一个字符后回归）
	if collapsed == expanded {
		t.Error("折叠与展开的指示完全相同，用户看不出哪一节能展开")
	}
	// 原生标记要藏掉，避免和自绘的重叠
	if body := styleRule(t, style, ".tree-node>summary::-webkit-details-marker{"); !strings.Contains(body, "display:none") {
		t.Errorf("没有隐藏 Chromium/WebKit 的原生三角：%s", body)
	}
	if body := styleRule(t, style, ".tree-node>summary{"); !strings.Contains(body, "list-style:none") {
		t.Errorf("没有关掉原生列表标记（Firefox 会多出一个三角）：%s", body)
	}
	// 占位宽度固定，子节点缩进才能和父节点键名对齐
	if !strings.Contains(collapsed, "width:10px") {
		t.Errorf("三角占位宽度没固定：%s", collapsed)
	}
}

// TestSettingsTreeStylesNestTheIndent 固定样式侧的口径：
// 缩进由嵌套的 .tree-children 提供（不再需要"按层级写死 padding"或 CSS 变量），
// 值按类型着色 —— 四种类型都必须有各自的颜色。
func TestSettingsTreeStylesNestTheIndent(t *testing.T) {
	style := readFrontendStyles(t)

	if body := styleRule(t, style, ".tree-children{"); !strings.Contains(body, "padding-left:") {
		t.Errorf("树的缩进应由 .tree-children 提供：%s", body)
	}
	for rule, want := range map[string]string{
		".tree-value.type-string{": "#1B7F3B",
		".tree-value.type-number{": "#A4262C",
		".tree-value.type-bool{":   "#B26A00",
	} {
		if body := styleRule(t, style, rule); !strings.Contains(body, want) {
			t.Errorf("%s 缺少颜色 %s：%s", rule, want, body)
		}
	}
	if body := styleRule(t, style, ".tree-value.type-null{"); !strings.Contains(body, "var(--md-on-surface-medium)") {
		t.Errorf("空值应该用正文灰：%s", body)
	}
	// 旧的分层排版清单必须已经删干净（否则两套规则会互相打架）
	for _, gone := range []string{".settings-body .md-h{", ".settings-body .md-kv{", ".settings-body .md-list{"} {
		if strings.Contains(style, gone) {
			t.Errorf("样式里还留着旧的 Markdown 排版规则 %s", gone)
		}
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
