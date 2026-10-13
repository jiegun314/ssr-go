// Package importer 是导入编排：读 Excel → value_mapping → 表头校验 → 必填校验 →
// 条件必填校验 → 写暂存表。
//
// 顺序不可交换（R3），任一校验失败整份文件都不写入（R7）。移植自
// services/import_service.py，错误文案由 tests/test_import_flow.py 逐字钉住。
package importer

import (
	"fmt"
	"strings"
	"time"

	"github.com/jiegun314/ssr-go/internal/config"
	"github.com/jiegun314/ssr-go/internal/excelio"
	"github.com/jiegun314/ssr-go/internal/rules"
	"github.com/jiegun314/ssr-go/internal/store"
)

// Column 是导入配置里的一列。
type Column struct {
	ChineseName     string
	DBField         string
	MandatoryStatus string
	ValueMapping    map[string]string
	RequiredWhen    any
	// AcceptedValues 是这一列允许的**原始写法**（Excel 里怎么填）。非空时，
	// 单元格的值必须命中其中之一，否则整份文件被拒绝。
	//
	// 校验原始写法而不是 value_mapping 归一化之后的值：两者职责不同 ——
	// accepted_values 管"输入只允许哪些写法"，value_mapping 管"入库/导出用什么写法"。
	// 空单元格不在这里判（由必填 / 条件必填负责）。
	AcceptedValues []string
}

// Rule 是一份来源的导入规则（get_file_import_rule 的 Go 版）。
type Rule struct {
	SourceName        string
	ChineseName       string
	TargetTable       string
	ReplaceOnImport   bool
	PreserveOnCleanup bool
	HeaderRow         int
	DataStartRow      int
	Columns           []Column
	// ColumnMapping 是 chinese_name → db_field；同名列只认第一次出现的那一列
	// （pandas 会给重复表头加 .1 后缀，于是第二列对不上任何配置列，等同于被忽略）。
	ColumnMapping map[string]string
	// RequiredFields 是所有 mandatory_status 非空的 db_field，顺序即配置顺序。
	RequiredFields []string
	EmptyValues    []any
}

// Result 是一次成功导入的结果。
type Result struct {
	RowCount   int
	FileName   string
	ImportTime string
}

// Importer 持有配置与数据库连接。
type Importer struct {
	Loader     *config.Loader
	Repository *store.Repository
	Rules      map[string]*Rule
	// Order 是 sources 的声明顺序，用于「Must be one of ...」提示与遍历。
	Order []string
	// Now 产出 import_time（`YYYY/MM/DD HH:MM`）；测试可以换成固定时钟。
	Now func() string
}

// NewImporter 读一次导入配置，之后每次导入复用。
func NewImporter(loader *config.Loader, repository *store.Repository) (*Importer, error) {
	ruleMap, order, err := LoadRules(loader)
	if err != nil {
		return nil, err
	}
	return &Importer{
		Loader:     loader,
		Repository: repository,
		Rules:      ruleMap,
		Order:      order,
		Now:        func() string { return time.Now().Format("2006/01/02 15:04") },
	}, nil
}

