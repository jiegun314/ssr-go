package app

// 整合表「导出当前视图」的后端行为：前端传来的表头与筛选结果原样落成 xlsx，
// 不经过模板、不要求四份来源已对齐，也不影响正式「生成文件」的合格行口径。

import (
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestExportConsolidationViewWritesDisplayedGrid(t *testing.T) {
	app := newShareApp(t, "")
	if login := app.LoginAdmin("SsrAdmin@2026", "local"); login.Failed {
		t.Fatalf("管理员登录失败：%+v", login)
	}
	target := filepath.Join(t.TempDir(), "整合表格.xlsx")
	columns := []string{"Primary DI", "Product Name", "Medical Insurance Code"}
	rows := [][]string{
		{"DI-001", "监护仪", "12345"},
		{"DI-002", "缺失", ""},
	}
	result := app.ExportConsolidationView(columns, rows, target)
	if result.Failed {
		t.Fatalf("导出当前视图失败：%+v", result)
	}
	if result.Path != target {
		t.Fatalf("返回路径不对：%q", result.Path)
	}

	workbook, err := excelize.OpenFile(target)
	if err != nil {
		t.Fatalf("打开导出文件失败：%v", err)
	}
	defer workbook.Close()
	cellValue := func(column, row int) string {
		t.Helper()
		cell, err := excelize.CoordinatesToCellName(column, row)
		if err != nil {
			t.Fatalf("坐标转换失败：%v", err)
		}
		value, err := workbook.GetCellValue("Sheet1", cell)
		if err != nil {
			t.Fatalf("读取单元格 %s 失败：%v", cell, err)
		}
		return value
	}
	for index, want := range columns {
		if got := cellValue(index+1, 1); got != want {
			t.Errorf("第 %d 列表头 = %q，期望 %q", index+1, got, want)
		}
	}
	for rowIndex, want := range rows {
		for columnIndex, value := range want {
			if got := cellValue(columnIndex+1, rowIndex+2); got != value {
				t.Errorf("第 %d 行第 %d 列 = %q，期望 %q", rowIndex+2, columnIndex+1, got, value)
			}
		}
	}
}
