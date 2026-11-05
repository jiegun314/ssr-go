package app

// 部署口径：新电脑、共享盘是一个空文件夹时，第一次登录（不论 Admin 还是普通角色）
// 都会自动创建目录骨架；Admin 选本地模式则完全不碰共享盘。

import (
	"os"
	"path/filepath"
	"testing"
)

func shareSkeleton(root string) map[string]bool {
	created := map[string]bool{}
	for _, directory := range []string{
		filepath.Join("published", "ra"),
		filepath.Join("published", "eama"),
		filepath.Join("published", "udi"),
		"state",
		"consolidated",
		"audit",
	} {
		if _, err := os.Stat(filepath.Join(root, directory)); err == nil {
			created[directory] = true
		}
	}
	return created
}

func TestFirstLoginCreatesTheShareSkeletonForEveryRole(t *testing.T) {
	// 普通角色登录
	rootA := filepath.Join(t.TempDir(), "share")
	if err := os.MkdirAll(rootA, 0o755); err != nil {
		t.Fatal(err)
	}
	appA := newUsersShareApp(t, rootA)
	if login := appA.Login("ra", ""); login.Failed {
		t.Fatalf("RA 登录失败：%+v", login)
	}
	if got := len(shareSkeleton(rootA)); got != 6 {
		t.Errorf("普通角色登录后应当创建 6 个骨架目录，实际 %d：%v", got, shareSkeleton(rootA))
	}
	// 登录时只有目录；状态文件要等第一次发布/整合
	if _, err := os.Stat(filepath.Join(rootA, "state", "state.json")); !os.IsNotExist(err) {
		t.Errorf("登录阶段不该生成 state.json：%v", err)
	}

	// Admin 联网模式登录
	rootB := filepath.Join(t.TempDir(), "share")
	if err := os.MkdirAll(rootB, 0o755); err != nil {
		t.Fatal(err)
	}
	appB := newUsersShareApp(t, rootB)
	if login := appB.LoginAdmin("SsrAdmin@2026", "online"); login.Failed {
		t.Fatalf("Admin 联网登录失败：%+v", login)
	}
	if got := len(shareSkeleton(rootB)); got != 6 {
		t.Errorf("Admin 联网登录后应当创建 6 个骨架目录，实际 %d：%v", got, shareSkeleton(rootB))
	}

	// Admin 本地模式：完全不碰共享盘
	rootC := filepath.Join(t.TempDir(), "share")
	if err := os.MkdirAll(rootC, 0o755); err != nil {
		t.Fatal(err)
	}
	appC := newUsersShareApp(t, rootC)
	if login := appC.LoginAdmin("SsrAdmin@2026", "local"); login.Failed {
		t.Fatalf("Admin 本地模式登录失败：%+v", login)
	}
	if got := shareSkeleton(rootC); len(got) != 0 {
		t.Errorf("Admin 本地模式不该在共享盘上创建任何目录：%v", got)
	}
}
