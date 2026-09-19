// Package consolidation 是整合编排（R10–R19）：
// 读四张来源表 → 按 merge_rules 匹配 / 拆行 / 合并 → 输出列映射 → 导出前最后一道校验
// → 变更检测 → 写 consolidation_staging。
//
// 移植自 services/consolidation_service.py。这里刻意保留 Python 版的宽松类型处理：
// 配置里的值可能是字符串、列表或映射，逐键判断而不是强类型反序列化，行为与文案才一致。
package consolidation

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jiegun314/ssr-go/internal/changedetect"
	"github.com/jiegun314/ssr-go/internal/config"
	"github.com/jiegun314/ssr-go/internal/rules"
	"github.com/jiegun314/ssr-go/internal/store"
	"github.com/jiegun314/ssr-go/internal/validation"
)

// 整合结果的四种状态；只有 Ready 会被导出。
const (
	StatusReady      = "Ready"
	StatusIncomplete = "Incomplete"
	StatusDuplicate  = "Duplicate"
	StatusConflict   = "Conflict"
)

// Config 是整合需要的全部配置（一次读入，多次使用）。
type Config struct {
	FieldNames []string
	Fields     map[string]map[string]any

	SourceRules          map[string]map[string]any
	SourceOrder          []string
	BaseSource           string
	BaseKeyField         string
	ResultIdentityFields []string
	IdentitySeparator    string

	SourceTables  map[string]string
	SourceColumns map[string]map[string]map[string]any
	EmptyValues   []any

	UserDefinedData map[string]any
	TargetTable     string

	IdentityFields []string
	ChangeRule     *changedetect.Rule
}

// LoadConfig 读 consolidation_mapping.yaml、excel_import_mapping.yaml 与 setting.yaml。
func LoadConfig(loader *config.Loader) (*Config, error) {
	consolidation, err := loader.LoadConsolidationMappingDocument()
	if err != nil {
		return nil, err
	}
	dataset, err := mappingAt(consolidation.Value, "target_dataset")
	if err != nil {
		return nil, err
	}
	mergeRules, err := mappingAt(dataset, "merge_rules")
	if err != nil {
		return nil, err
	}
	baseSource, err := mappingAt(mergeRules, "base_source")
	if err != nil {
		return nil, err
	}
	resultIdentity, err := mappingAt(mergeRules, "result_identity")
	if err != nil {
		return nil, err
	}
	sourceRules, err := mappingAt(mergeRules, "sources")
	if err != nil {
		return nil, err
	}
	fields, err := mappingAt(dataset, "fields")
	if err != nil {
		return nil, err
	}
	imports, err := loader.LoadImportMappingDocument()
	if err != nil {
		return nil, err
	}
	setting, err := loader.LoadSetting()
	if err != nil {
		return nil, err
	}
	identityFields, err := loader.LoadDuplicateCheckIdentityFields()
	if err != nil {
		return nil, err
	}
	changeRule, err := LoadChangeRule(loader)
	if err != nil {
		return nil, err
	}

	sources, _ := imports.Value["sources"].(map[string]any)
	globalSettings, _ := imports.Value["global_settings"].(map[string]any)
	emptyValues, _ := globalSettings["empty_values"].([]any)
	userDefinedData, _ := setting["user_defined_data"].(map[string]any)
	tables, _ := setting["tables"].(map[string]any)
	consolidationTable, _ := tables["consolidation_result"].(map[string]any)
	targetTable, _ := consolidationTable["name"].(string)

	configValue := &Config{
		FieldNames:           consolidation.Keys("target_dataset", "fields"),
		Fields:               map[string]map[string]any{},
		SourceRules:          map[string]map[string]any{},
		SourceOrder:          imports.Keys("sources"),
		BaseSource:           textOf(baseSource["source"]),
		BaseKeyField:         firstOf(baseSource["key"]),
		ResultIdentityFields: stringsOf(resultIdentity["fields"]),
		IdentitySeparator:    textOf(resultIdentity["separator"]),
		SourceTables:         map[string]string{},
		SourceColumns:        map[string]map[string]map[string]any{},
		EmptyValues:          emptyValues,
		UserDefinedData:      userDefinedData,
		TargetTable:          targetTable,
		IdentityFields:       identityFields,
		ChangeRule:           changeRule,
	}
	for _, name := range configValue.FieldNames {
		definition, _ := fields[name].(map[string]any)
		configValue.Fields[name] = definition
	}
	for name, rule := range sourceRules {
		sourceRule, _ := rule.(map[string]any)
		configValue.SourceRules[name] = sourceRule
	}
	for _, sourceName := range configValue.SourceOrder {
		source, _ := sources[sourceName].(map[string]any)
		configValue.SourceTables[sourceName] = textOf(source["target_table"])
		columns := map[string]map[string]any{}
		definitions, _ := source["columns"].([]any)
		for _, rawColumn := range definitions {
			column, _ := rawColumn.(map[string]any)
			columns[textOf(column["db_field"])] = column
		}
		configValue.SourceColumns[sourceName] = columns
	}
	return configValue, nil
}

