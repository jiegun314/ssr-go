package consolidation

// 同一型号对应多条医保编码时的合并口径（merge_records + record_separator）：
// 多个编码用英文逗号合并到一格、不判冲突、绝不留尾部逗号。

import (
	"os"
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/config"
	"github.com/jiegun314/ssr-go/internal/store"
)

const insuranceColumn = "Medical Insurance Code"

// addInsuranceRows 给指定 material_code 追加若干条医保行（编码 = 原编码 + suffix），
// 返回原编码与追加后的全部编码（来源顺序）。
func addInsuranceRows(t *testing.T, repository *store.Repository, suffixes ...string) (string, []string) {
	t.Helper()
	original := ""
	codes := []string{}
	replaceRows(t, repository, "medical_insurance_code", func(rows []store.Row) []store.Row {
		if len(rows) == 0 {
			t.Fatal("医保来源没有数据")
		}
		original = rows[0]["medical_insurance_code"]
		extra := []store.Row{}
		for _, suffix := range suffixes {
			copyRow := store.Row{}
			for column, value := range rows[0] {
				copyRow[column] = value
			}
			copyRow["medical_insurance_code"] = original + suffix
			extra = append(extra, copyRow)
			codes = append(codes, original+suffix)
		}
		return append(rows, extra...)
	})
	return original, append([]string{original}, codes...)
}

// TestManyToOneMergesAllInsuranceCodes 固定主口径：同一型号的两条不同医保行
// 合并成一格（英文逗号、来源顺序），而且**不算冲突**（这一行仍然可以导出）。
func TestManyToOneMergesAllInsuranceCodes(t *testing.T) {
	service, _, repository := consolidated(t)
	original, codes := addInsuranceRows(t, repository, "-B")

	outcome, err := service.Consolidate()
	if err != nil {
		t.Fatalf("整合失败：%v", err)
	}
	if len(outcome.ConflictData) != 0 {
		t.Errorf("同一型号多条医保编码不该再判冲突：%v", outcome.ConflictData)
	}
	want := strings.Join(codes, ",")
	found := false
	for _, row := range outcome.Rows {
		if row.Values[service.Config.BaseKeyField] != "000MAT-001" {
			continue
		}
		found = true
		if row.Values["status"] != StatusReady {
			t.Errorf("状态 = %q；合并后应当仍是 Ready", row.Values["status"])
		}
		if got := row.Values[insuranceColumn]; got != want {
			t.Errorf("%s = %q；want %q（原编码 %q）", insuranceColumn, got, want, original)
		}
	}
	if !found {
		t.Fatal("没找到 000MAT-001 的结果行")
	}
}

// TestManyToOneMergesWithoutDuplicates 固定去重口径：整行完全相同的重复行只出现一次。
func TestManyToOneMergesWithoutDuplicates(t *testing.T) {
	service, _, repository := consolidated(t)
	original, _ := addInsuranceRows(t, repository, "")

	outcome, err := service.Consolidate()
	if err != nil {
		t.Fatalf("整合失败：%v", err)
	}
	for _, row := range outcome.Rows {
		if row.Values[service.Config.BaseKeyField] != "000MAT-001" {
			continue
		}
		if got := row.Values[insuranceColumn]; got != original {
			t.Errorf("整行相同的重复记录应当去重：%q；want %q", got, original)
		}
	}
}

