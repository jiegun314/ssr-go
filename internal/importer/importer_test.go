package importer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/jiegun314/ssr-go/internal/config"
	"github.com/jiegun314/ssr-go/internal/store"
)

// newTestImporter 把真实配置复制到临时目录、开一个临时数据库，返回导入器与仓库。
// 测试因此不会碰仓库里的 data/ 与 output/（对应 tests/conftest.py 的隔离做法）。
func newTestImporter(t *testing.T) (*Importer, *store.Repository, *config.Loader) {
	t.Helper()
	source := filepath.Join("..", "..", "config")
	target := t.TempDir()
	for _, name := range []string{
		config.SettingFile, config.ImportMappingFile,
		config.ConsolidationMappingFile, config.LogColumnsFile,
	} {
		content, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			t.Fatalf("读取 %s 失败：%v", name, err)
		}
		if err := os.WriteFile(filepath.Join(target, name), content, 0o644); err != nil {
			t.Fatalf("写 %s 失败：%v", name, err)
		}
	}
	loader, err := config.NewLoader(target)
	if err != nil {
		t.Fatalf("读配置失败：%v", err)
	}
	repository, err := store.Open(filepath.Join(t.TempDir(), "test.sqlite3"))
	if err != nil {
		t.Fatalf("打开数据库失败：%v", err)
	}
	t.Cleanup(func() { repository.Close() })
	importer, err := NewImporter(loader, repository)
	if err != nil {
		t.Fatalf("构造导入器失败：%v", err)
	}
	return importer, repository, loader
}

func samplePath(sourceName string) string {
	return filepath.Join("..", "..", "testdata", "sample-valid", sourceName+".xlsx")
}

func invalidPath(sourceName string) string {
	return filepath.Join("..", "..", "testdata", "sample-invalid-conditions", sourceName+".xlsx")
}

// headerIndexes 返回表头里每个中文列名对应的列号（1-based）。
func headerIndexes(t *testing.T, file *excelize.File, sheet string, headerRow int) map[string]int {
	t.Helper()
	rows, err := file.GetRows(sheet)
	if err != nil {
		t.Fatalf("读工作表失败：%v", err)
	}
	if headerRow-1 >= len(rows) {
		t.Fatalf("表头行 %d 超出范围", headerRow)
	}
	indexes := map[string]int{}
	for position, title := range rows[headerRow-1] {
		if title == "" {
			continue
		}
		if _, taken := indexes[title]; !taken {
			indexes[title] = position + 1
		}
	}
	return indexes
}

// writeVariant 复制一份样本工作簿并按 mutate 改几个单元格，返回临时文件路径。
func writeVariant(
	t *testing.T,
	importer *Importer,
	sourceName string,
	mutate func(file *excelize.File, sheet string, headerRow int, indexes map[string]int),
) string {
	t.Helper()
	rule := importer.Rules[sourceName]
	file, err := excelize.OpenFile(samplePath(sourceName))
	if err != nil {
		t.Fatalf("打开样本失败：%v", err)
	}
	defer file.Close()
	sheets := file.GetSheetList()
	sheet := sheets[0]
	indexes := headerIndexes(t, file, sheet, rule.HeaderRow)
	mutate(file, sheet, rule.HeaderRow, indexes)
	path := filepath.Join(t.TempDir(), sourceName+".xlsx")
	if err := file.SaveAs(path); err != nil {
		t.Fatalf("保存变体失败：%v", err)
	}
	return path
}

// setCell 按中文列名写一个数据行的单元格（row 是 Excel 行号）。
func setCell(
	t *testing.T,
	file *excelize.File,
	sheet string,
	indexes map[string]int,
	row int,
	chineseName string,
	value string,
) {
	t.Helper()
	column, known := indexes[chineseName]
	if !known {
		t.Fatalf("表头里没有列 %q", chineseName)
	}
	cell, err := excelize.CoordinatesToCellName(column, row)
	if err != nil {
		t.Fatalf("算单元格地址失败：%v", err)
	}
	if err := file.SetCellValue(sheet, cell, value); err != nil {
		t.Fatalf("写单元格失败：%v", err)
	}
}

