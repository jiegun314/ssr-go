package config

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestDateOffsetColumnMustBeIgnoredByTheComparison 固定"运行日日期列"的配置契约：
//
//   - 仓库配置里的这一列合法（format/base/offset 都对，且已经写进 ignore_fields）；
//   - 漏写 ignore_fields 会被校验拦住 —— 这是最容易踩的坑：日期每天都变，
//     不忽略的话每次整合都会把所有行判成"有差异"。
//   - base / format 写错也会被拦住。
func TestDateOffsetColumnMustBeIgnoredByTheComparison(t *testing.T) {
	const column = "DI Record Publish Date"

	// 1) 仓库配置合法
	loader, err := NewLoader(filepath.Join("..", "..", "config"))
	if err != nil {
		t.Fatal(err)
	}
	if err := loader.ValidateAll(); err != nil {
		t.Fatalf("仓库配置应当通过校验：%v", err)
	}
	mapping, _ := loader.LoadConsolidationMapping()
	dataset, _ := mapping["target_dataset"].(map[string]any)
	fields, _ := dataset["fields"].(map[string]any)
	definition, _ := fields[column].(map[string]any)
	if definition == nil {
		t.Fatalf("配置里应当有 %s", column)
	}
	transform, _ := definition["transform"].(map[string]any)
	if transform["type"] != DateOffsetTransform {
		t.Fatalf("%s 的 transform.type = %v", column, transform["type"])
	}

	// 2) 漏写 ignore_fields → 拒绝
	cases := []struct {
		name    string
		mutate  func(content string) string
		keyword string
	}{
		{
			name: "漏写 ignore_fields",
			mutate: func(content string) string {
				return strings.Replace(content, "          - \"DI Record Publish Date\"\n", "", 1)
			},
			keyword: "ignore_fields",
		},
		{
			name: "base 写错",
			mutate: func(content string) string {
				return strings.Replace(content, "        base: \"today\"", "        base: \"tomorrow\"", 1)
			},
			keyword: "base must be",
		},
		{
			name: "format 写错",
			mutate: func(content string) string {
				return strings.Replace(content, "        format: \"YYYY-MM-DD\"", "        format: \"YYYY/MM/DD\"", 1)
			},
			keyword: "format must be",
		},
		{
			name: "offset_days 不是整数",
			mutate: func(content string) string {
				return strings.Replace(content, "        offset_days: 1", "        offset_days: \"1\"", 1)
			},
			keyword: "offset_days must be an integer",
		},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			workspace := t.TempDir()
			copyConfigDirForTest(t, filepath.Join("..", "..", "config"), workspace)
			path := filepath.Join(workspace, ConsolidationMappingFile)
			content := readFileForTest(t, path)
			mutated := item.mutate(content)
			if mutated == content {
				t.Fatalf("用例没有改到任何内容：%s", item.name)
			}
			if err := writeFileForTest(path, mutated); err != nil {
				t.Fatal(err)
			}
			loader, err := NewLoader(workspace)
			if err != nil {
				t.Fatal(err)
			}
			err = loader.ValidateAll()
			if err == nil {
				t.Fatal("应当被校验拦住，实际通过")
			}
			if !strings.Contains(err.Error(), item.keyword) {
				t.Errorf("报错应当提到 %q，实际：%v", item.keyword, err)
			}
			t.Logf("按预期被拦住：%v", err)
		})
	}
}
