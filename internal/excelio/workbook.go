package excelio

import (
	"fmt"
	"path/filepath"

	"github.com/xuri/excelize/v2"
)

// WorkbookSheet 是多工作表导出中的一张表。
type WorkbookSheet struct {
	Name    string
	Columns []string
	Rows    [][]string
}

// WriteWorkbook 把多张表写成一份 xlsx；所有数据按文本写入，保留前导零。
func WriteWorkbook(target string, sheets []WorkbookSheet) error {
	if len(sheets) == 0 {
		return fmt.Errorf("没有可导出的工作表")
	}
	workbook := excelize.NewFile()
	defer workbook.Close()

	textStyle, err := workbook.NewStyle(&excelize.Style{CustomNumFmt: textNumberFormat()})
	if err != nil {
		return err
	}
	headerStyle, err := workbook.NewStyle(&excelize.Style{
		Font:         &excelize.Font{Bold: true},
		CustomNumFmt: textNumberFormat(),
	})
	if err != nil {
		return err
	}

	firstSheet := workbook.GetSheetName(0)
	for index, sheet := range sheets {
		name := sheet.Name
		if name == "" {
			name = fmt.Sprintf("Sheet%d", index+1)
		}
		if index == 0 {
			if err := workbook.SetSheetName(firstSheet, name); err != nil {
				return err
			}
		} else {
			if _, err := workbook.NewSheet(name); err != nil {
				return err
			}
		}
		if len(sheet.Columns) == 0 {
			continue
		}
		lastColumn, err := excelize.ColumnNumberToName(len(sheet.Columns))
		if err != nil {
			return err
		}
		if err := workbook.SetColStyle(name, "A:"+lastColumn, textStyle); err != nil {
			return err
		}
		header := make([]any, len(sheet.Columns))
		for columnIndex, column := range sheet.Columns {
			header[columnIndex] = column
		}
		if err := workbook.SetSheetRow(name, "A1", &header); err != nil {
			return err
		}
		for columnIndex := range sheet.Columns {
			cell, err := excelize.CoordinatesToCellName(columnIndex+1, 1)
			if err != nil {
				return err
			}
			if err := workbook.SetCellStyle(name, cell, cell, headerStyle); err != nil {
				return err
			}
		}
		for rowIndex, row := range sheet.Rows {
			values := make([]any, len(sheet.Columns))
			for columnIndex := range sheet.Columns {
				if columnIndex < len(row) {
					values[columnIndex] = row[columnIndex]
				}
			}
			cell, err := excelize.CoordinatesToCellName(1, rowIndex+2)
			if err != nil {
				return err
			}
			if err := workbook.SetSheetRow(name, cell, &values); err != nil {
				return err
			}
		}
		if err := workbook.SetPanes(name, &excelize.Panes{Freeze: true, Split: false, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
			return err
		}
	}
	if directory := filepath.Dir(target); directory != "" {
		if err := ensureDirectory(directory); err != nil {
			return err
		}
	}
	return workbook.SaveAs(target)
}