func TestAllSourceFilesCanBeImportedAndReviewed(t *testing.T) {
	importer, repository, _ := newTestImporter(t)
	expectedRows := map[string]int{
		"ra_input": 6, "global_udi_input": 9,
		"medical_insurance_code": 6, "product_category": 6,
	}

	for sourceName, expected := range expectedRows {
		result, err := importer.Import(sourceName, samplePath(sourceName))
		if err != nil {
			t.Fatalf("导入 %s 失败：%v", sourceName, err)
		}
		if result.RowCount != expected {
			t.Errorf("%s 导入 %d 行；want %d", sourceName, result.RowCount, expected)
		}
		if result.FileName != importer.Rules[sourceName].ChineseName {
			t.Errorf("%s 的结果文件名应为中文名：%q", sourceName, result.FileName)
		}
		rows, err := importer.ReviewRows(sourceName)
		if err != nil {
			t.Fatalf("回顾 %s 失败：%v", sourceName, err)
		}
		if len(rows) != expected {
			t.Errorf("%s 回顾 %d 行；want %d", sourceName, len(rows), expected)
		}
	}
	// 四张暂存表都建出来了
	names, err := repository.TableNames()
	if err != nil {
		t.Fatalf("列表名失败：%v", err)
	}
	for _, sourceName := range importer.Order {
		tableName := importer.Rules[sourceName].TargetTable
		found := false
		for _, name := range names {
			if name == tableName {
				found = true
			}
		}
		if !found {
			t.Errorf("缺少暂存表 %s（表：%v）", tableName, names)
		}
	}
}

func TestImportRejectsUnknownFileType(t *testing.T) {
	importer, _, _ := newTestImporter(t)

	_, err := importer.Import("unknown", samplePath("ra_input"))

	if err == nil || !strings.Contains(err.Error(), "Invalid file type") {
		t.Fatalf("未知来源必须被拒绝，得到：%v", err)
	}
}

func TestImportRejectsAWorkbookMissingARequiredColumn(t *testing.T) {
	importer, repository, _ := newTestImporter(t)
	// ra_input 的第一列是必填的「注册证编号/备案凭证编号」
	path := writeVariant(t, importer, "ra_input", func(file *excelize.File, sheet string, _ int, _ map[string]int) {
		if err := file.RemoveCol(sheet, "A"); err != nil {
			t.Fatalf("删列失败：%v", err)
		}
	})

	_, err := importer.Import("ra_input", path)

	var missing *MissingRequiredColumnsError
	if !errors.As(err, &missing) {
		t.Fatalf("应当抛缺列错误，得到：%v", err)
	}
	message := err.Error()
	if !strings.Contains(message, "注册证编号/备案凭证编号") ||
		!strings.Contains(message, "registration_filing_certificate_number") {
		t.Fatalf("缺列文案要含中文名与 db_field：%s", message)
	}
	if !strings.Contains(message, "导入文件缺少必填列") {
		t.Fatalf("缺列标题不对：%s", message)
	}
	// 失败时整份文件不写入（R7）：目标表根本不存在
	count, err := repository.CountTableRows(importer.Rules["ra_input"].TargetTable)
	if err != nil || count != 0 {
		t.Fatalf("失败导入不该写库（count=%d, err=%v）", count, err)
	}
}

func TestRequiredValueViolationsAreReportedPerCell(t *testing.T) {
	importer, _, _ := newTestImporter(t)
	rule := importer.Rules["global_udi_input"]
	path := writeVariant(t, importer, "global_udi_input",
		func(file *excelize.File, sheet string, _ int, indexes map[string]int) {
			setCell(t, file, sheet, indexes, rule.DataStartRow, "产品代码", "")
			setCell(t, file, sheet, indexes, rule.DataStartRow+1, "产品代码", "")
			setCell(t, file, sheet, indexes, rule.DataStartRow+1, "事业部", "")
		})

	_, err := importer.Import("global_udi_input", path)

	var missing *MissingRowValuesError
	if !errors.As(err, &missing) {
		t.Fatalf("应当抛缺值错误，得到：%v", err)
	}
	if len(missing.Violations) != 3 {
		t.Fatalf("违规单元格数 = %d; want 3", len(missing.Violations))
	}
	// 汇总按行去重：第 4 行 1 个空格、第 5 行 2 个空格 → 2 行失败、3 个空单元格
	wantSummary := "UDI团队信息：有 2 行数据导入失败（3 个单元格为空）。"
	if missing.Summary() != wantSummary {
		t.Fatalf("汇总 = %q; want %q", missing.Summary(), wantSummary)
	}
	if strings.Join(intsToText(missing.FailedRows()), ",") != "4,5" {
		t.Fatalf("失败行号 = %v", missing.FailedRows())
	}
	message := err.Error()
	if !strings.Contains(message, "- 第 4 行：产品代码（material_code）为空") {
		t.Fatalf("明细行不对：%s", message)
	}
	if !strings.Contains(message, "导入文件存在必填列为空") {
		t.Fatalf("标题不对：%s", message)
	}
}

