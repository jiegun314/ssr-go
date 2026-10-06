package main

// 前端契约测试：参数设定窗口（结构树、结构化表单、对比度、底栏）。
//
// 公共前言与助手（源码读取、CSS 抽取、对比度）见 layout_helpers_test.go。
import (
	"strings"
	"testing"
)

// TestTheSettingsTreeUsesAntdTree 固定参数设定：Go 返回结构化树 → antd Tree 渲染，
// 容器标 {n}/[n]、标量按类型着色、空集合有占位，另有路径条与全部展开/折叠。
func TestTheSettingsTreeUsesAntdTree(t *testing.T) {
	modals := readFrontendSource(t, "src/components/Modals.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	for _, wanted := range []string{
		`} from "antd"`,
		"treeData={treeData}",
		"expandedKeys={expandedKeys}",
		"onSelect={(keys) => onSelectNode(",
		"tree-empty", // 空集合占位
		"全部展开",
		"全部折叠",
		"settings-path", // 路径条
	} {
		if !strings.Contains(modals, wanted) {
			t.Errorf("SettingsModal 里缺少：%s", wanted)
		}
	}
	// 类型着色：四类都有自己的颜色，且不是等宽字体
	for _, wanted := range []string{
		".tree-value.type-string", ".tree-value.type-number",
		".tree-value.type-bool", ".tree-value.type-null",
	} {
		if !strings.Contains(styles, wanted) {
			t.Errorf("样式里缺少值文本的类型配色：%s", wanted)
		}
	}
}

// TestTheSettingsTreeValuesMeetContrastOnWhite 固定参数设定里值文本的可读性：
// 三种有颜色的标量在白底上都要过 WCAG AA（≥ 4.5:1），
// 且 styles.css 里的颜色必须与 theme.ts 的 VALUE_COLORS 对得上（不许两处各写一套）。
func TestTheSettingsTreeValuesMeetContrastOnWhite(t *testing.T) {
	theme := readFrontendSource(t, "src/theme.ts")
	styles := readFrontendSource(t, "src/styles.css")

	colors := themeColors(t, theme, "export const VALUE_COLORS")
	for _, kind := range []string{"string", "number", "bool"} {
		hex, ok := colors[kind]
		if !ok {
			t.Fatalf("theme.ts 的 VALUE_COLORS 里缺少 %s", kind)
		}
		if ratio := contrastOnWhite(t, hex); ratio < 4.5 {
			t.Errorf("tree-value.type-%s 的颜色 %s 在白底上只有 %.2f:1，低于 AA 的 4.5:1", kind, hex, ratio)
		}
	}
	// 样式里那份必须跟主题一致（历史上这里漏改成 4.24:1 的 #B26A00）
	want := "color: " + colors["bool"] + ";"
	if !strings.Contains(styles, want) {
		t.Errorf("styles.css 的 .tree-value.type-bool 应当用 theme.ts 里的 %s（找不到 %q）", colors["bool"], want)
	}
}

