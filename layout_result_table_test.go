package main

// 前端契约测试：结果表、工具条、状态片与整合标题栏。
//
// 公共前言与助手（源码读取、CSS 抽取、对比度）见 layout_helpers_test.go。
import (
	"strings"
	"testing"
)

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
