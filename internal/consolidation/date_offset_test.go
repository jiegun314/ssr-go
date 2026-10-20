package consolidation

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/jiegun314/ssr-go/internal/config"
)

// TestDateOffsetColumnUsesTheRunDate 固定"运行日日期列"的口径：
// 值 = 运行日 + setting.yaml 里 user_defined_data.di_record_publish_date_offset_days 的天数
// （当前 5 天），格式 YYYY-MM-DD，每行同值，且**不**受来源表影响。
func TestDateOffsetColumnUsesTheRunDate(t *testing.T) {
	const column = "DI Record Publish Date"
	cases := []struct {
		name string
		now  time.Time
		want string
	}{
		{name: "普通日期", now: time.Date(2026, 10, 13, 9, 0, 0, 0, time.Local), want: "2026-10-18"},
		{name: "跨月", now: time.Date(2026, 10, 31, 23, 59, 0, 0, time.Local), want: "2026-11-05"},
		{name: "跨年", now: time.Date(2026, 12, 31, 0, 1, 0, 0, time.Local), want: "2027-01-05"},
		{name: "闰年二月", now: time.Date(2028, 2, 28, 12, 0, 0, 0, time.Local), want: "2028-03-04"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			loader, repository, importService := newWorkspace(t)
			for _, sourceName := range importService.Order {
				path := filepath.Join("..", "..", "testdata", "sample-valid", sourceName+".xlsx")
				if _, err := importService.Import(sourceName, path); err != nil {
					t.Fatalf("导入 %s 失败：%v", sourceName, err)
				}
			}
			service := newService(t, loader, repository)
			service.Now = func() time.Time { return item.now }
			outcome, err := service.Consolidate()
			if err != nil {
				t.Fatal(err)
			}
			if len(outcome.Rows) == 0 {
				t.Fatal("没有整合出任何行")
			}
			for position, row := range outcome.Rows {
				if got := row.Values[column]; got != item.want {
					t.Fatalf("第 %d 行的 %s = %q；want %q", position+1, column, got, item.want)
				}
			}
			t.Logf("%s → 全部 %d 行为 %s", item.now.Format("2006-01-02"), len(outcome.Rows), item.want)
		})
	}
}

