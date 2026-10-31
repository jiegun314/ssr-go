package importer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jiegun314/ssr-go/internal/store"
)

// CleanRule 是「导入后生成清洗表」的规则（excel_import_mapping 里来源的 clean 块）。
//
// 为什么要有它：RA 的来源文件按「注册证 × 规格型号」维护，同一个规格型号常有多张证、
// 每张证又有多条明细。整合只认「一个规格型号一行」，所以导入之后要把原文收成一张
// 干净的派生表 —— 原文照旧写进 target_table（数据回顾用），清洗结果写进另一张表（整合读它）。
//
// 规则按顺序生效：
//  1. 按 DedupeBy 分组，只留 LatestBy 最新的一行（同值取靠后的记录）；
//  2. 按 GroupBy 分成最终的行，组内按 LatestBy 倒序（最新证在前）；
//  3. Unique 里的列：组内必须只有一个非空值；多于一个时按 Conflict 处理
//     （latest = 取最新那行的值；error = 整份文件被拒）；
//  4. JoinComma / JoinBackslash 里的列：组内去重后用 "," / "\" 连接；
//     其余列（含 Drop 里的列）取最新那行的值 —— 输出列 = 全部列去掉 Drop。
//
// 用 db_field 描述（不是中文表头）：清洗发生在归一化之后，行里只有 db_field。
type CleanRule struct {
	// TargetTable 是清洗表表名；必须与来源的 target_table 不同。
	TargetTable string
	// GroupBy 是最终一行的分组键（当前 = material_code）。
	GroupBy []string
	// DedupeBy 是"同一张证"的键；LatestBy 是挑最新行用的日期列。
	DedupeBy []string
	LatestBy string
	// Drop 是清洗表里不要的列（仍参与去重/挑最新）；其余列按原顺序保留。
	Drop []string
	// Unique 是"一个分组只能有一个值"的列。
	Unique []string
	// Conflict 是 Unique 冲突时的口径：latest（默认）或 error。
	Conflict string
	// JoinComma / JoinBackslash 是合并时用 "," / "\" 连接的列。
	JoinComma     []string
	JoinBackslash []string
}

// CleanConflictError 是 Unique 列在同一分组里出现多个不同值（Conflict = error）时的业务拒绝。
type CleanConflictError struct {
	Message string
}

func (err *CleanConflictError) Error() string { return err.Message }

// cleanEntry 是带原始位置的行：位置用来做"同值取靠后记录"的稳定排序。
type cleanEntry struct {
	position int
	row      store.Row
}

