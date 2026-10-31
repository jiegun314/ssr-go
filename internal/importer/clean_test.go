package importer

// clean 规则（导入后生成清洗表）的单元测试：
//
//   - 同一张证（dedupe.by）只留 latest_by 最新的一行，同日期取靠后的记录；
//   - 分组（group_by）内：unique / 普通列取最新那一行，join_comma / join_backslash 去重后连接；
//   - conflict = latest 取最新证的值，conflict = error 抛业务拒绝；
//   - drop 的列不出现在清洗表里；空值不参与连接（不会出现首尾分隔符）。

import (
	"errors"
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/store"
)

func cleanTestColumns() []Column {
	return []Column{
		{ChineseName: "规格型号", DBField: "material_code"},
		{ChineseName: "注册证唯一码", DBField: "registration_certificate_unique_code"},
		{ChineseName: "注册证编号/备案凭证编号", DBField: "registration_filing_certificate_number"},
		{ChineseName: "RA生效日期", DBField: "ra_effective_date"},
		{ChineseName: "产品名称/通用名称", DBField: "product_name"},
		{ChineseName: "是否标记为一次性使用", DBField: "if_one_time_use"},
		{ChineseName: "灭菌方式", DBField: "sterilization_method"},
	}
}

func cleanTestRule(conflict string) *CleanRule {
	return &CleanRule{
		TargetTable:   "ra_input_cleaned",
		GroupBy:       []string{"material_code"},
		DedupeBy:      []string{"material_code", "registration_certificate_unique_code", "registration_filing_certificate_number"},
		LatestBy:      "ra_effective_date",
		Drop:          []string{"registration_certificate_unique_code", "ra_effective_date"},
		Unique:        []string{"if_one_time_use"},
		Conflict:      conflict,
		JoinComma:     []string{"registration_filing_certificate_number"},
		JoinBackslash: []string{"product_name", "sterilization_method"},
	}
}

func cleanTestRows() []store.Row {
	return []store.Row{
		// 同一张证的两条明细：旧日期先出现，新日期后出现
		{"material_code": "M1", "registration_certificate_unique_code": "A-1", "registration_filing_certificate_number": "CERT-1",
			"ra_effective_date": "2020-01-01", "product_name": "旧名字", "if_one_time_use": "是", "sterilization_method": ""},
		{"material_code": "M1", "registration_certificate_unique_code": "A-1", "registration_filing_certificate_number": "CERT-1",
			"ra_effective_date": "2024-01-01", "product_name": "新名字", "if_one_time_use": "是", "sterilization_method": ""},
		// 第二张证（较旧）
		{"material_code": "M1", "registration_certificate_unique_code": "A-2", "registration_filing_certificate_number": "CERT-2",
			"ra_effective_date": "2019-01-01", "product_name": "另一个名字", "if_one_time_use": "是", "sterilization_method": "EO"},
	}
}

