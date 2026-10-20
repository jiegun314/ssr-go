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
	// 天数写在 setting.yaml 的固定参数里（与统一社会信用代码同一类），不是映射里的字面量
	if transform["offset_days_from"] != "di_record_publish_date_offset_days" {
		t.Fatalf("%s.offset_days_from = %v", column, transform["offset_days_from"])
	}
	if _, literal := transform["offset_days"]; literal {
		t.Errorf("%s 不该再写字面量 offset_days：天数走 user_defined_data 参数", column)
	}
	setting, err := loader.LoadSetting()
	if err != nil {
		t.Fatalf("读 setting.yaml 失败：%v", err)
	}
	userDefined, _ := setting["user_defined_data"].(map[string]any)
	if got := userDefined["di_record_publish_date_offset_days"]; got != 5 {
		t.Fatalf("参数 di_record_publish_date_offset_days = %#v；want 5", got)
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
				return strings.Replace(content,
					"        offset_days_from: \"di_record_publish_date_offset_days\"",
					"        offset_days: \"1\"", 1)
			},
			keyword: "offset_days must be an integer",
		},
		{
			name: "offset_days_from 指向不存在的参数",
			mutate: func(content string) string {
				return strings.Replace(content,
					"offset_days_from: \"di_record_publish_date_offset_days\"",
					"offset_days_from: \"offset_days_typo\"", 1)
			},
			keyword: "unknown setting.user_defined_data key",
		},
		{
			name: "offset_days 与 offset_days_from 同时写",
			mutate: func(content string) string {
				return strings.Replace(content,
					"        offset_days_from: \"di_record_publish_date_offset_days\"",
					"        offset_days: 1\n        offset_days_from: \"di_record_publish_date_offset_days\"", 1)
			},
			keyword: "can not declare both offset_days and offset_days_from",
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
