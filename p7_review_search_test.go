package main

// 回顾窗口的查找（用户要求）：数据来源回顾与记录导出都支持关键字筛选，
// 口径与数据整合结果表一致（任一列包含即命中，分页在筛选后的结果上做）。

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewSourceSupportsKeywordFilter(t *testing.T) {
	app := newShareApp(t, "")
	if result := app.ImportSource("ra_input", filepath.Join("testdata", "sample-valid", "ra_input.xlsx")); result.Failed {
		t.Fatalf("导入失败：%+v", result)
	}
	all := app.ReviewSource("ra_input", 1, 100, "")
	if all.Failed || all.Total == 0 {
		t.Fatalf("应当有数据：%+v", all)
	}
	// 用第一行第一列的一个片段做关键字（保证命中且通常不是全部）
	sample := strings.TrimSpace(all.Rows[0][0])
	if sample == "" {
		t.Skip("样本第一列为空，跳过")
	}
	substring := sample
	if len([]rune(substring)) > 4 {
		substring = string([]rune(substring)[:4])
	}
	filtered := app.ReviewSource("ra_input", 1, 100, substring)
	if filtered.Failed {
		t.Fatalf("筛选失败：%+v", filtered)
	}
	if filtered.Total == 0 || filtered.Total > all.Total {
		t.Fatalf("筛选结果应当非空且不超过总数：%d / %d", filtered.Total, all.Total)
	}
	// 不存在的关键字 → 0 行
	none := app.ReviewSource("ra_input", 1, 100, "绝不可能出现的字符串-zzz")
	if none.Failed || none.Total != 0 || len(none.Rows) != 0 {
		t.Fatalf("不存在的关键字应当返回 0 行：%+v", none)
	}
}

func TestReviewLogSupportsKeywordFilter(t *testing.T) {
	app := newShareApp(t, "")
	for _, source := range []string{"ra_input", "global_udi_input", "medical_insurance_code", "product_category"} {
		if result := app.ImportSource(source, filepath.Join("testdata", "sample-valid", source+".xlsx")); result.Failed {
			t.Fatalf("导入 %s 失败：%+v", source, result)
		}
	}
	if result := app.Consolidate(); result.Failed {
		t.Fatalf("整合失败：%+v", result)
	}
	if result := app.Export("", filepath.Join(t.TempDir(), "log.xlsx")); result.Failed {
		t.Fatalf("导出失败：%+v", result)
	}
	all := app.ReviewLog("", "", 1, 1000, "")
	if all.Failed || all.Total == 0 {
		t.Fatalf("记录导出应当有数据：%+v", all)
	}
	first := strings.TrimSpace(all.Rows[0][0])
	if first == "" {
		t.Skip("记录第一列为空，跳过")
	}
	substring := first
	if len([]rune(substring)) > 4 {
		substring = string([]rune(substring)[:4])
	}
	filtered := app.ReviewLog("", "", 1, 1000, substring)
	if filtered.Failed || filtered.Total == 0 || filtered.Total > all.Total {
		t.Fatalf("记录导出筛选不对：%+v（总数 %d）", filtered, all.Total)
	}
	// 分页在筛选结果上做
	paged := app.ReviewLog("", "", 2, 1, substring)
	if paged.Failed || paged.Page != 2 || len(paged.Rows) > 1 {
		t.Fatalf("筛选后的分页不对：%+v", paged)
	}
}