// LoadRules 按声明顺序读出全部来源的导入规则。
func LoadRules(loader *config.Loader) (map[string]*Rule, []string, error) {
	document, err := loader.LoadImportMappingDocument()
	if err != nil {
		return nil, nil, err
	}
	globalSettings, _ := document.Value["global_settings"].(map[string]any)
	emptyValues, _ := globalSettings["empty_values"].([]any)
	sources, _ := document.Value["sources"].(map[string]any)
	order := document.Keys("sources")
	ruleMap := map[string]*Rule{}
	for _, sourceName := range order {
		source, _ := sources[sourceName].(map[string]any)
		if source == nil {
			continue
		}
		excel, _ := source["excel"].(map[string]any)
		rule := &Rule{
			SourceName:        sourceName,
			ChineseName:       textOf(source["chinese_name"]),
			TargetTable:       textOf(source["target_table"]),
			ReplaceOnImport:   boolOf(source["replace_on_import"], true),
			PreserveOnCleanup: boolOf(source["preserve_on_cleanup"], false),
			HeaderRow:         intOf(excel["chinese_header_row"], 1),
			DataStartRow:      intOf(excel["data_start_row"], 2),
			ColumnMapping:     map[string]string{},
			EmptyValues:       emptyValues,
		}
		columnDefinitions, _ := source["columns"].([]any)
		for _, rawColumn := range columnDefinitions {
			column, _ := rawColumn.(map[string]any)
			if column == nil {
				continue
			}
			definition := Column{
				ChineseName:     textOf(column["chinese_name"]),
				DBField:         textOf(column["db_field"]),
				MandatoryStatus: textOf(column["mandatory_status"]),
				ValueMapping:    stringMapOf(column["value_mapping"]),
				RequiredWhen:    column["required_when"],
				AcceptedValues:  stringListOf(column["accepted_values"]),
			}
			rule.Columns = append(rule.Columns, definition)
			if _, taken := rule.ColumnMapping[definition.ChineseName]; !taken {
				rule.ColumnMapping[definition.ChineseName] = definition.DBField
			}
			if definition.MandatoryStatus != "" {
				rule.RequiredFields = append(rule.RequiredFields, definition.DBField)
			}
		}
		// required_when 里的期望值按被引用列自己的 value_mapping 归一化：
		// 配置里可以按「人怎么填」来写（是 / 否 / Y …），程序换算成归一化后的值再比。
		mappingByField := make(map[string]map[string]string, len(rule.Columns))
		for _, column := range rule.Columns {
			mappingByField[column.DBField] = column.ValueMapping
		}
		for index := range rule.Columns {
			if rule.Columns[index].RequiredWhen == nil {
				continue
			}
			rule.Columns[index].RequiredWhen = rules.NormalizeConditionValues(
				rule.Columns[index].RequiredWhen,
				func(field string) map[string]string { return mappingByField[field] },
			)
		}
		ruleMap[sourceName] = rule
	}
	return ruleMap, order, nil
}

// Import 导入一份来源文件。返回的错误要么是上面三类「业务拒绝」，要么是包装过的失败。
func (importer *Importer) Import(fileType string, filePath string) (Result, error) {
	rule, known := importer.Rules[fileType]
	if !known {
		return Result{}, fmt.Errorf("Invalid file type: %s. Must be one of %v",
			fileType, importer.Order)
	}
	result, err := importer.importWithRule(rule, filePath)
	if err != nil {
		switch err.(type) {
		case *MissingRequiredColumnsError, *MissingRowValuesError:
			// 这两类携带用户要看的明细，原样抛出
			return Result{}, err
		default:
			return Result{}, err
		}
	}
	return result, nil
}

func (importer *Importer) importWithRule(rule *Rule, filePath string) (Result, error) {
	sheet, err := excelio.ReadFirstSheet(filePath)
	if err != nil {
		return Result{}, fmt.Errorf("Failed to import Excel file: %w", err)
	}
	// 表头：chinese_name → 列号（只认第一个匹配，重复表头等同于被忽略）
	columnIndexes := map[string]int{}
	header := sheet.HeaderRow(rule.HeaderRow)
	for position, title := range header {
		dbField, mappedColumn := rule.ColumnMapping[title]
		if !mappedColumn {
			continue
		}
		if _, taken := columnIndexes[dbField]; !taken {
			columnIndexes[dbField] = position + 1
		}
	}
	// 数据行 + value_mapping 归一（R3/R4）
	dataRows := sheet.DataRows(rule.DataStartRow)
	records := make([]store.Row, 0, len(dataRows))
	// rawCells 保留单元格的**原始写法**：accepted_values 校验的是输入写法，
	// 而归一化之后的值已经看不出用户到底填了什么。
	rawCells := make([]map[string]string, 0, len(dataRows))
	for _, values := range dataRows {
		record := store.Row{}
		raw := make(map[string]string, len(rule.Columns))
		for _, column := range rule.Columns {
			index, present := columnIndexes[column.DBField]
			cell := ""
			if present && index-1 < len(values) {
				cell = values[index-1]
			}
			raw[column.DBField] = cell
			record[column.DBField] = mappedValue(cell, column.ValueMapping)
		}
		records = append(records, record)
		rawCells = append(rawCells, raw)
	}
	// 校验（表头 → 必填 → 条件必填 → 取值白名单）
	if err := importer.validate(rule, columnIndexes, records, rawCells); err != nil {
		return Result{}, err
	}
	// 写暂存表
	table := store.SourceTable{
		SourceName:      rule.SourceName,
		TableName:       rule.TargetTable,
		Columns:         dbFields(rule),
		ReplaceOnImport: rule.ReplaceOnImport,
	}
	if err := store.ImportSourceRows(importer.Repository, table, records); err != nil {
		return Result{}, fmt.Errorf("Failed to import Excel data to database: %w", err)
	}
	return Result{
		RowCount:   len(records),
		FileName:   rule.ChineseName,
		ImportTime: importer.Now(),
	}, nil
}

