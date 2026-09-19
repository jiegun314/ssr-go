// Package rules 承载配置驱动的单元格条件求值。
//
// 这是 core/condition_rules.py 的 1:1 移植，行为由 tests/test_condition_rules.py
// 与 AGENTS.md 的规则编号定义：
//
//	R1  空值语义：IsBlankValue 只认「没有任何字符」，IsEmptyValue 用配置的空值词表
//	R6  required_when 条件规则：操作符白名单、AND/OR 分组、value_type、空值语义
//
// 本包只做比较本身：行值由调用方通过 getValue 提供（导入的 Excel 行、整合输出行
// 都能复用同一套词表）。包内不读文件、不依赖 YAML 库，因此可离线测试。
package rules

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// ConditionOperators 是条件可以声明的全部操作符（R6）。
var ConditionOperators = []string{
	"equals",
	"not_equals",
	"in",
	"not_in",
	"is_empty",
	"is_not_empty",
	"greater_than",
	"greater_or_equal",
	"less_than",
	"less_or_equal",
}

// UnaryOperators 不需要 value。
var UnaryOperators = []string{"is_empty", "is_not_empty"}

// OrderingOperators 比较数值，必须声明 value_type: number。
var OrderingOperators = []string{
	"greater_than",
	"greater_or_equal",
	"less_than",
	"less_or_equal",
}

// MultiValueOperators 接受列表。
var MultiValueOperators = []string{"in", "not_in"}

// ConditionValueTypes 是 value_type 的合法词表。
var ConditionValueTypes = []string{"text", "number"}

// DefaultValueType 是未声明 value_type 时的取值。
const DefaultValueType = "text"

// GroupKeys 是条件分组的两个键。
var GroupKeys = []string{"all", "any"}

// AllowedConditionKeys 是叶子条件允许出现的键。
var AllowedConditionKeys = map[string]bool{
	"column": true, "operator": true, "value": true, "value_type": true,
}

// OperatorSymbols 是报错文案里用的符号（与 Python 版逐字一致）。
var OperatorSymbols = map[string]string{
	"equals":           " = ",
	"not_equals":       " ≠ ",
	"in":               " 属于 ",
	"not_in":           " 不属于 ",
	"is_empty":         " 为空",
	"is_not_empty":     " 不为空",
	"greater_than":     " > ",
	"greater_or_equal": " ≥ ",
	"less_than":        " < ",
	"less_or_equal":    " ≤ ",
}

// ConditionSpecError 说明一条 required_when 规则违反了它的契约。
type ConditionSpecError struct {
	message string
}

func (e *ConditionSpecError) Error() string { return e.message }

func specErrorf(format string, arguments ...any) *ConditionSpecError {
	return &ConditionSpecError{message: fmt.Sprintf(format, arguments...)}
}

// IsEmptyValue 判断一个单元格在**条件判断**里算不算空。
//
// emptyValues 是 global_settings.empty_values 声明的词表。注意它只用于条件
// （R1）：必填校验用的是 IsBlankValue，两者对 NA / N/A 的结论正好相反。
func IsEmptyValue(value any, emptyValues []any) bool {
	if value == nil {
		return true
	}
	if number, ok := asFloat(value); ok && math.IsNaN(number) {
		return true
	}
	text := strings.TrimSpace(PyStr(value))
	if text == "" {
		return true
	}
	for _, configured := range emptyValues {
		if configured == nil {
			continue
		}
		if text == strings.TrimSpace(PyStr(configured)) {
			return true
		}
	}
	return false
}

// IsBlankValue 判断单元格是不是**完全没有任何字符**（R1，必填校验用）。
//
// 算空：None、Excel 空单元格读出的 NaN、空字符串、只由空白字符（半角/全角空格、
// 不换行空格、制表与换行）组成的单元格、只由不可见格式字符（零宽空格、BOM）
// 组成的单元格。不算空：NA、N/A、nan、0 这类用户填写的值。
func IsBlankValue(value any) bool {
	if value == nil {
		return true
	}
	if number, ok := asFloat(value); ok && math.IsNaN(number) {
		return true
	}
	for _, character := range PyStr(value) {
		if !isInvisibleRune(character) {
			return false
		}
	}
	return true
}

