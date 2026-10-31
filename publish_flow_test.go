package main

// P2：导入成功后自动发布到共享盘；共享盘不可用时在导入之前就拦住。
//
//   - 配置了 share.root：导入 → 写 published/<部门>/<version>.csv + manifest + state.json；
//   - 共享盘不可达：导入被拒（"共享盘不可用"），审计记 deny；
//   - 没有配置 share.root：本地模式，导入照常、不发布。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/share"
)

// newShareApp 准备一个"单机模式 + 配置了共享盘"的工作区。
func newShareApp(t *testing.T, shareRoot string) *App {
	t.Helper()
	workspace := t.TempDir()
	copyDirectoryForTest(t, "config", filepath.Join(workspace, "config"))
	if shareRoot != "" {
		workflow := "share:\n  root: '" + shareRoot + "'\nsync:\n  auto: true\nemail:\n  transport: disabled\n"
		if err := os.WriteFile(
			filepath.Join(workspace, "config", "workflow.yaml"), []byte(workflow), 0o644,
		); err != nil {
			t.Fatalf("写 workflow.yaml 失败：%v", err)
		}
	}
	t.Setenv("UDI_CONFIG_DIR", filepath.Join(workspace, "config"))
	app := NewApp(nil)
	app.startup(context.Background())
	if app.startupError != "" {
		t.Fatalf("启动失败：%s", app.startupError)
	}
	return app
}

func TestImportPublishesToTheShareAndGatesWhenUnreachable(t *testing.T) {
	shareRoot := filepath.Join(t.TempDir(), "share")
	if err := os.MkdirAll(shareRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	app := newShareApp(t, shareRoot)
	sample := filepath.Join("testdata", "sample-valid", "ra_input.xlsx")

	result := app.ImportSource("ra_input", sample)
	if result.Failed {
		t.Fatalf("导入并发布应当成功：%+v", result)
	}
	if result.PublishedVersion == "" {
		t.Fatal("配置了共享盘时应当返回发布版本号")
	}

	// 快照与 manifest 落盘
	csvPath := filepath.Join(shareRoot, "published", "ra", result.PublishedVersion+".csv")
	manifestPath := filepath.Join(shareRoot, "published", "ra", result.PublishedVersion+".manifest.json")
	for _, path := range []string{csvPath, manifestPath} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("快照文件应当存在：%v", err)
		}
	}

	// state.json 记录了当前版本与发布人
	state, err := share.LoadState(shareRoot)
	if err != nil {
		t.Fatalf("读状态失败：%v", err)
	}
	version, ok := state.Sources["ra_input"]
	if !ok || version.Version != result.PublishedVersion {
		t.Fatalf("state 里的 RA 版本不对：%+v", state.Sources)
	}
	if version.PublishedBy.Role != "admin" || version.PublishedBy.Name == "" {
		t.Errorf("发布人应当记入 state：%+v", version.PublishedBy)
	}

	// 共享盘不可达：导入被拒（不是导入成功后再报发布失败）
	if err := os.RemoveAll(shareRoot); err != nil {
		t.Fatal(err)
	}
	blocked := app.ImportSource("ra_input", sample)
	if !blocked.Failed || !strings.Contains(blocked.Message, "共享盘不可用") {
		t.Fatalf("共享盘不可达时应当拒绝导入：%+v", blocked)
	}
	if blocked.PublishedVersion != "" {
		t.Errorf("被拒时不该有发布版本：%q", blocked.PublishedVersion)
	}

	// 审计：发布成功 + 拒绝
	rows, err := app.repo.AuditRows()
	if err != nil {
		t.Fatalf("读审计失败：%v", err)
	}
	seen := map[string]bool{}
	for _, row := range rows {
		seen[row["action"]+"/"+row["result"]] = true
	}
	for _, wanted := range []string{"publish/ok", "deny/denied"} {
		if !seen[wanted] {
			t.Errorf("审计里缺少 %s；已有：%v", wanted, seen)
		}
	}
}

func TestImportInLocalModeDoesNotPublish(t *testing.T) {
	app := newShareApp(t, "")
	sample := filepath.Join("testdata", "sample-valid", "ra_input.xlsx")
	result := app.ImportSource("ra_input", sample)
	if result.Failed {
		t.Fatalf("本地模式导入应当成功：%+v", result)
	}
	if result.PublishedVersion != "" {
		t.Errorf("没有配置共享盘时不该发布：%q", result.PublishedVersion)
	}
}
