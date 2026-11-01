package main

// P3 的对齐面板与可见性口径（界面契约）：
//   - 只有 SC/Admin（有 align 动作）且配置了共享盘时才渲染 AlignPanel；
//   - 整合按钮在"未对齐"时禁用，并提示先对齐；
//   - 导入区按 importable / viewable 两套权限显隐（RA 看别人只有最后更新时间）；
//   - 底部状态栏显示"最后整合时间 + 操作人"。

import (
	"strings"
	"testing"
)

func TestTheAlignPanelAndVisibilityRules(t *testing.T) {
	app := readFrontendSource(t, "src/App.tsx")
	panels := readFrontendSource(t, "src/components/Panels.tsx")
	align := readFrontendSource(t, "src/components/AlignPanel.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	for _, wanted := range []string{
		`<AlignPanel`,
		`overview?.enabled`,
		`canAlign`,
		`importableSources={importableSources}`,
		`viewableSources={viewableSources}`,
		`disabled={shareBlocked || !allAligned}`,
		`最后整合：`,
	} {
		if !strings.Contains(app, wanted) {
			t.Errorf("App.tsx 缺少：%s", wanted)
		}
	}
	// 对齐模块与其它模块同构：Card + 红底白字标题栏；发布状态/自动对齐/全部对齐都在标题栏
	if !strings.Contains(app, `className="card align-card"`) || !strings.Contains(app, `title="数据对齐"`) {
		t.Error("数据对齐应当是 .card 模块（红底白字标题栏）")
	}
	for _, wanted := range []string{
		`className="align-head-extra"`,
		`className="align-status"`,
		`className="align-auto-label"`,
		`aria-label="全部对齐"`,
	} {
		if !strings.Contains(app, wanted) {
			t.Errorf("标题栏缺少：%s", wanted)
		}
	}

	// 卡片：对齐按钮在标题行（右上角），面板里不再有工具栏
	if !strings.Contains(align, `className="align-source-head"`) {
		t.Error("来源卡片应当有标题行")
	}
	headStart := strings.Index(align, `className="align-source-head"`)
	headEnd := strings.Index(align[headStart:], `</div>`)
	if headEnd < 0 || !strings.Contains(align[headStart:headStart+headEnd], `className="align-source-run"`) {
		t.Error("「对齐」按钮应当在标题行里（右上角）")
	}
	if strings.Contains(align, "align-toolbar") {
		t.Error("工具栏已合并进标题栏，面板里不该再有 align-toolbar")
	}

	// 导入区：看不到明细的来源只显示最后更新时间，且不渲染按钮
	for _, wanted := range []string{
		`最后更新：${info.publishedAt}`,
		`pending ? (`,
		`canImport ? (`,
	} {
		if !strings.Contains(panels, wanted) {
			t.Errorf("Panels.tsx 缺少：%s", wanted)
		}
	}

	// 布局：宽度够时一行四个，变窄 2×2（容器查询按模块自身宽度判断）
	wrap := cssBlock(t, styles, ".align-grid-wrap {")
	if !strings.Contains(wrap, "container-type: inline-size;") {
		t.Errorf("对齐网格的外层应当是容器，实际块：\n%s", wrap)
	}
	grid := cssBlock(t, styles, ".align-grid {")
	if !strings.Contains(grid, "repeat(2, minmax(0, 1fr))") {
		t.Errorf("窄宽度应当是 2 列，实际块：\n%s", grid)
	}
	if !strings.Contains(styles, "@container (min-width: 760px)") ||
		!strings.Contains(styles, "repeat(4, minmax(0, 1fr))") {
		t.Error("宽度够时四个卡片应当排成一行（@container min-width: 760px）")
	}
	// 卡片紧凑：右上角按钮与标题同高；标题栏状态是白字
	run := cssBlock(t, styles, ".align-source-run.ant-btn {")
	if !strings.Contains(run, "height: 22px;") || !strings.Contains(run, "margin-left: auto;") {
		t.Errorf("「对齐」按钮应当高 22px 并靠右，实际块：\n%s", run)
	}
	statusTag := cssBlock(t, styles, ".align-card .align-status.ant-tag {")
	if !strings.Contains(statusTag, "color: #fff;") {
		t.Errorf("标题栏状态标签在黑底上应当是白字，实际块：\n%s", statusTag)
	}
	runButton := cssBlock(t, styles, ".align-card .align-run.ant-btn {")
	if !strings.Contains(runButton, "height: 28px;") || !strings.Contains(runButton, "color: var(--brand-red);") {
		t.Errorf("标题栏「全部对齐」应当白底红字 28px，实际块：\n%s", runButton)
	}
}