// CleanRows 按规则把原始行收成「一个分组一行」，并返回清洗表的列（= 全部列去掉 Drop）。
//
// 返回的行仍带全部字段（含 Drop 的列）：调用方用返回的列去写表，多出来的键会被忽略；
// 这样条件必填校验还能看到被删掉的列（避免"删了列就绕过校验"）。
func CleanRows(rule *CleanRule, columns []Column, rows []store.Row) ([]store.Row, []string, error) {
	labels := map[string]string{}
	for _, column := range columns {
		labels[column.DBField] = column.ChineseName
	}

	// 1) 同一张证只留最新的一行
	latest := map[string]cleanEntry{}
	keys := []string{}
	for position, row := range rows {
		key := rowKey(row, rule.DedupeBy)
		current, seen := latest[key]
		if !seen {
			keys = append(keys, key)
			latest[key] = cleanEntry{position, row}
			continue
		}
		if rowValue(row, rule.LatestBy) >= rowValue(current.row, rule.LatestBy) {
			// 同值取靠后的记录（后导入的覆盖先导入的）
			latest[key] = cleanEntry{position, row}
		}
	}

	// 2) 按 GroupBy 分组，组内最新在前
	groups := map[string][]cleanEntry{}
	groupOrder := []string{}
	for _, key := range keys {
		item := latest[key]
		groupKey := rowKey(item.row, rule.GroupBy)
		if _, seen := groups[groupKey]; !seen {
			groupOrder = append(groupOrder, groupKey)
		}
		groups[groupKey] = append(groups[groupKey], item)
	}

	cleaned := make([]store.Row, 0, len(groupOrder))
	conflicts := []cleanConflict{}
	for _, groupKey := range groupOrder {
		group := groups[groupKey]
		sort.SliceStable(group, func(a, b int) bool {
			left, right := rowValue(group[a].row, rule.LatestBy), rowValue(group[b].row, rule.LatestBy)
			if left != right {
				return left > right
			}
			return group[a].position > group[b].position
		})
		newest := group[0].row

		// 先整合（算出这一组的行），再判冲突：把冲突收起来，全部整合完统一报
		if rule.Conflict == "error" {
			conflicts = append(conflicts, collectGroupConflicts(rule, groupKey, group)...)
		}

		record := store.Row{}
		for _, column := range columns {
			field := column.DBField
			switch {
			case containsField(rule.JoinComma, field):
				record[field] = joinValues(group, field, ",")
			case containsField(rule.JoinBackslash, field):
				record[field] = joinValues(group, field, `\`)
			default:
				// Unique 与普通列都取最新那一行的值
				record[field] = newest[field]
			}
		}
		cleaned = append(cleaned, record)
	}
	if err := conflictError(rule, labels, conflicts); err != nil {
		return nil, nil, err
	}
	return cleaned, outputColumns(columns, rule.Drop), nil
}

// cleanConflict 是某个分组在某个 unique 列上的冲突（用于汇总报错）。
type cleanConflict struct {
	group  string
	field  string
	values []string
}

// collectGroupConflicts 收集一个分组在 unique 列上的冲突（同组多个非空值）。
func collectGroupConflicts(rule *CleanRule, groupKey string, group []cleanEntry) []cleanConflict {
	conflicts := []cleanConflict{}
	display := strings.ReplaceAll(groupKey, "\x00", " + ")
	for _, field := range rule.Unique {
		values := collectValues(group, field)
		if len(values) < 2 {
			continue
		}
		conflicts = append(conflicts, cleanConflict{group: display, field: field, values: values})
	}
	return conflicts
}

// conflictError 把全部冲突汇总成一条中文错误（按字段分组，最多给两例）。
//
// 只报第一处对几十上百个型号没有可操作性，所以这里给的是"总数 + 每项的型号数 + 例子"。
func conflictError(rule *CleanRule, labels map[string]string, conflicts []cleanConflict) error {
	if len(conflicts) == 0 {
		return nil
	}
	groups := map[string]bool{}
	order := []string{}
	byField := map[string][]cleanConflict{}
	for _, conflict := range conflicts {
		groups[conflict.group] = true
		if _, seen := byField[conflict.field]; !seen {
			order = append(order, conflict.field)
		}
		byField[conflict.field] = append(byField[conflict.field], conflict)
	}
	lines := []string{fmt.Sprintf(
		"清洗时发现 %d 个规格型号有冲突（共 %d 处），整份文件不导入：",
		len(groups), len(conflicts))}
	for _, field := range order {
		items := byField[field]
		label := field
		if text, ok := labels[field]; ok && text != "" {
			label = text
		}
		examples := items
		if len(examples) > 2 {
			examples = examples[:2]
		}
		parts := make([]string, 0, len(examples))
		for _, example := range examples {
			values := example.values
			if len(values) > 2 {
				values = values[:2]
			}
			shortened := make([]string, 0, len(values))
			for _, value := range values {
				shortened = append(shortened, shortenText(value, 28))
			}
			parts = append(parts, fmt.Sprintf("%s「%s」", example.group, strings.Join(shortened, " / ")))
		}
		lines = append(lines, fmt.Sprintf("- %s：%d 个型号，例 %s",
			label, len(items), strings.Join(parts, "；")))
	}
	lines = append(lines,
		"请把这些项目改成一致（或把 clean.conflict 设为 latest 取最新证的值）后重新导入。")
	return &CleanConflictError{Message: strings.Join(lines, "\n")}
}

// shortenText 把过长的取值截断（例子只用来定位，不追求完整）。
func shortenText(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

// rowKey 用若干列拼出分组键；键里用 \x00 分隔（值里不会出现）。
func rowKey(row store.Row, fields []string) string {
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		parts = append(parts, strings.TrimSpace(row[field]))
	}
	return strings.Join(parts, "\x00")
}

// rowValue 取一列做排序/比较用（空串排最前）。
func rowValue(row store.Row, field string) string {
	if field == "" {
		return ""
	}
	return strings.TrimSpace(row[field])
}

// collectValues 收集组内某列的非空值（按组内顺序去重）。
func collectValues(group []cleanEntry, field string) []string {
	values := []string{}
	for _, item := range group {
		value := strings.TrimSpace(item.row[field])
		if value == "" || containsText(values, value) {
			continue
		}
		values = append(values, value)
	}
	return values
}

// joinValues 把组内某列的非空值去重后连接（顺序 = 组内顺序，即最新证在前）。
func joinValues(group []cleanEntry, field string, separator string) string {
	return strings.Join(collectValues(group, field), separator)
}

// outputColumns 返回清洗表的列：原顺序，去掉 Drop。
func outputColumns(columns []Column, drop []string) []string {
	result := make([]string, 0, len(columns))
	for _, column := range columns {
		if containsField(drop, column.DBField) {
			continue
		}
		result = append(result, column.DBField)
	}
	return result
}

func containsField(fields []string, field string) bool { return containsText(fields, field) }

func containsText(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

// parseCleanRule 解析来源的 clean 块；不完整时返回 nil（配置校验会先拦住）。
func parseCleanRule(raw any) *CleanRule {
	block, ok := raw.(map[string]any)
	if !ok || block == nil {
		return nil
	}
	dedupe, _ := block["dedupe"].(map[string]any)
	rule := &CleanRule{
		TargetTable:   textOf(block["target_table"]),
		GroupBy:       stringListOf(block["group_by"]),
		DedupeBy:      stringListOf(dedupe["by"]),
		LatestBy:      textOf(dedupe["latest_by"]),
		Drop:          stringListOf(block["drop"]),
		Unique:        stringListOf(block["unique"]),
		Conflict:      textOf(block["conflict"]),
		JoinComma:     stringListOf(block["join_comma"]),
		JoinBackslash: stringListOf(block["join_backslash"]),
	}
	if rule.Conflict == "" {
		rule.Conflict = "latest"
	}
	if rule.TargetTable == "" || len(rule.GroupBy) == 0 || len(rule.DedupeBy) == 0 || rule.LatestBy == "" {
		return nil
	}
	return rule
}

// CleanColumns 返回清洗表的列（中文名 + db_field，顺序 = 原顺序去掉 drop）。
//
// 没有 clean 规则时返回 nil。导入写表与「查看清洗后数据」共用这一份口径，
// 避免两边各写一套"哪些列被删了"。
func CleanColumns(rule *Rule) []Column {
	if rule == nil || rule.Clean == nil {
		return nil
	}
	dropped := map[string]bool{}
	for _, field := range rule.Clean.Drop {
		dropped[field] = true
	}
	columns := make([]Column, 0, len(rule.Columns))
	for _, column := range rule.Columns {
		if dropped[column.DBField] {
			continue
		}
		columns = append(columns, column)
	}
	return columns
}

// RebuildCleaned 按规则从本机原文表重建清洗表（对齐共享盘快照后使用）。
//
// 返回清洗表行数；没有 clean 规则时返回 0。原文表必须先由调用方写入。
func RebuildCleaned(repository *store.Repository, rule *Rule) (int, error) {
	if rule == nil || rule.Clean == nil {
		return 0, nil
	}
	rawRows, err := repository.Rows(rule.TargetTable)
	if err != nil {
		return 0, err
	}
	cleaned, columns, err := CleanRows(rule.Clean, rule.Columns, rawRows)
	if err != nil {
		return 0, err
	}
	if err := store.ImportSourceRows(repository, store.SourceTable{
		SourceName: rule.SourceName, TableName: rule.Clean.TargetTable,
		Columns: columns, ReplaceOnImport: true,
	}, cleaned); err != nil {
		return 0, err
	}
	return len(cleaned), nil
}
