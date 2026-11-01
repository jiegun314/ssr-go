package main

// P6：强制重跑、诊断包、断盘时的提示。

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/config"
)

func TestForceRerunExportsUnchangedRows(t *testing.T) {
	app := newShareApp(t, "")
	for _, source := range []string{"ra_input", "global_udi_input", "medical_insurance_code", "product_category"} {
		if result := app.ImportSource(source, filepath.Join("testdata", "sample-valid", source+".xlsx")); result.Failed {
			t.Fatalf("导入 %s 失败：%+v", source, result)
		}
	}
	if result := app.Consolidate(); result.Failed {
		t.Fatalf("首次整合失败：%+v", result)
	}
	if result := app.Export("", filepath.Join(t.TempDir(), "first.xlsx")); result.Failed {
		t.Fatalf("首次导出失败：%+v", result)
	}
	// 再整合一次：数据没变 → 全部判重复，导出应当失败
	second := app.Consolidate()
	if second.Failed {
		t.Fatalf("第二次整合失败：%+v", second)
	}
	duplicates := 0
	for _, status := range second.Statuses {
		if status == "Duplicate" {
			duplicates++
		}
	}
	if duplicates == 0 {
		t.Fatal("数据没变时应当判为重复")
	}
	if result := app.Export("", filepath.Join(t.TempDir(), "second.xlsx")); !result.Failed {
		t.Fatal("没有可导出数据时应当失败（这正是强制重跑要解决的）")
	}
	// 强制重跑：未变化的行也导出
	forced := app.ConsolidateForce()
	if forced.Failed {
		t.Fatalf("强制重跑失败：%+v", forced)
	}
	if !strings.Contains(forced.Notice, "强制重跑") {
		t.Errorf("强制重跑的提示不对：%q", forced.Notice)
	}
	exported := app.Export("", filepath.Join(t.TempDir(), "forced.xlsx"))
	if exported.Failed {
		t.Fatalf("强制重跑后应当能导出：%+v", exported)
	}
}

func TestDiagnosticsPackageMasksEmails(t *testing.T) {
	app := newShareApp(t, "")
	if result := app.ImportSource("ra_input", filepath.Join("testdata", "sample-valid", "ra_input.xlsx")); result.Failed {
		t.Fatalf("导入失败：%+v", result)
	}
	// 写一份带收件人的 workflow.yaml，用于验证打码
	var tab SettingsTab
	for _, item := range app.ConfigurationDocument() {
		if item.Key == config.WorkflowFile {
			tab = item
		}
	}
	content := strings.Replace(tab.Raw, "  ra_receivers: []",
		"  ra_receivers:\n    - \"ra.receiver@example.com\"", 1)
	if saved := app.SaveConfigurationFile(config.WorkflowFile, content); saved.Failed {
		t.Fatalf("保存收件人失败：%+v", saved)
	}

	result := app.ExportDiagnostics()
	if result.Failed {
		t.Fatalf("导出诊断包失败：%+v", result)
	}
	reader, err := zip.OpenReader(result.Path)
	if err != nil {
		t.Fatalf("诊断包打不开：%v", err)
	}
	defer reader.Close()
	entries := map[string]*zip.File{}
	for _, file := range reader.File {
		entries[file.Name] = file
	}
	for _, name := range []string{
		"version.txt", "share-status.json", "operation-log.txt", "audit.csv", "manifest.json",
		"config/" + config.SettingFile, "config/" + config.WorkflowFile,
	} {
		if entries[name] == nil {
			t.Errorf("诊断包缺少 %s", name)
		}
	}
	// 邮箱必须打码
	if workflow := entries["config/"+config.WorkflowFile]; workflow != nil {
		handle, _ := workflow.Open()
		content := make([]byte, workflow.UncompressedSize64)
		_, _ = handle.Read(content)
		handle.Close()
		if strings.Contains(string(content), "ra.receiver@example.com") {
			t.Error("诊断包里的邮箱应当打码")
		}
		if !strings.Contains(string(content), "***@***") {
			t.Error("打码后应当出现 ***@***")
		}
	}
}

func TestConsolidateRefusesWhenShareIsGone(t *testing.T) {
	shareRoot := filepath.Join(t.TempDir(), "share")
	if err := os.MkdirAll(shareRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	app := newShareApp(t, shareRoot)
	publishSamples(t, app)
	if result := app.AlignAll(); result.Failed {
		t.Fatalf("对齐失败：%+v", result)
	}
	moved := shareRoot + "-gone"
	if err := os.Rename(shareRoot, moved); err != nil {
		t.Fatal(err)
	}
	defer os.Rename(moved, shareRoot)
	result := app.Consolidate()
	if !result.Failed || !strings.Contains(result.Message, "共享盘不可用") {
		t.Fatalf("盘断了应当拒绝整合并给中文提示：%+v", result)
	}
}