// LoadChangeRule 把配置里的变更描述规则转成 changedetect 用的形式。
func LoadChangeRule(loader *config.Loader) (*changedetect.Rule, error) {
	rule, err := loader.LoadChangeDescriptionRule()
	if err != nil || rule == nil {
		return nil, err
	}
	return &changedetect.Rule{
		Field:          rule.Field,
		Fields:         rule.Fields,
		Labels:         rule.Labels,
		IdentityFields: rule.IdentityFields,
		CompareFields:  rule.CompareFields,
		IgnoreFields:   rule.IgnoreFields,
		OnNewRecord:    rule.OnNewRecord,
		OnNoChange:     rule.OnNoChange,
		OnOtherChange:  rule.OnOtherChange,
	}, nil
}

// Outcome 是一次整合的结果。
type Outcome struct {
	Rows          []changedetect.Row
	MissingData   []string
	DuplicateData []string
	ChangedData   []string
	ConflictData  []string
}

// Service 执行一次整合。
type Service struct {
	Config     *Config
	Loader     *config.Loader
	Repository *store.Repository
	// Log 用于变更比较；为 nil 时按「没有任何历史记录」处理。
	Log *store.OperationLog
}

// Consolidate 跑完整流程并写 consolidation_staging（每次整体重写）。
func (service *Service) Consolidate() (Outcome, error) {
	completeness := &validation.SourceCompleteness{
		Loader:     service.Loader,
		Repository: service.Repository,
	}
	if err := completeness.CheckTableCompleteness(); err != nil {
		return Outcome{}, err
	}
	if err := completeness.CheckSourceDataAvailable(); err != nil {
		return Outcome{}, err
	}
	data, conflicts, err := service.readSourceData()
	if err != nil {
		return Outcome{}, err
	}
	outcome, err := service.consolidateRows(data, conflicts)
	if err != nil {
		return Outcome{}, err
	}
	latest := []store.Row{}
	if service.Log != nil {
		latest, err = service.Log.ReadLatestRecords()
		if err != nil {
			return Outcome{}, err
		}
	}
	detector := changedetect.NewService(
		service.Config.ChangeRule, service.Config.IdentityFields, latest)
	duplicates, changed, err := detector.Apply(outcome.Rows)
	if err != nil {
		return Outcome{}, err
	}
	outcome.DuplicateData = duplicates
	outcome.ChangedData = changed
	if err := service.write(outcome.Rows); err != nil {
		return Outcome{}, err
	}
	return outcome, nil
}

// write 把结果写成 consolidation_staging：列 = status + material_code + 全部导出列。
func (service *Service) write(rows []changedetect.Row) error {
	columns := []string{"status", service.Config.BaseKeyField}
	columns = append(columns, service.Config.FieldNames...)
	tableRows := make([]store.Row, 0, len(rows))
	for _, row := range rows {
		tableRows = append(tableRows, row.Values)
	}
	return service.Repository.WriteRows(
		service.Config.TargetTable, columns, tableRows, store.WriteReplace)
}

// --- 读来源数据（R12/R14） ---

type orderedGroup struct {
	Keys []string
	Rows map[string]store.Row
}

func newOrderedGroup() *orderedGroup {
	return &orderedGroup{Keys: []string{}, Rows: map[string]store.Row{}}
}

func (group *orderedGroup) get(key string) (store.Row, bool) {
	row, present := group.Rows[key]
	return row, present
}

func (group *orderedGroup) set(key string, row store.Row) {
	if _, seen := group.Rows[key]; !seen {
		group.Keys = append(group.Keys, key)
	}
	group.Rows[key] = row
}

type sourceData struct {
	keyed   map[string]*store.KeyedTable
	grouped map[string]map[string]*orderedGroup
}

