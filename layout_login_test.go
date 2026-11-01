package main

// 登录页的 Admin 方块：品牌红底、白字（其余组别保持白底描边）。
//
// 固定口径：类名只给 Admin（role.needsPassword），颜色只用 design token --brand-red，
// 悬停/按下仍是白字，避免"红底黑字"看不清。

import (
	"strings"
	"testing"
)

// TestTheLoginShowsTheSSRLogo 固定"登录页顶部有 SSR 图标"：
// 图来自 public/logo.png（随 dist 一起打包），必须有 alt（读屏软件可读）。
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

func TestTheAdminLoginTileIsRedWithWhiteText(t *testing.T) {
	screen := readFrontendSource(t, "src/components/LoginScreen.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	if !strings.Contains(screen, "role.needsPassword ? \" login-tile-admin\" : \"\"") {
		t.Error("只有 Admin 方块应当带 login-tile-admin 类")
	}

	rule := cssBlock(t, styles, ".login-tile-admin.ant-btn {")
	for _, wanted := range []string{
		"background: var(--brand-red);",
		"border-color: var(--brand-red);",
		"color: #fff;",
	} {
		if !strings.Contains(rule, wanted) {
			t.Errorf("Admin 方块应当红底白字，缺少：%s，实际块：\n%s", wanted, rule)
		}
	}
	hover := cssBlock(t, styles, ".login-tile-admin.ant-btn:hover,")
	if !strings.Contains(hover, "color: #fff;") {
		t.Errorf("Admin 方块悬停时仍要白字，实际块：\n%s", hover)
	}
}
