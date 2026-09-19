package store

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// TimeFormat 是 log_time 的写法（R22）：YYYY/MM/DD HH:MM:SS。
const TimeFormat = "2006/01/02 15:04:05"

// OperationLogRecord 是要写进日志的一条记录：Key 是基准键（material_code），
// Values 是整合结果里的列。用切片而不是 map，是为了保留整合结果的顺序。
type OperationLogRecord struct {
	Key    string
	Values Row
}

// OperationLog 是 operation_log 表（R22/R23）。
type OperationLog struct {
	Repository *Repository
	TableName  string
	TimeColumn string
	// IdentityFields 是判断「这条记录是否已经存在」的列（来自 consolidation 配置）。
	IdentityFields []string
	// LogColumns 是一条日志记录的全部列，顺序即写入顺序、查询顺序、回顾显示顺序。
	LogColumns []string
	// Now 产出 log_time；测试与快照可以把它换成固定时钟。
	Now func() string
}

// OperationLogOptions 是构造 OperationLog 的显式参数（都由配置读出来）。
type OperationLogOptions struct {
	TableName      string
	TimeColumn     string
	IdentityFields []string
	LogColumns     []string
}

// OpenOperationLog 打开日志表：表不存在时按配置列建一张空表，已存在的表**不动**（R23）。
func OpenOperationLog(repository *Repository, options OperationLogOptions) (*OperationLog, error) {
	tableName := options.TableName
	if tableName == "" {
		tableName = "operation_log"
	}
	timeColumn := options.TimeColumn
	if timeColumn == "" {
		timeColumn = "log_time"
	}
	log := &OperationLog{
		Repository:     repository,
		TableName:      tableName,
		TimeColumn:     timeColumn,
		IdentityFields: options.IdentityFields,
		LogColumns:     options.LogColumns,
		Now:            func() string { return time.Now().Format(TimeFormat) },
	}
	if err := log.EnsureTable(); err != nil {
		return nil, err
	}
	return log, nil
}

// TableColumns 返回一条日志记录的列，顺序即配置顺序。
func (log *OperationLog) TableColumns() []string {
	return append([]string{}, log.LogColumns...)
}

// EnsureTable 只在缺表时建表：已存在的表不被改动，旧表缺的列在读日志时显示为空。
func (log *OperationLog) EnsureTable() error {
	exists, err := log.Repository.TableExists(log.TableName)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return log.Repository.WriteRows(log.TableName, log.LogColumns, nil, WriteReplace)
}

// InsertOperation 记录一条应用级操作（operation / details 留痕）。
//
// 只写这三列，其余列保持 NULL —— 与 Python 版一致，读日志时它们会被补成空字符串。
func (log *OperationLog) InsertOperation(operation string, details string) error {
	statement := fmt.Sprintf(
		"INSERT INTO %s (%s, %s, %s) VALUES (?, ?, ?)",
		quoteIdentifier(log.TableName),
		quoteIdentifier(log.TimeColumn),
		quoteIdentifier("operation"),
		quoteIdentifier("details"),
	)
	if err := log.Repository.Execute(
		statement, log.Now(), operation, details,
	); err != nil {
		return fmt.Errorf("Failed to insert log: %w", err)
	}
	return nil
}

// ReadByTime 读取一个时间区间内的记录，列 = 配置里的全部列（缺的列显示为空）。
func (log *OperationLog) ReadByTime(startTime string, endTime string) ([]Row, error) {
	rows, err := log.Repository.Rows(log.TableName)
	if err != nil {
		return nil, err
	}
	filtered := []Row{}
	for _, row := range rows {
		stamp := row[log.TimeColumn]
		// Python 版用 BETWEEN 比较字符串，闭区间
		if stamp >= startTime && stamp <= endTime {
			filtered = append(filtered, row)
		}
	}
	return log.complete(filtered), nil
}

