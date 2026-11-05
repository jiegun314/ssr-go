package app

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

// TestTheShareGateLocksImportAndOffersRecheck 固定 P2 的共享盘闸门在界面上的表现：
//   - 状态栏在"共享盘模式"下有「重新检查」图标（调 RecheckShare）；
//   - 共享盘异常时导入按钮锁定（ImportPanel 的 locked），整合按钮同样禁用；
//   - 导入成功的弹窗里展示"发布版本"。
//
// TestThePollingDoesNotFlashTheLoadingOverlay 固定"轮询不许闪载入图层"：
// 否则每 30 秒轮询一次 ShareOverview 都会闪一下遮罩，看起来就像界面在刷新。
func TestThePollingDoesNotFlashTheLoadingOverlay(t *testing.T) {
	bridge := readFrontendSource(t, "src/bridge.ts")
	if !strings.Contains(bridge, "const busyText = BUSY_TEXT[method];") {
		t.Error("call() 应当只对 BUSY_TEXT 里的长任务显示载入图层")
	}
	if !strings.Contains(bridge, "if (busyText) publishBusy(null);") {
		t.Error("只有显示过载入图层的调用才需要清空它")
	}
	app := readFrontendSource(t, "src/App.tsx")
	if !strings.Contains(app, "autoAlignInitialized") {
		t.Error("自动对齐开关应当只按配置初始化一次，不能被每轮询重置")
	}
}

func TestTheShareGateLocksImportAndOffersRecheck(t *testing.T) {
	app := readFrontendSource(t, "src/App.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	for _, wanted := range []string{
		`aria-label="重新检查共享盘"`,
		`call<ShareStatus>("RecheckShare")`,
		"locked={shareBlocked}",
		"发布版本",
	} {
		if !strings.Contains(app, wanted) {
			t.Errorf("共享盘闸门缺少：%s", wanted)
		}
	}
	if !strings.Contains(app, `share.mode === "share" && !share.healthy`) {
		t.Error("应当按「配置了共享盘且不健康」判定锁定")
	}
	if !strings.Contains(app, `disabled={shareBlocked || !allAligned}`) {
		t.Error("共享盘异常或数据未对齐时整合按钮应当禁用")
	}
	recheck := cssBlock(t, styles, ".status-recheck.ant-btn {")
	if !strings.Contains(recheck, "height: 20px;") {
		t.Errorf("重新检查图标应当与状态栏同高，实际块：\n%s", recheck)
	}
}
