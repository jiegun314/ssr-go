package main

// 前端契约测试：左列（数据导入 / 记录导出 / 操作日志）的布局与可读性。
//
// 公共前言与助手（源码读取、CSS 抽取、对比度）见 layout_helpers_test.go。
import (
	"strings"
	"testing"
)

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
