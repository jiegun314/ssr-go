package main

// 管理员登录时的运行模式：默认联网（用共享盘）；选本地模式要确认警告，
// 本次会话不发布、不对齐、不门禁；退出后恢复默认联网。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newUsersShareApp(t *testing.T, shareRoot string) *App {
	t.Helper()
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, "config")
	copyDirectoryForTest(t, "config", configDir)
	if err := os.WriteFile(filepath.Join(configDir, "users.yaml"), []byte(testUsersYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	workflow := "share:\n  root: '" + shareRoot + "'\n"
	if err := os.WriteFile(filepath.Join(configDir, "workflow.yaml"), []byte(workflow), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UDI_CONFIG_DIR", configDir)
	app := NewApp(nil)
	app.startup(context.Background())
	return app
}

func TestAdminLoginLocalModeSkipsShare(t *testing.T) {
	shareRoot := filepath.Join(t.TempDir(), "share")
	if err := os.MkdirAll(shareRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	app := newUsersShareApp(t, shareRoot)

	login := app.LoginAdmin("SsrAdmin@2026", "local")
	if login.Failed || login.Session == nil || login.Session.Role != "admin" {
		t.Fatalf("管理员本地模式登录失败：%+v", login)
	}
	if app.shareStatus.Mode != "local" {
		t.Errorf("本地模式登录后共享盘状态应当为 local：%+v", app.shareStatus)
	}
	if app.ShareOverview().Enabled {
		t.Error("本地模式不应启用共享盘总览")
	}

	// 把共享盘"拔掉"后导入：本地模式不检查共享盘，导入照常成功且不发布
	moved := shareRoot + "-off"
	if err := os.Rename(shareRoot, moved); err != nil {
		t.Fatal(err)
	}
	defer os.Rename(moved, shareRoot)
	result := app.ImportSource("ra_input", filepath.Join("testdata", "sample-valid", "ra_input.xlsx"))
	if result.Failed {
		t.Fatalf("本地模式导入不应被共享盘拦住：%+v", result)
	}
	if result.PublishedVersion != "" {
		t.Errorf("本地模式不应发布到共享盘：%s", result.PublishedVersion)
	}
}

func TestAdminLoginOnlineModeUsesShare(t *testing.T) {
	shareRoot := filepath.Join(t.TempDir(), "share")
	if err := os.MkdirAll(shareRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	app := newUsersShareApp(t, shareRoot)

	login := app.LoginAdmin("SsrAdmin@2026", "online")
	if login.Failed || login.Session == nil || login.Session.Role != "admin" {
		t.Fatalf("管理员联网模式登录失败：%+v", login)
	}
	if app.shareStatus.Mode != "share" {
		t.Errorf("联网模式登录后共享盘状态应当为 share：%+v", app.shareStatus)
	}
	result := app.ImportSource("ra_input", filepath.Join("testdata", "sample-valid", "ra_input.xlsx"))
	if result.Failed || result.PublishedVersion == "" {
		t.Fatalf("联网模式导入应当发布：%+v", result)
	}
	if _, err := os.Stat(filepath.Join(shareRoot, "published", "ra")); err != nil {
		t.Errorf("共享盘上应当有发布目录：%v", err)
	}
}

func TestAdminLoginModeValidationAndReset(t *testing.T) {
	shareRoot := filepath.Join(t.TempDir(), "share")
	if err := os.MkdirAll(shareRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	app := newUsersShareApp(t, shareRoot)

	if login := app.LoginAdmin("wrong-password", "local"); !login.Failed {
		t.Fatal("密码错误时应当拒绝登录")
	}
	if login := app.LoginAdmin("SsrAdmin@2026", "nonsense"); !login.Failed ||
		!strings.Contains(login.Message, "运行模式不合法") {
		t.Fatalf("非法模式应当被拒绝：%+v", login)
	}
	if login := app.LoginAdmin("SsrAdmin@2026", "local"); login.Failed {
		t.Fatalf("本地模式登录失败：%+v", login)
	}
	// 退出后恢复默认联网（老登录入口也是联网）
	app.Logout()
	if login := app.Login("admin", "SsrAdmin@2026"); login.Failed {
		t.Fatalf("重新登录失败：%+v", login)
	}
	if app.shareStatus.Mode != "share" {
		t.Errorf("退出后应恢复联网模式：%+v", app.shareStatus)
	}
}
