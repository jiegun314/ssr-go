package main

// 前端契约测试：窗口骨架与设计基底（产物内嵌、Wails 构建、模块卡片与描边、品牌色、无旧设计残留）。
//
// 公共前言与助手（源码读取、CSS 抽取、对比度）见 layout_helpers_test.go。
import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheFrontendShipsInsideTheBinary 固定「前端随二进制走」：dist 里有页面、打包后的
// 资源，以及三份静态文件（图标、附加窗口图片、音效）——它们由 public/ 复制而来。
func TestTheFrontendShipsInsideTheBinary(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("frontend", "dist"))
	if err != nil {
		t.Fatalf("读 frontend/dist 失败（先在 frontend/ 里跑 npm run build）：%v", err)
	}
	found := map[string]bool{}
	for _, entry := range entries {
		found[entry.Name()] = true
	}
	for _, wanted := range []string{"index.html", "logo.png", "puppy.png", "puppy-voice.mp3"} {
		if !found[wanted] {
			t.Errorf("frontend/dist 里缺少 %s（构建产物要提交进仓库）", wanted)
		}
	}
	assets, err := os.ReadDir(filepath.Join("frontend", "dist", "assets"))
	if err != nil {
		t.Fatalf("读 frontend/dist/assets 失败：%v", err)
	}
	hasJS, hasCSS := false, false
	for _, entry := range assets {
		if strings.HasSuffix(entry.Name(), ".js") {
			hasJS = true
		}
		if strings.HasSuffix(entry.Name(), ".css") {
			hasCSS = true
		}
	}
	if !hasJS || !hasCSS {
		t.Errorf("dist/assets 里应该有打包后的 js 与 css（js=%v css=%v）", hasJS, hasCSS)
	}
	// 源码里也要有那三份静态文件（public/ 是它们的家）
	for _, wanted := range []string{"logo.png", "puppy.png", "puppy-voice.mp3"} {
		if _, err := os.Stat(filepath.Join("frontend", "public", wanted)); err != nil {
			t.Errorf("frontend/public 里缺少 %s：%v", wanted, err)
		}
	}
}

// TestTheWailsBuildRunsTheFrontendBuild 固定构建链：wails build 之前先构建前端，
// 保证打进二进制的 dist 一定和源码一致。
func TestTheWailsBuildRunsTheFrontendBuild(t *testing.T) {
	raw, err := os.ReadFile("wails.json")
	if err != nil {
		t.Fatalf("读 wails.json 失败：%v", err)
	}
	if !strings.Contains(string(raw), "npm run build") {
		t.Error("wails.json 的 frontend:build 应当构建前端（避免 dist 与源码不一致）")
	}
}

// TestTheModulesUseAntdCards 固定四个模块的容器：用 antd 的 Card（size="small"），
// 标题排版 / 边框 / 圆角都由 antd 给，样式里不再手写卡片外观。
func TestTheModulesUseAntdCards(t *testing.T) {
	app := readFrontendSource(t, "src/App.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	for _, wanted := range []string{
		`className="card"`,
		`title="数据导入"`,
		`className="card grow consolidation-card"`,
		`className="consolidation-name"`,
		`className="card record-card"`,
		`title="记录导出"`,
	} {
		if !strings.Contains(app, wanted) {
			t.Errorf("App.tsx 的模块应当用 antd Card：找不到 %s", wanted)
		}
	}
	if !strings.Contains(app, `<Card`) || !strings.Contains(app, `from "antd"`) {
		t.Error("App.tsx 应当从 antd 引入 Card")
	}
	// 操作日志卡片复用同一个 Card
	if panels := readFrontendSource(t, "src/components/Panels.tsx"); !strings.Contains(panels, `<Card`) ||
		!strings.Contains(panels, `className="card log-card"`) {
		t.Error("操作日志卡片应当也是 antd Card（圆角与其它模块一致）")
	}
	card := cssRule(t, styles, ".card {")
	if !strings.Contains(card, "border-radius: 0;") {
		t.Errorf("功能模块应当是直角，实际块：\n%s", card)
	}
	if strings.Contains(card, "background") {
		t.Errorf(".card 不该自绘底色（交给 antd 的 Card），实际块：\n%s", card)
	}
}