// TestDateOffsetColumnIsIgnoredByTheChangeComparison 固定那个最容易踩的坑：
// 这一列每天都变，但它必须**不参与**变更比较 —— 否则每次整合都会把所有行判成"有差异"。
//
// 做法：先按"昨天"整合一次（写入历史记录），再按"今天"整合一次；第二次应当没有任何
// 变更描述里提到这一列（理想情况下这些行与历史记录一致 → Duplicate）。
func TestDateOffsetColumnIsIgnoredByTheChangeComparison(t *testing.T) {
	const column = "DI Record Publish Date"
	loader, repository, importService := newWorkspace(t)
	for _, sourceName := range importService.Order {
		path := filepath.Join("..", "..", "testdata", "sample-valid", sourceName+".xlsx")
		if _, err := importService.Import(sourceName, path); err != nil {
			t.Fatalf("导入 %s 失败：%v", sourceName, err)
		}
	}

	first := newService(t, loader, repository)
	first.Now = func() time.Time { return time.Date(2026, 10, 13, 9, 0, 0, 0, time.Local) }
	firstOutcome, err := first.Consolidate()
	if err != nil {
		t.Fatal(err)
	}
	if err := first.RecordConsolidationResult(time.Date(2026, 10, 13, 9, 0, 0, 0, time.Local)); err != nil {
		t.Fatalf("写历史记录失败：%v", err)
	}
	if got := firstOutcome.Rows[0].Values[column]; got != "2026-10-18" {
		t.Fatalf("第一次整合的日期列 = %q；want 2026-10-18", got)
	}

	second := newService(t, loader, repository)
	second.Now = func() time.Time { return time.Date(2026, 10, 14, 9, 0, 0, 0, time.Local) }
	secondOutcome, err := second.Consolidate()
	if err != nil {
		t.Fatal(err)
	}
	for position, row := range secondOutcome.Rows {
		if got := row.Values[column]; got != "2026-10-19" {
			t.Fatalf("第二次整合的日期列 = %q；want 2026-10-19", got)
		}
		description := row.Values["Change Description"]
		if description != "" && description != "MISSING" {
			t.Errorf("第 %d 行的变更描述不该提到运行日日期列：%q", position+1, description)
		}
	}
	// Choose Action 只看"有没有同一身份的历史记录"，所以第二次都是 Modify（这是对的）。
	// 真正要证明的是"日期列不算差异"：行状态应当是 Duplicate（与历史记录一致 → 不导出）。
	duplicate, other := 0, map[string]int{}
	for _, row := range secondOutcome.Rows {
		status := row.Values["status"]
		if status == StatusDuplicate {
			duplicate++
			continue
		}
		other[status]++
	}
	// 第一次整合有 8 行 Ready（另 1 行本来就是 Incomplete —— 没有存进历史记录，
	// 所以它第二次仍然是 Incomplete，这跟日期列无关）。
	if duplicate != 8 {
		t.Errorf("日期列被忽略后，已导出过的 8 行应当都是 Duplicate；实际 Duplicate=%d 其他=%v",
			duplicate, other)
	}
	if len(other) != 1 || other[StatusIncomplete] != 1 {
		t.Errorf("除 Duplicate 外应当只有那 1 行 Incomplete；实际 %v", other)
	}
	t.Logf("隔天再整合：%d 行全部 Duplicate（日期列不算差异）、变更描述为空 ✓", duplicate)
}

// TestDateOffsetDaysComeFromTheUserDefinedParameter 固定"天数写在参数里"的口径：
// setting.yaml 的 user_defined_data.di_record_publish_date_offset_days 改一下，导出日期
// 就跟着走（与统一社会信用代码同一类固定参数，在「参数设定」里可改），不用动整合映射。
func TestDateOffsetDaysComeFromTheUserDefinedParameter(t *testing.T) {
	const column = "DI Record Publish Date"
	loader, repository, importService := newWorkspace(t)

	path := loader.Resolver.ConfigFile(config.SettingFile)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 setting.yaml 失败：%v", err)
	}
	mutated := regexp.MustCompile(`(?m)^(\s*di_record_publish_date_offset_days:\s*)\d+`).
		ReplaceAllString(string(content), "${1}12")
	if mutated == string(content) {
		t.Fatalf("setting.yaml 里没找到 di_record_publish_date_offset_days")
	}
	if err := os.WriteFile(path, []byte(mutated), 0o644); err != nil {
		t.Fatalf("写 setting.yaml 失败：%v", err)
	}

	// 重新读配置（参数在 setting.yaml 里，改完要重新加载）
	fresh, err := config.NewLoader(loader.ConfigDir())
	if err != nil {
		t.Fatalf("重读配置失败：%v", err)
	}
	for _, sourceName := range importService.Order {
		filePath := filepath.Join("..", "..", "testdata", "sample-valid", sourceName+".xlsx")
		if _, err := importService.Import(sourceName, filePath); err != nil {
			t.Fatalf("导入 %s 失败：%v", sourceName, err)
		}
	}
	service := newService(t, fresh, repository)
	service.Now = func() time.Time { return time.Date(2026, 10, 13, 9, 0, 0, 0, time.Local) }

	outcome, err := service.Consolidate()
	if err != nil {
		t.Fatal(err)
	}
	if len(outcome.Rows) == 0 {
		t.Fatal("没有整合出任何行")
	}
	for _, row := range outcome.Rows {
		if got := row.Values[column]; got != "2026-10-25" {
			t.Fatalf("%s = %q；参数改成 12 天后应当是 2026-10-25", column, got)
		}
	}
}
