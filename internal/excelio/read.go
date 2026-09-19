// Package excelio 是 excelize 的封装：读输入工作簿、写导出模板（R21）。
//
// 读取侧必须复刻 Python 版 `pandas.read_excel(dtype=str, keep_default_na=False,
// header=chinese_header_row-1)` 的三条语义：
//   - **只读第一个工作表**（`excel.sheet_name` 不生效，见 AGENTS.md §3.3 / §10.1 #6）；
//   - 所有单元格都当**文本**读，空单元格读作空字符串，绝不做 `NA`/`N/A` 这类归一（R1）；
//   - 表头取配置声明的中文表头行，数据从配置的数据起始行开始。
package excelio

import (
	"fmt"

	"github.com/xuri/excelize/v2"
)

// Sheet 是一个读进内存的工作表。行、列都用 Excel 的 1-based 编号。
type Sheet struct {
	Name string
	Rows [][]string
}

// ReadFirstSheet 读一个工作簿的**第一个**工作表。
func ReadFirstSheet(path string) (*Sheet, error) {
	workbook, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("Failed to open workbook %s: %w", path, err)
	}
	defer workbook.Close()
	sheets := workbook.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("Workbook %s has no worksheet", path)
	}
	name := sheets[0]
	return ReadSheet(workbook, name)
}

// ReadSheet 读指定工作表（导出模板以外的场景用不到）。
func ReadSheet(workbook *excelize.File, sheetName string) (*Sheet, error) {
	rows, err := workbook.Rows(sheetName)
	if err != nil {
		return nil, fmt.Errorf("Failed to read worksheet %s: %w", sheetName, err)
	}
	defer rows.Close()
	sheet := &Sheet{Name: sheetName}
	for rows.Next() {
		values, err := rows.Columns()
		if err != nil {
			return nil, fmt.Errorf("Failed to read a row of %s: %w", sheetName, err)
		}
		sheet.Rows = append(sheet.Rows, values)
	}
	if err := rows.Error(); err != nil {
		return nil, fmt.Errorf("Failed to read worksheet %s: %w", sheetName, err)
	}
	sheet.trimTrailingBlankRows()
	return sheet, nil
}

// Cell 返回某个单元格的文本；越界或空单元格都是空字符串。
//
// row 与 column 都是 1-based，与 Excel 的行号列号一致。
func (sheet *Sheet) Cell(row int, column int) string {
	if row < 1 || row > len(sheet.Rows) {
		return ""
	}
	values := sheet.Rows[row-1]
	if column < 1 || column > len(values) {
		return ""
	}
	return values[column-1]
}

// HeaderRow 返回第 row 行的单元格文本（用于建「列名 → 列号」映射）。
func (sheet *Sheet) HeaderRow(row int) []string {
	if row < 1 || row > len(sheet.Rows) {
		return nil
	}
	return sheet.Rows[row-1]
}

// DataRows 返回从 startRow 起的每一行数据（1-based，含起始行）。
//
// 与 pandas 一致：**只有末尾的整行空白会被裁掉**，中间的空行仍然是数据行（它会被写进
// 暂存表，但不触发必填校验，见 R2）。
func (sheet *Sheet) DataRows(startRow int) [][]string {
	if startRow < 1 {
		startRow = 1
	}
	if startRow > len(sheet.Rows) {
		return nil
	}
	return sheet.Rows[startRow-1:]
}

// trimTrailingBlankRows 去掉末尾的整行空白（对应 pandas 对尾部空行的裁剪）。
func (sheet *Sheet) trimTrailingBlankRows() {
	last := len(sheet.Rows)
	for last > 0 {
		blank := true
		for _, value := range sheet.Rows[last-1] {
			if value != "" {
				blank = false
				break
			}
		}
		if !blank {
			break
		}
		last--
	}
	sheet.Rows = sheet.Rows[:last]
}