// TestTheSettingsFormConvertsYamlToControls 固定「参数设定」的结构化表单（方案 B）：
// 每一层 YAML 都分块显示、所有注释都显示出来；标量按后端给的 control 渲染成 antd 控件，
// 只读项显示值与原因；列清单用表格 + 抽屉且禁止增删排序；标量列表可增删；
// 改动先攒起来、由「保存改动」一次性提交给 SaveSettingsValues（后端只改被编辑的那几行）。
func TestTheSettingsFormConvertsYamlToControls(t *testing.T) {
	form := readFrontendSource(t, "src/components/SettingsForm.tsx")
	modals := readFrontendSource(t, "src/components/Modals.tsx")
	app := readFrontendSource(t, "src/App.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	// 1) 分块 + 注释（显示时去掉行首 "#"）+ 递归
	for _, wanted := range []string{
		`<section className="settings-block" data-depth={depth} data-kind={node.kind}>`,
		`<pre className="settings-comment">{stripCommentMarks(node.comment)}</pre>`,
		"export function stripCommentMarks(text: string): string",
		`return indent + trimmed.replace(/^#[ ]?/, "");`,
		"export function SettingsBlock(",
		"(node.children ?? []).map((child) => (",
	} {
		if !strings.Contains(form, wanted) {
			t.Errorf("分块表单缺少：%s", wanted)
		}
	}
	// 2) 控件映射：switch / number / select / input，只读项不给控件
	for _, wanted := range []string{
		`case "switch":`,
		`case "number":`,
		`case "select":`,
		"<InputNumber",
		"<Switch",
		"<Select",
		"<Input",
		"if (!node.editable) {",
		"<span className={`settings-readonly type-${node.kind}`}>{shown}</span>",
		`node.reason || "只读"`,
	} {
		if !strings.Contains(form, wanted) {
			t.Errorf("控件映射缺少：%s", wanted)
		}
	}
	// 3) 列清单：表格 + 抽屉 + 明确"不支持增删排序"
	for _, wanted := range []string{
		"function isColumnList(",
		"<Table<SettingsNode>",
		"<Drawer",
		"不支持新增 / 删除 / 排序",
	} {
		if !strings.Contains(form, wanted) {
			t.Errorf("列清单缺少：%s", wanted)
		}
	}
	// 4) 标量列表可增删
	for _, wanted := range []string{
		"function isScalarList(",
		`action: "append"`,
		`action: "remove"`,
		"添加一条",
	} {
		if !strings.Contains(form, wanted) {
			t.Errorf("标量列表缺少：%s", wanted)
		}
	}
	// 5) 三种视图 + 攒改动 + 保存
	for _, wanted := range []string{
		`type SettingsView = "form" | "tree" | "raw";`,
		`{ label: "表单", value: "form" }`,
		`{ label: "结构树", value: "tree" }`,
		`{ label: "原文", value: "raw" }`,
		"onSaveValues",
		"保存改动",
		"撤销改动",
		"<SettingsForm",
	} {
		if !strings.Contains(modals, wanted) {
			t.Errorf("参数设定窗口缺少：%s", wanted)
		}
	}
	if !strings.Contains(app, `call<SettingsSaveResult>("SaveSettingsValues", activeTab, changes)`) {
		t.Error("保存改动应当调 SaveSettingsValues（结构化写回，只改被编辑的行）")
	}
	if !strings.Contains(app, `call<SettingsTab[]>("ConfigurationDocument")`) {
		t.Error("保存成功后应当刷新配置文档")
	}
	// 6) 样式：分块描边直角 + 注释原样换行 + 只读灰
	for _, wanted := range []string{
		".settings-block {",
		"border-radius: 0;",
		".settings-comment {",
		"white-space: pre-wrap;",
		".settings-field {",
		".settings-readonly {",
		".settings-list-row {",
	} {
		if !strings.Contains(styles, wanted) {
			t.Errorf("参数设定样式缺少：%s", wanted)
		}
	}
	// 7) 原文视图还在（结构性改动与注释极端场景的兜底）
	if !strings.Contains(modals, "settings-editor") {
		t.Error("原文编辑作为兜底必须保留")
	}
}

// TestTheSettingsModalUsesAntdFooterAndKeepsTheCloseIcon 固定参数设定窗口：
// 关闭按钮回到 antd 默认（右上角 X），底栏按状态给出「关闭」或「取消 / 保存」。
func TestTheSettingsModalUsesAntdFooterAndKeepsTheCloseIcon(t *testing.T) {
	modals := readFrontendSource(t, "src/components/Modals.tsx")

	start := strings.Index(modals, "export function SettingsModal")
	end := strings.Index(modals, "export function AboutModal")
	if start < 0 || end < start {
		t.Fatal("没找到 SettingsModal 与 AboutModal 的边界")
	}
	settings := modals[start:end]
	if strings.Contains(settings, "closable={false}") {
		t.Error("参数设定窗口应当保留 antd 默认的右上角关闭按钮")
	}
	for _, wanted := range []string{"关闭", "取消", "保存", "全部展开", "全部折叠", "编辑原文"} {
		if !strings.Contains(settings, wanted) {
			t.Errorf("参数设定窗口缺少底栏/工具按钮：%s", wanted)
		}
	}
}
