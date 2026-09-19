package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jiegun314/ssr-go/internal/store"
)

// TestReviewLogFiltersByTimeRangeAndPages 固定「记录导出」的时间语义：
// 起止时间都是 YYYY/MM/DD HH:MM:SS（闭区间），结束取当天 23:59:59 时必须包含当天记录；
// 并把分页（每页 100 行）与总行数一起钉住。
func TestReviewLogFiltersByTimeRangeAndPages(t *testing.T) {
	workspace := t.TempDir()
	copyDirectoryForTest(t, "config", filepath.Join(workspace, "config"))
	t.Setenv("UDI_CONFIG_DIR", filepath.Join(workspace, "config"))
	app := NewApp(nil)
	app.startup(context.Background())
	if app.log == nil {
		t.Fatal("日志表没有初始化")
	}

	for _, stamp := range []string{
		"2026/01/05 10:00:00", "2026/01/20 10:00:00", "2026/03/01 10:00:00",
	} {
		app.log.Now = func() string { return stamp }
		if err := app.log.AppendRecords([]store.OperationLogRecord{{
			Key: "MAT-1",
			Values: store.Row{
				"Catalog or Reference Number": "MAT-1",
				"Primary DI":                  "DI-1",
			},
		}}, "material_code"); err != nil {
			t.Fatalf("写日志失败：%v", err)
		}
	}

	// ① 区间只覆盖 1 月的两条
	result := app.ReviewLog("2025/12/31 00:00:00", "2026/01/31 23:59:59", 1, 100)
	if result.Total != 2 || len(result.Rows) != 2 {
		t.Errorf("1 月区间：Total=%d rows=%d; want 2/2（日志：%s）", result.Total, len(result.Rows), result.Log)
	}
	if result.PageCount != 1 {
		t.Errorf("每页 100 行时总页数 = %d; want 1", result.PageCount)
	}

	// ② 结束时间取当天 23:59:59 时，当天 10:00 的记录必须在内（这是之前漏掉的那类 bug）
	boundary := app.ReviewLog("2026/01/20 00:00:00", "2026/01/20 23:59:59", 1, 100)
	if boundary.Total != 1 {
		t.Errorf("当天 23:59:59 为终点时应包含当天记录，Total=%d", boundary.Total)
	}

	// ③ 分页：每页 1 行 → 第 2 页仍有 1 行
	page2 := app.ReviewLog("2025/12/31 00:00:00", "2026/01/31 23:59:59", 2, 1)
	if page2.Total != 2 || page2.PageCount != 2 || len(page2.Rows) != 1 || page2.Page != 2 {
		t.Errorf("分页结果不对：Total=%d PageCount=%d Page=%d rows=%d",
			page2.Total, page2.PageCount, page2.Page, len(page2.Rows))
	}
}