// isInvisibleRune 判断一个字符是不是只有空白或不可见格式标记。
func isInvisibleRune(character rune) bool {
	return unicode.IsSpace(character) || unicode.Is(unicode.Cf, character)
}

// NormalizeText 做文本比较前的归一：去掉首尾空白，nil 读作空串。
func NormalizeText(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(PyStr(value))
}

// ToNumber 返回单元格的数值，非数字时返回 ok=false。
//
// 千分位会被去掉（"1,000" → 1000）。空值与无法解析的值返回 false：那种行不能
// 决定条件，规则对它保持沉默（R6）。
func ToNumber(value any) (float64, bool) {
	if IsEmptyValue(value, nil) {
		return 0, false
	}
	text := strings.ReplaceAll(NormalizeText(value), ",", "")
	number, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsNaN(number) {
		return 0, false
	}
	return number, true
}

// MatchesCondition 对一行数据求值一条条件、一个条件列表或一个分组（R6）。
//
// 列表与 all 组按 AND 组合，any 组按 OR 组合：这与 Excel 里「条件成立」的直觉
// 一致，也是配置里唯一允许的两种组合方式。
func MatchesCondition(
	condition any,
	getValue func(string) any,
	emptyValues []any,
) (bool, error) {
	switch typed := condition.(type) {
	case map[string]any:
		for _, groupKey := range GroupKeys {
			members, hasGroup := typed[groupKey]
			if !hasGroup {
				continue
			}
			memberList, ok := members.([]any)
			if !ok {
				// 单个成员写成映射时，Python 的列表推导同样会把它当成一条条件，
				// 所以这里只在类型完全不对时报配置错误。
				return false, specErrorf(
					"Unsupported condition group %s: %v", groupKey, members)
			}
			results := make([]bool, 0, len(memberList))
			for _, member := range memberList {
				result, err := MatchesCondition(member, getValue, emptyValues)
				if err != nil {
					return false, err
				}
				results = append(results, result)
			}
			if groupKey == "all" {
				return allTrue(results), nil
			}
			return anyTrue(results), nil
		}
		return matchesLeaf(typed, getValue, emptyValues)
	case []any:
		for _, member := range typed {
			result, err := MatchesCondition(member, getValue, emptyValues)
			if err != nil {
				return false, err
			}
			if !result {
				return false, nil
			}
		}
		return true, nil
	default:
		return false, specErrorf("Unsupported condition: %v", condition)
	}
}

// DescribeCondition 把条件渲染成报错文案（R6 的明细行用它）。
//
// describeColumn 把 db_field 变成读者友好的标签；getValue 不为 nil 时会在文案里
// 带上该行的实际值，例如 `数量（quantity_per_min_sales_unit） = 2 > 1`。
func DescribeCondition(
	condition any,
	describeColumn func(string) string,
	getValue func(string) any,
) string {
	switch typed := condition.(type) {
	case map[string]any:
		for _, groupKey := range GroupKeys {
			members, hasGroup := typed[groupKey]
			if !hasGroup {
				continue
			}
			joiner := " 且 "
			if groupKey == "any" {
				joiner = " 或 "
			}
			memberList, _ := members.([]any)
			parts := make([]string, 0, len(memberList))
			for _, member := range memberList {
				parts = append(parts, DescribeCondition(member, describeColumn, getValue))
			}
			return strings.Join(parts, joiner)
		}
		column := columnLabel(typed["column"], describeColumn)
		operatorName, _ := typed["operator"].(string)
		symbol, known := OperatorSymbols[operatorName]
		if !known {
			symbol = fmt.Sprintf(" %s ", operatorName)
		}
		if containsString(UnaryOperators, operatorName) {
			return column + symbol
		}
		expected := typed["value"]
		if getValue != nil {
			actual := getValue(columnName(typed["column"]))
			return fmt.Sprintf("%s = %s%s%v", column, NormalizeText(actual), symbol, expected)
		}
		return fmt.Sprintf("%s%s%v", column, symbol, expected)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, member := range typed {
			parts = append(parts, DescribeCondition(member, describeColumn, getValue))
		}
		return strings.Join(parts, " 且 ")
	default:
		return fmt.Sprint(condition)
	}
}

