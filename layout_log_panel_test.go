package main

// 前端契约测试：操作日志窗口（筛选分栏、详情弹窗与复制）。
//
// 公共前言与助手（源码读取、CSS 抽取、对比度）见 layout_helpers_test.go。
import (
	"os"
	"strings"
	"testing"
)

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

// TestWarningAndErrorLogsOpenACopyableDetailDialog 固定日志详情弹窗：
// 只有「警告 / 错误」那一行可点（信息 / 成功保持纯展示），鼠标悬浮仍能看到全文；
// 点开是完整正文（不截断）+「复制」「关闭」两个图标按钮，复制内容 = 时间 + 级别 + 全文，
// 先走后端剪贴板（Wails 运行时，webview 里比浏览器 Clipboard API 可靠），失败再退回浏览器兜底。
func TestWarningAndErrorLogsOpenACopyableDetailDialog(t *testing.T) {
	panels := readFrontendSource(t, "src/components/Panels.tsx")
	bridge := readFrontendSource(t, "src/bridge.ts")
	styles := readFrontendSource(t, "src/styles.css")
	methodsRaw, err := os.ReadFile("app_methods.go")
	if err != nil {
		t.Fatalf("读 app_methods.go 失败：%v", err)
	}
	methods := string(methodsRaw)

	for _, wanted := range []string{
		"onRow={(entry) => callbackRow(entry, setDetail)}",     // 行点击入口
		`entry.level !== "warning" && entry.level !== "error"`, // 只有警告 / 错误可点
		`className: "log-row-actionable"`,
		"tabIndex: 0",
		"查看${LOG_LEVEL_LABELS[entry.level]}详情", // 可读名称带级别
		"onKeyDown: (event) => {",              // 键盘也能打开
		"<LogDetailModal entry={detail}",
		`className="log-detail"`,
		`aria-label="复制全部信息"`,
		`aria-label="关闭"`,
		"icon={<Copy size={16} />}",
		"icon={<X size={16} />}",
		"const text = `[${entry.time}] [${LOG_LEVEL_LABELS[entry.level]}] ${entry.message}`;",
		"await copyToClipboard(text)",
		`message.success("已复制全部信息")`,
		`className="log-detail-text"`,
	} {
		if !strings.Contains(panels, wanted) {
			t.Errorf("日志详情弹窗缺少：%s", wanted)
		}
	}
	// 悬浮看全文的口径不变（原生 title 就是完整正文）
	if !strings.Contains(panels, "title={entry.message}") {
		t.Error("鼠标悬浮仍要能看到完整信息（title）")
	}
	// 复制：后端优先 + 浏览器兜底
	for _, wanted := range []string{
		"export async function copyToClipboard(text: string): Promise<boolean>",
		"app.CopyToClipboard(text)",
		"navigator.clipboard?.writeText",
		`document.execCommand("copy")`,
	} {
		if !strings.Contains(bridge, wanted) {
			t.Errorf("复制入口缺少：%s", wanted)
		}
	}
	if !strings.Contains(methods, "func (app *App) CopyToClipboard(text string) bool") ||
		!strings.Contains(methods, "runtime.ClipboardSetText(ctx, text)") {
		t.Error("后端要有 CopyToClipboard（Wails 运行时写系统剪贴板）")
	}
	// 可点行的光标 + 详情正文不截断（不 clamp，可滚动）
	row := cssRule(t, styles, ".log-row-actionable {")
	if !strings.Contains(row, "cursor: pointer;") {
		t.Errorf("可点行应当是手型光标，实际块：\n%s", row)
	}
	detail := cssRule(t, styles, ".log-detail-text {")
	for _, wanted := range []string{"white-space: pre-wrap;", "max-height:", "overflow: auto;"} {
		if !strings.Contains(detail, wanted) {
			t.Errorf("详情正文缺少 %s（要能看全、能滚动），实际块：\n%s", wanted, detail)
		}
	}
	if strings.Contains(detail, "-webkit-line-clamp") {
		t.Errorf("详情正文不该再截断行数，实际块：\n%s", detail)
	}
}
