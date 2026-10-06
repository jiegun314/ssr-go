package main

// 前端契约测试：操作日志窗口（筛选分栏、详情弹窗与复制）。
//
// 公共前言与助手（源码读取、CSS 抽取、对比度）见 layout_helpers_test.go。
import (
	"strings"
	"testing"
)

// TestTheLogWindowIsFilterableAndTwoColumn 固定操作日志窗口的形式：
// 级别栏目（全部 / 信息 / 成功 / 警告 / 错误）与「新日志置顶」放在标题条里；
// 内容是一张两列表格：左列只显示时间段（窄栏省宽度），右列是级别色字 + 正文；
// 正文最多两行、超出省略，完整内容在 title 里悬浮可见。
func TestTheLogWindowKeepsItsDesignContracts(t *testing.T) {
	panels := readFrontendSource(t, "src/components/Panels.tsx")
	styles := readFrontendSource(t, "src/styles.css")
	script := readFrontendSource(t, "src/log.ts")

	// 行为（筛选、置顶、点开详情、复制全文）由前端单元测试覆盖：
	// frontend/src/__tests__/logPanel.test.tsx。这里只留"画出来长什么样"的口径。
	for _, wanted := range []string{
		"showHeader={false}", // 两列表格，不再单画一行表头
		`title: "时间",`,
		`title: "信息",`,
		"width: 64",                // 时间列只要装下 HH:MM:SS（12px 等宽数字约 54px）
		"log-level-${entry.level}", // 级别用两个字 + 级别色
		"log-text",                 // 正文容器（两行截断）
	} {
		if !strings.Contains(panels, wanted) {
			t.Errorf("LogPanel 里缺少：%s", wanted)
		}
	}

	for _, wanted := range []string{
		".log-list {",
		".log-card > .ant-card-head {", // 灰底 + 红字的标题条
		".log-card > .ant-card-head .ant-card-head-title {",
		"background: var(--log-head-bg);",             // 标题条用更深一档的灰（值见 design-tokens.ts）
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
		"width: var(--log-level-slot);",
		".log-card .log-table .ant-table.ant-table-small .ant-table-tbody > tr > td.ant-table-cell {",
		"justify-content: center;",
		".log-level-info {", ".log-level-success {",
		".log-level-warning {", ".log-level-error {",
		".log-text {",
		"-webkit-line-clamp: 2;",   // 正文最多两行
		"overflow-wrap: anywhere;", // 长路径 / 长报错要换行
		"min-height: var(--log-min-height);", // 日志卡的下限（值见 design-tokens.ts）
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