func TestR1PlaceholderCountsAsAFilledCell(t *testing.T) {
	importer, repository, _ := newTestImporter(t)
	rule := importer.Rules["global_udi_input"]
	path := writeVariant(t, importer, "global_udi_input",
		func(file *excelize.File, sheet string, _ int, indexes map[string]int) {
			setCell(t, file, sheet, indexes, rule.DataStartRow, "产品代码", "NA")
		})

	result, err := importer.Import("global_udi_input", path)
	if err != nil {
		t.Fatalf("NA 是有效值，不该被拒：%v", err)
	}
	if result.RowCount != 9 {
		t.Fatalf("行数 = %d", result.RowCount)
	}
	rows, err := repository.Rows(rule.TargetTable)
	if err != nil {
		t.Fatalf("读表失败：%v", err)
	}
	if rows[0]["material_code"] != "NA" {
		t.Fatalf("占位符要原样入库：%q", rows[0]["material_code"])
	}
}

func TestR4ValueMappingNormalizesWordingButNeverErasesACell(t *testing.T) {
	importer, repository, _ := newTestImporter(t)
	rule := importer.Rules["global_udi_input"]
	path := writeVariant(t, importer, "global_udi_input",
		func(file *excelize.File, sheet string, _ int, indexes map[string]int) {
			// 是 → Yes（写法归一）；NA 被配置声明为空值，但填了内容就保留原文
			setCell(t, file, sheet, indexes, rule.DataStartRow, "中国市场是否拆包卖", "No")
			setCell(t, file, sheet, indexes, rule.DataStartRow, "发码机构编码名称", "NA")
		})

	if _, err := importer.Import("global_udi_input", path); err != nil {
		t.Fatalf("导入失败：%v", err)
	}
	rows, err := repository.Rows(rule.TargetTable)
	if err != nil {
		t.Fatalf("读表失败：%v", err)
	}
	if rows[0]["if_sold_as_individual_units"] != "否" {
		t.Errorf("映射应归一写法：%q", rows[0]["if_sold_as_individual_units"])
	}
	if rows[0]["udi_issuing_agency_name"] != "NA" {
		t.Errorf("映射不得清空已填内容：%q", rows[0]["udi_issuing_agency_name"])
	}
}

func TestR6ConditionalRequiredIsEnforcedWithRowNumbers(t *testing.T) {
	importer, repository, _ := newTestImporter(t)
	// invalid-conditions 的 UDI 文件在数量 > 1 的行上留空了使用单元产品标识
	_, err := importer.Import("global_udi_input", invalidPath("global_udi_input"))

	var missing *MissingRowValuesError
	if !errors.As(err, &missing) {
		t.Fatalf("应当抛条件必填错误，得到：%v", err)
	}
	if strings.Join(intsToText(missing.FailedRows()), ",") != "13,14" {
		t.Fatalf("被拒行号 = %v; want [13 14]", missing.FailedRows())
	}
	if missing.Violations[0].ChineseName != "使用单元产品标识" ||
		missing.Violations[0].DBField != "device_identifier_use_unit" {
		t.Fatalf("违规列不对：%+v", missing.Violations[0])
	}
	message := err.Error()
	if !strings.Contains(message, "导入文件存在条件必填列为空") {
		t.Fatalf("标题不对：%s", message)
	}
	// 命中的条件要逐字写进明细：第 13 行的数量是 2，第 14 行是 2.5
	if !strings.Contains(message,
		"因为 最小销售单元中使用单元的数量（quantity_per_min_sales_unit） = 2 > 1") {
		t.Fatalf("条件文案不对：%s", message)
	}
	if !strings.Contains(message,
		"因为 最小销售单元中使用单元的数量（quantity_per_min_sales_unit） = 2.5 > 1") {
		t.Fatalf("条件文案不对：%s", message)
	}
	// 失败导入不写库（R7）
	count, err := repository.CountTableRows(importer.Rules["global_udi_input"].TargetTable)
	if err != nil || count != 0 {
		t.Fatalf("失败导入不该写库（count=%d, err=%v）", count, err)
	}
}

