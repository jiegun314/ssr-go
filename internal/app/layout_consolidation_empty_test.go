package app

// 数据整合空态：不是一句所有角色共用的“暂无数据”，而是按权限与进度
// 告诉用户下一步（导入 / 对齐 / 整合）。视觉上也不做空表格。

import (
	"strings"
	"testing"
)

func TestTheConsolidationEmptyStateIsRoleAware(t *testing.T) {
	panel := readFrontendSource(t, "src/components/Panels.tsx")
	app := readFrontendSource(t, "src/App.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	for _, wanted := range []string{
		`emptyState?: { title: string; description: string };`,
		`className="consolidation-empty"`,
		`role="status"`,
		`<Layers size={28}`,
	} {
		if !strings.Contains(panel, wanted) {
			t.Errorf("ConsolidationPanel 缺少角色化空态结构：%s", wanted)
		}
	}

	for _, wanted := range []string{
		"当前为只读视图",
		"有整合权限的同事完成整合后，结果会在这里显示。",
		"还没有可整合的数据",
		"先在左侧导入 RA / UDI 数据，再开始整合。",
		"等待 RA / EAMA / UDI 发布后，在「数据对齐」里取回最新版本。",
		"数据等待对齐",
		"共享盘上的新版本还没有全部取到本机，先完成「数据对齐」。",
		"暂无整合结果",
		"点击右上角「整合」，生成可查看的结果表。",
	} {
		if !strings.Contains(app, wanted) {
			t.Errorf("App 缺少按角色/进度下发的空态文案：%s", wanted)
		}
	}

	empty := cssBlock(t, styles, ".consolidation-empty {")
	for _, wanted := range []string{"flex: 1 1 auto;", "min-height: 340px;", "text-align: center;"} {
		if !strings.Contains(empty, wanted) {
			t.Errorf("空态容器缺少样式：%s，实际块：\n%s", wanted, empty)
		}
	}
	if !strings.Contains(styles, ".consolidation-empty::before") ||
		!strings.Contains(styles, "radial-gradient(circle at 1px 1px") {
		t.Error("空态应当有极淡的点阵背景，而不是一块空白")
	}
	art := cssBlock(t, styles, ".consolidation-empty-art {")
	for _, wanted := range []string{
		"background: var(--brand-red-soft);",
		"border: 1px solid var(--brand-red-border);",
		"border-radius: 50%;",
	} {
		if !strings.Contains(art, wanted) {
			t.Errorf("空态插画外圈缺少样式：%s，实际块：\n%s", wanted, art)
		}
	}
}
