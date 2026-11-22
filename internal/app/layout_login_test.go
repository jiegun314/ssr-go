package app

// 登录页：左右分栏，右侧 2×2 组别卡；管理员入口是分隔线下的低强调文本按钮，
// 不再使用红底大方块。

import (
	"strings"
	"testing"
)

// TestTheLoginShowsTheSSRLogo 固定"登录页顶部有 SSR 图标"：
// 图来自 public/logo.png（随 dist 一起打包），必须有 alt（读屏软件可读）。
// TestTheAdminLoginOffersOnlineAndLocalModes 固定管理员登录的运行模式选择：
// 默认联网；本地模式必须先确认警告；警告选"否"回到身份选择。
func TestTheAdminLoginOffersOnlineAndLocalModes(t *testing.T) {
	screen := readFrontendSource(t, "src/components/LoginScreen.tsx")
	app := readFrontendSource(t, "src/App.tsx")
	for _, wanted := range []string{
		"联网模式（默认）",
		"本地模式",
		"本地模式不会使用共享盘",
		"否（返回身份选择）",
		"确认进入本地模式",
		"onAdminLogin",
	} {
		if !strings.Contains(screen, wanted) {
			t.Errorf("登录页缺少运行模式相关的界面：%s", wanted)
		}
	}
	if !strings.Contains(app, `call<LoginResult>("LoginAdmin", password, mode)`) {
		t.Error("App 应当通过 LoginAdmin(password, mode) 登录管理员")
	}
}

func TestTheLoginShowsTheSSRLogo(t *testing.T) {
	screen := readFrontendSource(t, "src/components/LoginScreen.tsx")
	styles := readFrontendSource(t, "src/styles.css")
	if !strings.Contains(screen, `className="login-logo" src="/logo.png" alt="SSR"`) {
		t.Error("登录页顶部应当有 SSR logo（/logo.png + alt=SSR）")
	}
	logo := cssBlock(t, styles, ".login-logo {")
	for _, wanted := range []string{"width: 72px;", "height: 72px;"} {
		if !strings.Contains(logo, wanted) {
			t.Errorf("登录页 logo 的尺寸应当是 72×72（缺 %s），实际块：\n%s", wanted, logo)
		}
	}
}

func TestTheLoginUsesASplitPanelAndQuietAdminEntry(t *testing.T) {
	screen := readFrontendSource(t, "src/components/LoginScreen.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	for _, wanted := range []string{
		`<aside className="login-brand">`,
		`<section className="login-access">`,
		`<div className="login-role-grid">{mainRoles.map(tile)}</div>`,
		`className="login-admin-entry"`,
		`aria-label={adminRole.label}`,
		`className="login-tile-copy"`,
	} {
		if !strings.Contains(screen, wanted) {
			t.Errorf("登录页缺少分栏结构或低强调管理员入口：%s", wanted)
		}
	}
	for _, unwanted := range []string{"login-row-main", "login-row-admin", "login-tile-admin"} {
		if strings.Contains(screen, unwanted) {
			t.Errorf("登录页仍在用旧布局类名：%s", unwanted)
		}
	}

	panel := cssBlock(t, styles, ".login-panel {")
	for _, wanted := range []string{
		"display: grid;",
		"grid-template-columns: minmax(0, 1.05fr) minmax(360px, 0.95fr);",
		"border-radius: 16px;",
	} {
		if !strings.Contains(panel, wanted) {
			t.Errorf("登录面板应当是左右分栏的圆角浮层，缺少：%s，实际块：\n%s", wanted, panel)
		}
	}

	grid := cssBlock(t, styles, ".login-role-grid {")
	if !strings.Contains(grid, "grid-template-columns: repeat(2, minmax(0, 1fr));") {
		t.Errorf("组别卡应当是 2×2 网格，实际块：\n%s", grid)
	}

	tile := cssBlock(t, styles, ".login-tile.ant-btn {")
	for _, wanted := range []string{"height: 72px;", "border-radius: 10px;"} {
		if !strings.Contains(tile, wanted) {
			t.Errorf("组别卡应当是紧凑横向卡片，缺少：%s，实际块：\n%s", wanted, tile)
		}
	}

	admin := cssBlock(t, styles, ".login-admin-entry.ant-btn {")
	if !strings.Contains(admin, "background: transparent;") {
		t.Errorf("管理员入口应当是低强调透明按钮，实际块：\n%s", admin)
	}
	if strings.Contains(admin, "var(--brand-red)") {
		t.Errorf("管理员入口不应再铺品牌红，实际块：\n%s", admin)
	}
}
