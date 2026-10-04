package main

import (
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// 前端是 React + antd + Lucide（源码在 frontend/src，构建产物在 frontend/dist）。
// 这里的契约测试读**源码**而不是打包产物：产物里的类名会被压缩，源码才是可维护的契约面。
// 少数几条（内嵌资源、提示字样）同时检查产物，因为那才是真正发给用户的东西。
//
// 两套口径：
//   - 行为契约（日志拖动、声音、弹窗尺寸、图标语义、导出门禁……）—— 与改版前一致；
//   - 设计契约 —— 现在是 **antd 默认值 + 强生红**：字号 / 圆角 / 间距 / 控件高度都不许再出现
//     PySide6 原型带过来的手工值（4px 小圆角、红底白字标题条、13px 基准字号、36px 日期框……）。

func readFrontendSource(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("frontend", name))
	if err != nil {
		t.Fatalf("读前端源码 %s 失败：%v", name, err)
	}
	return string(raw)
}

// frontendSourceText 把前端源码全部拼起来，用于"整个前端都不许出现某字样"这类检查。
func frontendSourceText(t *testing.T) string {
	t.Helper()
	var builder strings.Builder
	walk := func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		builder.Write(content)
		builder.WriteString("\n")
		return nil
	}
	if err := filepath.WalkDir(filepath.Join("frontend", "src"), walk); err != nil {
		t.Fatalf("遍历前端源码失败：%v", err)
	}
	for _, name := range []string{"index.html", "package.json", "vite.config.ts"} {
		if content, err := os.ReadFile(filepath.Join("frontend", name)); err == nil {
			builder.Write(content)
		}
	}
	return builder.String()
}

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

// TestTheHiddenWindowIsNotAdvertisedInTheFrontend 固定「不提示」的口径：
// 前端源码与界面文件里都不能出现说明附加窗口的文字或命名（它们会随界面打进程序）。
func TestTheHiddenWindowIsNotAdvertisedInTheFrontend(t *testing.T) {
	text := frontendSourceText(t)
	for _, hint := range []string{"彩蛋", "连点", "easter", "EASTER", "egg", "Egg", "EGG"} {
		if strings.Contains(text, hint) {
			t.Errorf("前端源码里出现了会提示附加窗口的文字：%q", hint)
		}
	}
	// 构建产物同样不许出现（前端资源是内嵌进二进制的）
	raw, err := os.ReadFile(filepath.Join("frontend", "dist", "index.html"))
	if err != nil {
		t.Fatalf("读构建产物失败：%v", err)
	}
	for _, hint := range []string{"彩蛋", "连点", "easter_egg"} {
		if strings.Contains(string(raw), hint) {
			t.Errorf("构建产物里出现了提示字样：%q", hint)
		}
	}
}

// TestTheHiddenWindowSoundShipsInsideTheApp 固定附加窗口声音的落地方式：
// 音频放在 frontend/public（构建时复制进 dist，再由 //go:embed 编进二进制），
// 不能放在会被整目录拷进发布包的 resource/；窗口打开时循环播放、关闭时立刻停。
func TestTheHiddenWindowSoundShipsInsideTheApp(t *testing.T) {
	app := readFrontendSource(t, "src/App.tsx")

	if stray, _ := filepath.Glob(filepath.Join("resource", "*.mp3")); len(stray) > 0 {
		t.Errorf("音频不要放在会被整目录拷进发布包的 resource/：%v", stray)
	}
	if !strings.Contains(app, `src="puppy-voice.mp3"`) || !strings.Contains(app, "loop") {
		t.Error("App.tsx 里缺少循环播放的音频元素")
	}
	for _, wanted := range []string{
		"voice.play()",  // 打开时播放（在点击回调里，属用户手势）
		"voice.pause()", // 关闭时立刻停
		"voice.currentTime = 0",
	} {
		if !strings.Contains(app, wanted) {
			t.Errorf("App.tsx 里缺少声音控制：%s", wanted)
		}
	}
}

// TestTheLogPanelHasNoDragHandle 固定「左列两个模块位置固定、高度不可手动调」：
// 数据导入按内容高度不动，窗口高度的变化全部由操作日志这张卡吸收（flex-basis 0），
// 所以两卡之间没有拖动条，也不该再留下拖动逻辑或 row-resize 光标。
func TestTheLogPanelHasNoDragHandle(t *testing.T) {
	panels := readFrontendSource(t, "src/components/Panels.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	for _, leftover := range []string{
		"log-resizer",
		"log-resizing",
		"role=\"separator\"",
		"aria-orientation",
		"otherContentHeight",
	} {
		if strings.Contains(panels, leftover) || strings.Contains(styles, leftover) {
			t.Errorf("拖动条已经去掉，不该再有：%s", leftover)
		}
	}
	if strings.Contains(styles, "row-resize") {
		t.Error("不该再有调整高度的光标")
	}
}

// TestTheLogWindowIsFilterableAndTwoColumn 固定操作日志窗口的形式：
// 级别栏目（全部 / 信息 / 成功 / 警告 / 错误）与「新日志置顶」放在标题条里；
// 内容是一张两列表格：左列只显示时间段（窄栏省宽度），右列是级别色字 + 正文；
// 正文最多两行、超出省略，完整内容在 title 里悬浮可见。
func TestTheLogWindowIsFilterableAndTwoColumn(t *testing.T) {
	panels := readFrontendSource(t, "src/components/Panels.tsx")
	styles := readFrontendSource(t, "src/styles.css")
	script := readFrontendSource(t, "src/log.ts")

	for _, wanted := range []string{
		"parseLog(",  // 按行契约把日志文本拆成条目
		"shortTime(", // 时间段只留 HH:MM:SS
		"<Segmented", // 级别栏目
		`className="log-filter log-filter-all"`,
		"LOG_LEVELS.map", // 信息 / 成功 / 警告 / 错误
		"新日志置顶",          // 排序开关（在标题栏里，靠右）
		"<Switch",
		`className="log-sort"`, // 它挂在标题栏的 extra 上
		"extra={",
		`className="log-toolbar"`, // 筛选单独一行
		"{entries.length} 条",      // 条数在标题里
		"log-filter-${level}",     // 栏目文字按级别上色
		"showHeader={false}",      // 两列表格，不再单画一行表头
		`title: "时间",`,
		`title: "信息",`,
		"width: 64",                // 时间列只要装下 HH:MM:SS（12px 等宽数字约 54px）
		"log-level-${entry.level}", // 级别用两个字 + 级别色
		"log-text",                 // 正文容器（两行截断）
		"暂无日志",
	} {
		if !strings.Contains(panels, wanted) {
			t.Errorf("LogPanel 里缺少：%s", wanted)
		}
	}

	for _, wanted := range []string{
		".log-list {",
		".log-card > .ant-card-head {", // 灰底 + 红字的标题条
		".log-card > .ant-card-head .ant-card-head-title {",
		"background: #e8e8e8;",                        // 标题条用更深一档的灰
		".log-toolbar .ant-segmented-item-label {",    // 一行里塞下 5 个栏目，内边距收紧
		"font-size: 12px;",                            // 栏目文字比正文小一号
		".log-filter-info {", ".log-filter-success {", // 栏目颜色与日志里级别色一致
		".log-filter-warning {", ".log-filter-error {",
		".log-toolbar .ant-segmented-item-selected .log-filter {", // 选中时红底白字
		".log-sort .ant-typography {",                             // 「新日志置顶」小一号
		".log-card > .ant-card-head .ant-card-head-wrapper {",
		".log-table .log-time {",
		"text-align: left;",                   // 时间靠左
		"white-space: nowrap;",                // 时间列不换行
		"font-variant-numeric: tabular-nums;", // 数字等宽，时间才对得齐
		".log-level {",                        // 级别标签：32px 槽位内横向居中（比原来的 44px 窄）
		"width: 32px;",
		".log-card .log-table .ant-table.ant-table-small .ant-table-tbody > tr > td.ant-table-cell {",
		"justify-content: center;",
		".log-level-info {", ".log-level-success {",
		".log-level-warning {", ".log-level-error {",
		".log-text {",
		"-webkit-line-clamp: 2;",   // 正文最多两行
		"overflow-wrap: anywhere;", // 长路径 / 长报错要换行
		"min-height: 72px;",        // 日志卡的下限
	} {
		if !strings.Contains(styles, wanted) {
			t.Errorf("日志窗口样式里缺少：%s", wanted)
		}
	}
	// 日志卡不再是底部横带：不许再有按内容撑开的 max-height
	if strings.Contains(styles, "max-height: 240px;") {
		t.Error("日志卡已经改成吃左列剩余高度，不该再有 240px 的固定封顶")
	}
	// 字体族不覆盖：日志跟界面其它地方一样用 antd 主题里的字体栈
	logCell := cssBlock(t, styles, ".log-table .ant-table-cell {")
	if !strings.Contains(logCell, "font-size: 12px;") {
		t.Errorf("日志字号应当比正文小一号（12px），实际块：\n%s", logCell)
	}
	if strings.Contains(logCell, "font-family") {
		t.Error("日志不该覆盖字体族（要用 antd 主题里的那套）")
	}
	if !strings.Contains(script, "export function shortTime(") {
		t.Error("log.ts 里应当有 shortTime（只取时段的辅助函数）")
	}
	// 浅色分割线用 antd Table 自带的行分隔线，不许在样式里抹掉
	if strings.Contains(styles, "border-bottom: none") {
		t.Error("不该把日志表格的分割线抹掉")
	}
}

