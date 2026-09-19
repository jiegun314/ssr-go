package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// copyTree 把配置与模板复制进临时工作区（导入与整合都按配置根解析路径）。
func copyTree(t *testing.T, source string, target string) {
	t.Helper()
	entries, err := os.ReadDir(source)
	if err != nil {
		t.Fatalf("读目录 %s 失败：%v", source, err)
	}
	for _, entry := range entries {
		from := filepath.Join(source, entry.Name())
		to := filepath.Join(target, entry.Name())
		if entry.IsDir() {
			if err := os.MkdirAll(to, 0o755); err != nil {
				t.Fatalf("建目录失败：%v", err)
			}
			copyTree(t, from, to)
			continue
		}
		content, err := os.ReadFile(from)
		if err != nil {
			t.Fatalf("读 %s 失败：%v", from, err)
		}
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			t.Fatalf("建目录失败：%v", err)
		}
		if err := os.WriteFile(to, content, 0o644); err != nil {
			t.Fatalf("写 %s 失败：%v", to, err)
		}
	}
}

// TestGeneratedSamplesReproduceTheDocumentedOutcome 对应 Python 的
// tests/test_sample_data.py：Go 生成的样本必须能跑出文档写明的结果。
func TestGeneratedSamplesReproduceTheDocumentedOutcome(t *testing.T) {
	workspace := t.TempDir()
	copyTree(t, filepath.Join("..", "..", "config"), filepath.Join(workspace, "config"))
	copyTree(t, filepath.Join("..", "..", "data"), filepath.Join(workspace, "data"))
	configDir := filepath.Join(workspace, "config")
	var stdout, stderr bytes.Buffer

	if code := run([]string{
		"gensample", "--config", configDir, "--output", "data/input/sample", "--rows", "5",
	}, &stdout, &stderr); code != 0 {
		t.Fatalf("gensample 失败（%d）：%s", code, stderr.String())
	}
	sampleRoot := filepath.Join(workspace, "data", "input", "sample")
	expectedRows := map[string]string{
		"ra_input": "6", "global_udi_input": "9",
		"medical_insurance_code": "6", "product_category": "6",
	}
	for sourceName, expected := range expectedRows {
		stdout.Reset()
		stderr.Reset()
		code := run([]string{
			"import", "--config", configDir, "--source", sourceName,
			"--file", filepath.Join(sampleRoot, "valid", sourceName+".xlsx"),
		}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("导入 %s 失败（%d）：%s", sourceName, code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "Rows imported: "+expected) {
			t.Errorf("%s 的行数不对：%s", sourceName, stdout.String())
		}
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"consolidate", "--config", configDir},
		&stdout, &stderr); code != 0 {
		t.Fatalf("整合失败（%d）：%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Rows consolidated: 9") ||
		!strings.Contains(stdout.String(), "Rows missing: 1") {
		t.Fatalf("整合结果不对：%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code := run([]string{
		"import", "--config", configDir, "--source", "global_udi_input",
		"--file", filepath.Join(sampleRoot, "invalid-conditions", "global_udi_input.xlsx"),
	}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("invalid-conditions 的文件必须被拒绝")
	}
	message := stderr.String()
	if !strings.Contains(message, "- 第 13 行：使用单元产品标识（device_identifier_use_unit）为空") ||
		!strings.Contains(message, "- 第 14 行：使用单元产品标识（device_identifier_use_unit）为空") {
		t.Fatalf("被拒行号文案不对：%s", message)
	}
	if !strings.Contains(message, "导入文件存在条件必填列为空") {
		t.Fatalf("被拒标题不对：%s", message)
	}
}
