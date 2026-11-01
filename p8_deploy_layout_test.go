package main

// 部署口径（发布前最后确认）：第一次运行、共享盘目录已由 IT 建好但为空时，
// 程序会在第一次健康检查通过后补齐自己的目录骨架；根目录不存在时要明确报错、不乱建。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFirstRunCreatesTheShareSkeleton(t *testing.T) {
	root := filepath.Join(t.TempDir(), "share")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	app := newShareApp(t, root)
	status := app.RecheckShare()
	if !status.Healthy {
		t.Fatalf("空目录应当检查通过：%+v", status)
	}
	for _, directory := range []string{
		filepath.Join("published", "ra"),
		filepath.Join("published", "eama"),
		filepath.Join("published", "udi"),
		"state",
		"consolidated",
		"audit",
	} {
		if info, err := os.Stat(filepath.Join(root, directory)); err != nil || !info.IsDir() {
			t.Errorf("第一次检查通过后应当自动创建 %s：%v", directory, err)
		}
	}

	// 导入 → 发布目录与 state.json 自动出现（含原始 xlsx 留档）
	result := app.ImportSource("ra_input", filepath.Join("testdata", "sample-valid", "ra_input.xlsx"))
	if result.Failed {
		t.Fatalf("导入失败：%+v", result)
	}
	entries, err := os.ReadDir(filepath.Join(root, "published", "ra"))
	if err != nil {
		t.Fatal(err)
	}
	suffixes := map[string]bool{}
	for _, entry := range entries {
		switch {
		case strings.HasSuffix(entry.Name(), ".csv"):
			suffixes["csv"] = true
		case strings.HasSuffix(entry.Name(), ".manifest.json"):
			suffixes["manifest"] = true
		case strings.HasSuffix(entry.Name(), ".xlsx"):
			suffixes["xlsx"] = true
		}
	}
	for _, wanted := range []string{"csv", "manifest", "xlsx"} {
		if !suffixes[wanted] {
			t.Errorf("published/ra 下应有 %s：%v", wanted, entries)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "state", "state.json")); err != nil {
		t.Errorf("发布后应有 state/state.json：%v", err)
	}
}

func TestFirstRunWithMissingRootReportsClearly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-created-yet")
	app := newShareApp(t, root)
	status := app.RecheckShare()
	if status.Healthy {
		t.Fatalf("根目录不存在时不应当报健康：%+v", status)
	}
	if !strings.Contains(status.Detail, "共享盘不可达") {
		t.Errorf("提示应当说明共享盘不可达：%q", status.Detail)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Errorf("程序不该替用户创建根目录：%v", err)
	}
}
