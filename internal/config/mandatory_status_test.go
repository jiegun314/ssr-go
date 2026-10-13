package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExportMandatoryStatusUsesTheExplicitVocabulary 固定导出列必填口径的词表：
// optional / required / required_if_applicable / user_defined。
//
// 空串是**旧写法**（等价 optional），校验仍然接受它 —— 发布包不覆盖用户配置，
// 老配置必须继续能跑；但仓库自己的配置不许再写空串（读起来要靠猜）。
func TestExportMandatoryStatusUsesTheExplicitVocabulary(t *testing.T) {
	loader, err := NewLoader(filepath.Join("..", "..", "config"))
	if err != nil {
		t.Fatalf("装载配置失败：%v", err)
	}
	allowed := map[string]bool{
		"optional": true, "required": true,
		"required_if_applicable": true, "user_defined": true,
	}
	mapping, err := loader.LoadConsolidationMapping()
	if err != nil {
		t.Fatal(err)
	}
	dataset, _ := mapping["target_dataset"].(map[string]any)
	fields, _ := dataset["fields"].(map[string]any)
	if len(fields) == 0 {
		t.Fatal("没有解析到导出列")
	}
	for name, raw := range fields {
		definition, _ := raw.(map[string]any)
		status, _ := definition["mandatory_status"].(string)
		if status == "" {
			t.Errorf("导出列 %q 用了旧的空串写法，请写 optional", name)
			continue
		}
		if !allowed[status] {
			t.Errorf("导出列 %q 的 mandatory_status = %q，不在词表内", name, status)
		}
	}
}

// TestLegacyEmptyMandatoryStatusIsStillAccepted 固定向后兼容：
// 老配置里的空串仍然能通过校验（并等价 optional）。
func TestLegacyEmptyMandatoryStatusIsStillAccepted(t *testing.T) {
	workspace := t.TempDir()
	copyConfigDirForTest(t, filepath.Join("..", "..", "config"), workspace)
	path := filepath.Join(workspace, ConsolidationMappingFile)
	content := readFileForTest(t, path)
	// 把某个 optional 列改回空串（模拟老配置）
	updated := replaceOnce(t, content, `mandatory_status: "optional"`, `mandatory_status: ""`)
	if err := writeFileForTest(path, updated); err != nil {
		t.Fatal(err)
	}
	loader, err := NewLoader(workspace)
	if err != nil {
		t.Fatalf("装载失败：%v", err)
	}
	if err := loader.ValidateAll(); err != nil {
		t.Fatalf("空串（旧写法）应当继续被接受：%v", err)
	}
}

/* ---------- 本文件用到的两个小助手 ---------- */

func copyConfigDirForTest(t *testing.T, source, target string) {
	t.Helper()
	entries, err := os.ReadDir(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, err := os.ReadFile(filepath.Join(source, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, entry.Name()), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func readFileForTest(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func writeFileForTest(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

func replaceOnce(t *testing.T, text, old, new string) string {
	t.Helper()
	if strings.Count(text, old) == 0 {
		t.Fatalf("找不到要替换的内容：%s", old)
	}
	return strings.Replace(text, old, new, 1)
}

func removeFileForTest(path string) error {
	return os.Remove(path)
}
