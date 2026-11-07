package app

// 人员名单同步：Admin 保存后发布到共享盘；其它电脑启动/登录时拉取。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/config"
	"github.com/jiegun314/ssr-go/internal/usersync"
)

const syncedUsersYAML = `users:
  RA:
    name: "张三"
    email: "zhangsan@example.com"
  UDI:
    name: "王五"
    email: "wangwu@example.com"
  SUPPLY CHAIN:
    name: "赵六"
    email: "zhaoliu@example.com"
  ADMIN:
    name: "管理员"
    email: "admin@example.com"
`

func TestAdminSavePublishesUsersToShare(t *testing.T) {
	shareRoot := filepath.Join(t.TempDir(), "share")
	if err := os.MkdirAll(shareRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	app := newUsersShareApp(t, shareRoot)
	if login := app.LoginAdmin("SsrAdmin@2026", "online"); login.Failed {
		t.Fatalf("Admin 登录失败：%+v", login)
	}
	result := app.SaveConfigurationFile(config.UsersFile, syncedUsersYAML)
	if result.Failed {
		t.Fatalf("保存 users.yaml 失败：%+v", result)
	}
	if !strings.Contains(result.Message, "已同步到共享盘") {
		t.Errorf("保存结果应当说明已同步：%s", result.Message)
	}
	content, manifest, err := usersync.Fetch(shareRoot)
	if err != nil {
		t.Fatalf("共享盘上应当有人员名单：%v", err)
	}
	if !strings.Contains(string(content), "王五") || manifest.UpdatedBy == "" {
		t.Errorf("共享盘内容/manifest 不对：%s / %+v", content, manifest)
	}
}

func TestOtherMachinePullsUsersOnStartup(t *testing.T) {
	shareRoot := filepath.Join(t.TempDir(), "share")
	if err := os.MkdirAll(shareRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	// 管理员机器发布新名单
	admin := newUsersShareApp(t, shareRoot)
	admin.LoginAdmin("SsrAdmin@2026", "online")
	if result := admin.SaveConfigurationFile(config.UsersFile, syncedUsersYAML); result.Failed {
		t.Fatalf("发布人员名单失败：%+v", result)
	}

	// 另一台机器：启动时拉取（本地还是默认名单）
	other := newUsersShareApp(t, shareRoot)
	found := false
	for _, user := range other.users {
		if user.Name == "王五" && user.Email == "wangwu@example.com" {
			found = true
		}
	}
	if !found {
		t.Errorf("启动后应当拉到管理员发布的新名单：%+v", other.users)
	}
	content, err := os.ReadFile(filepath.Join(other.loader.Resolver.ConfigDir, config.UsersFile))
	if err != nil || !strings.Contains(string(content), "王五") {
		t.Errorf("本机 users.yaml 应被更新：%v", err)
	}
	// 再启动一次不应重复改写（标记命中）
	before, _ := os.Stat(filepath.Join(other.loader.Resolver.ConfigDir, config.UsersFile))
	again := newUsersShareApp(t, shareRoot)
	after, _ := os.Stat(filepath.Join(again.loader.Resolver.ConfigDir, config.UsersFile))
	if before == nil || after == nil {
		t.Fatal("读不到 users.yaml")
	}
}

func TestPendingUsersAreNotOverwritten(t *testing.T) {
	shareRoot := filepath.Join(t.TempDir(), "share")
	if err := os.MkdirAll(shareRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	app := newUsersShareApp(t, shareRoot)
	// 本机改过名单并打了 pending（模拟离线保存）
	app.writeUsersMarker(usersMarker{SHA256: "local-hash", Pending: true})
	app.syncUsersFromShare("startup")
	// 远端放一份不同的名单
	remote := []byte(syncedUsersYAML)
	if _, err := usersync.Publish(shareRoot, remote, "admin"); err != nil {
		t.Fatal(err)
	}
	app.syncUsersFromShare("startup")
	for _, user := range app.users {
		if user.Name == "王五" {
			t.Error("pending 状态下不该被共享盘覆盖")
		}
	}
}

func TestAdminPendingIsRetriedOnNextLogin(t *testing.T) {
	shareRoot := filepath.Join(t.TempDir(), "share")
	if err := os.MkdirAll(shareRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	app := newUsersShareApp(t, shareRoot)
	app.LoginAdmin("SsrAdmin@2026", "online")
	// 共享盘暂时移走：保存本机名单 → 发布失败 → pending
	moved := shareRoot + "-off"
	if err := os.Rename(shareRoot, moved); err != nil {
		t.Fatal(err)
	}
	result := app.SaveConfigurationFile(config.UsersFile, syncedUsersYAML)
	if result.Failed {
		t.Fatalf("本机保存不应失败：%+v", result)
	}
	if !strings.Contains(result.Message, "同步到共享盘失败") {
		t.Errorf("应当提示共享盘同步失败：%s", result.Message)
	}
	if marker, ok := app.readUsersMarker(); !ok || !marker.Pending {
		t.Errorf("应当留下 pending 标记：%+v / %v", marker, ok)
	}
	// 共享盘恢复后，Admin 再登录会重试发布
	if err := os.Rename(moved, shareRoot); err != nil {
		t.Fatal(err)
	}
	app.LoginAdmin("SsrAdmin@2026", "online")
	if _, _, err := usersync.Fetch(shareRoot); err != nil {
		t.Errorf("重新登录后应当已发布：%v", err)
	}
	if marker, _ := app.readUsersMarker(); marker.Pending {
		t.Error("发布成功后不应还是 pending")
	}
}