// TestTheLeftColumnHoldsImportAndLog 固定左列的内部分工（方案 A）：
//
//	左列 = 数据导入（按内容高度）+ 操作日志（flex-basis 0，吃掉剩余高度），日志上沿可拖动；
//	flex-basis 必须是 0：否则日志正文的固有高度会把左列（乃至整行）撑得比窗口还高。
//	右列 = 数据整合（吃剩余高度）+ 记录导出（贴底，标题与内容同一行）。
func TestTheLeftColumnHoldsImportAndLog(t *testing.T) {
	panels := readFrontendSource(t, "src/components/Panels.tsx")
	styles := readFrontendSource(t, "src/styles.css")
	app := readFrontendSource(t, "src/App.tsx")

	left := cssRule(t, styles, ".left {")
	if !strings.Contains(left, "gap: 8px;") {
		t.Errorf("左列两张卡之间的间隔应当是 8px，实际块：\n%s", left)
	}
	logCard := cssRule(t, styles, ".left > .log-card {")
	if !strings.Contains(logCard, "flex: 1 1 0;") {
		t.Errorf("操作日志卡必须 flex-basis 0（吃剩余高度，不许按内容撑开），实际块：\n%s", logCard)
	}
	importCard := cssRule(t, styles, ".left > .card {")
	if !strings.Contains(importCard, "flex: 0 0 auto;") {
		t.Errorf("数据导入卡应当保持内容高度，实际块：\n%s", importCard)
	}

	// DOM：数据导入 → 操作日志（都在左列），然后才是分隔按钮与右列
	importIndex := strings.Index(app, `title="数据导入"`)
	logIndex := strings.Index(app, "<LogPanel text={log} />")
	toggleIndex := strings.Index(app, "left-toggle")
	tableIndex := strings.Index(app, `className="card grow consolidation-card"`)
	recordIndex := strings.Index(app, `className="card record-card"`)
	if !(importIndex < recordIndex && recordIndex < logIndex && logIndex < toggleIndex && toggleIndex < tableIndex) {
		t.Errorf("App.tsx 顺序应当是 数据导入 → 记录导出 → 操作日志 → 分隔按钮 → 数据整合，实际 %d/%d/%d/%d/%d",
			importIndex, recordIndex, logIndex, toggleIndex, tableIndex)
	}

	// 高度由 flex 决定，没有手动拖动：数据导入不动、窗口变化全给日志
	if strings.Contains(panels, "setPinned") {
		t.Error("高度不可手动调整，不该再有 pinned 状态")
	}
}

// TestTheLeftColumnCanBeCollapsedFromTheDivider 固定竖向分隔上的收起/展开按钮：
// 点一下收起整个左列（数据导入 + 操作日志），右侧结果表吃满整个窗口宽度；再点一下展开。
func TestTheLeftColumnCanBeCollapsedFromTheDivider(t *testing.T) {
	app := readFrontendSource(t, "src/App.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	for _, wanted := range []string{
		"const [leftCollapsed, setLeftCollapsed] = useState(false);",
		"left-collapsed",          // 收起状态挂在 .columns 上
		`className="left-toggle"`, // 分隔上的按钮
		"aria-expanded={!leftCollapsed}",
		"展开左侧面板", "收起左侧面板", // 提示与无障碍名称
		"<ChevronLeft size={12} />",  // 展开状态：箭头指向左侧（点它收起）
		"<ChevronRight size={12} />", // 收起状态：箭头指向右侧（点它展开）
	} {
		if !strings.Contains(app, wanted) {
			t.Errorf("收起/展开按钮缺少：%s", wanted)
		}
	}

	toggle := cssRule(t, styles, ".left-toggle.ant-btn {")
	for _, wanted := range []string{
		"flex: 0 0 auto;",
		"align-self: center;",
		"width: 12px;",        // 按钮本身尽量窄
		"margin-inline: 2px;", // 整条分隔只占 16px
		"border-radius: 0;",   // 与模块一样直角
	} {
		if !strings.Contains(toggle, wanted) {
			t.Errorf("分隔按钮缺少 %s，实际块：\n%s", wanted, toggle)
		}
	}
	collapsed := cssRule(t, styles, ".columns.left-collapsed > .left {")
	if !strings.Contains(collapsed, "display: none;") {
		t.Errorf("收起时应当把整个左列藏起来（结果表吃满宽度），实际块：\n%s", collapsed)
	}
}

