package main

// 底部状态栏（P1）：登录人 / 共享盘状态 / 切换身份（图标）都收进窗口最底下的那一条。
//
// 固定口径：
//   - 旧的顶部会话条删掉，状态栏在主界面（.columns）之后；
//   - 「切换身份」是图标按钮（aria-label + Tooltip），不再显示文字；
//   - 窗口是两段式（.app-frame）：上面主界面可伸缩，下面状态栏贴底、整宽、一条淡色上边框。

import (
	"strings"
	"testing"
)

func TestTheStatusBarSitsAtTheBottomWithASwitchIcon(t *testing.T) {
	app := readFrontendSource(t, "src/App.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	if strings.Contains(app, "session-bar") {
		t.Error("旧的顶部会话条应当已经删掉（内容搬到 .status-bar）")
	}

	columnsAt := strings.Index(app, "`columns${leftCollapsed")
	statusAt := strings.Index(app, `className="status-bar"`)
	if columnsAt < 0 || statusAt < 0 {
		t.Fatal("找不到 .columns 或 .status-bar")
	}
	if statusAt < columnsAt {
		t.Error("状态栏应当在主界面之后（贴着窗口底部）")
	}

	for _, wanted := range []string{
		"已登录：",
		"共享盘：",
		`<Tooltip title="切换身份">`,
		`aria-label="切换身份"`,
		"icon={<Users size={14} />}",
	} {
		if !strings.Contains(app, wanted) {
			t.Errorf("状态栏缺少：%s", wanted)
		}
	}
	if strings.Contains(app, "切换身份</Button>") {
		t.Error("「切换身份」应当是图标按钮，不再显示文字")
	}

	frame := cssBlock(t, styles, ".app-frame {")
	if !strings.Contains(frame, "flex-direction: column;") {
		t.Errorf(".app-frame 应当是两段式纵向布局，实际块：\n%s", frame)
	}
	bar := cssBlock(t, styles, ".status-bar {")
	for _, wanted := range []string{
		"flex: 0 0 auto;",
		"border-top: 1px solid var(--module-border);",
		"padding: 4px 12px;",
	} {
		if !strings.Contains(bar, wanted) {
			t.Errorf("状态栏样式缺少：%s，实际块：\n%s", wanted, bar)
		}
	}
}