// validate 复刻 validate_excel_data：先表头，再单元格。
//
// rawCells 与 records 一一对应：前者是 Excel 里的原始写法，后者是归一化之后的值。
func (importer *Importer) validate(
	rule *Rule,
	columnIndexes map[string]int,
	records []store.Row,
	rawCells []map[string]string,
) error {
	missingRequired := []Column{}
	for _, column := range rule.Columns {
		if column.MandatoryStatus != "required" {
			continue
		}
		if _, present := columnIndexes[column.DBField]; !present {
			missingRequired = append(missingRequired, column)
		}
	}
	if len(missingRequired) > 0 {
		return NewMissingRequiredColumnsError(rule.ChineseName, missingRequired)
	}
	// 条件必填列必须出现在表头里（值可以为空），否则无法判断条件是否成立
	missingConditional := []Column{}
	for _, column := range rule.Columns {
		if column.RequiredWhen == nil {
			continue
		}
		if _, present := columnIndexes[column.DBField]; !present {
			missingConditional = append(missingConditional, column)
		}
	}
	if len(missingConditional) > 0 {
		return NewMissingConditionalColumnsError(rule.ChineseName, missingConditional)
	}
	if violations := requiredViolations(rule, records); len(violations) > 0 {
		return NewMissingRequiredValuesError(rule.ChineseName, violations)
	}
	if violations := conditionalViolations(rule, records); len(violations) > 0 {
		return NewMissingConditionalValuesError(rule.ChineseName, violations)
	}
	if violations := acceptedValueViolations(rule, rawCells); len(violations) > 0 {
		return NewInvalidValuesError(rule.ChineseName, violations)
	}
	return nil
}

// acceptedValueViolations 找「列声明了 accepted_values、单元格却填了别的写法」的格子。
//
// 与必填校验的分工：空单元格一律归必填 / 条件必填管，这里只看非空的写法对不对，
// 所以它排在最后一道 —— 用户先补空值，补完再看写法是否合法。
// 比较时忽略首尾空白与英文大小写（"YES" 与 "Yes" 等价，"是" 与 " 是 " 等价）。
func acceptedValueViolations(rule *Rule, rawCells []map[string]string) []ValueViolation {
	restricted := []Column{}
	for _, column := range rule.Columns {
		if len(column.AcceptedValues) > 0 {
			restricted = append(restricted, column)
		}
	}
	if len(restricted) == 0 {
		return nil
	}
	violations := []ValueViolation{}
	for position, raw := range rawCells {
		rowNumber := rule.DataStartRow + position
		for _, column := range restricted {
			cell := strings.TrimSpace(raw[column.DBField])
			if cell == "" {
				continue
			}
			if matchesAcceptedValue(cell, column.AcceptedValues) {
				continue
			}
			violations = append(violations, ValueViolation{
				RowNumber:      rowNumber,
				ChineseName:    column.ChineseName,
				DBField:        column.DBField,
				Value:          cell,
				AcceptedValues: column.AcceptedValues,
			})
		}
	}
	return violations
}

// matchesAcceptedValue 判断一个原始写法是否在白名单里（忽略首尾空白与大小写）。
func matchesAcceptedValue(cell string, accepted []string) bool {
	for _, candidate := range accepted {
		if strings.EqualFold(strings.TrimSpace(candidate), cell) {
			return true
		}
	}
	return false
}

// requiredViolations 找 mandatory_status: required 列上的空单元格（R2/R5/R16 的空值口径）。
func requiredViolations(rule *Rule, records []store.Row) []Violation {
	requiredColumns := []Column{}
	for _, column := range rule.Columns {
		if column.MandatoryStatus == "required" {
			requiredColumns = append(requiredColumns, column)
		}
	}
	if len(requiredColumns) == 0 {
		return nil
	}
	violations := []Violation{}
	for position, record := range records {
		if allBlank(record) {
			// 整行都没有内容的行不是数据行：不触发必填校验（R2）
			continue
		}
		rowNumber := rule.DataStartRow + position
		for _, column := range requiredColumns {
			if !rules.IsBlankValue(record[column.DBField]) {
				continue
			}
			violations = append(violations, Violation{
				RowNumber:   rowNumber,
				ChineseName: column.ChineseName,
				DBField:     column.DBField,
			})
		}
	}
	return violations
}

