package main

// P5 界面契约：底部状态栏有「运行状态」入口（SC/Admin），运行状态弹窗包含
// 共享盘 / 四份来源 / 最后一次整合 / 维护（清理 + 查看审计）四块。

import (
	"strings"
	"testing"
)

func TestTheStatusModalHasTheAdminSections(t *testing.T) {
	app := readFrontendSource(t, "src/App.tsx")
	modal := readFrontendSource(t, "src/components/StatusModal.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	for _, wanted := range []string{
		`<StatusModal`,
		`aria-label="运行状态"`,
		`can("audit:any")`,
		`can("share:write")`,
		`call<CleanupResult>("CleanupShareVersions"`,
		`call<ReviewResult>("ReviewAudit", page, pageSize, query)`,
		`call<ReviewResult>("ReviewLog", session.start, session.end, page, pageSize, query)`,
		`call<ReviewResult>("ReviewSource", session.source, page, pageSize, query)`,
		`onSearch={handleReviewSearch}`,
		`call<ConsolidateResult>("ConsolidateForce")`,
		`强制重跑`,
		`call<DiagnosticsResult>("ExportDiagnostics")`,
	} {
		if !strings.Contains(app, wanted) {
			t.Errorf("App.tsx 缺少：%s", wanted)
		}
	}
	for _, wanted := range []string{
		"四份来源",
		"最后一次整合",
		"整合锁",
		"清理",
		"查看审计",
		"重新检查",
		"导出诊断包",
	} {
		if !strings.Contains(modal, wanted) {
			t.Errorf("运行状态弹窗缺少：%s", wanted)
		}
	}
	section := cssBlock(t, styles, ".status-section-title {")
	if !strings.Contains(section, "color: var(--brand-red);") {
		t.Errorf("运行状态的分节标题应当用品牌红，实际块：\n%s", section)
	}
}