func (service *Service) readSourceData() (*sourceData, map[string]map[string]string, error) {
	data := &sourceData{
		keyed:   map[string]*store.KeyedTable{},
		grouped: map[string]map[string]*orderedGroup{},
	}
	conflicts := map[string]map[string]string{}
	for _, sourceName := range service.Config.SourceOrder {
		rule := service.Config.SourceRules[sourceName]
		tableName := service.Config.SourceTables[sourceName]
		switch textOf(rule["cardinality"]) {
		case "one_to_many":
			rows, err := service.Repository.Rows(tableName)
			if err != nil {
				return nil, nil, err
			}
			grouped, conflictDetails := service.groupOneToMany(sourceName, rows, rule)
			data.grouped[sourceName] = grouped
			conflicts[sourceName] = conflictDetails
		case "many_to_one":
			rows, err := service.Repository.Rows(tableName)
			if err != nil {
				return nil, nil, err
			}
			keyed, conflictDetails := service.groupManyToOne(sourceName, rows, rule)
			data.keyed[sourceName] = keyed
			conflicts[sourceName] = conflictDetails
		default:
			match, _ := rule["match"].(map[string]any)
			indexField := ""
			if keys := sortedKeysOf(match); len(keys) > 0 {
				indexField = textOf(match[keys[0]])
			}
			keyed, err := service.Repository.KeyedTable(tableName, indexField)
			if err != nil {
				return nil, nil, err
			}
			data.keyed[sourceName] = keyed
		}
	}
	return data, conflicts, nil
}

// groupOneToMany 按匹配值 + 拆行键收口：同键内容不同的行记为冲突（R14）。
func (service *Service) groupOneToMany(
	sourceName string,
	rows []store.Row,
	rule map[string]any,
) (map[string]*orderedGroup, map[string]string) {
	grouped := map[string]*orderedGroup{}
	conflicts := map[string]string{}
	matchField := firstOf(mapValues(rule["match"]))
	expansionFields := stringsOf(rule["expand_by"])
	duplicateRule, _ := rule["duplicates"].(map[string]any)
	for _, row := range rows {
		matchValue := normalizeKey(row[matchField])
		expansionValues := make([]string, 0, len(expansionFields))
		for _, field := range expansionFields {
			expansionValues = append(expansionValues, normalizeKey(row[field]))
		}
		if matchValue == "" || containsEmpty(expansionValues) {
			continue
		}
		expansionKey := strings.Join(expansionValues, "\x00")
		group, present := grouped[matchValue]
		if !present {
			group = newOrderedGroup()
			grouped[matchValue] = group
		}
		existing, seen := group.get(expansionKey)
		if !seen {
			group.set(expansionKey, row)
			continue
		}
		if !rowsEqual(existing, row) && textOf(duplicateRule["different_content"]) == "conflict" {
			identity := []string{}
			for _, field := range stringsOf(duplicateRule["compare_by"]) {
				identity = append(identity, field+"="+normalizeKey(row[field]))
			}
			conflicts[matchValue+"\x00"+expansionKey] = service.sourceLabel(sourceName) +
				"：" + strings.Join(identity, ", ") + " 存在内容不同的重复记录"
		}
	}
	return grouped, conflicts
}

// groupManyToOne 按匹配值收口：同一键出现多个不同值记为冲突（R14）。
func (service *Service) groupManyToOne(
	sourceName string,
	rows []store.Row,
	rule map[string]any,
) (*store.KeyedTable, map[string]string) {
	keyed := &store.KeyedTable{Keys: []string{}, Rows: map[string]store.Row{}}
	comparisonValues := map[string]map[string]bool{}
	matchField := firstOf(mapValues(rule["match"]))
	duplicateRule, _ := rule["duplicates"].(map[string]any)
	compareBy := stringsOf(duplicateRule["compare_by"])
	selectedRecord := "first"
	if configured := textOf(duplicateRule["selected_record"]); configured != "" {
		selectedRecord = configured
	}
	for _, row := range rows {
		matchValue := normalizeKey(row[matchField])
		if matchValue == "" {
			continue
		}
		parts := make([]string, 0, len(compareBy))
		anyValue := false
		for _, field := range compareBy {
			value := normalizeKey(row[field])
			parts = append(parts, value)
			if value != "" {
				anyValue = true
			}
		}
		if anyValue {
			if comparisonValues[matchValue] == nil {
				comparisonValues[matchValue] = map[string]bool{}
			}
			comparisonValues[matchValue][strings.Join(parts, "\x00")] = true
		}
		_, already := keyed.Rows[matchValue]
		if selectedRecord == "last" || !already {
			if !already {
				keyed.Keys = append(keyed.Keys, matchValue)
			}
			keyed.Rows[matchValue] = row
		}
	}
	conflicts := map[string]string{}
	for matchValue, values := range comparisonValues {
		if len(values) <= 1 || textOf(duplicateRule["different_value"]) != "conflict" {
			continue
		}
		joined := make([]string, 0, len(values))
		for value := range values {
			joined = append(joined, strings.ReplaceAll(value, "\x00", "+"))
		}
		sort.Strings(joined)
		conflicts[matchValue] = service.sourceLabel(sourceName) + "：" + matchField +
			"=" + matchValue + " 存在多个不同值：" + strings.Join(joined, ", ")
	}
	return keyed, conflicts
}

