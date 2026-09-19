package main

import (
	"os"
	"path/filepath"

	"github.com/xuri/excelize/v2"
)

// writeReviewWorkbook 把回顾数据写成一份新的 Excel：第一行是中文表头，之后是数据。
//
// 与现状实现一致（pandas `to_excel(index=False)`）：新建工作簿、列序 = 配置顺序、
// 所有值按文本写（导入的单元格本来就是文本，前导零不会丢）。
func writeReviewWorkbook(target string, columns []string, rows [][]string) error {
	workbook := excelize.NewFile()
	defer workbook.Close()
	const sheet = "Sheet1"
	for index, column := range columns {
		cell, err := excelize.CoordinatesToCellName(index+1, 1)
		if err != nil {
			return err
		}
		if err := workbook.SetCellStr(sheet, cell, column); err != nil {
			return err
		}
	}
	textStyle, err := workbook.NewStyle(&excelize.Style{CustomNumFmt: textNumberFormat()})
	if err != nil {
		return err
	}
	for rowIndex, row := range rows {
		for columnIndex, value := range row {
			cell, err := excelize.CoordinatesToCellName(columnIndex+1, rowIndex+2)
			if err != nil {
				return err
			}
			if err := workbook.SetCellStr(sheet, cell, value); err != nil {
				return err
			}
			if err := workbook.SetCellStyle(sheet, cell, cell, textStyle); err != nil {
				return err
			}
		}
	}
	if directory := filepath.Dir(target); directory != "" {
		if err := ensureDirectory(directory); err != nil {
			return err
		}
	}
	return workbook.SaveAs(target)
}

// textNumberFormat 是「按文本」的数字格式（与导出模板写入时一致，前导零不丢）。
func textNumberFormat() *string {
	format := "@"
	return &format
}

func ensureDirectory(path string) error {
	return os.MkdirAll(path, 0o755)
}
