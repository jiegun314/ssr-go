package rules

import (
	"math"
	"strings"
	"testing"
)

// configured 是 global_settings.empty_values 的当前取值（R1 的条件词表）。
var configured = []any{"", nil, "NA", "N/A", "nan"}

func lookup(row map[string]any) func(string) any {
	return func(column string) any { return row[column] }
}

func mustMatch(t *testing.T, condition any, row map[string]any, emptyValues []any) bool {
	t.Helper()
	result, err := MatchesCondition(condition, lookup(row), emptyValues)
	if err != nil {
		t.Fatalf("MatchesCondition returned an error: %v", err)
	}
	return result
}

func TestR6ToNumberIgnoresEmptyAndUnparsableValues(t *testing.T) {
	for input, expected := range map[any]float64{
		"2":     2,
		2:       2,
		"1.0":   1,
		"1,000": 1000,
		" 2 ":   2,
	} {
		number, ok := ToNumber(input)
		if !ok || number != expected {
			t.Errorf("ToNumber(%#v) = %v, %v; want %v", input, number, ok, expected)
		}
	}
	for _, input := range []any{"", "NA", "2套", math.NaN(), nil} {
		if number, ok := ToNumber(input); ok {
			t.Errorf("ToNumber(%#v) = %v, true; want not a number", input, number)
		}
	}
}

func TestR1IsEmptyValueFollowsConfiguredEmptyValues(t *testing.T) {
	if !IsEmptyValue("", nil) || !IsEmptyValue("   ", nil) || !IsEmptyValue(math.NaN(), nil) {
		t.Error("空白、空串与 NaN 在条件里都算空")
	}
	if !IsEmptyValue(nil, nil) {
		t.Error("nil 在条件里算空")
	}
	if !IsEmptyValue("NA", configured) {
		t.Error("配置把 NA 声明成空值，所以 is_empty 命中")
	}
	if IsEmptyValue("Not Applicable", configured) {
		t.Error("Not Applicable 不在空值词表里，不算空")
	}
	if IsEmptyValue("2", configured) {
		t.Error("有内容的单元格不算空")
	}
}

func TestR1BlankValueOnlyReportsACellWithoutCharacters(t *testing.T) {
	// 注意 Go 与 Python 的字面量差异：Python 的 "\xa0" 是 U+00A0 这个字符，
	// Go 的 "\xa0" 是一个裸字节（不是合法 UTF-8），所以这里写 "\u00a0"。
	blank := []any{"", nil, math.NaN(), "   ", "\u3000", "\u00a0\t\r\n", "\u200b\ufeff"}
	for _, value := range blank {
		if !IsBlankValue(value) {
			t.Errorf("IsBlankValue(%#v) = false; want true", value)
		}
	}
	filled := []any{"NA", "N/A", "nan", " 2 ", "0"}
	for _, value := range filled {
		if IsBlankValue(value) {
			t.Errorf("IsBlankValue(%#v) = true; want false（占位符是用户填写的值）", value)
		}
	}
}

func TestR6NormalizeTextTrimsCellPadding(t *testing.T) {
	if NormalizeText("  Yes ") != "Yes" {
		t.Error("文本比较忽略首尾空格")
	}
	if NormalizeText(nil) != "" {
		t.Error("nil 读作空串")
	}
}

func TestR6NumericConditionComparesValuesNotText(t *testing.T) {
	rule := map[string]any{
		"column":     "quantity_per_min_sales_unit",
		"operator":   "greater_than",
		"value":      1,
		"value_type": "number",
	}
	column := "quantity_per_min_sales_unit"
	for _, value := range []any{"2", " 2 ", "2.0", "10"} {
		if !mustMatch(t, rule, map[string]any{column: value}, nil) {
			t.Errorf("%#v 应该让条件成立（数值比较，不是字符串比较）", value)
		}
	}
	for _, value := range []any{"1", "0", "", "NA", "two"} {
		if mustMatch(t, rule, map[string]any{column: value}, nil) {
			t.Errorf("%#v 不该让条件成立（空值与非数字让规则保持沉默）", value)
		}
	}
}