func TestCleanRowsKeepsLatestPerCertificateAndMerges(t *testing.T) {
	rows, columns, err := CleanRows(cleanTestRule("latest"), cleanTestColumns(), cleanTestRows())
	if err != nil {
		t.Fatalf("清洗失败：%v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("一个规格型号应当只剩一行，实际 %d 行", len(rows))
	}
	row := rows[0]
	// 注册证编号：去重后逗号连接，最新证在前
	if row["registration_filing_certificate_number"] != "CERT-1,CERT-2" {
		t.Errorf("注册证编号 = %q；want CERT-1,CERT-2", row["registration_filing_certificate_number"])
	}
	// 文本项：反斜杠连接，空值跳过（不会出现尾部反斜杠）
	if row["product_name"] != "新名字\\另一个名字" {
		t.Errorf("产品名称 = %q；want 新名字\\\\另一个名字", row["product_name"])
	}
	if row["sterilization_method"] != "EO" {
		t.Errorf("灭菌方式 = %q；want EO（空值跳过、不出现首尾分隔符）", row["sterilization_method"])
	}
	// unique 列取最新证的值
	if row["if_one_time_use"] != "是" {
		t.Errorf("是否一次性 = %q；want 是", row["if_one_time_use"])
	}
	// drop 的列不在输出列里，但行里仍保留（供校验使用）
	for _, dropped := range []string{"registration_certificate_unique_code", "ra_effective_date"} {
		if containsField(columns, dropped) {
			t.Errorf("%s 应当被 drop 掉", dropped)
		}
	}
	if row["ra_effective_date"] != "2024-01-01" {
		t.Errorf("行里仍要带 ra_effective_date（校验可见），实际 %q", row["ra_effective_date"])
	}
}

func TestCleanRowsDedupesIdenticalValuesAndTies(t *testing.T) {
	rows := []store.Row{
		{"material_code": "M1", "registration_certificate_unique_code": "A-1", "registration_filing_certificate_number": "CERT-1",
			"ra_effective_date": "2024-01-01", "product_name": "同名", "if_one_time_use": "是"},
		{"material_code": "M1", "registration_certificate_unique_code": "A-1", "registration_filing_certificate_number": "CERT-1",
			"ra_effective_date": "2024-01-01", "product_name": "同名", "if_one_time_use": "是"},
	}
	cleaned, _, err := CleanRows(cleanTestRule("latest"), cleanTestColumns(), rows)
	if err != nil {
		t.Fatalf("清洗失败：%v", err)
	}
	if len(cleaned) != 1 {
		t.Fatalf("行数 = %d；want 1", len(cleaned))
	}
	if cleaned[0]["product_name"] != "同名" {
		t.Errorf("相同值只出现一次，实际 %q", cleaned[0]["product_name"])
	}
}

func TestCleanRowsConflictLatestPicksNewest(t *testing.T) {
	rows := cleanTestRows()
	rows[2]["if_one_time_use"] = "否" // 旧证的该列与最新证不同
	cleaned, _, err := CleanRows(cleanTestRule("latest"), cleanTestColumns(), rows)
	if err != nil {
		t.Fatalf("conflict=latest 不该报错：%v", err)
	}
	if cleaned[0]["if_one_time_use"] != "是" {
		t.Errorf("应当取最新证的值「是」，实际 %q", cleaned[0]["if_one_time_use"])
	}
}

func TestCleanRowsConflictErrorRejectsTheFile(t *testing.T) {
	rows := cleanTestRows()
	rows[2]["if_one_time_use"] = "否"
	_, _, err := CleanRows(cleanTestRule("error"), cleanTestColumns(), rows)
	if err == nil {
		t.Fatal("conflict=error 应当拒绝")
	}
	var conflict *CleanConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("应当是 CleanConflictError，实际 %T", err)
	}
	for _, want := range []string{"是否标记为一次性使用", "是", "否"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("错误文案里应当有 %q：%s", want, err.Error())
		}
	}
}

func TestCleanRowsConflictErrorSummarisesEveryConflict(t *testing.T) {
	// 两个型号各有一处冲突（其中 M2 有两项）→ 报错要给出总数与分项，而不是只报第一处
	rows := []store.Row{
		{"material_code": "M1", "registration_certificate_unique_code": "A-1", "registration_filing_certificate_number": "C1",
			"ra_effective_date": "2024-01-01", "if_one_time_use": "是", "mr_safety_information": "安全"},
		{"material_code": "M1", "registration_certificate_unique_code": "A-2", "registration_filing_certificate_number": "C2",
			"ra_effective_date": "2023-01-01", "if_one_time_use": "否", "mr_safety_information": "不安全"},
		{"material_code": "M2", "registration_certificate_unique_code": "B-1", "registration_filing_certificate_number": "C3",
			"ra_effective_date": "2024-01-01", "if_one_time_use": "是", "mr_safety_information": "安全"},
		{"material_code": "M2", "registration_certificate_unique_code": "B-2", "registration_filing_certificate_number": "C4",
			"ra_effective_date": "2022-01-01", "if_one_time_use": "否", "mr_safety_information": "不安全"},
	}
	columns := append(cleanTestColumns(), Column{ChineseName: "磁共振（MR）安全相关信息", DBField: "mr_safety_information"})
	rule := cleanTestRule("error")
	rule.Unique = []string{"if_one_time_use", "mr_safety_information"}
	_, _, err := CleanRows(rule, columns, rows)
	if err == nil {
		t.Fatal("应当报冲突")
	}
	message := err.Error()
	for _, want := range []string{
		"2 个规格型号", "4 处", "是否标记为一次性使用：2 个型号",
		"磁共振（MR）安全相关信息：2 个型号", "M1「是 / 否」", "M2「是 / 否」",
	} {
		if !strings.Contains(message, want) {
			t.Errorf("汇总文案里应当有 %q：\n%s", want, message)
		}
	}
}
