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
		`disabled={!can("consolidate") || shareBlocked || !allAligned}`,
		`最后整合：`,
	} {
		if !strings.Contains(app, wanted) {
			t.Errorf("App.tsx 缺少：%s", wanted)
		}
	}
	if !strings.Contains(app, `disabled={!can("consolidate") || shareBlocked || !allAligned}`) {
		t.Error("未对齐时整合按钮应当禁用")
	}
	// 对齐模块与其它模块同构：Card + 红底白字标题栏（.card 已由全局规则给），"全部对齐"在标题栏
	if !strings.Contains(app, `className="card align-card"`) || !strings.Contains(app, `title="数据对齐"`) {
		t.Error("数据对齐应当是 .card 模块（红底白字标题栏）")
	}
	if !strings.Contains(app, `aria-label="全部对齐"`) {
		t.Error("标题栏里应当有「全部对齐」按钮")
	}
	// 四个来源是四张卡片
	if count := strings.Count(align, `className="align-source-card"`); count != 1 {
		t.Errorf("来源卡片应当由同一个模板渲染（找到 %d 处）", count)
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
	// 标题栏按钮与其它模块同一形式（白底红字、28px）
	run := cssBlock(t, styles, ".align-card .align-run.ant-btn {")
	if !strings.Contains(run, "height: 28px;") || !strings.Contains(run, "color: var(--brand-red);") {
		t.Errorf("标题栏「全部对齐」应当白底红字 28px，实际块：\n%s", run)
	}
}
