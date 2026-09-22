package sample

import "testing"

// TestSampleHintReadsTheNewSubBlockAndFallsBackToTheOldShape 固定样本工厂读配置的方式：
// 新配置把 english_name / data_type / allowed_values 收在列的 sample: 子块里（一眼看得出
// 导入不读），老配置是平铺在列上的。两者都要能读 —— 用户目录里那份不会被升级覆盖的旧配置
// 必须继续生成出同样的样本文件。
func TestSampleHintReadsTheNewSubBlockAndFallsBackToTheOldShape(t *testing.T) {
	newShape := map[string]any{
		"chinese_name": "批准日期",
		"sample": map[string]any{
			"english_name":   "Approval Date",
			"data_type":      "date",
			"allowed_values": []any{"2026-01-01"},
		},
	}
	oldShape := map[string]any{
		"chinese_name":   "批准日期",
		"english_name":   "Approval Date",
		"data_type":      "date",
		"allowed_values": []any{"2026-01-01"},
	}
	for name, column := range map[string]map[string]any{"新写法(sample 子块)": newShape, "老写法(平铺)": oldShape} {
		if got := textValue(sampleHint(column, "english_name")); got != "Approval Date" {
			t.Errorf("%s 的 english_name = %q", name, got)
		}
		if got := textValue(sampleHint(column, "data_type")); got != "date" {
			t.Errorf("%s 的 data_type = %q", name, got)
		}
		values, _ := sampleHint(column, "allowed_values").([]any)
		if len(values) != 1 || values[0] != "2026-01-01" {
			t.Errorf("%s 的 allowed_values = %v", name, values)
		}
	}

	// 两种写法同时存在时以子块为准（新结构是权威）
	both := map[string]any{
		"english_name": "旧值",
		"sample":       map[string]any{"english_name": "新值"},
	}
	if got := textValue(sampleHint(both, "english_name")); got != "新值" {
		t.Errorf("两种写法同时存在时应以 sample 子块为准，得到 %q", got)
	}
	// 都没有时返回 nil，交给调用方兜底
	if got := sampleHint(map[string]any{"chinese_name": "x"}, "data_type"); got != nil {
		t.Errorf("没有该键时应返回 nil，得到 %#v", got)
	}
}