func TestR6TextConditionOperators(t *testing.T) {
	row := map[string]any{"if_direct_marking": " Yes "}
	cases := []struct {
		condition map[string]any
		expected  bool
	}{
		{map[string]any{"column": "if_direct_marking", "operator": "equals", "value": "Yes"}, true},
		{map[string]any{"column": "if_direct_marking", "operator": "not_equals", "value": "No"}, true},
		{map[string]any{"column": "if_direct_marking", "operator": "in", "value": []any{"是", "Yes"}}, true},
		{map[string]any{"column": "if_direct_marking", "operator": "not_in", "value": []any{"No"}}, true},
		{map[string]any{"column": "if_direct_marking", "operator": "is_not_empty"}, true},
		{map[string]any{"column": "if_direct_marking", "operator": "is_empty"}, false},
	}
	for _, testCase := range cases {
		got := mustMatch(t, testCase.condition, row, nil)
		if got != testCase.expected {
			t.Errorf("%v = %v; want %v", testCase.condition["operator"], got, testCase.expected)
		}
	}
}

func TestR6EmptyConditionUsesConfiguredEmptyValues(t *testing.T) {
	rule := map[string]any{"column": "package_identifier", "operator": "is_empty"}
	row := map[string]any{"package_identifier": "NA"}
	if mustMatch(t, rule, row, nil) {
		t.Error("没有空值词表时 NA 不算空")
	}
	for _, value := range []any{"NA", "N/A"} {
		row := map[string]any{"package_identifier": value}
		if !mustMatch(t, rule, row, configured) {
			t.Errorf("空值词表里声明了 %#v，条件应命中", value)
		}
	}
}

func TestR6ConditionListsAndGroupsCombineConditions(t *testing.T) {
	row := map[string]any{"a": "2", "b": "Yes"}
	numeric := map[string]any{"column": "a", "operator": "greater_than", "value": 1, "value_type": "number"}
	text := map[string]any{"column": "b", "operator": "equals", "value": "No"}
	otherText := map[string]any{"column": "b", "operator": "equals", "value": "Yes"}

	if !mustMatch(t, []any{numeric, otherText}, row, nil) {
		t.Error("列表内的条件按 AND 组合")
	}
	if mustMatch(t, []any{numeric, text}, row, nil) {
		t.Error("列表内有一条不成立，整体不成立")
	}
	if !mustMatch(t, map[string]any{"all": []any{numeric, otherText}}, row, nil) {
		t.Error("all 等价于列表")
	}
	if mustMatch(t, map[string]any{"all": []any{numeric, text}}, row, nil) {
		t.Error("all 要求全部成立")
	}
	if !mustMatch(t, map[string]any{"any": []any{numeric, text}}, row, nil) {
		t.Error("any 是或（OR）")
	}
	if mustMatch(t, map[string]any{"any": []any{text}}, row, nil) {
		t.Error("any 里没有成立的条件时不成立")
	}
}

func TestR6ConditionCanBeRenderedForErrorMessages(t *testing.T) {
	rule := map[string]any{
		"column":     "quantity_per_min_sales_unit",
		"operator":   "greater_than",
		"value":      1,
		"value_type": "number",
	}
	describeColumn := func(dbField string) string { return dbField + "（数量）" }

	withoutValue := DescribeCondition(rule, describeColumn, nil)
	if withoutValue != "quantity_per_min_sales_unit（数量） > 1" {
		t.Errorf("文案不对：%q", withoutValue)
	}
	withValue := DescribeCondition(rule, describeColumn, lookup(map[string]any{
		"quantity_per_min_sales_unit": "2",
	}))
	if withValue != "quantity_per_min_sales_unit（数量） = 2 > 1" {
		t.Errorf("带行值的文案不对：%q", withValue)
	}
}

