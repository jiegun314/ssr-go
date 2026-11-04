package main

// RA 导入第一步：删除失效注册证（RA失效日期 < 今天 的行），原文表/清洗表都不再包含它们。

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func TestRAImportDropsExpiredCertificates(t *testing.T) {
	app := newShareApp(t, "")
	// 以样本为底，追加三行：昨天到期 / 今天到期 / 一年后到期
	source := filepath.Join("testdata", "sample-valid", "ra_input.xlsx")
	workbook, err := excelize.OpenFile(source)
	if err != nil {
		t.Fatal(err)
	}
	sheet := workbook.GetSheetList()[0]
	rows, err := workbook.GetRows(sheet)
	if err != nil {
		t.Fatal(err)
	}
	header := rows[0]
	column := func(title string) int {
		for index, value := range header {
			if value == title {
				return index
			}
		}
		t.Fatalf("样本里找不到列 %q", title)
		return -1
	}
	material := column("规格型号")
	cert := column("注册证编号/备案凭证编号")
	expiry := column("RA失效日期")
	today := time.Now()
	dates := []string{
		today.AddDate(0, 0, -1).Format("2006-01-02"),
		today.Format("2006/01/02"),
		today.AddDate(1, 0, 0).Format("2006-01-02"),
	}
	base := append([]string{}, rows[1]...)
	for index, date := range dates {
		row := append([]string{}, base...)
		for len(row) < len(header) {
			row = append(row, "")
		}
		row[material] = "ZZ-EXP-" + string(rune('A'+index))
		row[cert] = "国械注进2099000000" + string(rune('1'+index))
		row[expiry] = date
		rows = append(rows, row)
	}
	for rowIndex, row := range rows {
		for columnIndex, value := range row {
			cell, _ := excelize.CoordinatesToCellName(columnIndex+1, rowIndex+1)
			if value != "" {
				_ = workbook.SetCellStr(sheet, cell, value)
			}
		}
	}
	target := filepath.Join(t.TempDir(), "ra-expiry.xlsx")
	if err := workbook.SaveAs(target); err != nil {
		t.Fatal(err)
	}
	workbook.Close()

	result := app.ImportSource("ra_input", target)
	if result.Failed {
		t.Fatalf("导入失败：%+v", result)
	}
	if result.DroppedExpired != 1 {
		t.Errorf("应当删除 1 行失效注册证，实际 %d（行数 %d）", result.DroppedExpired, result.RowCount)
	}
	// 原文表与清洗表都不包含被删的那一行
	staging, err := app.repo.CountTableRows("ra_input_staging")
	if err != nil {
		t.Fatal(err)
	}
	if staging != result.RowCount {
		t.Errorf("原文表行数应当等于导入行数：%d / %d", staging, result.RowCount)
	}
	cleaned, err := app.repo.Rows("ra_input_cleaned")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range cleaned {
		if row["material_code"] == "ZZ-EXP-A" {
			t.Error("失效注册证不应出现在清洗表里")
		}
	}
	_ = os.Remove(target)
}