func TestR6ConditionalRequiredStaysOptionalWhenTheConditionDoesNotMatch(t *testing.T) {
	importer, _, _ := newTestImporter(t)
	rule := importer.Rules["global_udi_input"]
	path := writeVariant(t, importer, "global_udi_input",
		func(file *excelize.File, sheet string, _ int, indexes map[string]int) {
			setCell(t, file, sheet, indexes, rule.DataStartRow, "最小销售单元中使用单元的数量", "1")
			setCell(t, file, sheet, indexes, rule.DataStartRow, "使用单元产品标识", "")
			// 非数字的数量同样无法让条件成立
			setCell(t, file, sheet, indexes, rule.DataStartRow+3, "最小销售单元中使用单元的数量", "多套")
			setCell(t, file, sheet, indexes, rule.DataStartRow+3, "使用单元产品标识", "")
		})

	if _, err := importer.Import("global_udi_input", path); err != nil {
		t.Fatalf("条件不成立时该列允许为空：%v", err)
	}
}

func TestR2ACompletelyEmptyRowIsNotADataRow(t *testing.T) {
	importer, _, _ := newTestImporter(t)
	rule := importer.Rules["global_udi_input"]
	path := writeVariant(t, importer, "global_udi_input",
		func(file *excelize.File, sheet string, _ int, indexes map[string]int) {
			for chineseName := range indexes {
				setCell(t, file, sheet, indexes, rule.DataStartRow, chineseName, "")
			}
		})

	result, err := importer.Import("global_udi_input", path)
	if err != nil {
		t.Fatalf("整行空不触发必填校验（R2）：%v", err)
	}
	if result.RowCount != 9 {
		t.Fatalf("行数 = %d; want 9（空行仍是数据行，只是不触发校验）", result.RowCount)
	}
}

func TestR5AnOptionalColumnMayBeMissingFromTheHeader(t *testing.T) {
	importer, repository, _ := newTestImporter(t)
	rule := importer.Rules["ra_input"]
	// 删掉一个 optional 列（备注）所在的列
	path := writeVariant(t, importer, "ra_input",
		func(file *excelize.File, sheet string, _ int, indexes map[string]int) {
			column := indexes["备注"]
			name, err := excelize.ColumnNumberToName(column)
			if err != nil {
				t.Fatalf("算列名失败：%v", err)
			}
			if err := file.RemoveCol(sheet, name); err != nil {
				t.Fatalf("删列失败：%v", err)
			}
		})

	if _, err := importer.Import("ra_input", path); err != nil {
		t.Fatalf("optional 列不在表头里也能导入（R5）：%v", err)
	}
	rows, err := repository.Rows(rule.TargetTable)
	if err != nil {
		t.Fatalf("读表失败：%v", err)
	}
	if rows[0]["remarks"] != "" {
		t.Fatalf("缺的 optional 列写空值：%q", rows[0]["remarks"])
	}
	// 暂存表仍然有全部配置列（R8）
	columns, err := repository.TableColumns(rule.TargetTable)
	if err != nil {
		t.Fatalf("读列失败：%v", err)
	}
	if len(columns) != len(rule.Columns) {
		t.Fatalf("暂存列数 = %d; want %d", len(columns), len(rule.Columns))
	}
}

func intsToText(values []int) []string {
	texts := make([]string, 0, len(values))
	for _, value := range values {
		texts = append(texts, itoa(value))
	}
	return texts
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	return digits
}