// --- 整合与映射（R11/R13/R15/R16） ---

// consolidateRows 以 RA 为基准逐条产出结果（R11：顺序由 RA 的记录顺序决定）。
func (service *Service) consolidateRows(
	data *sourceData,
	conflicts map[string]map[string]string,
) (Outcome, error) {
	outcome := Outcome{
		Rows:          []changedetect.Row{},
		MissingData:   []string{},
		DuplicateData: []string{},
		ChangedData:   []string{},
		ConflictData:  []string{},
	}
	base := data.keyed[service.Config.BaseSource]
	if base == nil {
		return outcome, nil
	}
	expansionSource, expansionRule := service.expansionRule()
	noMatch, _ := expansionRule["no_match"].(map[string]any)
	noMatchValue := textOf(noMatch["value"])
	expansionFields := stringsOf(expansionRule["expand_by"])
	incomplete := map[string]bool{}
	baseKeyField := service.Config.BaseKeyField

	for _, baseKey := range base.Keys {
		expanded := data.grouped[expansionSource][baseKey]
		expansionCount := 0
		if expanded != nil {
			expansionCount = len(expanded.Keys)
		}
		switch {
		case expansionCount == 0 && textOf(noMatch["action"]) == "skip":
			continue
		case expansionCount == 0 && textOf(noMatch["action"]) == "error":
			return outcome, fmt.Errorf("No expansion record found for %s", baseKey)
		}
		expansionKeys := []string{}
		if expansionCount == 0 {
			expansionKeys = append(expansionKeys, noMatchValue)
		} else {
			expansionKeys = append(expansionKeys, expanded.Keys...)
		}
		for _, expansionKey := range expansionKeys {
			var expansionRow store.Row
			if expansionCount > 0 {
				expansionRow = expanded.Rows[expansionKey]
			}
			context := store.Row{baseKeyField: baseKey}
			for column, value := range expansionRow {
				context[column] = value
			}
			identityParts := make([]string, 0, len(service.Config.ResultIdentityFields))
			for _, field := range service.Config.ResultIdentityFields {
				value := normalizeKey(context[field])
				if value == "" {
					value = noMatchValue
				}
				identityParts = append(identityParts, value)
			}
			resultKey := strings.Join(identityParts, service.Config.IdentitySeparator)

			conflictDetails := service.conflictDetails(
				conflicts, baseKey, expansionSource, expansionKey, expansionRow)
			row := store.Row{"status": StatusReady, baseKeyField: baseKey}
			if len(conflictDetails) > 0 {
				row["status"] = StatusConflict
			}
			for _, detail := range conflictDetails {
				if !contains(outcome.ConflictData, detail) {
					outcome.ConflictData = append(outcome.ConflictData, detail)
				}
			}
			selected := map[string]store.Row{}
			for _, sourceName := range service.Config.SourceOrder {
				if sourceName == expansionSource {
					selected[sourceName] = expansionRow
					continue
				}
				if keyed := data.keyed[sourceName]; keyed != nil {
					selected[sourceName] = keyed.Rows[baseKey]
				}
			}
			if err := service.mapResultRow(
				row, selected, &outcome, incomplete, resultKey,
				expansionSource, expansionFields, noMatchValue,
			); err != nil {
				return outcome, err
			}
			outcome.Rows = append(outcome.Rows, changedetect.Row{Key: resultKey, Values: row})
		}
	}
	return outcome, nil
}