// ReadByMaterialCode 读取一个产品代码的记录（列同上）。
func (log *OperationLog) ReadByMaterialCode(materialCode string) ([]Row, error) {
	rows, err := log.Repository.Rows(log.TableName)
	if err != nil {
		return nil, err
	}
	filtered := []Row{}
	for _, row := range rows {
		if row["material_code"] == materialCode {
			filtered = append(filtered, row)
		}
	}
	return log.complete(filtered), nil
}

// ReadIdentityFields 只读身份字段：判断某条记录是否已经导出过。
func (log *OperationLog) ReadIdentityFields() ([]Row, error) {
	if err := log.requireIdentityColumns(); err != nil {
		return nil, err
	}
	rows, err := log.Repository.Rows(log.TableName)
	if err != nil {
		return nil, err
	}
	projected := make([]Row, 0, len(rows))
	for _, row := range rows {
		identity := Row{}
		for _, field := range log.IdentityFields {
			identity[field] = row[field]
		}
		projected = append(projected, identity)
	}
	return projected, nil
}

// ReadLatestRecords 读同一身份的最新一条记录，列 = 表里**真实存在**的列。
//
// 变更比较只能拿存储里真的有的列作证：不存在的列无法证明变化（R17）。没有
// log_time 的行排在有时间的行前面，所以同一身份里时间最大的那条胜出。
func (log *OperationLog) ReadLatestRecords() ([]Row, error) {
	if err := log.requireIdentityColumns(); err != nil {
		return nil, err
	}
	rows, err := log.Repository.Rows(log.TableName)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return rows, nil
	}
	stableSortByTime(rows, log.TimeColumn)
	latest := map[string]Row{}
	order := []string{}
	for _, row := range rows {
		key := log.identityKey(row)
		if _, seen := latest[key]; !seen {
			order = append(order, key)
		}
		latest[key] = row
	}
	results := make([]Row, 0, len(order))
	for _, key := range order {
		results = append(results, latest[key])
	}
	return results, nil
}

// AppendRecords 追加整合结果：每条记录都写全部配置列 + 表里真实存在的额外列。
//
// 值与列都按配置顺序落地，整合行没有提供的列写空（不是 NULL），所以读回来的日志
// 永远有全部配置列。
func (log *OperationLog) AppendRecords(records []OperationLogRecord, indexColumn string) error {
	tableColumns, err := log.Repository.TableColumns(log.TableName)
	if err != nil {
		return err
	}
	configured := []string{}
	for _, column := range log.LogColumns {
		if containsString(tableColumns, column) {
			configured = append(configured, column)
		}
	}
	columnOrder := append([]string{}, configured...)
	extra := map[string]bool{}
	for _, record := range records {
		for column := range record.Values {
			if containsString(tableColumns, column) && !containsString(columnOrder, column) {
				extra[column] = true
			}
		}
	}
	extraColumns := make([]string, 0, len(extra))
	for column := range extra {
		extraColumns = append(extraColumns, column)
	}
	sort.Strings(extraColumns)
	columnOrder = append(columnOrder, extraColumns...)

	rows := make([]Row, 0, len(records))
	for _, record := range records {
		row := Row{}
		for column, value := range record.Values {
			row[column] = storedValue(value)
		}
		if _, present := row[indexColumn]; !present {
			row[indexColumn] = record.Key
		}
		row[log.TimeColumn] = log.Now()
		rows = append(rows, row)
	}
	return log.Repository.WriteRows(log.TableName, columnOrder, rows, WriteAppend)
}