// TestTheLogExportRowSurvivesWiderPlatformFonts 固定「记录导出」在右列顶上的横条布局：
// 两个日期并排、中间一个箭头、回顾按钮靠右；日期框固定宽度且可压缩，
// 任何窗口宽度下都不会换行或冒出横向滚动条。
func TestTheLogExportRowSurvivesWiderPlatformFonts(t *testing.T) {
	panels := readFrontendSource(t, "src/components/Panels.tsx")
	styles := readFrontendSource(t, "src/styles.css")
	app := readFrontendSource(t, "src/App.tsx")

	rangeBlock := cssRule(t, styles, ".range {")
	for _, wanted := range []string{
		"display: flex;",
		"align-items: center;",
		"justify-content: center;",
	} {
		if !strings.Contains(rangeBlock, wanted) {
			t.Errorf("记录导出应当是居中摆放的一行，缺少 %s，实际块：\n%s", wanted, rangeBlock)
		}
	}
	// 模块本身：左列里的普通卡（红标题栏 + 正文），不再有红块/白条那套
	if strings.Contains(styles, ".record-row") || strings.Contains(styles, ".record-title") || strings.Contains(styles, ".record-content") {
		t.Error("记录导出已经是普通模块，不该再有红块/白条（record-row / record-title / record-content）")
	}
	recordCard := cssRule(t, styles, ".record-card {")
	if !strings.Contains(recordCard, "flex: 0 0 auto;") {
		t.Errorf("记录导出应当按内容高度排在左列中间，实际块：\n%s", recordCard)
	}
	recordBody := cssRule(t, styles, ".record-card > .ant-card-body {")
	if !strings.Contains(recordBody, "padding: 8px 12px;") {
		t.Errorf("记录导出正文上下留白应当是 8px（模块 86px），实际块：\n%s", recordBody)
	}
	if strings.Contains(styles, ".range .spacer") {
		t.Error("日期与按钮整组居中，不需要把按钮推到右边的 spacer")
	}
	picker := cssBlock(t, styles, ".range .ant-picker {")
	if !strings.Contains(picker, "flex: 0 1 122px;") || !strings.Contains(picker, "min-width: 110px;") {
		t.Errorf("日期框应当按窄栏收窄到 122px 且可压缩到 110px，实际块：\n%s", picker)
	}
	for _, wanted := range []string{
		".range .date-sep {",
		".range .ant-btn {",
	} {
		if !strings.Contains(styles, wanted) {
			t.Errorf("记录导出样式缺少：%s", wanted)
		}
	}
	for _, wanted := range []string{`className="date-sep"`, `aria-label="回顾"`, `title="回顾"`} {
		if !strings.Contains(panels, wanted) && !strings.Contains(app, wanted) {
			t.Errorf("记录导出结构缺少：%s", wanted)
		}
	}
	if strings.Contains(styles, ".range .date-row") {
		t.Error("日期已经改成并排，不该再有上下两行的 date-row")
	}
	// 它住在左列中间：与其余模块同构（antd Card + 红标题栏），标题文字格式自然一致
	if !strings.Contains(app, `className="card record-card"`) || !strings.Contains(app, `title="记录导出"`) {
		t.Error("记录导出应当是左列里的普通模块（class=card record-card + title）")
	}
	// 回顾是纯图标按钮（窄栏里带文字一行放不下），语义靠 aria-label + Tooltip
	reviewStart := strings.Index(panels, `aria-label="回顾"`)
	reviewButton := panels[strings.LastIndex(panels[:reviewStart], "<Button"):]
	reviewButton = reviewButton[:strings.Index(reviewButton, "/>")]
	if strings.Contains(reviewButton, "回顾<") || strings.Contains(reviewButton, ">回顾") {
		t.Error("「回顾」在窄栏里应当是纯图标按钮，不该带文字")
	}

}