// TestTheModuleHeadersAndBordersSeparateTheModules 固定"模块之间要有区隔"的做法：
//   - 除操作日志外的三个模块：品牌红底 + 白字的标题条；
//   - 操作日志：灰底 + 品牌红字的标题条（栏目就放在这一行里）；
//   - 每个模块一条淡色描边，模块之间只留 8px 间距 —— 区隔靠颜色和线条，不靠留白。
func TestTheModuleHeadersAndBordersSeparateTheModules(t *testing.T) {
	styles := readFrontendSource(t, "src/styles.css")
	tokens := readFrontendSource(t, "src/design-tokens.ts")
	brand := themeConstant(t, tokens, "export const BRAND_RED")

	header := cssBlock(t, styles, ".card:not(.log-card) > .ant-card-head {")
	if !strings.Contains(header, "background: "+brand+";") {
		t.Errorf("模块标题条应当是品牌红 %s，实际块：\n%s", brand, header)
	}
	title := cssBlock(t, styles, ".card:not(.log-card) > .ant-card-head .ant-card-head-title {")
	if !strings.Contains(title, "color: #fff;") {
		t.Errorf("模块标题条文字应当是白色，实际块：\n%s", title)
	}

	logHead := cssBlock(t, styles, ".log-card > .ant-card-head {")
	if !strings.Contains(logHead, "background: var(--log-head-bg);") {
		t.Errorf("操作日志的标题条应当是更深一档的灰底，实际块：\n%s", logHead)
	}
	logTitle := cssBlock(t, styles, ".log-card > .ant-card-head .ant-card-head-title {")
	if !strings.Contains(logTitle, "color: "+brand+";") {
		t.Errorf("操作日志标题应当是品牌红字，实际块：\n%s", logTitle)
	}

	card := cssRule(t, styles, ".card {")
	if !strings.Contains(card, "border: 1px solid var(--module-border);") {
		t.Errorf("模块应当有一条淡色描边，实际块：\n%s", card)
	}
	shell := cssBlock(t, styles, ".app-shell {")
	if !strings.Contains(shell, "padding: 12px;") {
		t.Errorf("窗口内边距应当收紧到 12px，实际块：\n%s", shell)
	}
	columns := cssRule(t, styles, ".columns {")
	if !strings.Contains(columns, "gap: 0;") {
		t.Errorf("左右两列之间不留 gap（间距由分隔按钮撑起），实际块：\n%s", columns)
	}
	// 日志住在左列里，与数据导入的间距由 .left 的 gap 给
	// 左右两列之间不再留 gap：整条分隔由收起按钮自己撑（见 TestTheLeftColumnCanBeCollapsedFromTheDivider）
	columnsBlock := cssRule(t, styles, ".columns {")
	if !strings.Contains(columnsBlock, "gap: 0;") {
		t.Errorf(".columns 不该再留 gap（间距由分隔按钮撑起），实际块：\n%s", columnsBlock)
	}
	left := cssRule(t, styles, ".left {")
	if !strings.Contains(left, "gap: 8px;") {
		t.Errorf("左列两张卡之间的间距应当是 8px，实际块：\n%s", left)
	}
}

// TestTheFunctionalModulesAreSquare 固定「功能模块一律直角」：
// 卡片（含红色标题条）、导入分组框、结果表容器与表头、导入状态框 全部没有圆角。
func TestTheFunctionalModulesAreSquare(t *testing.T) {
	styles := readFrontendSource(t, "src/styles.css")

	for _, selector := range []string{
		".card {",
		".card > .ant-card-head {",
		".group {",
		".tblwrap {",
		".actions .status {",
	} {
		block := cssRule(t, styles, selector)
		if strings.Contains(block, "border-radius:") && !strings.Contains(block, "border-radius: 0;") {
			t.Errorf("%s 应当是直角，实际块：\n%s", selector, block)
		}
	}
	// 表格自己的表头圆角也要抹平（antd 默认给表头首尾格 8px）
	corners := cssRule(t, styles, ".tblwrap .ant-table-container,")
	if !strings.Contains(corners, "border-start-start-radius: 0;") || !strings.Contains(corners, "border-start-end-radius: 0;") {
		t.Errorf("表头的圆角也要抹平，实际块：\n%s", corners)
	}
}

