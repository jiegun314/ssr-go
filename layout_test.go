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
	// 上下调节的光标只能由分隔条自己提供：在 body 上覆盖 cursor 会让松手后光标卡住
	if body := styleRule(t, html, "body.log-resizing{"); strings.Contains(body, "cursor:") {
		t.Errorf("拖动状态不应在 body 上覆盖光标（松手后会卡在上下调节）：%s", body)
	}
	// 拖动逻辑：向上拖＝变高，下限是默认高度，上限由中间列的最低高度决定
	for _, wanted := range []string{
		"function setupLogResizer()",
		"setupLogResizer()",
		"drag.startY - event.clientY",
		"columnMinimumHeight()",
		`section.style.flex = "0 0 auto"`,
		`window.addEventListener("pointerup", finishDrag)`,
		`window.addEventListener("pointercancel", finishDrag)`,
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
	// 关于窗口的图标保持普通指针（不额外提示可点击）
	if body := styleRule(t, html, "#about-icon{"); strings.Contains(body, "cursor:") {
		t.Errorf("关于窗口图标不应设光标：%s", body)
	}
	if body := styleRule(t, html, ".about img{"); strings.Contains(body, "cursor:") {
		t.Errorf("关于窗口图标不应设光标：%s", body)
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

// TestTheHiddenWindowIsNotAdvertisedInTheFrontend 固定「不提示」的口径：
// 界面、脚本与资源文件名里都不能出现说明隐藏窗口的文字或命名
// （它们会随界面打进程序，用 strings 就能看到）。
func TestTheHiddenWindowIsNotAdvertisedInTheFrontend(t *testing.T) {
	hints := []string{"彩蛋", "连点", "easter", "EASTER", "egg", "Egg", "EGG"}
	for _, name := range []string{"index.html", "app.js"} {
		content := readFrontendFile(t, name)
		for _, hint := range hints {
			if strings.Contains(content, hint) {
				t.Errorf("frontend/dist/%s 里出现了会提示隐藏窗口的文字：%q", name, hint)
			}
		}
	}
	entries, err := os.ReadDir(filepath.Join("frontend", "dist"))
	if err != nil {
		t.Fatalf("读 frontend/dist 失败：%v", err)
	}
	for _, entry := range entries {
		for _, hint := range hints {
			if strings.Contains(entry.Name(), hint) {
				t.Errorf("资源文件名带提示：%s（命中 %q）", entry.Name(), hint)
			}
		}
	}
}

// TestTheLogExportRowSurvivesWiderPlatformFonts 固定「记录导出」整行可压缩的口径：
// Windows（WebView2）下日期控件的固有宽度比 macOS 宽很多，一旦整行不可压缩，
// 模块底部就会冒出横向滚动条（用户报的 bug）。所以：
//   - 日期容器允许收缩（不能写死 flex:0 0 auto）
//   - 日期框各自可退让，但不允许超出容器（max-width:100%）
//   - 右侧图标按钮不参与收缩，否则会被压扁
func TestTheLogExportRowSurvivesWiderPlatformFonts(t *testing.T) {
	html := readFrontendFile(t, "index.html")

	if body := styleRule(t, html, ".range{"); !strings.Contains(body, "min-width:0") {
		t.Errorf("记录导出整行必须允许收缩：%s", body)
	}
	if body := styleRule(t, html, ".range .dates{"); !strings.Contains(body, "flex:0 1 auto") {
		t.Errorf("日期容器必须可收缩（Windows 下日期控件更宽）：%s", body)
	}
	dates := styleRule(t, html, ".dates input[type=date]{")
	if !strings.Contains(dates, "flex:1 1 auto") || !strings.Contains(dates, "max-width:100%") {
		t.Errorf("日期框必须可收缩且不得超出容器：%s", dates)
	}
	if !strings.Contains(dates, "min-width:") {
		t.Errorf("日期框需要保留一个最小可读宽度：%s", dates)
	}
	if body := styleRule(t, html, ".range .icon-btn{"); !strings.Contains(body, "flex:0 0 auto") {
		t.Errorf("回顾按钮不应参与收缩：%s", body)
	}
}

// TestTheOperationLogUsesTheUIFontFamily 固定操作日志的字体口径：
// 日志是整句中英混排文本、不是表格，所以不能用等宽栈 —— ui-monospace 在 Chromium
// 内核里不被识别、Menlo 只有 macOS 有，Windows 会退化成 Consolas（英文/数字）
// 加微软雅黑（中文）两种字体混排。改成界面自己的系统字体栈后，
// Windows：Segoe UI + 微软雅黑；macOS：系统字体 + 苹方。
func TestTheOperationLogUsesTheUIFontFamily(t *testing.T) {
	rule := styleRule(t, readFrontendFile(t, "index.html"), "textarea#log{")
	if strings.Contains(rule, "monospace") {
		t.Errorf("操作日志不应使用等宽字体（Windows 上会退化成 Consolas + 微软雅黑混排）：%s", rule)
	}
	for _, wanted := range []string{`"Segoe UI"`, "-apple-system", `"PingFang SC"`, `"Microsoft YaHei"`} {
		if !strings.Contains(rule, wanted) {
			t.Errorf("操作日志字体栈里缺少 %s：%s", wanted, rule)
		}
	}
	// 参数设定的值文本同理（参数里会夹中文）
	value := styleRule(t, readFrontendFile(t, "index.html"), ".settings-body .md-v{")
	if strings.Contains(value, "monospace") {
		t.Errorf("参数设定的值文本不应使用等宽字体：%s", value)
	}
	if !strings.Contains(value, `"Microsoft YaHei"`) {
		t.Errorf("参数设定的值文本缺少跨平台字体栈：%s", value)
	}
}

// TestTheHiddenWindowSoundShipsInsideTheApp 固定附加窗口声音的落地方式：
// 音频必须放在 frontend/dist（构建时由 //go:embed 编进二进制），不能放在
// resource/（发布脚本会整目录拷进发布包，那样就会留下一个单独的文件）；
// 窗口打开时循环播放、关闭时停掉。
func TestTheHiddenWindowSoundShipsInsideTheApp(t *testing.T) {
	html := readFrontendFile(t, "index.html")
	script := readFrontendFile(t, "app.js")

	if !strings.Contains(html, `<audio id="puppy-voice" src="puppy-voice.mp3" loop`) {
		t.Error("界面里缺少循环播放的音频元素")
	}
	if stray, _ := filepath.Glob(filepath.Join("resource", "*.mp3")); len(stray) > 0 {
		t.Errorf("音频不要放在会被整目录拷进发布包的 resource/：%v", stray)
	}
	info, err := os.Stat(filepath.Join("frontend", "dist", "puppy-voice.mp3"))
	if err != nil {
		t.Fatalf("音频没有随界面资源一起内嵌：%v", err)
	}
	if info.Size() < 1024 {
		t.Errorf("音频文件过小（%d 字节），可能不是有效的 mp3", info.Size())
	}
	for _, wanted := range []string{
		"function startPuppyVoice()",
		"function stopPuppyVoice()",
		"startPuppyVoice();",
		`puppyDialog.addEventListener("close", stopPuppyVoice)`,
		`puppyDialog.addEventListener("cancel", stopPuppyVoice)`,
		`voiceCloseButton.addEventListener("click", stopPuppyVoice)`,
	} {
		if !strings.Contains(script, wanted) {
			t.Errorf("app.js 里缺少声音控制：%s", wanted)
		}
	}
}