// conditionalViolations 找「条件成立但单元格为空」的行（R6）。
func conditionalViolations(rule *Rule, records []store.Row) []Violation {
	conditionalColumns := []Column{}
	for _, column := range rule.Columns {
		if column.RequiredWhen != nil {
			conditionalColumns = append(conditionalColumns, column)
		}
	}
	if len(conditionalColumns) == 0 {
		return nil
	}
	describeColumn := columnLabelBuilder(rule)
	violations := []Violation{}
	for position, record := range records {
		rowNumber := rule.DataStartRow + position
		getValue := func(dbField string) any { return record[dbField] }
		for _, column := range conditionalColumns {
			matches, err := rules.MatchesCondition(column.RequiredWhen, getValue, rule.EmptyValues)
			if err != nil || !matches {
				continue
			}
			if !rules.IsBlankValue(record[column.DBField]) {
				continue
			}
			violations = append(violations, Violation{
				RowNumber:   rowNumber,
				ChineseName: column.ChineseName,
				DBField:     column.DBField,
				Condition: rules.DescribeCondition(
					column.RequiredWhen, describeColumn, getValue),
			})
		}
	}
	return violations
}

// ReviewRows 读回暂存表并把列名换成中文（数据回顾窗口用）。
func (importer *Importer) ReviewRows(fileType string) ([]store.Row, error) {
	rule, known := importer.Rules[fileType]
	if !known {
		return nil, fmt.Errorf("Invalid file type: %s. Must be one of %v",
			fileType, importer.Order)
	}
	rows, err := importer.Repository.Rows(rule.TargetTable)
	if err != nil {
		return nil, err
	}
	labels := map[string]string{}
	for dbField, chineseName := range rule.ColumnMapping {
		labels[chineseName] = dbField
	}
	columns := make([]string, 0, len(rule.RequiredFields))
	for _, dbField := range rule.RequiredFields {
		for chineseName, mapped := range rule.ColumnMapping {
			if mapped == dbField {
				columns = append(columns, chineseName)
				break
			}
		}
	}
	reviewed := make([]store.Row, 0, len(rows))
	for _, row := range rows {
		review := store.Row{}
		for _, chineseName := range columns {
			review[chineseName] = row[labels[chineseName]]
		}
		reviewed = append(reviewed, review)
	}
	return reviewed, nil
}

// CountImportedRows 读当前来源表已有的条目数（启动时显示「现有N条」用）。
func (importer *Importer) CountImportedRows(fileType string) (int, error) {
	rule, known := importer.Rules[fileType]
	if !known {
		return 0, fmt.Errorf("Invalid file type: %s. Must be one of %v",
			fileType, importer.Order)
	}
	return importer.Repository.CountTableRows(rule.TargetTable)
}

// mappedValue 复刻 _mapped_value（R4）：只归一写法，绝不把填了内容的单元格变空。
func mappedValue(value string, valueMapping map[string]string) string {
	if len(valueMapping) == 0 {
		return value
	}
	if rules.IsBlankValue(value) {
		return value
	}
	mapped, present := valueMapping[value]
	if !present {
		return value
	}
	if rules.IsBlankValue(mapped) {
		return value
	}
	return mapped
}

func allBlank(record store.Row) bool {
	for _, value := range record {
		if !rules.IsBlankValue(value) {
			return false
		}
	}
	return true
}

func dbFields(rule *Rule) []string {
	fields := make([]string, 0, len(rule.Columns))
	for _, column := range rule.Columns {
		fields = append(fields, column.DBField)
	}
	return fields
}

func columnLabelBuilder(rule *Rule) func(string) string {
	labels := map[string]string{}
	for _, column := range rule.Columns {
		labels[column.DBField] = column.ChineseName + "（" + column.DBField + "）"
	}
	return func(dbField string) string {
		if label, known := labels[dbField]; known {
			return label
		}
		return dbField
	}
}

func textOf(value any) string {
	text, _ := value.(string)
	return text
}

func boolOf(value any, fallback bool) bool {
	flag, ok := value.(bool)
	if !ok {
		return fallback
	}
	return flag
}

func intOf(value any, fallback int) int {
	number, ok := value.(int)
	if !ok {
		return fallback
	}
	return number
}

// stringListOf 取配置里的字符串列表（非字符串项忽略），空值返回 nil。
func stringListOf(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			values = append(values, text)
		}
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

func stringMapOf(value any) map[string]string {
	mapping := map[string]string{}
	entries, ok := value.(map[string]any)
	if !ok {
		return mapping
	}
	for key, raw := range entries {
		mapping[key] = textOf(raw)
	}
	return mapping
}