// TestTheUIKeepsTheProductColorsAndFont 固定主题口径：品牌红＝强生企业红（PANTONE 485 C），
// 且落到 antd 的 colorPrimary / colorInfo / colorLink 上；字体用跨平台栈（含中文回退），
// 不许再用等宽字体（Windows 上会退化成 Consolas + 微软雅黑混排）。
func TestTheUIKeepsTheProductColorsAndFont(t *testing.T) {
	theme := readFrontendSource(t, "src/theme.ts")
	styles := readFrontendSource(t, "src/styles.css")

	// 颜色 token 的真源在 design-tokens.ts（CSS 变量的一致性由前端测试校验）
	tokens := readFrontendSource(t, "src/design-tokens.ts")
	if !strings.Contains(tokens, `export const BRAND_RED = "#DA291C"`) {
		t.Error("品牌红应当是强生企业红 #DA291C")
	}
	if !strings.Contains(readFrontendSource(t, "src/theme.ts"), `from "./design-tokens"`) {
		t.Error("theme.ts 应当从 design-tokens.ts 取颜色，而不是自己再写一遍色值")
	}
	for _, tokenName := range []string{"colorPrimary", "colorInfo", "colorLink"} {
		if !strings.Contains(theme, tokenName+": BRAND_RED") {
			t.Errorf("theme.ts 的 %s 应当用 BRAND_RED", tokenName)
		}
	}
	if !strings.Contains(theme, `"Microsoft YaHei"`) || !strings.Contains(theme, `"PingFang SC"`) {
		t.Error("字体栈要跨平台（含微软雅黑/苹方）")
	}
	if strings.Contains(theme, "monospace") || strings.Contains(styles, "monospace") {
		t.Error("界面不该用等宽字体（Windows 上会退化成两种字体混排）")
	}
	// 结果表的四种状态在样式里都有自己的类
	for _, wanted := range []string{
		".result-row.status-Ready", ".result-row.status-Incomplete",
		".result-row.status-Duplicate", ".result-row.status-Conflict",
	} {
		if !strings.Contains(styles, wanted) {
			t.Errorf("结果表缺少状态着色：%s", wanted)
		}
	}
}

// TestNoPySideLeftoversInTheDesign 固定"不再残留 PySide6 原型的设计语言"：
// 手工 4px 圆角、红底白字标题条、13px 基准字号、36px 日期框、2px 表格内边距、
// 仿只读输入框的状态条、居中按钮的 QDialog 底栏——这些手工值一个都不许再出现。
func TestNoPySideLeftoversInTheDesign(t *testing.T) {
	text := frontendSourceText(t)

	for _, leftover := range []string{
		"border-radius: 4px",    // 统一小圆角：改回 antd 的 borderRadius
		"background: #d71600",   // 红底标题条
		"#b3261e",               // 旧主红
		"#d71600",               // 旧标题条红
		"font-size: 13px",       // 13px 基准字号（antd 默认 14px）
		"height: 36px",          // 36px 日期框 / 工具条按钮
		"padding: 2px 5px",      // 表格 2px 内边距
		`className="title-bar"`, // 手写标题条
		"status-strip .dot",     // 自绘状态圆点
		"border-bottom: 1px solid rgba(0, 0, 0, 0.24)",        // 仿只读输入框的底部描边
		"justify-content: center;\n  padding: 12px 24px 16px", // QDialog 式居中底栏
	} {
		if strings.Contains(text, leftover) {
			t.Errorf("前端源码里还残留旧设计：%q", leftover)
		}
	}
}
