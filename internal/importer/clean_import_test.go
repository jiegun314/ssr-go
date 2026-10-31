package importer

// 导入后「原文表 + 清洗表」两张表的集成测试（用仓库里的样本工作簿）：
//
//   - 原文表（ra_input_staging）：照样是可回顾的全部列；
//   - 清洗表（ra_input_cleaned）：列 = 配置列去掉 clean.drop，行 = 一个分组一行；
//   - 清洗表的行数写进 Result.CleanedRowCount（界面/命令行可以报出来）。

import (
	"errors"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestImportWritesRawAndCleanedTables(t *testing.T) {
	importer, repository, _ := newTestImporter(t)
	rule := importer.Rules["ra_input"]
	if rule.Clean == nil {
		t.Fatal("仓库配置里 ra_input 应当声明 clean 规则")
	}

	result, err := importer.Import("ra_input", samplePath("ra_input"))
	if err != nil {
		t.Fatalf("导入失败：%v", err)
	}
	if result.RowCount != 6 {
		t.Fatalf("原文行数 = %d；want 6", result.RowCount)
	}
	if result.CleanedRowCount != 6 {
		t.Fatalf("清洗行数 = %d；want 6（样本每个规格型号一行）", result.CleanedRowCount)
	}

	rawColumns, err := repository.TableColumns("ra_input_staging")
	if err != nil {
		t.Fatalf("读原文表列失败：%v", err)
	}
	if len(rawColumns) != len(rule.Columns) {
		t.Errorf("原文表列数 = %d；want %d（全部列）", len(rawColumns), len(rule.Columns))
	}

	cleanedColumns, err := repository.TableColumns("ra_input_cleaned")
	if err != nil {
		t.Fatalf("读清洗表列失败：%v", err)
	}
	for _, dropped := range rule.Clean.Drop {
		if containsField(cleanedColumns, dropped) {
			t.Errorf("清洗表不该有 drop 掉的列 %s", dropped)
		}
	}
	if len(cleanedColumns)+len(rule.Clean.Drop) != len(rawColumns) {
		t.Errorf("清洗表列数 %d + drop %d != 原文列数 %d",
			len(cleanedColumns), len(rule.Clean.Drop), len(rawColumns))
	}

	// 清洗表的行：规格型号唯一，且保留可导出的字段
	rows, err := repository.Rows("ra_input_cleaned")
	if err != nil {
		t.Fatalf("读清洗表失败：%v", err)
	}
	if len(rows) != 6 {
		t.Fatalf("清洗表行数 = %d；want 6", len(rows))
	}
	seen := map[string]bool{}
	for _, row := range rows {
		code := row["material_code"]
		if code == "" {
			t.Error("清洗表里不该有空的 material_code")
		}
		if seen[code] {
			t.Errorf("清洗表里 material_code 重复：%s", code)
		}
		seen[code] = true
		if strings.TrimSpace(row["product_name"]) == "" {
			t.Errorf("%s 的产品名称不该为空", code)
		}
		if strings.TrimSpace(row["registration_filing_certificate_number"]) == "" {
			t.Errorf("%s 的注册证编号不该为空", code)
		}
	}
}

// TestImportRejectsConflictingGroupsBeforeWritingAnyTable 固定「先整合、再判冲突」的落地顺序：
// 同一规格型号在关键项上出现两个值 → 整份文件被拒，两张表都不写（原文表也不写）。
func TestImportRejectsConflictingGroupsBeforeWritingAnyTable(t *testing.T) {
	importer, repository, _ := newTestImporter(t)
	path := writeVariant(t, importer, "ra_input", func(
		file *excelize.File, sheet string, headerRow int, indexes map[string]int,
	) {
		set := func(row int, header string, value string) {
			name, err := excelize.CoordinatesToCellName(indexes[header], row)
			if err != nil {
				t.Fatalf("列名换算失败：%v", err)
			}
			if err := file.SetCellValue(sheet, name, value); err != nil {
				t.Fatalf("写单元格失败：%v", err)
			}
		}
		// 第二行数据改成与第一行同一个规格型号，但「是否标记为一次性使用」不同 → 冲突
		set(headerRow+2, "规格型号", "000MAT-001")
		set(headerRow+2, "是否标记为一次性使用", "否")
	})

	_, err := importer.Import("ra_input", path)
	if err == nil {
		t.Fatal("有冲突时应当整份文件被拒")
	}
	var conflict *CleanConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("应当是 CleanConflictError，实际 %T：%v", err, err)
	}
	for _, want := range []string{"个规格型号有冲突", "整份文件不导入", "是否标记为一次性使用"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("报错文案里应当有 %q：%s", want, err.Error())
		}
	}
	for _, table := range []string{"ra_input_staging", "ra_input_cleaned"} {
		exists, err := repository.TableExists(table)
		if err != nil {
			t.Fatalf("检查表 %s 失败：%v", table, err)
		}
		if exists {
			t.Errorf("%s 不该被写入（整份文件被拒）", table)
		}
	}
}
