package app

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
// 标题排版与内边距交给 antd；圆角与阴影由设计 token 给（分级圆角 + 表面分层）。
func TestTheModulesUseAntdCards(t *testing.T) {
	app := readFrontendSource(t, "src/App.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	for _, wanted := range []string{
		`className="card"`,
		`title={alignMode ? "数据来源" : "数据导入"}`,
		`card grow consolidation-card`,
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
	if !strings.Contains(card, "border-radius: var(--module-radius);") {
		t.Errorf("功能模块应当用外层圆角 token（6px），实际块：\n%s", card)
	}
	if !strings.Contains(card, "box-shadow: var(--card-shadow);") {
		t.Errorf("模块应当靠浅阴影做表面分层，实际块：\n%s", card)
	}
	if strings.Contains(card, "background") {
		t.Errorf(".card 不该自绘底色（交给 antd 的 Card），实际块：\n%s", card)
	}
}

// TestTheModuleHeadersAndBordersSeparateTheModules 固定"模块之间要有区隔"的做法：
//   - 所有标题条都是白底 + 左侧 3px 品牌红竖条 + 深色标题；红色从"整条填充"退到"线"；
//   - 数据整合作为主决策区，额外保留一档品牌红 4% 淡底；
//   - 操作日志仍是灰底标题条（栏目在这一行里），但标题字与其他模块一致；
//   - 模块外框是极淡描边 + 浅阴影，窗口底色再深一档 —— 区隔靠表面分层，不靠深灰线。
func TestTheModuleHeadersAndBordersSeparateTheModules(t *testing.T) {
	styles := readFrontendSource(t, "src/styles.css")
	tokens := readFrontendSource(t, "src/design-tokens.ts")
	if !strings.Contains(tokens, `export const BRAND_RED = "#DA291C"`) {
		t.Error("竖条与主按钮的品牌红仍应当来自 design-tokens.ts")
	}

	header := cssBlock(t, styles, ".card > .ant-card-head {")
	for _, wanted := range []string{
		"background: var(--brand-red-soft);",
		"border-bottom: 1px solid var(--brand-red-border);",
	} {
		if !strings.Contains(header, wanted) {
			t.Errorf("模块标题条应当是浅红标题带（缺 %s），实际块：\n%s", wanted, header)
		}
	}
	bar := cssBlock(t, styles, ".card > .ant-card-head::before {")
	for _, wanted := range []string{"width: 3px;", "background: var(--brand-red);"} {
		if !strings.Contains(bar, wanted) {
			t.Errorf("标题条左侧品牌红竖条缺少 %s，实际块：\n%s", wanted, bar)
		}
	}
	title := cssBlock(t, styles, ".card:not(.log-card) > .ant-card-head .ant-card-head-title {")
	if !strings.Contains(title, "color: var(--brand-red-deep);") {
		t.Errorf("模块标题应当是深红字，实际块：\n%s", title)
	}
	accent := cssBlock(t, styles, ".consolidation-card > .ant-card-head {")
	for _, wanted := range []string{
		"background: var(--brand-red);",
		"border-bottom: 1px solid var(--brand-red-deep);",
	} {
		if !strings.Contains(accent, wanted) {
			t.Errorf("数据整合标题条应当是实底品牌红（缺 %s），实际块：\n%s", wanted, accent)
		}
	}
	if strings.Contains(accent, "color-mix") {
		t.Errorf("数据整合标题条不再用 4%% 淡底，实际块：\n%s", accent)
	}

	logHead := cssBlock(t, styles, ".log-card > .ant-card-head {")
	if !strings.Contains(logHead, "border-bottom-color: var(--brand-red-border);") {
		t.Errorf("操作日志标题条应当用浅红收边，实际块：\n%s", logHead)
	}
	logTitle := cssBlock(t, styles, ".log-card > .ant-card-head .ant-card-head-title {")
	if !strings.Contains(logTitle, "color: var(--brand-red-deep);") {
		t.Errorf("操作日志标题应当与其他模块同为深红，实际块：\n%s", logTitle)
	}

	card := cssRule(t, styles, ".card {")
	if !strings.Contains(card, "border: 1px solid var(--card-border);") {
		t.Errorf("模块外框应当用极淡描边，实际块：\n%s", card)
	}
	body := cssBlock(t, styles, "body {")
	if !strings.Contains(body, "background: var(--app-bg);") {
		t.Errorf("窗口底色应当比白卡深一档，实际块：\n%s", body)
	}
	shell := cssBlock(t, styles, ".app-shell {")
	if !strings.Contains(shell, "padding: 12px;") {
		t.Errorf("窗口内边距应当收紧到 12px，实际块：\n%s", shell)
	}
	columns := cssRule(t, styles, ".columns {")
	if !strings.Contains(columns, "gap: 0;") {
		t.Errorf("左右两列之间不留 gap（间距由分隔按钮撑起），实际块：\n%s", columns)
	}
	left := cssRule(t, styles, ".left {")
	if !strings.Contains(left, "gap: 8px;") {
		t.Errorf("左列两张卡之间的间距应当是 8px，实际块：\n%s", left)
	}
}

// TestTheRadiusHierarchy 固定「分级圆角」：外层模块 6px、内层容器 4px，
// 标题条跟着外层走，表头跟着结果表容器走；控制类细条（分隔按钮）仍是直角。
func TestTheRadiusHierarchy(t *testing.T) {
	styles := readFrontendSource(t, "src/styles.css")

	if card := cssRule(t, styles, ".card {"); !strings.Contains(card, "border-radius: var(--module-radius);") {
		t.Errorf("外层模块应当用 --module-radius，实际块：\n%s", card)
	}
	if head := cssRule(t, styles, ".card > .ant-card-head {"); !strings.Contains(head, "border-radius: var(--module-radius) var(--module-radius) 0 0;") {
		t.Errorf("标题条应当只圆上方两角，实际块：\n%s", head)
	}
	for _, selector := range []string{".tblwrap {", ".align-source-card {"} {
		block := cssRule(t, styles, selector)
		if !strings.Contains(block, "border-radius: var(--inner-radius);") {
			t.Errorf("%s 应当用内层圆角 4px，实际块：\n%s", selector, block)
		}
	}
	// 参数设定的顶层配置块是模块卡；嵌套层只留引导线，不再描边、不再做圆角。
	if block := cssRule(t, styles, `.settings-block[data-depth="0"] {`); !strings.Contains(block, "border-radius: var(--module-radius);") {
		t.Errorf("参数设定顶层配置块应当用模块圆角 6px，实际块：\n%s", block)
	}
	if block := cssRule(t, styles, ".settings-block {"); !strings.Contains(block, "border-radius: 0;") {
		t.Errorf("参数设定嵌套块应当是直角无卡片，实际块：\n%s", block)
	}
	// 表头首尾格的圆角跟着内层容器走（antd 默认 8px，不抹平会露出尖角）
	corners := cssRule(t, styles, ".tblwrap .ant-table-container,")
	if !strings.Contains(corners, "border-start-start-radius: var(--inner-radius);") ||
		!strings.Contains(corners, "border-start-end-radius: var(--inner-radius);") {
		t.Errorf("表头圆角应当跟着结果表容器，实际块：\n%s", corners)
	}
	// 按钮是窄胶囊，并且左右留出空隙；不能再贴住或压住两侧面板
	toggle := cssRule(t, styles, ".left-toggle.ant-btn {")
	if !strings.Contains(toggle, "border-radius: 7px;") {
		t.Errorf("分隔按钮应当使用窄胶囊圆角，实际块：\n%s", toggle)
	}
	if !strings.Contains(toggle, "margin-inline: 4px;") {
		t.Errorf("分隔按钮左右应当留出呼吸空间，实际块：\n%s", toggle)
	}
	if strings.Contains(toggle, "margin-inline: -") {
		t.Errorf("分隔按钮不该用负外边距压住两侧面板，实际块：\n%s", toggle)
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
// 写死的 4px 圆角字面量（现在只走 var(--inner-radius)）、旧红底白字标题条、13px 基准字号、
// 36px 日期框、2px 表格内边距、仿只读输入框的状态条、居中按钮的 QDialog 底栏——
// 这些手工值一个都不许再出现。
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

// TestTheWindowNeverScrollsAsAWhole 固定「整窗永不出现文档级滚动条」：
//
// antd 的下拉与弹层挂在 body 下（position: absolute），会把文档的可滚动高度撑出去 ——
// 典型现象是"在结果表里选每页行数时，右侧冒出一条多余的垂直滚动条"。
// 窗口外壳本来就设计成"刚好一屏 + 各面板内部各自滚动"（.left / .tblwrap / 日志正文
// 都有自己的 overflow），所以根节点链上必须 overflow: hidden。
func TestTheWindowNeverScrollsAsAWhole(t *testing.T) {
	styles := readFrontendSource(t, "src/styles.css")

	root := cssBlock(t, styles, "#root > .ant-app {")
	if !strings.Contains(root, "height: 100%;") {
		t.Errorf("根节点应当撑满窗口高度，实际块：\n%s", root)
	}
	if !strings.Contains(root, "overflow: hidden;") {
		t.Errorf("根节点必须 overflow: hidden（否则 antd 弹层会把文档撑出滚动条），实际块：\n%s", root)
	}
}

// TestMotionIsShortAndRespectsReducedMotion 固定动效口径：只做 120–180ms 的反馈，
// 系统要求减少动态时立即结束，不让界面在低性能机器上抖动。
func TestMotionIsShortAndRespectsReducedMotion(t *testing.T) {
	styles := readFrontendSource(t, "src/styles.css")
	for _, want := range []string{
		"--motion-fast: 120ms;",
		"--motion-base: 160ms;",
		"--motion-slow: 180ms;",
		"transition: background-color var(--motion-fast) ease;",
		"transition: background-color var(--motion-base) ease",
		"@media (prefers-reduced-motion: reduce)",
		"transition-duration: 0.01ms !important;",
		"animation-duration: 0.01ms !important;",
	} {
		if !strings.Contains(styles, want) {
			t.Errorf("动效口径缺少：%s", want)
		}
	}
}
