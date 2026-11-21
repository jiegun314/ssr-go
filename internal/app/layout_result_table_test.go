package app

// 前端契约测试：结果表、工具条、状态片与整合标题栏。
//
// 公共前言与助手（源码读取、CSS 抽取、对比度）见 layout_helpers_test.go。
import (
	"regexp"
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
		// 关于窗口的 Descriptions 可以用 bordered；这里只约束 <Table ...> 自身的属性
		for index := 0; index < len(source); {
			start := strings.Index(source[index:], "<Table")
			if start < 0 {
				break
			}
			start += index
			end := strings.Index(source[start:], ">")
			if end < 0 {
				break
			}
			tag := source[start : start+end]
			if strings.Contains(tag, "bordered") {
				t.Errorf("%s 的结果表不该用 bordered（antd Table 默认无边框）", name)
			}
			index = start + end
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
// 查找框 + 查找/导出按钮居左、状态筛选片居右；状态片等宽（86px），文字不换行。
// **出现冲突片（4 个）时最小窗口放不下整行**：4×86 + 3×8 = 368，再加查找框最小 150
// 与两个按钮；此时整组状态片换到第二行（仍靠右），不压缩状态片、也不裁掉最后一片。
// 因此这里允许 flex-wrap: wrap，但禁止用媒体查询手工搬动状态片。
func TestTheToolbarFitsTheNarrowestWindow(t *testing.T) {
	styles := readFrontendSource(t, "src/styles.css")

	toolbar := cssBlock(t, styles, ".table-toolbar {")
	if !strings.Contains(toolbar, "flex-wrap: wrap;") {
		t.Errorf("工具条在冲突片 + 导出按钮放不下时应当可换行，实际块：\n%s", toolbar)
	}
	if strings.Contains(styles, "@media (max-width: 1040px)") {
		t.Error("不该再有把状态片挤到第二行的媒体查询")
	}

	// 工具条这一行：查找框居左、筛选片居右（两端对齐）
	if !strings.Contains(toolbar, "justify-content: space-between;") {
		t.Errorf("查找框要居左、筛选片要居右（space-between），实际块：\n%s", toolbar)
	}
	if strings.Contains(styles, ".table-search-row {") {
		t.Error("查找框不再单独占一行（已与状态片同排）")
	}
	search := cssBlock(t, styles, ".table-search {")
	if !strings.Contains(search, "min-width: 150px;") {
		t.Errorf("查找框要能压缩但不能挤掉按钮（min-width: 150px），实际块：\n%s", search)
	}
	strip := cssBlock(t, styles, ".status-strip {")
	if !strings.Contains(strip, "justify-content: flex-end;") {
		t.Errorf("状态片自身靠右排布，实际块：\n%s", strip)
	}
	// 状态片不参与收缩：否则 grid 内容会溢出自己的盒子，第四个（冲突）片被卡片边缘裁掉
	if !strings.Contains(strip, "flex: 0 0 auto;") {
		t.Errorf("状态片不该被压缩（flex: 0 0 auto，宽度先由查找框让出），实际块：\n%s", strip)
	}
	if !strings.Contains(strip, "margin-left: auto;") {
		t.Errorf("状态片换到第二行时仍应靠右（margin-left: auto），实际块：\n%s", strip)
	}

	chips := cssBlock(t, styles, ".status-chips {")
	if !strings.Contains(chips, "grid-auto-columns: var(--status-chip-width);") {
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

// TestTheExportActionIsSecondaryAndWaitsForReadyRows 固定数据整合标题栏的层级与门禁：
// 一个区域只留一个**主按钮**（数据整合 = primary 文字按钮）；「生成文件」是红底上的
// 白色图标按钮（无文字，tooltip / aria-label 说明用途），且没有 Ready 行时禁用。
func TestTheExportActionIsSecondaryAndWaitsForReadyRows(t *testing.T) {
	app := readFrontendSource(t, "src/App.tsx")
	panels := readFrontendSource(t, "src/components/Panels.tsx")

	// 门禁在 App 侧算（按钮住在标题栏里）：只有合格（Ready）行会被导出
	if !strings.Contains(app, `const readyCount = statuses.filter((status) => status === "Ready").length;`) {
		t.Error("标题栏应当按 Ready 行数决定「生成文件」能不能点")
	}
	start := strings.Index(app, `className="consolidation-export"`)
	if start < 0 {
		t.Fatal("没找到标题栏里的「生成文件」图标按钮")
	}
	exportButton := app[start:]
	end := strings.Index(exportButton, "onClick={handleExport}")
	if end < 0 {
		t.Fatal("「生成文件」图标按钮缺少 onClick")
	}
	exportButton = exportButton[:end]
	for _, wanted := range []string{
		`label="生成文件"`, // hover 提示 + aria-label
		`inverse`,      // 红底标题栏
		`icon={<FileDown />}`,
		`disabled={!can("export") || readyCount === 0}`,
	} {
		if !strings.Contains(exportButton, wanted) {
			t.Errorf("「生成文件」图标按钮缺少 %s，实际块：\n%s", wanted, exportButton)
		}
	}
	if strings.Contains(exportButton, `type="primary"`) {
		t.Error("「生成文件」应当是次要动作：一个区域只留「整合」一个主按钮")
	}
	if strings.Contains(exportButton, "生成文件</") {
		t.Error("「生成文件」改成纯图标按钮后不该再带文字")
	}
	if !strings.Contains(app, "还没有可导出的行（只有合格的行会导出）") {
		t.Error("禁用时要给出原因（Tooltip）")
	}
	// 「整合」是标题栏里唯一的主按钮，仍保留文字
	if !strings.Contains(app, `className="consolidation-run"`) ||
		!strings.Contains(app, `type="primary"`) ||
		!strings.Contains(app, "整合") {
		t.Error("「整合」仍然是这个区域的主按钮")
	}
	// 官方「整合 / 生成文件」按钮住在标题栏，面板不再持有这两个回调；
	// 面板自己的「导出当前视图」是另一件事（onExportView），允许保留。
	if regexp.MustCompile(`\bonConsolidate\b|\bonExport\b`).MatchString(panels) {
		t.Error("官方整合/生成文件按钮已经搬到标题栏，面板里不该再持有这两个回调")
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
		// 每个状态片悬停给出「这个状态是什么意思」
		`const STATUS_HINTS: Record<string, string> = {`,
		`{STATUS_HINTS[status]}`,
		`mouseEnterDelay={CONFLICT_TOOLTIP_DELAY}`,
	} {
		if !strings.Contains(panels, wanted) {
			t.Errorf("状态汇总缺少：%s", wanted)
		}
	}
	// 含义提示走 antd Tooltip，不用原生 title（延迟由系统定、要停在文字上）
	if strings.Contains(panels, "title={`只看「") {
		t.Error("状态片不该再用原生 title 给提示（改用 Tooltip + STATUS_HINTS）")
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

// TestConflictCellsAreMarkedInPlace 固定冲突单元格的标识口径（AGENTS.md §11 第 5 步）：
// 有提示的格渲染成 antd Tooltip 包着的 <span className="cell-conflict">，样式与状态色
// Conflict 同色、加粗，并且 span 撑满整格 —— 原生 title 的延迟由系统定（几秒），
// 而且要精确停在文字上，所以提示一律走 Tooltip。
func TestConflictCellsAreMarkedInPlace(t *testing.T) {
	panels := readFrontendSource(t, "src/components/Panels.tsx")
	styles := readFrontendSource(t, "src/styles.css")
	theme := readFrontendSource(t, "src/theme.ts")

	for _, wanted := range []string{
		// 提示矩阵与表格同形同序，冲突格按格取值（不写死列下标）
		`conflicts?: string[][];`,
		`const note = entry.notes[index];`,
		`<Tooltip title={note} mouseEnterDelay={CONFLICT_TOOLTIP_DELAY}>`,
		`<span className="cell-conflict">{value}</span>`,
	} {
		if !strings.Contains(panels, wanted) {
			t.Errorf("冲突格的渲染缺少：%s", wanted)
		}
	}
	if !strings.Contains(panels, "const CONFLICT_TOOLTIP_DELAY = 0.1;") {
		t.Error("冲突提示要用 antd Tooltip 的短延迟（0.1s），别退回原生 title 的几秒等待")
	}
	if strings.Contains(panels, `className="cell-conflict" title=`) {
		t.Error("冲突格不该再用原生 title（延迟由系统控制，且要停在文字上）")
	}

	block := cssBlock(t, styles, ".cell-conflict {")
	if !strings.Contains(block, "display: block;") {
		t.Errorf("冲突格要撑满整格，鼠标不必精确停在文字上，实际块：\n%s", block)
	}
	if !strings.Contains(block, "font-weight: 600;") {
		t.Errorf("冲突格要加粗，实际块：\n%s", block)
	}
	colors := themeColors(t, theme, "export const STATUS_COLORS")
	if !strings.Contains(strings.ToLower(block), colors["Conflict"]) {
		t.Errorf("冲突格要用状态色 Conflict（%s），实际块：\n%s", colors["Conflict"], block)
	}
	// 清晰定位：整格浅底 + 内侧描边 + 右上角小三角
	if !strings.Contains(block, "background: #fff0f6;") || !strings.Contains(block, "inset 0 0 0 1px #ffadd2") {
		t.Errorf("冲突格要有整格浅底与内侧描边，实际块：\n%s", block)
	}
	// 定位：列头圆点 + （点冲突片时的）闪烁；「定位冲突」按钮按用户要求已取消
	for _, wanted := range []string{".conflict-dot {", ".cell-flash {", "conflict-flash"} {
		if !strings.Contains(styles, wanted) {
			t.Errorf("冲突定位缺少样式：%s", wanted)
		}
	}
	if strings.Contains(panels, "locate-conflict") {
		t.Error("「定位冲突」按钮已取消，不该再出现")
	}
	if !strings.Contains(panels, "conflict-dot") {
		t.Error("有冲突的表头应当加小圆点")
	}
	// 冲突字段不在导出列时：整行挂行级原因（鼠标停在行内任意处可见）
	if !strings.Contains(panels, "cell-row-conflict") || !strings.Contains(panels, "rowConflicts") {
		t.Error("没有单元格级提示的冲突行应当挂行级原因")
	}
	// 所有表格查找：输入框旁边有小查找按钮，点按钮/回车才生效
	if !strings.Contains(panels, "table-search-group") || !strings.Contains(panels, `label="查找"`) {
		t.Error("整合表的查找应当有独立的小查找按钮")
	}
	modals := readFrontendSource(t, "src/components/Modals.tsx")
	if !strings.Contains(modals, "table-search-group") || !strings.Contains(modals, `label="查找"`) {
		t.Error("回顾窗口的查找应当有独立的小查找按钮")
	}
	triangle := cssBlock(t, styles, ".cell-conflict::after {")
	if !strings.Contains(triangle, "border-top: 6px solid") {
		t.Errorf("冲突格右上角要有小三角标记，实际块：\n%s", triangle)
	}
	// 缺失格：12px 小胶囊，不再是加粗大写 MISSING
	missing := cssBlock(t, styles, ".cell-missing {")
	for _, wanted := range []string{"font-size: 12px;", "background: #fff1f0;", "border: 1px solid #ffccc7;"} {
		if !strings.Contains(missing, wanted) {
			t.Errorf("缺失格标记应当是 12px 小胶囊（缺 %s），实际块：\n%s", wanted, missing)
		}
	}
	if strings.Contains(panels, ".cell-MISSING") {
		t.Error("缺失格不该再用加粗大写 MISSING 的旧样式")
	}
}

// TestTheConsolidationHeadButtonsSitOnTheRedBar 固定数据整合标题栏的两个层级：
// 「整合」是唯一的品牌红主按钮（白底红字、24px 紧凑档，占标题栏正中）；
// 「生成文件」是红底上的白色纯图标按钮（24px，tooltip + aria-label），不再用白底文字按钮。
func TestTheConsolidationHeadButtonsSitOnTheRedBar(t *testing.T) {
	app := readFrontendSource(t, "src/App.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	runButton := cssRule(t, styles, ".consolidation-card .consolidation-run.ant-btn {")
	for _, wanted := range []string{
		"position: absolute;",
		"top: 50%;",
		"left: 50%;",
		"transform: translate(-50%, -50%);",
		"height: 24px;",
		"background: #fff;",
		"color: #da291c;",
	} {
		if !strings.Contains(runButton, wanted) {
			t.Errorf("「整合」按钮缺少 %s，实际块：\n%s", wanted, runButton)
		}
	}
	start := strings.Index(app, `className="consolidation-export"`)
	if start < 0 {
		t.Fatal("没找到标题栏里的「生成文件」图标按钮")
	}
	exportButton := app[start:]
	end := strings.Index(exportButton, "onClick={handleExport}")
	if end < 0 {
		t.Fatal("「生成文件」图标按钮缺少 onClick")
	}
	exportButton = exportButton[:end]
	for _, wanted := range []string{`label="生成文件"`, "inverse", "icon={<FileDown />}"} {
		if !strings.Contains(exportButton, wanted) {
			t.Errorf("「生成文件」图标按钮缺少 %s，实际块：\n%s", wanted, exportButton)
		}
	}
	if strings.Contains(exportButton, "生成文件</") {
		t.Error("「生成文件」改成纯图标按钮后不该再带文字")
	}
	if strings.Contains(styles, ".consolidation-card .consolidation-export.ant-btn {") {
		t.Error("旧的「生成文件」白底文字按钮样式应当删掉，改用统一的图标按钮")
	}
	base := cssBlock(t, styles, ".ui-icon-button.ant-btn {")
	for _, wanted := range []string{"width: 24px;", "height: 24px;", "color: rgba(0, 0, 0, 0.65);"} {
		if !strings.Contains(base, wanted) {
			t.Errorf("图标按钮基础档缺少 %s，实际块：\n%s", wanted, base)
		}
	}
	middle := cssBlock(t, styles, ".ui-icon-button-middle.ant-btn {")
	for _, wanted := range []string{"width: 32px;", "height: 32px;"} {
		if !strings.Contains(middle, wanted) {
			t.Errorf("图标按钮标准档缺少 %s，实际块：\n%s", wanted, middle)
		}
	}
	inverse := cssBlock(t, styles, ".ui-icon-button-inverse.ant-btn {")
	if !strings.Contains(inverse, "color: #fff;") {
		t.Errorf("红底图标按钮应当是白色，实际块：\n%s", inverse)
	}
	danger := cssBlock(t, styles, ".ui-icon-button-danger.ant-btn {")
	if !strings.Contains(danger, "color: var(--danger-red);") {
		t.Errorf("危险图标按钮应当用危险红，实际块：\n%s", danger)
	}
	// 清空按钮仍走无边框白图标那套基础规则；禁用时退回半透明白
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
