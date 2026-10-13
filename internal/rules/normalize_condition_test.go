package rules

import "testing"

// TestNormalizeConditionValuesUsesTheReferencedColumnsMapping 固定 C-1 口径：
// 条件里的期望值按**被引用列自己**的 value_mapping 归一化，所以配置可以按
// "人怎么填"来写（是 / 否 / Y），程序换算成归一化后的值再比。
func TestNormalizeConditionValuesUsesTheReferencedColumnsMapping(t *testing.T) {
	yesNoMixed := map[string]string{"是": "Yes", "否": "No", "Y": "Yes", "N": "No", "Yes": "Yes", "No": "No"}
	mappingOf := func(column string) map[string]string {
		if column == "sterilization_flag" {
			return yesNoMixed
		}
		return nil
	}
	cases := []struct {
		name      string
		condition any
		want      any
	}{
		{
			name:      "列表形态（AND）：中文期望值被归一化",
			condition: []any{map[string]any{"column": "sterilization_flag", "operator": "equals", "value": "是", "value_type": "text"}},
			want:      []any{map[string]any{"column": "sterilization_flag", "operator": "equals", "value": "Yes", "value_type": "text"}},
		},
		{
			name: "分组形态（any）：同样归一化",
			condition: map[string]any{"any": []any{
				map[string]any{"column": "sterilization_flag", "operator": "equals", "value": "否"},
			}},
			want: map[string]any{"any": []any{
				map[string]any{"column": "sterilization_flag", "operator": "equals", "value": "No"},
			}},
		},
		{
			name:      "大小写与空白也算命中",
			condition: []any{map[string]any{"column": "sterilization_flag", "operator": "equals", "value": " YES "}},
			want:      []any{map[string]any{"column": "sterilization_flag", "operator": "equals", "value": "Yes"}},
		},
		{
			name:      "列表期望值（in）逐个归一化",
			condition: []any{map[string]any{"column": "sterilization_flag", "operator": "in", "value": []any{"是", "Y"}}},
			want:      []any{map[string]any{"column": "sterilization_flag", "operator": "in", "value": []any{"Yes", "Yes"}}},
		},
		{
			name:      "没有映射的列原样保留",
			condition: []any{map[string]any{"column": "quantity", "operator": "greater_than", "value": 1.0, "value_type": "number"}},
			want:      []any{map[string]any{"column": "quantity", "operator": "greater_than", "value": 1.0, "value_type": "number"}},
		},
		{
			name:      "映射里没有的写法原样保留",
			condition: []any{map[string]any{"column": "sterilization_flag", "operator": "equals", "value": "或许"}},
			want:      []any{map[string]any{"column": "sterilization_flag", "operator": "equals", "value": "或许"}},
		},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			got := NormalizeConditionValues(item.condition, mappingOf)
			if !sameCondition(got, item.want) {
				t.Errorf("归一化结果 = %#v；want %#v", got, item.want)
			}
		})
	}
}

// sameCondition 递归比较条件树（数字统一按 float64 比）。
func sameCondition(left, right any) bool {
	switch leftTyped := left.(type) {
	case []any:
		rightTyped, ok := right.([]any)
		if !ok || len(leftTyped) != len(rightTyped) {
			return false
		}
		for index := range leftTyped {
			if !sameCondition(leftTyped[index], rightTyped[index]) {
				return false
			}
		}
		return true
	case map[string]any:
		rightTyped, ok := right.(map[string]any)
		if !ok || len(leftTyped) != len(rightTyped) {
			return false
		}
		for key, value := range leftTyped {
			other, present := rightTyped[key]
			if !present || !sameCondition(value, other) {
				return false
			}
		}
		return true
	default:
		return left == right
	}
}
