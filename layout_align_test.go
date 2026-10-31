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
	// 对齐面板：本机版本 → 共享盘版本 + 两种状态 + 全部对齐
	if !strings.Contains(align, `→ {source.remoteVersion || "（未发布）"}`) {
		t.Error("对齐面板应当显示「本机版本 → 共享盘版本」")
	}
	for _, wanted := range []string{"已对齐 · 待整合", "未对齐", "全部对齐"} {
		if !strings.Contains(align, wanted) {
			t.Errorf("对齐面板缺少：%s", wanted)
		}
	}
	panel := cssBlock(t, styles, ".align-panel {")
	if !strings.Contains(panel, "border: 1px solid var(--module-border);") {
		t.Errorf("对齐面板应当与其它模块同一描边，实际块：\n%s", panel)
	}
}
