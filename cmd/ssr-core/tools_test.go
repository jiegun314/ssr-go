package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/config"
	"github.com/jiegun314/ssr-go/internal/store"
)

// TestLogColumnsAreGeneratedFromTheTemplate 对应 Python 的
// tests/test_log_columns.py::test_committed_log_columns_match_the_export_template：
// config/log_columns.yaml 必须等于「按模板表头生成」的结果，否则列会与模板漂移。
func TestLogColumnsAreGeneratedFromTheTemplate(t *testing.T) {
	loader, err := config.NewLoader(filepath.Join("..", "..", "config"))
	if err != nil {
		t.Fatalf("读配置失败：%v", err)
	}
	definition, err := BuildLogColumns(loader)
	if err != nil {
		t.Fatalf("生成日志列失败：%v", err)
	}
	committed, err := os.ReadFile(filepath.Join("..", "..", "config", "log_columns.yaml"))
	if err != nil {
		t.Fatalf("读已提交的 log_columns.yaml 失败：%v", err)
	}
	if RenderLogColumns(definition) != string(committed) {
		t.Fatal("config/log_columns.yaml 与模板表头不一致：请运行 genlogcolumns")
	}
	// 分组口径：log_time + status，模板表头 75 列，material_code + operation + details
	if len(definition.Before) != 2 || len(definition.Columns) != 75 || len(definition.After) != 3 {
		t.Fatalf("分组 = before %d / columns %d / after %d",
			len(definition.Before), len(definition.Columns), len(definition.After))
	}
	if definition.Version != "1.0" {
		t.Errorf("version = %q", definition.Version)
	}
}

// TestGenerateLogColumnsIsIdempotent 重新生成两次的结果必须一致（生成物不是手写文件）。
func TestGenerateLogColumnsIsIdempotent(t *testing.T) {
	source := filepath.Join("..", "..", "config")
	target := t.TempDir()
	for _, name := range []string{
		config.SettingFile, config.ImportMappingFile,
		config.ConsolidationMappingFile, config.LogColumnsFile,
	} {
		content, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			t.Fatalf("读 %s 失败：%v", name, err)
		}
		if err := os.WriteFile(filepath.Join(target, name), content, 0o644); err != nil {
			t.Fatalf("写 %s 失败：%v", name, err)
		}
	}
	// 模板要被 genlogcolumns 读到，所以把真实模板放到工作区的配置根下
	templateSource := filepath.Join("..", "..", "data", "template", "SingleSource-DCF-Row-China-UDI.xlsx")
	templateTarget := filepath.Join(
		target, "..", "data", "template", "SingleSource-DCF-Row-China-UDI.xlsx")
	if err := os.MkdirAll(filepath.Dir(templateTarget), 0o755); err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	content, err := os.ReadFile(templateSource)
	if err != nil {
		t.Fatalf("读模板失败：%v", err)
	}
	if err := os.WriteFile(templateTarget, content, 0o644); err != nil {
		t.Fatalf("写模板失败：%v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{
		"genlogcolumns", "--config", target,
	}, &stdout, &stderr); code != 0 {
		t.Fatalf("生成失败（%d）：%s", code, stderr.String())
	}
	first, err := os.ReadFile(filepath.Join(target, config.LogColumnsFile))
	if err != nil {
		t.Fatalf("读生成结果失败：%v", err)
	}
	if code := run([]string{
		"genlogcolumns", "--config", target, "--check",
	}, &stdout, &stderr); code != 0 {
		t.Fatalf("自检失败（%d）：%s", code, stderr.String())
	}
	second, err := os.ReadFile(filepath.Join(target, config.LogColumnsFile))
	if err != nil {
		t.Fatalf("读生成结果失败：%v", err)
	}
	if string(first) != string(second) {
		t.Fatal("生成物不稳定")
	}
	if !strings.Contains(string(first), "before:") ||
		!strings.Contains(string(first), "after:") {
		t.Fatal("生成的内容缺少分组")
	}
}

// TestPrepareDatabaseCreatesAnEmptyLogTable 对应 Python 的
// tests/test_release_database.py：发布包空库只有一张空的 operation_log，列为配置顺序。
func TestPrepareDatabaseCreatesAnEmptyLogTable(t *testing.T) {
	outputRoot := t.TempDir()
	var stdout, stderr bytes.Buffer

	code := run([]string{
		"preparedb",
		"--config", filepath.Join("..", "..", "config"),
		"--output-root", outputRoot,
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("preparedb 失败（%d）：%s", code, stderr.String())
	}
	loader, err := config.NewLoader(filepath.Join("..", "..", "config"))
	if err != nil {
		t.Fatalf("读配置失败：%v", err)
	}
	setting, err := loader.LoadSetting()
	if err != nil {
		t.Fatalf("读 setting.yaml 失败：%v", err)
	}
	database, _ := setting["database"].(map[string]any)
	databasePath, _ := database["path"].(string)
	repository, err := store.Open(filepath.Join(outputRoot, databasePath))
	if err != nil {
		t.Fatalf("打开生成的数据库失败：%v", err)
	}
	defer repository.Close()

	names, err := repository.TableNames()
	if err != nil {
		t.Fatalf("列表名失败：%v", err)
	}
	if len(names) != 1 || names[0] != "operation_log" {
		t.Fatalf("发布包空库应只有 operation_log：%v", names)
	}
	expected, err := loader.LoadLogColumns()
	if err != nil {
		t.Fatalf("读日志列失败：%v", err)
	}
	columns, err := repository.TableColumns("operation_log")
	if err != nil {
		t.Fatalf("读列失败：%v", err)
	}
	if strings.Join(columns, "|") != strings.Join(expected, "|") {
		t.Fatalf("列与配置不一致：%d 列", len(columns))
	}
	count, err := repository.CountTableRows("operation_log")
	if err != nil || count != 0 {
		t.Fatalf("空库的日志表应为空（count=%d, err=%v）", count, err)
	}
}
