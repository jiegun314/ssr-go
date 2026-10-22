package config

// merge_records / record_separator 的配置契约（同一键多条记录合并成一格）：
//
//   - 只有 many_to_one 能合并，且与 duplicates 互斥（合并了就不该再判冲突）；
//   - separator 必须非空，order 只能 source / sorted；
//   - record_separator 只能用在 concatenate 列上，且它读取的来源必须声明 merge_records。

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeRecordsConfigContract(t *testing.T) {
	// 1) 仓库配置合法，且确实是"合并医保代码"的口径
	loader, err := NewLoader(filepath.Join("..", "..", "config"))
	if err != nil {
		t.Fatal(err)
	}
	if err := loader.ValidateAll(); err != nil {
		t.Fatalf("仓库配置应当通过校验：%v", err)
	}
	mapping, _ := loader.LoadConsolidationMapping()
	dataset, _ := mapping["target_dataset"].(map[string]any)
	mergeRules, _ := dataset["merge_rules"].(map[string]any)
	sources, _ := mergeRules["sources"].(map[string]any)
	insuranceSource, _ := sources["medical_insurance_code"].(map[string]any)
	merge, _ := insuranceSource["merge_records"].(map[string]any)
	if merge == nil {
		t.Fatal("医保来源应当声明 merge_records")
	}
	if _, present := insuranceSource["duplicates"]; present {
		t.Error("合并口径下不该再有 duplicates（互斥）")
	}
	if merge["separator"] != "," {
		t.Errorf("merge_records.separator = %#v；want 英文逗号", merge["separator"])
	}
	if merge["deduplicate"] != true {
		t.Errorf("merge_records.deduplicate = %#v；want true（整行相同才去重）", merge["deduplicate"])
	}
	if merge["order"] != "source" {
		t.Errorf("merge_records.order = %#v；want source", merge["order"])
	}

	fields, _ := dataset["fields"].(map[string]any)
	insuranceColumn, _ := fields["Medical Insurance Code"].(map[string]any)
	transform, _ := insuranceColumn["transform"].(map[string]any)
	if transform["record_separator"] != "," {
		t.Errorf("导出列的 record_separator = %#v；want 英文逗号", transform["record_separator"])
	}
	source, _ := insuranceColumn["source"].(map[string]any)
	fieldList, _ := source["field"].([]any)
	if len(fieldList) != 1 || fieldList[0] != "medical_insurance_code" {
		t.Errorf("只合并医保代码本身（不带序列号），实际字段 = %v", fieldList)
	}
	if _, dead := insuranceColumn["validation"]; dead {
		t.Error("长度校验是死配置（没有代码读它），应当删掉")
	}

	// 2) 错误写法逐条被拦住（每次只改一行/一块）
	cases := []struct {
		name    string
		mutate  func(content string) string
		keyword string
	}{
		{
			name: "缺 separator",
			mutate: func(content string) string {
				return strings.Replace(content, "          separator: \",\"\n", "", 1)
			},
			keyword: "separator must be a non-empty string",
		},
		{
			name: "同时写 duplicates",
			mutate: func(content string) string {
				return strings.Replace(content, "        merge_records:\n",
					"        duplicates:\n          compare_by:\n            - medical_insurance_code\n"+
						"        merge_records:\n", 1)
			},
			keyword: "can not be combined with duplicates",
		},
		{
			name: "不是 many_to_one",
			mutate: func(content string) string {
				return strings.Replace(content,
					"      medical_insurance_code:\n        match_type: lookup\n        cardinality: many_to_one",
					"      medical_insurance_code:\n        match_type: lookup\n        cardinality: one_to_one", 1)
			},
			keyword: "only supported for many_to_one",
		},
		{
			name: "order 写错",
			mutate: func(content string) string {
				return strings.Replace(content, "order: source", "order: random", 1)
			},
			keyword: "order must be one of",
		},
		{
			name: "record_separator 的来源没有 merge_records",
			mutate: func(content string) string {
				return strings.Replace(content,
					"        merge_records:\n          separator: \",\"\n"+
						"          deduplicate: true   # 整行完全相同才去重\n"+
						"          order: source       # 来源表顺序（sorted = 合并后按文本字典序）\n",
					"", 1)
			},
			keyword: "needs merge_records on its source",
		},
		{
			name: "record_separator 用在非 concatenate 列",
			mutate: func(content string) string {
				return strings.Replace(content,
					"        type: concatenate\n        # 同一型号有多条医保行时",
					"        type: direct\n        # 同一型号有多条医保行时", 1)
			},
			keyword: "only supported by concatenate columns",
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