// ValidateConditionSpec 在配置加载阶段校验一条 required_when 规则（R24）。
//
// availableColumns 是该来源允许引用的 db_field；label 把错误定位到配置文件里；
// ownerColumn 非空时禁止条件引用列自己。
func ValidateConditionSpec(
	condition any,
	availableColumns []string,
	label string,
	ownerColumn string,
) error {
	available := map[string]bool{}
	for _, name := range availableColumns {
		available[name] = true
	}
	switch typed := condition.(type) {
	case map[string]any:
		for _, groupKey := range GroupKeys {
			members, hasGroup := typed[groupKey]
			if !hasGroup {
				continue
			}
			unknown := []string{}
			for key := range typed {
				if !containsString(GroupKeys, key) {
					unknown = append(unknown, key)
				}
			}
			if len(unknown) > 0 {
				sort.Strings(unknown)
				return specErrorf("%s: a condition group cannot mix %s with %s",
					label, groupKey, strings.Join(unknown, ", "))
			}
			memberList, ok := members.([]any)
			if !ok || len(memberList) == 0 {
				return specErrorf("%s.%s must be a non-empty list of conditions",
					label, groupKey)
			}
			for index, member := range memberList {
				if err := ValidateConditionSpec(
					member,
					availableColumns,
					fmt.Sprintf("%s.%s[%d]", label, groupKey, index),
					ownerColumn,
				); err != nil {
					return err
				}
			}
			return nil
		}
		return validateLeafCondition(typed, available, label, ownerColumn)
	case []any:
		if len(typed) == 0 {
			return specErrorf("%s must not be an empty list", label)
		}
		for index, member := range typed {
			if err := ValidateConditionSpec(
				member,
				availableColumns,
				fmt.Sprintf("%s[%d]", label, index),
				ownerColumn,
			); err != nil {
				return err
			}
		}
		return nil
	default:
		return specErrorf("%s must be a condition mapping or a list of conditions", label)
	}
}