func TestR6ValidateConditionSpecAcceptsADeclaredRule(t *testing.T) {
	available := []string{"quantity_per_min_sales_unit", "device_identifier_use_unit"}
	if err := ValidateConditionSpec(map[string]any{
		"column": "quantity_per_min_sales_unit", "operator": "greater_than",
		"value": 1, "value_type": "number",
	}, available, "required_when", "device_identifier_use_unit"); err != nil {
		t.Errorf("合法规则被判为非法：%v", err)
	}
	if err := ValidateConditionSpec(map[string]any{
		"any": []any{
			map[string]any{"column": "package_identifier", "operator": "is_not_empty"},
			map[string]any{"column": "package_identifier", "operator": "in",
				"value": []any{"Yes", "No"}},
		},
	}, []string{"package_identifier"}, "required_when", ""); err != nil {
		t.Errorf("合法分组被判为非法：%v", err)
	}
	if err := ValidateConditionSpec([]any{
		map[string]any{"column": "package_identifier", "operator": "is_not_empty"},
		map[string]any{"column": "package_identifier", "operator": "equals", "value": "Yes"},
	}, []string{"package_identifier"}, "required_when", ""); err != nil {
		t.Errorf("合法列表被判为非法：%v", err)
	}
}

func TestR6ValidateConditionSpecRejectsInvalidRules(t *testing.T) {
	cases := []struct {
		name      string
		condition any
		message   string
	}{
		{"unknown column", map[string]any{"column": "unknown", "operator": "equals", "value": "1"}, "unknown condition column"},
		{"unknown operator", map[string]any{"column": "a", "operator": "between", "value": 1}, "unknown condition operator"},
		{"ordering without number", map[string]any{"column": "a", "operator": "greater_than", "value": 1}, "requires value_type: number"},
		{"non numeric value", map[string]any{"column": "a", "operator": "equals", "value": "one", "value_type": "number"}, "must be numeric"},
		{"missing value", map[string]any{"column": "a", "operator": "equals"}, "requires a value"},
		{"unary with value", map[string]any{"column": "a", "operator": "is_empty", "value": "x"}, "does not take a value"},
		{"in with a scalar", map[string]any{"column": "a", "operator": "in", "value": "Yes"}, "non-empty list"},
		{"unknown key", map[string]any{"column": "a", "operator": "equals", "value": "x", "extra": "y"}, "unknown keys"},
		{"empty list", []any{}, "must not be an empty list"},
		{"mixed group", map[string]any{"all": []any{map[string]any{"column": "a", "operator": "is_not_empty"}}, "unexpected": "x"}, "cannot mix"},
		{"group without list", map[string]any{"all": "Yes"}, "non-empty list of conditions"},
		{"not a condition", "quantity", "must be a condition mapping"},
		{"unknown value type", map[string]any{"column": "a", "operator": "equals", "value": "x", "value_type": "json"}, "value_type must be one of"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := ValidateConditionSpec(testCase.condition, []string{"a"}, "required_when", "")
			if err == nil {
				t.Fatalf("非法规则 %s 被接受了", testCase.name)
			}
			if !strings.Contains(err.Error(), testCase.message) {
				t.Errorf("错误文案 %q 不含 %q", err.Error(), testCase.message)
			}
		})
	}
}

func TestR6ValidateConditionSpecRejectsSelfReference(t *testing.T) {
	err := ValidateConditionSpec(
		map[string]any{"column": "a", "operator": "is_empty"},
		[]string{"a"},
		"required_when",
		"a",
	)
	if err == nil || !strings.Contains(err.Error(), "must reference another column") {
		t.Errorf("自引用应被拒绝，得到：%v", err)
	}
}

func TestR6ConditionOperatorWhitelistCoversOrderingAndEquality(t *testing.T) {
	if !containsString(ConditionOperators, "greater_than") {
		t.Error("greater_than 应在白名单里")
	}
	if !containsString(ConditionOperators, "is_empty") {
		t.Error("is_empty 应在白名单里")
	}
	if containsString(ConditionOperators, "like") {
		t.Error("未声明的操作符不该在白名单里")
	}
}

func TestR6MatchesConditionRejectsUnsupportedConditionShape(t *testing.T) {
	_, err := MatchesCondition("quantity > 1", lookup(map[string]any{}), nil)
	if err == nil || !strings.Contains(err.Error(), "Unsupported condition") {
		t.Errorf("字符串条件应被拒绝，得到：%v", err)
	}
}
