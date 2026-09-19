package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheOperationLogCardHasADragHandle 固定「操作日志高度可拖动」的界面契约：
// 卡片上沿有一个分隔条（浮在上边框上，不占卡片高度），它是 row-resize 的拖动条，
// 在操作日志标题之前；拖动逻辑在 app.js 的 setupLogResizer 里。
func TestTheOperationLogCardHasADragHandle(t *testing.T) {
	html := readFrontendFile(t, "index.html")
	script := readFrontendFile(t, "app.js")

	handle := `<div class="log-resizer" id="log-resizer"`
	if !strings.Contains(html, handle) {
		t.Fatalf("操作日志卡片缺少分隔条：界面里找不到 %s", handle)
	}
	if strings.Index(html, handle) > strings.Index(html, "<h2>操作日志</h2>") {
		t.Error("分隔条应该在「操作日志」标题之前（贴在卡片上沿）")
	}
	for _, wanted := range []string{`role="separator"`, `aria-orientation="horizontal"`, `tabindex="0"`} {
		if !strings.Contains(html, wanted) {
			t.Errorf("分隔条缺少 %s（键盘可调、语义正确）", wanted)
		}
	}
	// 浮在上沿：卡片要有定位上下文，分隔条自己用负的 top
	if body := styleRule(t, html, ".log-section{"); !strings.Contains(body, "position:relative") {
		t.Errorf("操作日志卡片缺少定位上下文：%s", body)
	}
	if body := styleRule(t, html, ".log-resizer{"); !strings.Contains(body, "cursor:row-resize") {
		t.Errorf("分隔条不是上下拖动的手型：%s", body)
	}
	// 拖动逻辑：向上拖＝变高，下限是默认高度，上限由中间列的最低高度决定
	for _, wanted := range []string{
		"function setupLogResizer()",
		"setupLogResizer()",
		"drag.startY - event.clientY",
		"columnMinimumHeight()",
		`section.style.flex = "0 0 auto"`,
	} {
		if !strings.Contains(script, wanted) {
			t.Errorf("app.js 里缺少拖动逻辑：%s", wanted)
		}
	}
	// 日志窗口要跟着卡片一起变大：内容区必须撑满卡片，文本框再撑满内容区
	if body := styleRule(t, html, ".log-section>.body{"); !strings.Contains(body, "flex:1 1 auto") {
		t.Errorf("操作日志的内容区没有撑满卡片，拖动时文本窗口不会跟着变大：%s", body)
	}
	if body := styleRule(t, html, "textarea#log{"); !strings.Contains(body, "flex:1 1 auto") {
		t.Errorf("日志文本框没有撑满内容区：%s", body)
	}
}

// TestTheOperationLogResizerKeepsTheLeftColumnGap 说明上限口径的口径来源：
// 上限＝左侧「数据导入 / 记录导出」只剩 .spacer 的最小间隔（12px），
// 下限＝启动时的默认高度。三条 CSS 事实都要在，否则夹紧算出来的上限没有意义。
func TestTheOperationLogResizerKeepsTheLeftColumnGap(t *testing.T) {
	html := readFrontendFile(t, "index.html")

	if body := styleRule(t, html, ".left>.spacer{"); !strings.Contains(body, "min-height:12px") {
		t.Errorf("左侧两个模块之间的最小间隔不是 12px：%s", body)
	}
	if body := styleRule(t, html, ".left{"); !strings.Contains(body, "flex-direction:column") {
		t.Errorf("左列不是纵向排列：%s", body)
	}
	if body := styleRule(t, html, ".log-section{"); !strings.Contains(body, "flex:0 1 auto") {
		t.Errorf("操作日志卡片默认仍要可收缩（窗口变矮时先让出高度）：%s", body)
	}
}

// readFrontendFile 读前端的静态文件（界面与脚本都在 frontend/dist 下）。
func readFrontendFile(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("frontend", "dist", name))
	if err != nil {
		t.Fatalf("读前端文件 %s 失败：%v", name, err)
	}
	return string(raw)
}