func validateLeafCondition(
	condition map[string]any,
	available map[string]bool,
	label string,
	ownerColumn string,
) error {
	unknown := []string{}
	for key := range condition {
		if !AllowedConditionKeys[key] {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return specErrorf("%s has unknown keys: %s", label, strings.Join(unknown, ", "))
	}
	column, _ := condition["column"].(string)
	if column == "" {
		return specErrorf("%s.column must be a non-empty db_field name", label)
	}
	if !available[column] {
		return specErrorf("%s: unknown condition column: %s", label, column)
	}
	if ownerColumn != "" && column == ownerColumn {
		return specErrorf("%s must reference another column, not %s itself", label, column)
	}
	operatorName, _ := condition["operator"].(string)
	if !containsString(ConditionOperators, operatorName) {
		return specErrorf("%s: unknown condition operator %q. Allowed operators: %s",
			label, operatorName, strings.Join(ConditionOperators, ", "))
	}
	valueType := DefaultValueType
	if configured, hasValueType := condition["value_type"]; hasValueType {
		valueType, _ = configured.(string)
	}
	if !containsString(ConditionValueTypes, valueType) {
		return specErrorf("%s.value_type must be one of %s",
			label, strings.Join(ConditionValueTypes, ", "))
	}
	// 文本比较会把 "10" 读成小于 "2"，所以大小比较只允许数值
	if containsString(OrderingOperators, operatorName) && valueType != "number" {
		return specErrorf("%s: operator %s requires value_type: number", label, operatorName)
	}
	if containsString(UnaryOperators, operatorName) {
		if _, hasValue := condition["value"]; hasValue {
			return specErrorf("%s: operator %s does not take a value", label, operatorName)
		}
		return nil
	}
	configuredValue, hasValue := condition["value"]
	if !hasValue {
		return specErrorf("%s: operator %s requires a value", label, operatorName)
	}
	var candidates []any
	if containsString(MultiValueOperators, operatorName) {
		list, ok := configuredValue.([]any)
		if !ok || len(list) == 0 {
			return specErrorf("%s.value must be a non-empty list for %s", label, operatorName)
		}
		candidates = list
	} else {
		candidates = []any{configuredValue}
	}
	if valueType == "number" {
		for _, candidate := range candidates {
			if _, ok := ToNumber(candidate); !ok {
				return specErrorf(
					"%s.value must be numeric for value_type: number: %v", label, candidate)
			}
		}
	}
	return nil
}

func matchesLeaf(
	condition map[string]any,
	getValue func(string) any,
	emptyValues []any,
) (bool, error) {
	operatorName, _ := condition["operator"].(string)
	if !containsString(ConditionOperators, operatorName) {
		return false, specErrorf("Unknown condition operator: %q", operatorName)
	}
	actualValue := getValue(columnName(condition["column"]))
	switch operatorName {
	case "is_empty":
		return IsEmptyValue(actualValue, emptyValues), nil
	case "is_not_empty":
		return !IsEmptyValue(actualValue, emptyValues), nil
	}
	expectedValue := condition["value"]
	valueType := DefaultValueType
	if configured, hasValueType := condition["value_type"]; hasValueType {
		valueType, _ = configured.(string)
	}
	if valueType == "number" {
		actualNumber, ok := ToNumber(actualValue)
		if !ok {
			// 没有数值的行不能决定条件：规则保持沉默，而不是报出一条违规
			return false, nil
		}
		if containsString(MultiValueOperators, operatorName) {
			candidates := []float64{}
			for _, value := range asList(expectedValue) {
				if number, ok := ToNumber(value); ok {
					candidates = append(candidates, number)
				}
			}
			contained := containsFloat(candidates, actualNumber)
			if operatorName == "in" {
				return contained, nil
			}
			return !contained, nil
		}
		expectedNumber, ok := ToNumber(expectedValue)
		if !ok {
			return false, specErrorf("Condition value is not numeric: %v", expectedValue)
		}
		return compareNumbers(operatorName, actualNumber, expectedNumber)
	}
	actualText := NormalizeText(actualValue)
	if containsString(MultiValueOperators, operatorName) {
		candidates := []string{}
		for _, value := range asList(expectedValue) {
			candidates = append(candidates, NormalizeText(value))
		}
		contained := containsString(candidates, actualText)
		if operatorName == "in" {
			return contained, nil
		}
		return !contained, nil
	}
	expectedText := NormalizeText(expectedValue)
	switch operatorName {
	case "equals":
		return actualText == expectedText, nil
	case "not_equals":
		return actualText != expectedText, nil
	default:
		return false, specErrorf("Operator %s requires value_type: number", operatorName)
	}
}

func compareNumbers(operatorName string, actual, expected float64) (bool, error) {
	switch operatorName {
	case "equals":
		return actual == expected, nil
	case "not_equals":
		return actual != expected, nil
	case "greater_than":
		return actual > expected, nil
	case "greater_or_equal":
		return actual >= expected, nil
	case "less_than":
		return actual < expected, nil
	case "less_or_equal":
		return actual <= expected, nil
	default:
		return false, specErrorf("Operator %s requires value_type: number", operatorName)
	}
}

// PyStr 近似 Python 的 str()：条件值可能来自 YAML 的数字/布尔，报错文案与比较
// 都要和 Python 版一致（例如 True 而不是 true，1.0 而不是 1）。
func PyStr(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool:
		if typed {
			return "True"
		}
		return "False"
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		if !math.IsInf(typed, 0) && !math.IsNaN(typed) && typed == math.Trunc(typed) {
			return strconv.FormatFloat(typed, 'f', 1, 64)
		}
		return strconv.FormatFloat(typed, 'g', -1, 64)
	case float32:
		return PyStr(float64(typed))
	default:
		return fmt.Sprint(value)
	}
}

func asFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	default:
		return 0, false
	}
}

func asList(value any) []any {
	if list, ok := value.([]any); ok {
		return list
	}
	return []any{value}
}

func columnName(value any) string {
	if name, ok := value.(string); ok {
		return name
	}
	return ""
}

func columnLabel(value any, describeColumn func(string) string) string {
	if describeColumn == nil {
		return fmt.Sprint(value)
	}
	name, ok := value.(string)
	if !ok {
		return fmt.Sprint(value)
	}
	return describeColumn(name)
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func containsFloat(values []float64, wanted float64) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func allTrue(values []bool) bool {
	for _, value := range values {
		if !value {
			return false
		}
	}
	return true
}

func anyTrue(values []bool) bool {
	for _, value := range values {
		if value {
			return true
		}
	}
	return false
}