// DropUnchangedRecords 丢掉「身份相同且各列值一致」的记录（R22 的幂等口径）。
//
// 导出成功后写日志，而暂存行的 Ready 状态会保留到下一次整合，所以同一份结果导出两次
// 会把同一条记录写两遍。值已经一致的记录不再追加；值变了的记录照常追加，日志因此保留
// 变更历史。比较只覆盖**存储表真的有的列**（按配置顺序），旧版本存的少列记录同样能
// 被认出来。
func (log *OperationLog) DropUnchangedRecords(
	records []OperationLogRecord,
	indexColumn string,
) ([]OperationLogRecord, error) {
	stored, err := log.ReadLatestRecords()
	if err != nil {
		return nil, err
	}
	if len(stored) == 0 {
		return records, nil
	}
	compared, err := log.comparisonColumns(records)
	if err != nil {
		return nil, err
	}
	storedByIdentity := map[string]Row{}
	for _, row := range stored {
		storedByIdentity[log.identityKey(row)] = row
	}
	remaining := []OperationLogRecord{}
	for _, record := range records {
		row := Row{}
		for column, value := range record.Values {
			row[column] = value
		}
		if _, present := row[indexColumn]; !present {
			row[indexColumn] = record.Key
		}
		storedRow, known := storedByIdentity[log.identityKey(row)]
		if !known {
			remaining = append(remaining, record)
			continue
		}
		changed := false
		for _, column := range compared {
			if column == log.TimeColumn {
				continue
			}
			if normalizeCell(row[column]) != normalizeCell(storedRow[column]) {
				changed = true
				break
			}
		}
		if changed {
			remaining = append(remaining, record)
		}
	}
	return remaining, nil
}

// comparisonColumns 是比较范围：配置列 ∩ 表里真实存在的列，加上待写记录额外提供的列。
func (log *OperationLog) comparisonColumns(records []OperationLogRecord) ([]string, error) {
	tableColumns, err := log.Repository.TableColumns(log.TableName)
	if err != nil {
		return nil, err
	}
	ordered := []string{}
	for _, column := range log.LogColumns {
		if containsString(tableColumns, column) {
			ordered = append(ordered, column)
		}
	}
	extra := map[string]bool{}
	for _, record := range records {
		for column := range record.Values {
			if containsString(tableColumns, column) && !containsString(ordered, column) {
				extra[column] = true
			}
		}
	}
	extraColumns := make([]string, 0, len(extra))
	for column := range extra {
		extraColumns = append(extraColumns, column)
	}
	sort.Strings(extraColumns)
	return append(ordered, extraColumns...), nil
}

// complete 把行投影到配置列顺序上：表里没有的列与 NULL 都读作空字符串（R22）。
func (log *OperationLog) complete(rows []Row) []Row {
	projected := make([]Row, 0, len(rows))
	for _, row := range rows {
		completeRow := Row{}
		for _, column := range log.LogColumns {
			completeRow[column] = normalizeCell(row[column])
		}
		projected = append(projected, completeRow)
	}
	return projected
}

func (log *OperationLog) requireIdentityColumns() error {
	tableColumns, err := log.Repository.TableColumns(log.TableName)
	if err != nil {
		return err
	}
	missing := []string{}
	for _, field := range log.IdentityFields {
		if !containsString(tableColumns, field) {
			missing = append(missing, field)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf(
			"Table %s is missing configured duplicate check columns: %s",
			log.TableName, strings.Join(missing, ", "))
	}
	return nil
}

func (log *OperationLog) identityKey(row Row) string {
	parts := make([]string, 0, len(log.IdentityFields))
	for _, field := range log.IdentityFields {
		parts = append(parts, normalizeCell(row[field]))
	}
	return strings.Join(parts, "\x00")
}

// stableSortByTime 按 log_time 升序稳定排序（空值排最前，与 pandas 的 na_position="first" 一致）。
func stableSortByTime(rows []Row, timeColumn string) {
	sort.SliceStable(rows, func(left, right int) bool {
		return rows[left][timeColumn] < rows[right][timeColumn]
	})
}

// storedValue 把「没值」写成一个空单元格而不是 NULL。
func storedValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

// normalizeCell 是变更比较与身份判定使用的归一：去首尾空白，NULL 读作空串。
func normalizeCell(value string) string {
	return strings.TrimSpace(value)
}