// TestTheUIKeepsTheProductColorsAndFont 固定主题口径：品牌红＝强生企业红（PANTONE 485 C），
// 且落到 antd 的 colorPrimary / colorInfo / colorLink 上；字体用跨平台栈（含中文回退），
// 不许再用等宽字体（Windows 上会退化成 Consolas + 微软雅黑混排）。
func TestTheUIKeepsTheProductColorsAndFont(t *testing.T) {
	theme := readFrontendSource(t, "src/theme.ts")
	styles := readFrontendSource(t, "src/styles.css")

	if !strings.Contains(theme, `export const BRAND_RED = "#DA291C"`) {
		t.Error("品牌红应当是强生企业红 #DA291C")
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
	theme := readFrontendSource(t, "src/theme.ts")
	brand := themeConstant(t, theme, "export const BRAND_RED")

	header := cssBlock(t, styles, ".card:not(.log-card) > .ant-card-head {")
	if !strings.Contains(header, "background: "+brand+";") {
		t.Errorf("模块标题条应当是品牌红 %s，实际块：\n%s", brand, header)
	}
	title := cssBlock(t, styles, ".card:not(.log-card) > .ant-card-head .ant-card-head-title {")
	if !strings.Contains(title, "color: #fff;") {
		t.Errorf("模块标题条文字应当是白色，实际块：\n%s", title)
	}

	logHead := cssBlock(t, styles, ".log-card > .ant-card-head {")
	if !strings.Contains(logHead, "background: #e8e8e8;") {
		t.Errorf("操作日志的标题条应当是更深一档的灰底，实际块：\n%s", logHead)
	}
	logTitle := cssBlock(t, styles, ".log-card > .ant-card-head .ant-card-head-title {")
	if !strings.Contains(logTitle, "color: "+brand+";") {
		t.Errorf("操作日志标题应当是品牌红字，实际块：\n%s", logTitle)
	}

	card := cssRule(t, styles, ".card {")
	if !strings.Contains(card, "border: 1px solid #d9d9d9;") {
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

// themeConstant 读 theme.ts 里 `export const NAME = "#RRGGBB"` 这种简单常量（返回小写）。
func themeConstant(t *testing.T, theme, declaration string) string {
	t.Helper()
	pattern := regexp.MustCompile(regexp.QuoteMeta(declaration) + `\s*=\s*"(#[0-9A-Fa-f]{6})"`)
	match := pattern.FindStringSubmatch(theme)
	if match == nil {
		t.Fatalf("没有从 theme.ts 里解析出 %s", declaration)
	}
	return strings.ToLower(match[1])
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

// TestTheResultTableUsesAntdDefaults 固定结果表的密度口径：
// 不加 bordered、不覆盖结果表的单元格内边距、不做圆角对齐 hack；
// .tblwrap 只加一条淡色描边做区隔（用户要求），投影与底色仍然交给 antd。
func TestTheResultTableUsesAntdDefaults(t *testing.T) {
	panels := readFrontendSource(t, "src/components/Panels.tsx")
	modals := readFrontendSource(t, "src/components/Modals.tsx")
	styles := readFrontendSource(t, "src/styles.css")
	theme := readFrontendSource(t, "src/theme.ts")

	for name, source := range map[string]string{"Panels.tsx": panels, "Modals.tsx": modals} {
		if strings.Contains(source, "bordered") {
			t.Errorf("%s 的结果表不该用 bordered（antd Table 默认无边框）", name)
		}
		if !strings.Contains(source, `size="small"`) {
			t.Errorf("%s 的结果表应当用 antd 的小尺寸密度", name)
		}
	}
	if strings.Contains(styles, ".tblwrap .ant-table-cell") {
		t.Error("不该再覆盖结果表的单元格内边距")
	}
	if strings.Contains(styles, "border-start-start-radius: 4px") || strings.Contains(styles, "border-start-start-radius: 8px") {
		t.Error("结果表的表头不该再有圆角（功能模块一律直角）")
	}
	wrap := cssBlock(t, styles, ".tblwrap {")
	if !strings.Contains(wrap, "border: 1px solid #f0f0f0;") {
		t.Errorf(".tblwrap 应当有一条淡色描边，实际块：\n%s", wrap)
	}
	if strings.Contains(wrap, "border-radius") {
		t.Errorf(".tblwrap 应当是直角，实际块：\n%s", wrap)
	}
	for _, selfDrawn := range []string{"box-shadow", "background"} {
		if strings.Contains(wrap, selfDrawn) {
			t.Errorf(".tblwrap 不该自绘 %s，实际块：\n%s", selfDrawn, wrap)
		}
	}
	if strings.Contains(theme, "Table:") {
		t.Error("theme.ts 不该再给 Table 写组件级覆盖（用 antd 默认）")
	}
}

// TestTheDisplayDialogsKeepTheirFixedSize 固定两个展示型窗口（关于 / 附加窗口）：
// 都用 antd Modal（默认自带的右上角 X、遮罩可关）、尺寸固定 323×323。
func TestTheDisplayDialogsKeepTheirFixedSize(t *testing.T) {
	modals := readFrontendSource(t, "src/components/Modals.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	for _, wanted := range []string{
		`className="about-window"`,  // 关于窗口
		`className="hidden-window"`, // 附加窗口
		"maskClosable",              // 点遮罩也能关
		"width={323}",
	} {
		if !strings.Contains(modals, wanted) {
			t.Errorf("Modals.tsx 里缺少展示型窗口的契约：%s", wanted)
		}
	}
	// 关闭按钮用 antd 自带的，不再自绘
	if strings.Contains(modals, "dialog-close") {
		t.Error("展示型窗口应当用 antd 自带的关闭按钮，不再自绘 dialog-close")
	}
	for _, wanted := range []string{
		".about-window {",
		".hidden-window {",
		"width: 323px !important;",
		"height: 323px;",
		"object-fit: cover", // 附加窗口的图片要铺满
		".about-window .ant-modal-close,",
		".hidden-window .ant-modal-close",
	} {
		if !strings.Contains(styles, wanted) {
			t.Errorf("样式里缺少展示型窗口的规则：%s", wanted)
		}
	}
	// 打开窗口时不要把焦点丢给右上角的 X（那样一开窗就像"被选中"）
	if strings.Contains(modals, "autoFocus") {
		t.Error("展示型窗口的关闭按钮不应自动聚焦")
	}
}

// TestTheAboutWindowKeepsTheOriginalIntro 固定关于窗口那段说明文字：
//   - 文案与原版 319×323 的 .ui 逐字一致，& 号就是 & 本身
//     （写成 HTML 实体 &amp; 会原样显示成 "&amp;"，这是之前漏掉的转义残留）；
//   - 原版那个 label 没写 alignment（Qt 默认左对齐），所以说明段左对齐，
//     图标 / 名称 / 版本仍然居中。
func TestTheAboutWindowKeepsTheOriginalIntro(t *testing.T) {
	modals := readFrontendSource(t, "src/components/Modals.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	if strings.Contains(modals, "&amp;") {
		t.Error("前端源码里不该残留 HTML 实体 &amp;（会被原样显示出来）")
	}
	for _, wanted := range []string{
		"SS Ready provides data validation, cleansing,",
		"Developed by",
		"Greater China Supply Chain & RA Team",
		"© 2026 JJMT",
	} {
		if !strings.Contains(modals, wanted) {
			t.Errorf("关于窗口的说明文字缺少原版内容：%q", wanted)
		}
	}
	intro := cssBlock(t, styles, ".about-body .intro {")
	if !strings.Contains(intro, "white-space: pre-wrap;") {
		t.Error("说明段要保留原版的换行")
	}
	if !strings.Contains(intro, "text-align: left;") {
		t.Errorf("说明段应当左对齐，实际样式块：\n%s", intro)
	}
}

// TestTheImportPanelKeepsTheSourceOrderAndIcons 固定导入区：四个来源的顺序（医保代码信息
// 在最上、与下面三组之间留空行）、以及三个图标按钮的可读名称。
func TestTheImportPanelKeepsTheSourceOrderAndIcons(t *testing.T) {
	bridge := readFrontendSource(t, "src/bridge.ts")
	panels := readFrontendSource(t, "src/components/Panels.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	order := []string{"medical_insurance_code", "product_category", "global_udi_input", "ra_input"}
	position := -1
	for _, key := range order {
		index := strings.Index(bridge, `key: "`+key+`"`)
		if index < 0 {
			t.Fatalf("SOURCES 里缺少来源 %s", key)
		}
		if index < position {
			t.Errorf("来源顺序不对：%s 应该排在前面", key)
		}
		position = index
	}
	// 四个来源分组的间隔完全一致：不再有"第一个分组之后多留一块"的 horizontalSpacer
	if strings.Contains(bridge, "GROUP_GAP_AFTER") || strings.Contains(panels, "group-gap") || strings.Contains(styles, ".group-gap") {
		t.Error("第一个分组之后的额外空行已经去掉，不该再有 GROUP_GAP_AFTER / group-gap")
	}
	group := cssRule(t, styles, ".group {")
	if !strings.Contains(group, "margin-bottom: 6px;") {
		t.Errorf("四个分组的间隔应当统一（margin-bottom: 6px），实际块：\n%s", group)
	}
	for _, wanted := range []string{`aria-label="载入文件"`, `aria-label="数据回顾"`} {
		if !strings.Contains(panels, wanted) {
			t.Errorf("导入区的图标按钮缺少可读名称：%s", wanted)
		}
	}
	// 图标全部用 Lucide，语义按用户口径挑：
	//   FileSpreadsheet（载入文件 = 一个 Excel 文件）/ TableIcon（数据回顾 = 看数据表格）/
	//   Trash2（清空，现在挂在标题栏上）/ Search（记录导出的回顾）
	for _, icon := range []string{"FileSpreadsheet", "TableIcon", "Search"} {
		if !strings.Contains(panels, icon) {
			t.Errorf("图标应当使用 Lucide 的 %s", icon)
		}
	}
	// 载入文件既不是文件夹图标（语义不对），也不是导入箭头（文案写着"载入文件"）
	for _, rejected := range []string{"FolderOpen", "FileText", "Upload"} {
		if strings.Contains(panels, rejected) {
			t.Errorf("载入文件/数据回顾不该再用 %s 图标", rejected)
		}
	}
	if strings.Contains(panels, `"anticon"`) || strings.Contains(panels, "<path d=") {
		t.Error("图标不应再手写 SVG 路径")
	}
}

// TestTheLeftColumnIsNarrowAndTheStatusBoxIsReadable 固定左列的"窄而整齐"：
//   - 左列收窄到 280（日期上下叠放换来的横向空间让给右侧数据整合）；
//   - 导入状态是一个淡灰底、文字居中、单行不换行的状态框（高度与两侧图标按钮一致）；
//   - 清空按钮是"图标 + 文字"。
func TestTheLeftColumnIsNarrowAndTheStatusBoxIsReadable(t *testing.T) {
	styles := readFrontendSource(t, "src/styles.css")
	panels := readFrontendSource(t, "src/components/Panels.tsx")

	left := cssRule(t, styles, ".left {")
	for _, wanted := range []string{"width: 340px;", "min-width: 320px;", "max-width: 40%;"} {
		if !strings.Contains(left, wanted) {
			t.Errorf("左列宽度口径缺少 %s，实际块：\n%s", wanted, left)
		}
	}

	// 窗口变矮/表格很长时，整行都要收得回窗口内（.columns 允许收缩）；
	// 左列内部由日志让位：数据导入保持内容高度、日志 flex-basis 0 吃掉剩余高度，
	// 两张卡都完整显示，也不会冒出纵向滚动条。
	columns := cssRule(t, styles, ".columns {")
	if !strings.Contains(columns, "flex: 1 1 auto;") {
		t.Errorf(".columns 应当允许收缩（表格行数很多时也能收回到窗口内），实际块：\n%s", columns)
	}
	logCard := cssRule(t, styles, ".left > .log-card {")
	if !strings.Contains(logCard, "flex: 1 1 0;") {
		t.Errorf("操作日志应当 flex-basis 0 吃掉左列剩余高度，实际块：\n%s", logCard)
	}
	importCard := cssRule(t, styles, ".left > .card {")
	if !strings.Contains(importCard, "flex: 0 0 auto;") {
		t.Errorf("数据导入应当保持内容高度，实际块：\n%s", importCard)
	}
	// 四个来源分组竖着叠起来要能在最小窗口（760 高）里放得下，所以留白是收紧过的
	group := cssRule(t, styles, ".group {")
	if !strings.Contains(group, "padding: 8px;") {
		t.Errorf("导入分组的竖向留白应当是收紧后的 8px，实际块：\n%s", group)
	}

	status := cssBlock(t, styles, ".actions .status {")
	for _, wanted := range []string{
		"background: rgba(0, 0, 0, 0.04);", // 淡灰底
		"text-align: center;",              // 信息居中
		"white-space: nowrap;",             // 不换行
		"text-overflow: ellipsis;",         // 真放不下才省略号
		"height: 32px;",                    // 与两侧图标按钮同高
	} {
		if !strings.Contains(status, wanted) {
			t.Errorf("导入状态框缺少 %s，实际块：\n%s", wanted, status)
		}
	}

	// 清空按钮搬到了「数据导入」的标题栏（App.tsx 的 extra）：只有白色图标，无文字无边框，
	// 这样数据导入模块本身少一行高度。
	app := readFrontendSource(t, "src/App.tsx")
	if strings.Contains(panels, "清空导入数据") {
		t.Error("清空按钮应当搬进标题栏（App.tsx 的 extra），不该再留在导入面板里")
	}
	for _, wanted := range []string{
		`className="head-icon-action"`,
		`aria-label="清空导入数据"`,
		`icon={<Trash2 size={16} />}`,
		"extra={",
	} {
		if !strings.Contains(app, wanted) {
			t.Errorf("标题栏里的清空按钮缺少：%s", wanted)
		}
	}
	// 按钮体（到 /> 为止）里不能出现文字
	clearButton := app[strings.Index(app, `className="head-icon-action"`):]
	clearButton = clearButton[:strings.Index(clearButton, "/>")]
	if strings.Contains(clearButton, ">清空") || strings.Contains(clearButton, "清空<") {
		t.Error("标题栏里的清空按钮不该带文字")
	}
	action := cssRule(t, styles, ".card > .ant-card-head .head-icon-action.ant-btn {")
	for _, wanted := range []string{"color: #fff;", "background: transparent;", "border: none;"} {
		if !strings.Contains(action, wanted) {
			t.Errorf("标题栏图标按钮缺少 %s，实际块：\n%s", wanted, action)
		}
	}
}

// TestTheImportStatusUsesAntdBadge 固定导入区的状态表达：用 antd 的 Badge（状态语义色）
// 与 Typography 的次级文字，不再自绘圆点、也不再做"仿只读输入框"的状态条。
func TestTheImportStatusUsesAntdBadge(t *testing.T) {
	panels := readFrontendSource(t, "src/components/Panels.tsx")

	if !strings.Contains(panels, "Badge") || !strings.Contains(panels, "BadgeProps") {
		t.Error("导入状态应当用 antd Badge 表达（状态语义色由 antd 给）")
	}
	if !strings.Contains(panels, "type=\"secondary\"") || !strings.Contains(panels, "ellipsis={{ tooltip:") {
		t.Error("状态文字应当是次级色 + 过长省略（antd Typography）")
	}
	if !strings.Contains(panels, `{current.label || "尚未导入"}`) {
		t.Error("状态标签要有「尚未导入」占位")
	}
	if strings.Contains(panels, `className="dot`) {
		t.Error("不该再自绘状态圆点")
	}
}

// TestTheEmptyResultTableHasNoFrame 固定空结果表的口径：没有数据时不画那个带边框的
// 长方形（它会和模块外框叠成两层），而是在整个模块区域正中间显示"暂无数据"。
func TestTheEmptyResultTableHasNoFrame(t *testing.T) {
	panels := readFrontendSource(t, "src/components/Panels.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	if !strings.Contains(panels, "rows.length === 0 ?") {
		t.Error("结果表要按「有没有数据」分支渲染")
	}
	if !strings.Contains(panels, "<Empty description=\"暂无数据\" />") {
		t.Error("空状态应当用 antd 的 Empty + 暂无数据")
	}
	if !strings.Contains(styles, ".table-empty {") ||
		!strings.Contains(styles, "justify-content: center") {
		t.Error("空状态应当在模块区域里居中显示")
	}
}

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

// TestTheReviewModalKeepsTheFooterLayout 固定回顾窗口的底栏：
// 导出在最左、翻页居中、关闭在最右（同一行），每页 100 行。
func TestTheReviewModalKeepsTheFooterLayout(t *testing.T) {
	modals := readFrontendSource(t, "src/components/Modals.tsx")
	if !strings.Contains(modals, `justify="center"`) {
		t.Error("翻页组件应当在底栏居中")
	}
	if !strings.Contains(modals, "<Pagination") || !strings.Contains(modals, "pageSize={result?.pageSize ?? 100}") {
		t.Error("回顾窗口要分页，默认每页 100 行")
	}
	if !strings.Contains(modals, "导出") || !strings.Contains(modals, "关闭") {
		t.Error("底栏要有导出与关闭按钮")
	}
}

// TestTheReviewModalGrowsWithTheWindowKeepingTheGaps 固定回顾窗口（数据回顾 / 记录导出）的尺寸口径：
// 窗口变大时弹窗跟着变大，四周留白始终等于**最小窗口**下的那一份，不再把宽度卡在 1100。
//
// 留白来自最小窗口的实测：主窗口最窄 969、弹窗宽 80vw = 775，两侧各 97；
// 弹窗高 131 + 56vh = 556（WebView 内容区最小高度就是 760），上下各 102。
func TestTheReviewModalGrowsWithTheWindowKeepingTheGaps(t *testing.T) {
	modals := readFrontendSource(t, "src/components/Modals.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	if !strings.Contains(modals, "const REVIEW_GAP_X = 97;") || !strings.Contains(modals, "const REVIEW_GAP_Y = 102;") {
		t.Error("留白常量应当写死成最小窗口下的实测值（97 / 102）")
	}
	// 宽度必须走 width 属性：antd 的 width 是后写的内联样式，会盖掉 style.width
	if !strings.Contains(modals, "width={`calc(100vw - ${REVIEW_GAP_X * 2}px)`}") {
		t.Error("弹窗宽度应当是「窗口宽度 - 两侧留白」，并走 antd 的 width 属性")
	}
	if !strings.Contains(modals, "style={{ height: `calc(100vh - ${REVIEW_GAP_Y * 2}px)` }}") {
		t.Error("弹窗高度应当是「窗口高度 - 上下留白」")
	}
	if strings.Contains(modals, "min(80vw, 1100px)") {
		t.Error("宽度不该再封顶在 1100（窗口变大时弹窗要跟着变大）")
	}
	if strings.Contains(modals, `maxHeight: "56vh"`) {
		t.Error("表格高度不该再用 56vh 封顶（高度改由弹窗的弹性链分配）")
	}
	// 内部那条竖直弹性链：表格吃掉多出来的高度并自己滚动
	for _, wanted := range []string{
		".review-modal > div {", // antd 夹在中间的容器要先撑满
		"height: 100%;",
		".review-modal .ant-modal-content {",
		".review-modal .ant-modal-body {",
		"flex: 1 1 auto;",
		"min-height: 0;",
		".review-modal .tblwrap {",
	} {
		if !strings.Contains(styles, wanted) {
			t.Errorf("回顾窗口的弹性链样式缺少：%s", wanted)
		}
	}
}

// TestAllDialogsUseAntdDefaults 固定弹窗回到 antd 默认形态：
// 五个弹窗（提示 / 回顾 / 参数设定 / 关于 / 附加窗口）全部居中；
// 提示弹窗就是 antd 默认的 Modal（标题 + 自带 X + 右对齐底栏），
// 不再有红底标题条 / 居中按钮 / 手动去掉关闭按钮那套 QDialog 布局。
func TestAllDialogsUseAntdDefaults(t *testing.T) {
	panels := readFrontendSource(t, "src/components/Panels.tsx")
	modals := readFrontendSource(t, "src/components/Modals.tsx")
	styles := readFrontendSource(t, "src/styles.css")
	main := readFrontendSource(t, "src/main.tsx")

	// 1) 五个弹窗全部居中
	if got := strings.Count(modals, "centered"); got < 4 {
		t.Errorf("回顾/参数设定/关于/附加窗口都要居中（centered），实际 %d 处", got)
	}
	if !strings.Contains(panels, "centered") {
		t.Error("提示弹窗也要居中（centered）")
	}
	// 2) 提示弹窗：antd 默认底栏（右对齐）+ 单个「确定」主按钮
	if !strings.Contains(panels, `className="app-message"`) {
		t.Error("提示弹窗要保留 app-message 这个稳定挂点")
	}
	if !strings.Contains(panels, "确定") || !strings.Contains(panels, `type="primary"`) {
		t.Error("提示弹窗要有一个「确定」主按钮")
	}
	// 3) 不许再给弹窗写红底标题条 / 居中底栏的覆盖样式
	for _, leftover := range []string{".app-message ", ".app-modal"} {
		if strings.Contains(styles, leftover) {
			t.Errorf("styles.css 里不该再出现弹窗外观覆盖：%s", leftover)
		}
	}
	// 4) 中文按钮不要自动插空格
	if !strings.Contains(main, "autoInsertSpace: false") {
		t.Error("要关掉 antd 在两个汉字之间插空格的默认行为")
	}
	// 5) 展示型窗口的内容区仍然固定 323 高、内容纵向居中
	for _, wanted := range []string{"justify-content: center", "height: 323px"} {
		if !strings.Contains(styles, wanted) {
			t.Errorf("关于窗口内容区样式缺少：%s", wanted)
		}
	}
	if !strings.Contains(styles, "margin-top: 16px") {
		t.Error("图标与下方文字之间要留出间距")
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

// ---------- 发布前的设计体检（A 组：可读性 / 可点性 / 一致性） ----------

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

// TestTheStatusColorsStayInSyncWithTheTheme 固定结果表状态色只有一份来源：
// theme.ts 的 STATUS_COLORS 与 styles.css 里的字面值必须一致，
// 否则"工具栏标签 / 行底色 / 竖条"迟早会三套颜色。
func TestTheStatusColorsStayInSyncWithTheTheme(t *testing.T) {
	theme := readFrontendSource(t, "src/theme.ts")
	styles := readFrontendSource(t, "src/styles.css")

	colors := themeColors(t, theme, "export const STATUS_COLORS")
	for _, status := range []string{"Ready", "Incomplete", "Duplicate", "Conflict"} {
		hex, ok := colors[status]
		if !ok {
			t.Fatalf("theme.ts 的 STATUS_COLORS 里缺少 %s", status)
		}
		if !strings.Contains(strings.ToLower(styles), hex) {
			t.Errorf("styles.css 里找不到 %s 的状态色 %s（行底色/竖条应与主题一致）", status, hex)
		}
	}
}

// TestTheToolbarFitsTheNarrowestWindow 固定数据整合工具条在最小窗口（969 宽）下的行为：
// 一行放完 —— 左边「数据整合」、中间状态筛选片、右边「生成文件」，**不换行**；
// 三个状态片等宽（取最宽那个）、与左右按钮同高（32px），文字不换行（放不下就收窄）。
func TestTheToolbarFitsTheNarrowestWindow(t *testing.T) {
	styles := readFrontendSource(t, "src/styles.css")

	toolbar := cssBlock(t, styles, ".table-toolbar {")
	if !strings.Contains(toolbar, "flex-wrap: nowrap;") {
		t.Errorf("工具条必须一行放完，不许换行，实际块：\n%s", toolbar)
	}
	if strings.Contains(styles, "@media (max-width: 1040px)") {
		t.Error("不该再有把状态片挤到第二行的媒体查询")
	}

	// 工具条这一行：查找框靠左、状态片靠右
	search := cssBlock(t, styles, ".table-search {")
	if !strings.Contains(search, "min-width: 150px;") {
		t.Errorf("查找框要能压缩但不能挤掉状态片（min-width: 150px），实际块：\n%s", search)
	}
	strip := cssBlock(t, styles, ".status-strip {")
	if !strings.Contains(strip, "justify-content: flex-end;") {
		t.Errorf("三个状态片应当靠右，实际块：\n%s", strip)
	}

	chips := cssBlock(t, styles, ".status-chips {")
	if !strings.Contains(chips, "grid-auto-columns: 86px;") {
		t.Errorf("状态片要固定等宽（86px ≈ 原 57px × 1.5），不随数字位数变化，实际块：\n%s", chips)
	}
	count := cssBlock(t, styles, ".status-count {")
	if !strings.Contains(count, "font-weight: 600;") {
		t.Errorf("按钮里的数字要加粗，实际块：\n%s", count)
	}

	chip := cssBlock(t, styles, ".status-chip.ant-tag {")
	for _, wanted := range []string{
		"height: 32px;",        // 与 antd 按钮同高
		"white-space: nowrap;", // 组件内文字不换行
		"text-overflow: ellipsis;",
	} {
		if !strings.Contains(chip, wanted) {
			t.Errorf("状态片样式缺少：%s，实际块：\n%s", wanted, chip)
		}
	}
	if !strings.Contains(styles, `.status-chip.ant-tag[aria-pressed="true"] {`) {
		t.Error("缺少「当前筛选」的样式")
	}
	if !strings.Contains(styles, `.status-strip[data-filtered="true"] .status-chip:not([aria-pressed="true"]) {`) {
		t.Error("缺少「有筛选时其余状态片淡出」的样式")
	}
}

// ---------- 发布前的设计体检（B 组：层级 / 状态 / 一致性） ----------

// TestTheExportActionIsSecondaryAndWaitsForReadyRows 固定数据整合工具条的层级与门禁：
// 一个区域只留一个主按钮（数据整合 = primary，生成文件 = 次按钮），
// 且没有 Ready 行时「生成文件」禁用（只有 Ready 会导出，点了也是白点）。
func TestTheExportActionIsSecondaryAndWaitsForReadyRows(t *testing.T) {
	app := readFrontendSource(t, "src/App.tsx")
	panels := readFrontendSource(t, "src/components/Panels.tsx")

	// 门禁在 App 侧算（按钮住在标题栏里）：只有合格（Ready）行会被导出
	if !strings.Contains(app, `const readyCount = statuses.filter((status) => status === "Ready").length;`) {
		t.Error("标题栏应当按 Ready 行数决定「生成文件」能不能点")
	}
	exportButton := app[strings.Index(app, `aria-label="生成文件"`):]
	exportButton = exportButton[:strings.Index(exportButton, "/>")]
	if strings.Contains(exportButton, `type="primary"`) {
		t.Error("「生成文件」应当是次/图标按钮：一个区域只留「整合」一个主按钮")
	}
	if !strings.Contains(app, "disabled={readyCount === 0}") {
		t.Error("没有 Ready 行时「生成文件」要禁用")
	}
	if strings.Contains(exportButton, ">生成文件<") {
		t.Error("「生成文件」在标题栏里只留图标，不该带文字")
	}
	if !strings.Contains(app, "还没有可导出的行（只有合格的行会导出）") {
		t.Error("禁用时要给出原因（Tooltip）")
	}
	// 「整合」是标题栏里唯一的主按钮
	if !strings.Contains(app, `className="consolidation-run"`) || !strings.Contains(app, "整合") {
		t.Error("「整合」仍然是这个区域的主按钮")
	}
	if strings.Contains(panels, "onConsolidate") || strings.Contains(panels, "onExport") {
		t.Error("两个按钮已经搬到标题栏，面板里不该再持有这两个回调")
	}
}

// TestTheStatusSummaryUsesClickableChips 固定状态汇总的表达：
// 三个状态片是 antd Tag（success / error / warning，冲突时多一个 magenta），
// 但要能点：role=button、aria-pressed 标出当前筛选、键盘 Enter/Space 也能切。
func TestTheStatusSummaryUsesClickableChips(t *testing.T) {
	panels := readFrontendSource(t, "src/components/Panels.tsx")

	for _, wanted := range []string{
		`{ status: "Ready", color: "success" }`,
		`{ status: "Incomplete", color: "error" }`,
		`{ status: "Duplicate", color: "warning" }`,
		`chips.push({ status: "Conflict", color: "magenta" })`,
		`role="button"`,
		`aria-pressed={statusFilter === status}`,
		"onKeyDown={(event) => {",
		`event.key !== "Enter" && event.key !== " "`,
		"toggleStatus(status)",
		`id="consolidation-summary"`,
	} {
		if !strings.Contains(panels, wanted) {
			t.Errorf("状态汇总缺少：%s", wanted)
		}
	}
}

// TestTheStatusChipsFilterTheResultTable 固定「点状态片就筛选数据列表」：
// 只显示该状态的行，再点一次回到全部；导出（只看 Ready）与筛选互不影响。
func TestTheStatusChipsFilterTheResultTable(t *testing.T) {
	panels := readFrontendSource(t, "src/components/Panels.tsx")

	for _, wanted := range []string{
		"const [statusFilter, setStatusFilter] = useState<string | null>(null);",
		"setStatusFilter((current) => (current === status ? null : status));",
		".filter((entry) => statusFilter === null || entry.status === statusFilter)",
		"rowClassName={(entry) => `result-row status-${entry.status}`}",
		"data-filtered={statusFilter !== null}",
		"`${statusLabel(statusFilter)} 状态没有数据`", // 空态也用中文状态名
	} {
		if !strings.Contains(panels, wanted) {
			t.Errorf("状态筛选缺少：%s", wanted)
		}
	}
	// 状态片文案是中文（合格 / 缺失 / 重复），与日志口径一致
	if !strings.Contains(panels, "statusLabel(status)") {
		t.Error("状态片文案应当走 statusLabel（中文）")
	}
	theme := readFrontendSource(t, "src/theme.ts")
	for _, wanted := range []string{
		`Ready: "合格"`,
		`Incomplete: "缺失"`,
		`Duplicate: "重复"`,
		`Conflict: "冲突"`,
	} {
		if !strings.Contains(theme, wanted) {
			t.Errorf("状态中文名缺少：%s", wanted)
		}
	}
	// 筛选只影响显示：导出门禁在 App 侧按全部行里的 Ready 数量算，不跟着筛选走
	app := readFrontendSource(t, "src/App.tsx")
	if !strings.Contains(app, `const readyCount = statuses.filter((status) => status === "Ready").length;`) {
		t.Error("导出门禁不该跟着筛选走（仍是全部行里的 Ready 数量）")
	}
}

// TestTheConsolidationHeadButtonsSitOnTheRedBar 固定数据整合标题栏里两个按钮的规格：
// 「整合」白底红字、整条栏的正中、28px 高（38px 栏里上下各留 5px，不许出栏）；
// 「生成文件」是去掉边框的白色图标按钮、与「整合」同高（28px），图标在 App 侧放大到 18px。
func TestTheConsolidationHeadButtonsSitOnTheRedBar(t *testing.T) {
	app := readFrontendSource(t, "src/App.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	runButton := cssRule(t, styles, ".consolidation-card .consolidation-run.ant-btn {")
	for _, wanted := range []string{
		"position: absolute;",
		"top: 50%;", // 少了它绝对定位会从标题文字上沿往下排，下缘就出栏了
		"left: 50%;",
		"transform: translate(-50%, -50%);",
		"height: 28px;", // 收窄后的高度：38px 的标题栏里上下各留 5px
		"background: #fff;",
		"color: #da291c;",
	} {
		if !strings.Contains(runButton, wanted) {
			t.Errorf("「整合」按钮缺少 %s，实际块：\n%s", wanted, runButton)
		}
	}
	// 图标按钮：无边框、白图标（沿用清空按钮那套），尺寸与「整合」同高
	icon := cssRule(t, styles, ".consolidation-card > .ant-card-head .head-icon-action.ant-btn {")
	for _, wanted := range []string{"width: 28px;", "height: 28px;", "padding: 0;"} {
		if !strings.Contains(icon, wanted) {
			t.Errorf("生成文件图标按钮缺少 %s，实际块：\n%s", wanted, icon)
		}
	}
	for _, forbidden := range []string{"border:", "background:", "color:"} {
		if strings.Contains(icon, forbidden) {
			t.Errorf("生成文件图标按钮不该自己写 %s（无边框白图标由基础规则给），实际块：\n%s", forbidden, icon)
		}
	}
	if !strings.Contains(app, "icon={<FileDown size={18} />}") {
		t.Error("生成文件图标要放大到 18px")
	}
	// 两个标题栏按钮共用无边框白图标的基础规则；禁用时退回半透明白
	clear := cssRule(t, styles, ".card > .ant-card-head .head-icon-action.ant-btn {")
	for _, wanted := range []string{"color: #fff;", "border: none;", "background: transparent;"} {
		if !strings.Contains(clear, wanted) {
			t.Errorf("标题栏图标按钮的基础规则缺少 %s，实际块：\n%s", wanted, clear)
		}
	}
	if !strings.Contains(styles, ".card > .ant-card-head .head-icon-action.ant-btn:disabled,") {
		t.Error("标题栏图标按钮禁用时应当退回半透明白")
	}
	head := cssRule(t, styles, ".consolidation-card > .ant-card-head .ant-card-head-wrapper,")
	if !strings.Contains(head, "align-items: center;") {
		t.Errorf("标题栏内容要纵向居中，实际块：\n%s", head)
	}
}

// TestTheConsolidationSearchFiltersTheTable 固定数据整合的查找框：
// 它靠左、状态片靠右；输入即按整行任意单元格做包含匹配（不区分大小写），
// 与状态筛选叠加生效；查不到时给专门的空态文案。
func TestTheConsolidationSearchFiltersTheTable(t *testing.T) {
	panels := readFrontendSource(t, "src/components/Panels.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	for _, wanted := range []string{
		"<Input", // antd 默认输入框
		`className="table-search"`,
		"allowClear",
		`aria-label="查找表格内容"`,
		`placeholder="查找表格内容"`,
		"prefix={<Search size={14} aria-hidden=\"true\" />}",
		`const [query, setQuery] = useState("");`,
		"const needle = query.trim().toLowerCase();",
		"entry.cells.some((cell) => String(cell ?? \"\").toLowerCase().includes(needle))",
		"没有找到匹配的数据",
	} {
		if !strings.Contains(panels, wanted) {
			t.Errorf("查找框缺少：%s", wanted)
		}
	}
	// 关键字与状态筛选叠加：两个 filter 串在一起
	if !strings.Contains(panels, ".filter((entry) => statusFilter === null || entry.status === statusFilter)") {
		t.Error("关键字应当与状态筛选叠加生效")
	}
	// 查找框靠左、状态片靠右
	search := cssRule(t, styles, ".table-search {")
	if !strings.Contains(search, "flex: 0 1 240px;") {
		t.Errorf("查找框宽度口径不对，实际块：\n%s", search)
	}
}

// TestTheMessageDialogShowsStructuredDetails 固定提示弹窗的形态：
// 标题前一个状态图标（成功绿对勾 / 失败红警示），结果摊成 antd 默认的键值表
// （信息类别 / 导入数量），数量带千分位；失败时保留原来的多行原因文本。
func TestTheMessageDialogShowsStructuredDetails(t *testing.T) {
	panels := readFrontendSource(t, "src/components/Panels.tsx")
	app := readFrontendSource(t, "src/App.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	// 1) 数据类型：明细是「标签 + 值」的数组
	for _, wanted := range []string{
		"export type MessageDetail = { label: string; value: string };",
		"details?: MessageDetail[];",
		"failed?: boolean;",
	} {
		if !strings.Contains(panels, wanted) {
			t.Errorf("提示弹窗的数据类型缺少：%s", wanted)
		}
	}
	// 2) 标题里的状态图标（Lucide，不用手写 SVG）
	for _, wanted := range []string{
		"<CheckCircle2",
		"<CircleAlert",
		"message-icon-ok",
		"message-icon-failed",
		`aria-hidden="true"`,
	} {
		if !strings.Contains(panels, wanted) {
			t.Errorf("提示弹窗标题的状态图标缺少：%s", wanted)
		}
	}
	// 3) 明细用 antd 默认的 Descriptions：一列、小尺寸、无冒号
	for _, wanted := range []string{"<Descriptions", "column={1}", `size="small"`, "colon={false}", "<Typography.Text strong>"} {
		if !strings.Contains(panels, wanted) {
			t.Errorf("提示弹窗的键值表缺少：%s", wanted)
		}
	}
	// 4) 导入成功后要给出「信息类别 + 导入数量」，数量带千分位
	for _, wanted := range []string{
		`{ label: "信息类别", value: category }`,
		"  SOURCES,", // 从 bridge 引入来源表，好把 key 翻成中文类别名
		".find((item) => item.key === source)",
		`toLocaleString("zh-CN")`,
		"failed: result.failed",
	} {
		if !strings.Contains(app, wanted) {
			t.Errorf("导入结果的组装缺少：%s", wanted)
		}
	}
	// 5) 图标颜色走主题：成功用与日志「成功」同一个绿，失败用危险红
	ok := cssRule(t, styles, ".message-icon-ok {")
	if !strings.Contains(ok, "color: #389e0d;") {
		t.Errorf("成功图标应当是状态绿，实际块：\n%s", ok)
	}
	failed := cssRule(t, styles, ".message-icon-failed {")
	if !strings.Contains(failed, "color: #cf1322;") {
		t.Errorf("失败图标应当是危险红，实际块：\n%s", failed)
	}
	label := cssRule(t, styles, ".message-details .ant-descriptions-item-label {")
	if !strings.Contains(label, "color: rgba(0, 0, 0, 0.45);") {
		t.Errorf("键值表的标签应当是次级灰，实际块：\n%s", label)
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

// cssRule 与 cssBlock 相同，但要求选择器出现在行首：
// 否则 ".log-card {" 会命中 ".left > .log-card {" 这种带父选择器的规则。
func cssRule(t *testing.T, source, selector string) string {
	t.Helper()
	return cssBlock(t, source, "\n"+selector)
}

// cssBlock 取出 `selector` 开头那一对花括号之间的内容（只用于读源码里的常量）。
func cssBlock(t *testing.T, source, selector string) string {
	t.Helper()
	start := strings.Index(source, selector)
	if start < 0 {
		t.Fatalf("样式中找不到选择器 %q", selector)
	}
	rest := source[start:]
	end := strings.Index(rest, "}")
	if end < 0 {
		t.Fatalf("选择器 %q 的样式块没有收尾", selector)
	}
	return rest[:end]
}

// themeColors 从 theme.ts 里读出 `export const NAME = {...}` 中的 {名字: #RRGGBB}（全部小写）。
func themeColors(t *testing.T, theme, declaration string) map[string]string {
	t.Helper()
	block := cssBlock(t, theme, declaration)
	pattern := regexp.MustCompile(`(\w+):\s*"(#[0-9A-Fa-f]{6})"`)
	colors := map[string]string{}
	for _, match := range pattern.FindAllStringSubmatch(block, -1) {
		colors[match[1]] = strings.ToLower(match[2])
	}
	if len(colors) == 0 {
		t.Fatalf("没有从 %s 里解析出任何颜色", declaration)
	}
	return colors
}

// contrastOnWhite 算 #RRGGBB 在白底上的 WCAG 对比度。
func contrastOnWhite(t *testing.T, hex string) float64 {
	t.Helper()
	value := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(hex)), "#")
	if len(value) != 6 {
		t.Fatalf("颜色 %q 不是 #RRGGBB", hex)
	}
	luminance := func(part string) float64 {
		number, err := strconv.ParseUint(part, 16, 8)
		if err != nil {
			t.Fatalf("颜色 %q 解析失败：%v", hex, err)
		}
		channel := float64(number) / 255
		if channel <= 0.03928 {
			return channel / 12.92
		}
		return math.Pow((channel+0.055)/1.055, 2.4)
	}
	lum := 0.2126*luminance(value[0:2]) + 0.7152*luminance(value[2:4]) + 0.0722*luminance(value[4:6])
	return 1.05 / (lum + 0.05)
}
