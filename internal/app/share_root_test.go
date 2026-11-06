package app

// 共享盘位置：所有角色都能改这一项；其它配置仍然只有 Admin 能改。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/auth"
	"github.com/jiegun314/ssr-go/internal/config"
)

func TestEveryRoleMaySetShareRoot(t *testing.T) {
	for _, role := range []auth.Role{auth.RoleRA, auth.RoleEAMA, auth.RoleUDI, auth.RoleSupplyChain, auth.RoleAdmin} {
		if !auth.Can(role, auth.ActionSetShareRoot) {
			t.Errorf("%s 应当可以设置共享盘位置", role)
		}
		if role != auth.RoleAdmin && auth.Can(role, auth.ActionEditSettings) {
			t.Errorf("%s 不该能修改完整配置", role)
		}
	}
}

func TestNonAdminCanSetShareRootAndNothingElse(t *testing.T) {
	initial := filepath.Join(t.TempDir(), "share")
	if err := os.MkdirAll(initial, 0o755); err != nil {
		t.Fatal(err)
	}
	app := newUsersShareApp(t, initial)
	if login := app.Login("ra", ""); login.Failed {
		t.Fatalf("RA 登录失败：%+v", login)
	}
	if info := app.GetShareRoot(); info.Root != initial {
		t.Errorf("当前共享盘位置不对：%+v", info)
	}

	next := filepath.Join(t.TempDir(), "share-next")
	if err := os.MkdirAll(next, 0o755); err != nil {
		t.Fatal(err)
	}
	result := app.SaveShareRoot(next)
	if result.Failed {
		t.Fatalf("RA 保存共享盘位置失败：%+v", result)
	}
	if app.shareConfig.Root != next {
		t.Errorf("热重载后共享盘位置没变：%s", app.shareConfig.Root)
	}
	content, err := os.ReadFile(filepath.Join(app.loader.Resolver.ConfigDir, config.WorkflowFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "root: '"+next+"'") {
		t.Errorf("workflow.yaml 里没有新位置：\n%s", content)
	}

	// 其它设置（完整配置）仍然只有 Admin 能改
	if denied := app.SaveSettingsValues(config.SettingFile, []SettingsChange{
		{Path: []string{"APP_NAME"}, Value: "x"},
	}); !denied.Failed {
		t.Error("RA 不该能修改 setting.yaml")
	}
}

func TestShareRootRejectsOnlineSharePointAddress(t *testing.T) {
	root := filepath.Join(t.TempDir(), "share")
	os.MkdirAll(root, 0o755)
	app := newUsersShareApp(t, root)
	app.Login("ra", "")
	result := app.SaveShareRoot("https://contoso.sharepoint.com/sites/SSR/Shared Documents")
	if !result.Failed || !strings.Contains(result.Message, "OneDrive") {
		t.Errorf("SharePoint 在线地址应当被拒绝并提示先同步：%+v", result)
	}
}

func TestAdminCanStillEditFullSettings(t *testing.T) {
	root := filepath.Join(t.TempDir(), "share")
	os.MkdirAll(root, 0o755)
	app := newUsersShareApp(t, root)
	app.Login("admin", "SsrAdmin@2026")
	if result := app.SaveShareRoot(root); result.Failed {
		t.Fatalf("Admin 保存共享盘位置失败：%+v", result)
	}
}
