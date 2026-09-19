package store

import (
	"strings"

	"github.com/jiegun314/ssr-go/internal/config"
)

// SourceTable 描述一份来源的暂存表：表名与建表列（= 配置里的全部 db_field，顺序即配置顺序）。
type SourceTable struct {
	SourceName string
	TableName  string
	Columns    []string
	// ReplaceOnImport 为 true 时导入重建整表，false 时追加（R8）。
	ReplaceOnImport bool
}

// SourceTables 按配置顺序读出四份来源的表定义（顺序来自 YAML 声明顺序）。
func SourceTables(loader *config.Loader) ([]SourceTable, error) {
	document, err := loader.LoadImportMappingDocument()
	if err != nil {
		return nil, err
	}
	sources, _ := document.Value["sources"].(map[string]any)
	tables := []SourceTable{}
	for _, sourceName := range document.Keys("sources") {
		source, _ := sources[sourceName].(map[string]any)
		if source == nil {
			continue
		}
		tableName, _ := source["target_table"].(string)
		replaceOnImport, _ := source["replace_on_import"].(bool)
		columns := []string{}
		columnDefinitions, _ := source["columns"].([]any)
		for _, rawColumn := range columnDefinitions {
			column, _ := rawColumn.(map[string]any)
			if field, ok := column["db_field"].(string); ok {
				columns = append(columns, field)
			}
		}
		tables = append(tables, SourceTable{
			SourceName:      sourceName,
			TableName:       tableName,
			Columns:         columns,
			ReplaceOnImport: replaceOnImport,
		})
	}
	return tables, nil
}

// ImportSourceRows 按来源声明的策略写入一张来源表（R8）。
func ImportSourceRows(repository *Repository, table SourceTable, rows []Row) error {
	mode := WriteAppend
	if table.ReplaceOnImport {
		mode = WriteReplace
	}
	return repository.WriteRows(table.TableName, table.Columns, rows, mode)
}

// SourceCleanupNames 返回「清空导入数据」要清掉的来源键，顺序即配置顺序（R25）。
func SourceCleanupNames(loader *config.Loader) ([]string, error) {
	document, err := loader.LoadImportMappingDocument()
	if err != nil {
		return nil, err
	}
	sources, _ := document.Value["sources"].(map[string]any)
	names := []string{}
	for _, sourceName := range document.Keys("sources") {
		source, _ := sources[sourceName].(map[string]any)
		preserve, _ := source["preserve_on_cleanup"].(bool)
		if !preserve {
			names = append(names, sourceName)
		}
	}
	return names, nil
}

// SourceCleanupTableNames 返回被清掉的来源表表名，顺序即配置顺序。
func SourceCleanupTableNames(loader *config.Loader) ([]string, error) {
	document, err := loader.LoadImportMappingDocument()
	if err != nil {
		return nil, err
	}
	names, err := SourceCleanupNames(loader)
	if err != nil {
		return nil, err
	}
	sources, _ := document.Value["sources"].(map[string]any)
	tables := make([]string, 0, len(names))
	for _, sourceName := range names {
		source, _ := sources[sourceName].(map[string]any)
		if tableName, ok := source["target_table"].(string); ok {
			tables = append(tables, tableName)
		}
	}
	return tables, nil
}

// CleanupTableNames 是启动清理的删除清单（R9）：来源表里 preserve_on_cleanup 为 false 的，
// 加上 setting.yaml 的 tables.* 里同样声明为不保留的应用表。
func CleanupTableNames(loader *config.Loader) ([]string, error) {
	sourceTables, err := SourceCleanupTableNames(loader)
	if err != nil {
		return nil, err
	}
	document, err := loader.LoadSettingDocument()
	if err != nil {
		return nil, err
	}
	tables, _ := document.Value["tables"].(map[string]any)
	applicationTables := []string{}
	for _, tableKey := range document.Keys("tables") {
		if tableKey == "operation_log_time_column" {
			continue
		}
		definition, _ := tables[tableKey].(map[string]any)
		if definition == nil {
			continue
		}
		preserve, _ := definition["preserve_on_cleanup"].(bool)
		if preserve {
			continue
		}
		if name, ok := definition["name"].(string); ok {
			applicationTables = append(applicationTables, name)
		}
	}
	return append(sourceTables, applicationTables...), nil
}

// TableFailure 是一次删表失败。
type TableFailure struct {
	Table string
	Error string
}

// DropTables 逐张删表并汇报结果；单张失败不阻断其余的表（R25 的失败分支靠它）。
func DropTables(repository *Repository, tableNames []string) (dropped []string, failed []TableFailure) {
	for _, tableName := range tableNames {
		if err := repository.DropTable(tableName); err != nil {
			failed = append(failed, TableFailure{Table: tableName, Error: err.Error()})
			continue
		}
		dropped = append(dropped, tableName)
	}
	return dropped, failed
}

// FormatTableFailure 生成「清空导入数据」失败分支的文案（R25）。
func FormatTableFailure(failures []TableFailure) string {
	parts := make([]string, 0, len(failures))
	for _, failure := range failures {
		parts = append(parts, failure.Table+": "+failure.Error)
	}
	return "Failed to drop table(s): " + strings.Join(parts, "; ")
}
