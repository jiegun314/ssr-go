package app

// 「数据整合」模块的界面契约（用户反馈）：
//   - 标题与右侧动作（强制重跑/生成文件）之间要有间隔；
//   - RA/EAMA/UDI 没有整合权限：标题栏与数据窗口灰化、只显示「只读」，并隐藏动作按钮。

import (
	"strings"
	"testing"
)

func TestConsolidationModuleSpacingAndReadOnlyState(t *testing.T) {
	app := readFrontendSource(t, "src/App.tsx")
	panels := readFrontendSource(t, "src/components/Panels.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	for _, wanted := range []string{
		`canConsolidate`,
		`consolidation-card-locked`,
		`consolidation-locked-tag`,
		`locked={!canConsolidate}`,
		`className="consolidation-force"`,
		`className="consolidation-export"`,
	} {
		if !strings.Contains(app, wanted) {
			t.Errorf("App.tsx 缺少：%s", wanted)
		}
	}
	// 没有整合权限时不渲染整合/强制重跑/生成文件按钮
	if strings.Contains(app, `disabled={!can("consolidate") || shareBlocked || !allAligned}`) {
		t.Error("没有整合权限时应当直接不渲染「整合」，而不是只置灰")
	}
	if !strings.Contains(panels, "consolidation-locked-hint") {
		t.Error("数据窗口应当有只读说明")
	}
	// 标题与右侧动作的间隔（用整段匹配：同一个选择器前面还有一条分组规则）
	if !strings.Contains(styles, ".consolidation-card > .ant-card-head .ant-card-extra {\n  gap: 8px;\n  margin-inline-start: 16px;") {
		t.Error("标题与右侧动作之间应当留出 gap: 8px 与左边距 16px")
	}
	// 灰化：标题栏变浅灰、标题字变浅灰、正文变淡
	head := cssBlock(t, styles, ".card.consolidation-card-locked > .ant-card-head {")
	if !strings.Contains(head, "background: #f5f5f5;") {
		t.Errorf("没有整合权限时标题栏应当变灰，实际块：\n%s", head)
	}
	title := cssBlock(t, styles, ".card.consolidation-card-locked > .ant-card-head .ant-card-head-title,")
	if !strings.Contains(title, "color: rgba(0, 0, 0, 0.45);") {
		t.Errorf("灰化的标题字应当变淡，实际块：\n%s", title)
	}
	body := cssBlock(t, styles, ".card.consolidation-card-locked > .ant-card-body {")
	if !strings.Contains(body, "opacity: 0.55;") {
		t.Errorf("灰化的数据窗口应当变淡，实际块：\n%s", body)
	}
}