// mapResultRow 按配置把来源字段写进输出列（R15），随后跑导出前的最后一道校验（R16）。
func (service *Service) mapResultRow(
	row store.Row,
	selected map[string]store.Row,
	outcome *Outcome,
	incomplete map[string]bool,
	resultKey string,
	expansionSource string,
	expansionFields []string,
	noMatchValue string,
) error {
	for _, columnName := range service.Config.FieldNames {
		definition := service.Config.Fields[columnName]
		sourceDefinition, _ := definition["source"].(map[string]any)
		transform, _ := definition["transform"].(map[string]any)
		transformType := textOf(transform["type"])
		if transformType == config.ChangeDescriptionTransform && len(sourceDefinition) == 0 {
			// 派生列：等所有列都映射完，再由变更比较填值
			row[columnName] = ""
			continue
		}
		mandatoryStatus := textOf(definition["mandatory_status"])
		switch {
		case mandatoryStatus == "user_defined":
			key := textOf(sourceDefinition["key"])
			value, present := service.Config.UserDefinedData[key]
			if !present {
				return fmt.Errorf(
					"setting.yaml 的 user_defined_data 中未找到键 %q（输出字段：%s）",
					key, columnName)
			}
			row[columnName] = textOf(value)
		case mandatoryStatus == "":
			row[columnName] = ""
		case len(sourceDefinition) == 0:
			// 固定文本列（例如 Choose Action = "Add or Modify"）
			row[columnName] = textOf(transform["value"])
		default:
			dataSource := textOf(sourceDefinition["file"])
			sourceRow := selected[dataSource]
			if sourceRow == nil {
				if service.noMatchAction(dataSource, expansionSource) == "empty" {
					row[columnName] = ""
					continue
				}
				markIncomplete(outcome, incomplete, resultKey, row)
				if dataSource == expansionSource &&
					contains(expansionFields, firstOf(sourceDefinition["field"])) {
					row[columnName] = noMatchValue
				} else {
					row[columnName] = "MISSING"
				}
				continue
			}
			if transformType == "concatenate" {
				parts := []string{}
				for _, field := range stringsOf(sourceDefinition["field"]) {
					text := normalizeKey(sourceRow[field])
					if !rules.IsEmptyValue(text, service.Config.EmptyValues) {
						parts = append(parts, text)
					}
				}
				row[columnName] = strings.Join(parts, textOf(transform["separator"]))
				continue
			}
			row[columnName] = sourceRow[firstOf(sourceDefinition["field"])]
		}
	}
	service.flagMissingRequiredValues(row, selected, outcome, incomplete, resultKey, noMatchValue)
	return nil
}

// flagMissingRequiredValues 是导出前的最后一道校验（R16）。
func (service *Service) flagMissingRequiredValues(
	row store.Row,
	selected map[string]store.Row,
	outcome *Outcome,
	incomplete map[string]bool,
	resultKey string,
	noMatchValue string,
) {
	for _, columnName := range service.Config.FieldNames {
		definition := service.Config.Fields[columnName]
		status := textOf(definition["mandatory_status"])
		if status != "required" && status != "required_if_applicable" {
			continue
		}
		if !service.isRequiredForRow(definition, selected) {
			continue
		}
		if !rules.IsBlankValue(row[columnName]) {
			continue
		}
		markIncomplete(outcome, incomplete, resultKey, row)
		row[columnName] = noMatchValue
	}
}

// isRequiredForRow 判断一个必填输出列在当前行是否必须填写。
//
// required 永远适用；required_if_applicable 用它读取的来源列上声明的**同一条**
// required_when 判断；来源列没有声明规则时保持「来源给什么就是什么」（R16）。
func (service *Service) isRequiredForRow(
	definition map[string]any,
	selected map[string]store.Row,
) bool {
	if textOf(definition["mandatory_status"]) == "required" {
		return true
	}
	source, _ := definition["source"].(map[string]any)
	sourceFields := stringsOf(source["field"])
	if len(source) == 0 || len(sourceFields) == 0 {
		return false
	}
	sourceName := textOf(source["file"])
	sourceRow := selected[sourceName]
	if sourceRow == nil {
		return false
	}
	var condition any
	for _, field := range sourceFields {
		column := service.Config.SourceColumns[sourceName][field]
		if column == nil {
			continue
		}
		if declared, present := column["required_when"]; present && declared != nil {
			condition = declared
			break
		}
	}
	if condition == nil {
		return false
	}
	matches, err := rules.MatchesCondition(
		condition, func(key string) any { return sourceRow[key] }, service.Config.EmptyValues)
	return err == nil && matches
}

