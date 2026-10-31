package main

// P1：登录、角色权限与审计的端到端契约。
//
//   - 有 users.yaml → 启动先登录；不同角色只能做自己的事（后端拒绝，不只是藏按钮）；
//   - Admin 需要密码（密码写死在 internal/auth）；
//   - 没有 users.yaml → 单机模式，自动以 Admin 进入（旧行为不变）；
//   - 登录、拒绝、导入都会写 audit_log。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testUsersYAML = `users:
  - name: "张三"
    role: "ra"
    email: "zhangsan@example.com"
  - name: "赵六"
    role: "supply_chain"
    email: "zhaoliu@example.com"
  - name: "管理员"
    role: "admin"
    email: "admin@example.com"
`

// newUsersApp 准备一个带 users.yaml 的工作区并完成启动（未登录）。
func newUsersApp(t *testing.T) *App {
	t.Helper()
	workspace := t.TempDir()
	copyDirectoryForTest(t, "config", filepath.Join(workspace, "config"))
	if err := os.WriteFile(
		filepath.Join(workspace, "config", "users.yaml"), []byte(testUsersYAML), 0o644,
	); err != nil {
		t.Fatalf("写 users.yaml 失败：%v", err)
	}
	t.Setenv("UDI_CONFIG_DIR", filepath.Join(workspace, "config"))
	app := NewApp(nil)
	app.startup(context.Background())
	if app.startupError != "" {
		t.Fatalf("启动失败：%s", app.startupError)
	}
	return app
}

func containsAction(actions []string, wanted string) bool {
	for _, action := range actions {
		if action == wanted {
			return true
		}
	}
	return false
}

func TestLoginRolesGateTheActions(t *testing.T) {
	app := newUsersApp(t)

	initial := app.InitialState()
	if initial.SingleUser {
		t.Fatal("有 users.yaml 时不该是单机模式")
	}
	if initial.Session != nil {
		t.Fatal("有 users.yaml 时启动后应当还没有会话（先登录）")
	}
	// 登录页按组别（固定 5 个），不按人名；用户名用于 Welcome, XXX
	if len(initial.Roles) != 5 {
		t.Fatalf("登录页组别数 = %d；want 5", len(initial.Roles))
	}
	if initial.Username == "" {
		t.Error("应当带上本机用户名（Welcome, XXX）")
	}
	if initial.Roles[4].Key != "admin" || !initial.Roles[4].NeedsPassword {
		t.Errorf("最后一行应当是 Admin 且需要密码：%+v", initial.Roles[4])
	}

	// 未登录：任何动作都拒绝
	if result := app.Consolidate(); !result.Failed || !strings.Contains(result.Message, "请先选择登录身份") {
		t.Fatalf("未登录整合应当被拒：%+v", result)
	}

	// RA 登录（按组别）：只能导入 RA
	login := app.Login("ra", "")
	if login.Failed || login.Session == nil {
		t.Fatalf("RA 登录失败：%+v", login)
	}
	if login.Session.Role != "ra" || !containsAction(login.Session.Actions, "import:ra_input") {
		t.Fatalf("RA 会话不对：%+v", login.Session)
	}
	if containsAction(login.Session.Actions, "consolidate") {
		t.Error("RA 不该有整合权限")
	}
	udiSample := filepath.Join("testdata", "sample-valid", "global_udi_input.xlsx")
	if result := app.ImportSource("global_udi_input", udiSample); !result.Failed ||
		!strings.Contains(result.Message, "没有这个来源的权限") {
		t.Fatalf("RA 导入 UDI 应当被拒：%+v", result)
	}
	if result := app.Consolidate(); !result.Failed || !strings.Contains(result.Message, "没有这个操作的权限") {
		t.Fatalf("RA 整合应当被拒：%+v", result)
	}
	if result := app.ImportSource("ra_input", filepath.Join("testdata", "sample-valid", "ra_input.xlsx")); result.Failed {
		t.Fatalf("RA 导入自己的数据应当成功：%+v", result)
	}

	// 切换身份：Admin 需要密码
	if result := app.Login("admin", "wrong"); !result.Failed || !strings.Contains(result.Message, "密码不正确") {
		t.Fatalf("错误的 Admin 密码应当被拒：%+v", result)
	}
	admin := app.Login("admin", "SsrAdmin@2026")
	if admin.Failed || admin.Session == nil || admin.Session.Role != "admin" {
		t.Fatalf("Admin 登录失败：%+v", admin)
	}
	if !containsAction(admin.Session.Actions, "consolidate") ||
		!containsAction(admin.Session.Actions, "settings:write") {
		t.Errorf("Admin 应当有全部动作：%v", admin.Session.Actions)
	}

	// 审计里有登录、拒绝、导入
	rows, err := app.repo.AuditRows()
	if err != nil {
		t.Fatalf("读审计失败：%v", err)
	}
	seen := map[string]bool{}
	for _, row := range rows {
		seen[row["action"]+"/"+row["result"]] = true
	}
	for _, wanted := range []string{"login/ok", "login/denied", "deny/denied", "import/ok"} {
		if !seen[wanted] {
			t.Errorf("审计里缺少 %s；已有：%v", wanted, seen)
		}
	}
}

func TestSingleUserModeNeedsNoLogin(t *testing.T) {
	workspace := t.TempDir()
	copyDirectoryForTest(t, "config", filepath.Join(workspace, "config"))
	t.Setenv("UDI_CONFIG_DIR", filepath.Join(workspace, "config"))
	app := NewApp(nil)
	app.startup(context.Background())

	initial := app.InitialState()
	if !initial.SingleUser {
		t.Fatal("没有 users.yaml 时应当进入单机模式")
	}
	if initial.Session == nil || initial.Session.Role != "admin" {
		t.Fatalf("单机模式应当自动以 Admin 进入：%+v", initial.Session)
	}
	if !initial.Session.SingleUser {
		t.Error("单机模式的会话应当标记 SingleUser（前端据此跳过登录页）")
	}
	if result := app.ImportSource("ra_input", filepath.Join("testdata", "sample-valid", "ra_input.xlsx")); result.Failed {
		t.Fatalf("单机模式导入应当照常可用：%+v", result)
	}
}

func TestUnknownRoleIsRejected(t *testing.T) {
	app := newUsersApp(t)
	result := app.Login("boss", "")
	if !result.Failed || !strings.Contains(result.Message, "找不到这个组别") {
		t.Fatalf("未知组别应当被拒：%+v", result)
	}
}
