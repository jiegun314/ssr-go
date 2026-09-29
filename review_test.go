package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// TestReviewLogSendsSlashedTimestamps 固定「界面 → 日志表」的时间戳格式契约。
//
// operation_log.log_time 存的是 `YYYY/MM/DD HH:MM:SS`（R22），按区间取记录又是**字符串
// 比较**。日期框给的是 `YYYY-MM-DD`，如果直接把横线格式拼上时刻，同一个年份里
// `/`(0x2F) > `-`(0x2D) 会让 `记录 <= 结束时间` 恒为假 —— 记录导出窗口一条都查不出来
// （这就是"表里有数据、窗口却全是空"的根因）。所以 toLogTime 必须换成斜杠，
// 而日期框本身的显示格式保持 YYYY-MM-DD。
func TestReviewLogSendsSlashedTimestamps(t *testing.T) {
	bridge := readFrontendSource(t, "src/bridge.ts")
	body := functionBody(t, bridge, "export function toLogTime")
	if !strings.Contains(body, `replace(/-/g, "/")`) {
		t.Errorf("toLogTime 要把日期框的 YYYY-MM-DD 转成 YYYY/MM/DD 再拼接时刻，实际实现：\n%s", body)
	}
	if !strings.Contains(body, "23:59:59") || !strings.Contains(body, "00:00:00") {
		t.Errorf("起止时刻应当补成 00:00:00 / 23:59:59，实际：\n%s", body)
	}
	panels := readFrontendSource(t, "src/components/Panels.tsx")
	if !strings.Contains(panels, `format="YYYY-MM-DD"`) {
		t.Error("日期框的显示格式应当保持 YYYY-MM-DD（只有查询用的时间戳改格式）")
	}
}

// TestReviewLogFindsTheRecordWrittenToday 复现用户场景：数据库里有记录，
// 用界面默认的「一年前到今天 23:59:59」时间窗必须能查到今天写进去的那条。
func TestReviewLogFindsTheRecordWrittenToday(t *testing.T) {
	workspace := t.TempDir()
	copyDirectoryForTest(t, "config", filepath.Join(workspace, "config"))
	t.Setenv("UDI_CONFIG_DIR", filepath.Join(workspace, "config"))
	app := NewApp(nil)
	app.startup(context.Background())
	if app.log == nil {
		t.Fatal("日志表没有初始化")
	}

	now := time.Now()
	app.log.Now = func() string { return now.Format("2006/01/02 15:04:05") }
	if err := app.log.AppendRecords([]store.OperationLogRecord{{
		Key: "MAT-1",
		Values: store.Row{
			"Catalog or Reference Number": "MAT-1",
			"Primary DI":                  "DI-1",
		},
	}}, "material_code"); err != nil {
		t.Fatalf("写日志失败：%v", err)
	}

	// 界面侧的计算方式：起始＝一年前的今天 00:00:00，结束＝今天 23:59:59（斜杠格式）
	start := now.AddDate(-1, 0, 0).Format("2006/01/02") + " 00:00:00"
	end := now.Format("2006/01/02") + " 23:59:59"
	result := app.ReviewLog(start, end, 1, 100)
	if result.Total != 1 || len(result.Rows) != 1 {
		t.Fatalf("默认时间窗（%s → %s）应当查到刚写入的那条记录，实际 Total=%d rows=%d；日志：%s",
			start, end, result.Total, len(result.Rows), result.Log)
	}
	if len(result.Columns) == 0 || len(result.Rows[0]) != len(result.Columns) {
		t.Errorf("返回的表头与行宽不一致：columns=%d，第一行=%d", len(result.Columns), len(result.Rows[0]))
	}
	if result.Columns[0] != "log_time" {
		t.Errorf("第一列应当是 log_time，实际 %q", result.Columns[0])
	}
}

// functionBody 从源码里截出某个函数的实现（前端契约测试只读源码文本）。
func functionBody(t *testing.T, source string, signature string) string {
	t.Helper()
	start := strings.Index(source, signature)
	if start < 0 {
		t.Fatalf("源码里找不到函数：%s", signature)
	}
	rest := source[start:]
	end := strings.Index(rest, "\n}")
	if end < 0 {
		t.Fatalf("函数 %s 没有找到收尾", signature)
	}
	return rest[:end+2]
}
