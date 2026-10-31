package config

// 清洗表（clean 块）的配置契约：
//
//   - 仓库里 ra_input 声明的 clean 就是「一张证留最新一行、一个规格型号一行」的口径；
//   - 表名要独立（不能等于原文表，也不能和别的来源撞车）；
//   - 分组/去重/最新日期/丢弃/唯一/连接列必须是该来源已声明的 db_field；
//   - 连接与唯一列不能被 drop；同一列不能既逗号又反斜杠；conflict 只收 latest / error。

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanConfigContract(t *testing.T) {
	loader, err := NewLoader(filepath.Join("..", "..", "config"))
	if err != nil {
		t.Fatal(err)
	}
	if err := loader.ValidateAll(); err != nil {
		t.Fatalf("仓库配置应当通过校验：%v", err)
	}
	document, err := loader.LoadImportMappingDocument()
	if err != nil {
		t.Fatal(err)
	}
	sources, _ := document.Value["sources"].(map[string]any)
	raInput, _ := sources["ra_input"].(map[string]any)
	clean, _ := raInput["clean"].(map[string]any)
	if clean == nil {
		t.Fatal("ra_input 应当声明 clean")
	}
	if clean["target_table"] != "ra_input_cleaned" {
		t.Errorf("clean.target_table = %#v；want ra_input_cleaned", clean["target_table"])
	}
	groupBy, _ := clean["group_by"].([]any)
	if len(groupBy) != 1 || groupBy[0] != "material_code" {
		t.Errorf("group_by = %v；want [material_code]（一个规格型号一行）", groupBy)
	}
	dedupe, _ := clean["dedupe"].(map[string]any)
	if dedupe["latest_by"] != "ra_effective_date" {
		t.Errorf("dedupe.latest_by = %#v；want ra_effective_date", dedupe["latest_by"])
	}
	if clean["conflict"] != "error" {
		t.Errorf("conflict = %#v；want error（先整合、再判冲突，有冲突整份不导入）", clean["conflict"])
	}
	dropped, _ := clean["drop"].([]any)
	if len(dropped) != 8 {
		t.Errorf("drop 应当有 8 列，实际 %v", dropped)
	}
	unique, _ := clean["unique"].([]any)
	if len(unique) != 9 {
		t.Errorf("unique 应当有 9 列，实际 %v", unique)
	}
	joinComma, _ := clean["join_comma"].([]any)
	if len(joinComma) != 1 || joinComma[0] != "registration_filing_certificate_number" {
		t.Errorf("join_comma = %v；want [registration_filing_certificate_number]", joinComma)
	}
	joinBackslash, _ := clean["join_backslash"].([]any)
	if len(joinBackslash) != 5 {
		t.Errorf("join_backslash 应当有 5 列，实际 %v", joinBackslash)
	}
}

func TestCleanConfigRejections(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(content string) string
		keyword string
	}{
		{
			name: "表名与原文表相同",
			mutate: func(content string) string {
				return strings.Replace(content,
					`      target_table: "ra_input_cleaned"`,
					`      target_table: "ra_input_staging"`, 1)
			},
			keyword: "must differ from target_table",
		},
		{
			name: "表名与别的来源撞车",
			mutate: func(content string) string {
				return strings.Replace(content,
					`      target_table: "ra_input_cleaned"`,
					`      target_table: "global_udi_input_staging"`, 1)
			},
			keyword: "Duplicate source clean table",
		},
		{
			name: "分组列不存在",
			mutate: func(content string) string {
				return strings.Replace(content, `      group_by: ["material_code"]`,
					`      group_by: ["no_such_field"]`, 1)
			},
			keyword: "unknown db_field no_such_field",
		},
		{
			name: "去重日期列不存在",
			mutate: func(content string) string {
				return strings.Replace(content, `        latest_by: "ra_effective_date"`,
					`        latest_by: "no_such_field"`, 1)
			},
			keyword: "dedupe.latest_by: unknown db_field",
		},
		{
			name: "conflict 写错",
			mutate: func(content string) string {
				return strings.Replace(content, `      conflict: "error"`, `      conflict: "first"`, 1)
			},
			keyword: "conflict must be latest or error",
		},
		{
			name: "drop 里的列又被连接",
			mutate: func(content string) string {
				return strings.Replace(content,
					`      join_comma: ["registration_filing_certificate_number"]`,
					`      join_comma: ["registration_filing_certificate_number", "ra_effective_date"]`, 1)
			},
			keyword: "is dropped, cannot be used",
		},
		{
			name: "同一列既逗号又反斜杠",
			mutate: func(content string) string {
				return strings.Replace(content,
					`      join_comma: ["registration_filing_certificate_number"]`,
					`      join_comma: ["registration_filing_certificate_number", "product_name"]`, 1)
			},
			keyword: "cannot be both join_comma and join_backslash",
		},
		{
			name: "unique 里的列不存在",
			mutate: func(content string) string {
				return strings.Replace(content,
					`        - "if_one_time_use"`,
					`        - "no_such_field"`, 1)
			},
			keyword: "unique: unknown db_field no_such_field",
		},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			workspace := t.TempDir()
			copyConfigDirForTest(t, filepath.Join("..", "..", "config"), workspace)
			path := filepath.Join(workspace, ImportMappingFile)
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
