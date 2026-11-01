package main

// 右列布局口径（用户反馈）：窗口变矮时「数据对齐」不许被压成一条缝，
// 也不许出现"看不到、也没法滚"的情况：
//   - 右列整体可纵向滚动；
//   - 数据对齐按内容高度固定（flex: 0 0 auto）；
//   - 数据整合表吃掉剩余高度，但保住 min-height（标题 + 工具条 + 若干行），
//     放不下时交给右列滚动。

import (
	"strings"
	"testing"
)

func TestTheRightColumnKeepsTheAlignPanelVisible(t *testing.T) {
	styles := readFrontendSource(t, "src/styles.css")

	right := cssRule(t, styles, ".right {")
	if !strings.Contains(right, "overflow-y: auto;") {
		t.Errorf("右列应当可以纵向滚动（否则矮窗口里对齐面板无法查看），实际块：\n%s", right)
	}

	card := cssRule(t, styles, ".right > .card {")
	if !strings.Contains(card, "flex: 0 0 auto;") {
		t.Errorf("右列的普通模块（数据对齐）应当按内容高度固定、不参与压缩，实际块：\n%s", card)
	}

	consolidation := cssRule(t, styles, ".right > .consolidation-card {")
	if !strings.Contains(consolidation, "flex: 1 1 auto;") {
		t.Errorf("数据整合应当吃掉剩余高度，实际块：\n%s", consolidation)
	}
	if !strings.Contains(consolidation, "min-height: 320px;") {
		t.Errorf("数据整合应当保住最小可用高度（320px），实际块：\n%s", consolidation)
	}
}