// expansionRule 取唯一的 one_to_many 来源（拆行来源）。
func (service *Service) expansionRule() (string, map[string]any) {
	for _, sourceName := range service.Config.SourceOrder {
		if textOf(service.Config.SourceRules[sourceName]["cardinality"]) == "one_to_many" {
			return sourceName, service.Config.SourceRules[sourceName]
		}
	}
	return "", map[string]any{}
}

// noMatchAction 返回来源没有该基准记录时的动作；没声明时按 emit_incomplete 处理（R13）。
func (service *Service) noMatchAction(dataSource string, expansionSource string) string {
	if dataSource == expansionSource {
		noMatch, _ := service.Config.SourceRules[expansionSource]["no_match"].(map[string]any)
		return textOf(noMatch["action"])
	}
	if rule := service.Config.SourceRules[dataSource]; rule != nil {
		if noMatch, present := rule["no_match"].(map[string]any); present {
			return textOf(noMatch["action"])
		}
	}
	return "emit_incomplete"
}

// conflictDetails 收集当前结果行涉及的冲突文案，顺序稳定（便于逐字节比较）。
func (service *Service) conflictDetails(
	conflicts map[string]map[string]string,
	baseKey string,
	expansionSource string,
	expansionKey string,
	expansionRow store.Row,
) []string {
	details := []string{}
	for sourceName, conflictMap := range conflicts {
		key := baseKey
		if sourceName == expansionSource {
			if expansionRow == nil {
				continue
			}
			key = baseKey + "\x00" + expansionKey
		}
		if detail, present := conflictMap[key]; present {
			details = append(details, detail)
		}
	}
	sort.Strings(details)
	return details
}

// sourceLabel 是冲突文案里的来源名（中文名优先，其次 display_name，最后来源键）。
func (service *Service) sourceLabel(sourceName string) string {
	document, err := service.Loader.LoadImportMappingDocument()
	if err != nil {
		return sourceName
	}
	sources, _ := document.Value["sources"].(map[string]any)
	source, _ := sources[sourceName].(map[string]any)
	if chineseName := textOf(source["chinese_name"]); chineseName != "" {
		return chineseName
	}
	if displayName := textOf(source["display_name"]); displayName != "" {
		return displayName
	}
	return sourceName
}

// markIncomplete 把结果行降级为 Incomplete 并记录一次（R16）。
func markIncomplete(
	outcome *Outcome,
	incomplete map[string]bool,
	resultKey string,
	row store.Row,
) {
	if row["status"] == StatusReady {
		row["status"] = StatusIncomplete
	}
	if incomplete[resultKey] {
		return
	}
	incomplete[resultKey] = true
	outcome.MissingData = append(outcome.MissingData, resultKey)
}

// --- 小工具（对应 Python 版的 dict/list 处理） ---

func mappingAt(document map[string]any, key string) (map[string]any, error) {
	value, present := document[key]
	if !present {
		return nil, fmt.Errorf("%s must be a mapping", key)
	}
	mapping, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a mapping", key)
	}
	return mapping, nil
}

func textOf(value any) string {
	text, _ := value.(string)
	return text
}

// stringsOf 把「字符串或字符串列表」统一成切片（配置里两种写法都有）。
func stringsOf(value any) []string {
	entries, ok := value.([]any)
	if !ok {
		if text, isText := value.(string); isText {
			return []string{text}
		}
		return nil
	}
	results := make([]string, 0, len(entries))
	for _, entry := range entries {
		results = append(results, textOf(entry))
	}
	return results
}

func firstOf(value any) string {
	values := stringsOf(value)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func mapValues(value any) any {
	mapping, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	values := make([]any, 0, len(mapping))
	for _, key := range sortedKeysOf(mapping) {
		values = append(values, mapping[key])
	}
	return values
}

func sortedKeysOf(mapping map[string]any) []string {
	keys := make([]string, 0, len(mapping))
	for key := range mapping {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func normalizeKey(value string) string { return strings.TrimSpace(value) }

func containsEmpty(values []string) bool {
	for _, value := range values {
		if value == "" {
			return true
		}
	}
	return false
}

func rowsEqual(left store.Row, right store.Row) bool {
	if len(left) != len(right) {
		return false
	}
	for column, value := range left {
		if right[column] != value {
			return false
		}
	}
	return true
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
