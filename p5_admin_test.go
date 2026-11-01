package main

// P5：运行状态的数据面（审计）、旧版本清理、人员名单页签。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jiegun314/ssr-go/internal/publish"
	"github.com/jiegun314/ssr-go/internal/share"
	"github.com/jiegun314/ssr-go/internal/store"
)

func TestReviewAuditShowsNewestFirstAndIsAdminOnly(t *testing.T) {
	app := newShareApp(t, "")
	if result := app.ImportSource("ra_input", filepath.Join("testdata", "sample-valid", "ra_input.xlsx")); result.Failed {
		t.Fatalf("导入失败：%+v", result)
	}
	review := app.ReviewAudit(1, 100, "")
	if review.Failed {
		t.Fatalf("读审计失败：%+v", review)
	}
	if len(review.Columns) != 9 {
		t.Errorf("审计列数 = %d；want 9", len(review.Columns))
	}
	if review.Total == 0 || len(review.Rows) == 0 {
		t.Fatal("审计应当有内容")
	}
	// 最新的在最前（导入在启动之后）
	if !strings.Contains(strings.Join(review.Rows[0], "|"), "import") {
		t.Errorf("第一条应当是最新的导入事件：%v", review.Rows[0])
	}

	// 业务人员不能看全部审计
	usersApp := newUsersApp(t)
	if login := usersApp.Login("ra", ""); login.Failed {
		t.Fatalf("登录失败：%+v", login)
	}
	denied := usersApp.ReviewAudit(1, 100, "")
	if !denied.Failed || !strings.Contains(denied.Message, "没有这个操作的权限") {
		t.Fatalf("RA 不该能看审计：%+v", denied)
	}
}

func TestCleanupShareVersionsKeepsLatestAndProtected(t *testing.T) {
	root := filepath.Join(t.TempDir(), "share")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := share.EnsureLayout(root); err != nil {
		t.Fatal(err)
	}
	// 造三个 RA 版本（用固定的 Now 保证时间戳递增）
	versions := []string{}
	for index := 0; index < 3; index++ {
		snapshot, err := publish.Publish(publish.Options{
			Root: root, Source: "ra_input", Columns: []string{"material_code"},
			Rows: []store.Row{{"material_code": "M" + string(rune('1'+index))}},
			Now:  time.Date(2026, 11, 1, 10, index, 0, 0, time.Local),
		})
		if err != nil {
			t.Fatalf("发布失败：%v", err)
		}
		versions = append(versions, snapshot.Version)
	}
	// 两个整合结果目录
	oldID := "20261101T090000"
	newID := "20261101T100000"
	for _, id := range []string{oldID, newID} {
		if err := os.MkdirAll(filepath.Join(root, "consolidated", id), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	app := newShareApp(t, root)
	result := app.CleanupShareVersions(1)
	if result.Failed {
		t.Fatalf("清理失败：%+v", result)
	}
	// 最新版本保留，前两个删除
	latest := versions[len(versions)-1]
	if _, err := os.Stat(filepath.Join(root, "published", "ra", latest+".csv")); err != nil {
		t.Errorf("最新版本应当保留：%v", err)
	}
	for _, version := range versions[:len(versions)-1] {
		if _, err := os.Stat(filepath.Join(root, "published", "ra", version+".csv")); !os.IsNotExist(err) {
			t.Errorf("旧版本应当被删除：%s", version)
		}
	}
	// 整合目录：保留最新一个，旧的删掉
	if _, err := os.Stat(filepath.Join(root, "consolidated", newID)); err != nil {
		t.Errorf("最新整合目录应当保留：%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "consolidated", oldID)); !os.IsNotExist(err) {
		t.Error("旧整合目录应当被删除")
	}

	// RA 不能清理（ManageShare 只给 Admin）
	usersApp := newUsersApp(t)
	if login := usersApp.Login("ra", ""); login.Failed {
		t.Fatalf("登录失败：%+v", login)
	}
	denied := usersApp.CleanupShareVersions(1)
	if !denied.Failed || !strings.Contains(denied.Message, "没有这个操作的权限") {
		t.Fatalf("RA 不该能清理：%+v", denied)
	}
}

func TestUsersTabOffersTemplateAndSaves(t *testing.T) {
	app := newShareApp(t, "")
	tabs := app.ConfigurationDocument()
	found := false
	for _, tab := range tabs {
		if tab.Key != "users.yaml" {
			continue
		}
		found = true
		if tab.Raw == "" || !strings.Contains(tab.Raw, "users:") {
			t.Errorf("没有 users.yaml 时应当给模板：%+v", tab)
		}
		if !strings.Contains(tab.Note, "单机模式") {
			t.Errorf("说明里应当提示当前是单机模式：%q", tab.Note)
		}
		saved := app.SaveConfigurationFile("users.yaml", tab.Raw)
		if saved.Failed {
			t.Fatalf("保存人员名单失败：%+v", saved)
		}
	}
	if !found {
		t.Fatal("参数设定里应当有「人员名单」页签")
	}
	if _, err := os.Stat(filepath.Join(app.loader.Resolver.ConfigDir, "users.yaml")); err != nil {
		t.Errorf("保存后应当生成 users.yaml：%v", err)
	}
}