// TestManyToOneMergesSortedWhenConfigured 固定 order: sorted —— 合并后按文本字典序。
func TestManyToOneMergesSortedWhenConfigured(t *testing.T) {
	loader, repository, importService := newWorkspace(t)
	// 把来源的合并顺序改成 sorted（其余配置不动）
	path := loader.Resolver.ConfigFile(config.ConsolidationMappingFile)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读整合映射失败：%v", err)
	}
	mutated := strings.Replace(string(content), "order: source", "order: sorted", 1)
	if mutated == string(content) {
		t.Fatal("整合映射里没找到 merge_records.order")
	}
	if err := os.WriteFile(path, []byte(mutated), 0o644); err != nil {
		t.Fatalf("写整合映射失败：%v", err)
	}
	fresh, err := config.NewLoader(loader.ConfigDir())
	if err != nil {
		t.Fatalf("重读配置失败：%v", err)
	}
	for _, sourceName := range importService.Order {
		filePath := "../../testdata/sample-valid/" + sourceName + ".xlsx"
		if _, err := importService.Import(sourceName, filePath); err != nil {
			t.Fatalf("导入 %s 失败：%v", sourceName, err)
		}
	}
	service := newService(t, fresh, repository)

	// 追加一条"字典序更小"的编码，验证排序生效
	rows, err := repository.Rows(service.Config.SourceTables["medical_insurance_code"])
	if err != nil {
		t.Fatalf("读医保来源失败：%v", err)
	}
	if len(rows) == 0 {
		t.Fatal("医保来源没有数据")
	}
	original := rows[0]["medical_insurance_code"]
	smaller := "000AAA-" + original
	replaceRows(t, repository, "medical_insurance_code", func(rows []store.Row) []store.Row {
		copyRow := store.Row{}
		for column, value := range rows[0] {
			copyRow[column] = value
		}
		copyRow["medical_insurance_code"] = smaller
		return append(rows, copyRow)
	})

	outcome, err := service.Consolidate()
	if err != nil {
		t.Fatalf("整合失败：%v", err)
	}
	want := smaller + "," + original
	for _, row := range outcome.Rows {
		if row.Values[service.Config.BaseKeyField] != "000MAT-001" {
			continue
		}
		if got := row.Values[insuranceColumn]; got != want {
			t.Errorf("%s = %q；order: sorted 时 want %q", insuranceColumn, got, want)
		}
	}
}

// TestJoinRecordsSkipsEmptyItems 固定空值口径：没有编码的记录整条丢掉，不会出现尾部逗号。
func TestJoinRecordsSkipsEmptyItems(t *testing.T) {
	service, _, _ := consolidated(t)
	rows := []store.Row{
		{"medical_insurance_code": "000INS-001"},
		{"medical_insurance_code": "   "},
		{"medical_insurance_code": "000INS-002"},
	}
	got := service.joinRecords(
		rows,
		map[string]any{"file": "medical_insurance_code", "field": []any{"medical_insurance_code"}},
		map[string]any{"separator": "-"},
		",")
	if got != "000INS-001,000INS-002" {
		t.Errorf("合并结果 = %q；want 000INS-001,000INS-002（空值不参与、无尾逗号）", got)
	}
	if strings.HasSuffix(got, ",") {
		t.Errorf("不该出现尾部分隔符：%q", got)
	}
}

// TestManyToOneMergesDistinctItemsOnce 固定单元格层面的去重：两条来源记录**整行不同**
// （例如登记证信息不一样）但渲染出来的医保编码一样时，格子里只出现一次。
func TestManyToOneMergesDistinctItemsOnce(t *testing.T) {
	service, _, repository := consolidated(t)
	original := ""
	replaceRows(t, repository, "medical_insurance_code", func(rows []store.Row) []store.Row {
		original = rows[0]["medical_insurance_code"]
		copyRow := store.Row{}
		for column, value := range rows[0] {
			copyRow[column] = value
		}
		// 改成别的登记证号：整行不再相同，但医保编码没变
		copyRow["registration_certificate_number"] = "DIFFERENT-CERT"
		return append(rows, copyRow)
	})

	outcome, err := service.Consolidate()
	if err != nil {
		t.Fatalf("整合失败：%v", err)
	}
	for _, row := range outcome.Rows {
		if row.Values[service.Config.BaseKeyField] != "000MAT-001" {
			continue
		}
		if got := row.Values[insuranceColumn]; got != original {
			t.Errorf("%s = %q；同一编码只该出现一次（want %q）", insuranceColumn, got, original)
		}
	}
}
