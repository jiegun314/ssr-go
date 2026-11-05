package app

// P4：整合租约、结果归档到 consolidated/<id>/、block 策略。
//
//   - 导出成功后：文件复制到 consolidated/<id>/、写 summary.json、state.result 指向它；
//   - block 策略：10 分钟内有发布 → 整合前直接拒绝；
//   - 两人同时整合：第二个人被租约拦下（见 internal/share 的单测）。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportArchivesToConsolidatedFolder(t *testing.T) {
	shareRoot := filepath.Join(t.TempDir(), "share")
	if err := os.MkdirAll(shareRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	app := newShareApp(t, shareRoot)
	publishSamples(t, app)
	if result := app.AlignAll(); result.Failed {
		t.Fatalf("对齐失败：%+v", result)
	}
	if result := app.Consolidate(); result.Failed {
		t.Fatalf("整合失败：%+v", result)
	}
	target := filepath.Join(t.TempDir(), "result.xlsx")
	exported := app.Export("", target)
	if exported.Failed {
		t.Fatalf("导出失败：%+v", exported)
	}
	if exported.SharedPath == "" {
		t.Fatal("配置了共享盘时导出应当归档并返回相对路径")
	}
	if !strings.HasPrefix(exported.SharedPath, "consolidated/") {
		t.Errorf("归档路径应当在 consolidated/ 下：%s", exported.SharedPath)
	}
	archived := filepath.Join(shareRoot, filepath.FromSlash(exported.SharedPath))
	if _, err := os.Stat(archived); err != nil {
		t.Errorf("归档文件应当存在：%v", err)
	}
	summaryPath := filepath.Join(filepath.Dir(archived), "summary.json")
	content, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("summary.json 应当存在：%v", err)
	}
	var summary map[string]any
	if err := json.Unmarshal(content, &summary); err != nil {
		t.Fatalf("summary.json 不合法：%v", err)
	}
	if summary["file"] == "" || summary["versions"] == nil || summary["counts"] == nil {
		t.Errorf("summary.json 应当包含文件、版本与计数：%+v", summary)
	}
	// state.last_consolidation.result 指向归档文件
	state, err := app.shareState()
	if err != nil {
		t.Fatal(err)
	}
	if state.LastConsolidation == nil || state.LastConsolidation.Result != exported.SharedPath {
		t.Errorf("state.result 应当指向归档：%+v", state.LastConsolidation)
	}
}

func TestBlockStrategyRejectsRecentPublish(t *testing.T) {
	shareRoot := filepath.Join(t.TempDir(), "share")
	if err := os.MkdirAll(shareRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	app := newShareApp(t, shareRoot)
	publishSamples(t, app)
	if result := app.AlignAll(); result.Failed {
		t.Fatalf("对齐失败：%+v", result)
	}
	app.shareConfig.OnNewVersion = "block"
	result := app.Consolidate()
	if !result.Failed || !strings.Contains(result.Message, "最近 10 分钟") {
		t.Fatalf("block 策略下刚发布过应当拒绝整合：%+v", result)
	}
}
