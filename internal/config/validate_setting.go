package config

import (
	"sort"
	"strings"
)

// ValidateSetting 校验 setting.yaml 的契约（R24）。
func (loader *Loader) ValidateSetting(setting map[string]any) error {
	for _, key := range []string{
		"database", "folder", "export_template", "tables", "user_defined_data", "startup",
	} {
		if _, err := requireMapping(setting[key], "setting."+key); err != nil {
			return err
		}
	}
	database, _ := setting["database"].(map[string]any)
	if path, _ := database["path"].(string); path == "" {
		return errorf("setting.database.path is required")
	}
	startup, _ := setting["startup"].(map[string]any)
	if _, ok := startup["cleanup_on_startup"].(bool); !ok {
		return errorf("setting.startup.cleanup_on_startup must be boolean")
	}
	exportTemplate, _ := setting["export_template"].(map[string]any)
	for _, key := range []string{"path", "sheet_name", "header_row", "data_start_row"} {
		if _, present := exportTemplate[key]; !present {
			return errorf("setting.export_template.%s is required", key)
		}
	}
	tables, _ := setting["tables"].(map[string]any)
	for _, tableName := range []string{"consolidation_result", "operation_log"} {
		table, err := requireMapping(tables[tableName], "setting.tables."+tableName)
		if err != nil {
			return err
		}
		for _, key := range []string{"name", "replace_on_import", "preserve_on_cleanup"} {
			if _, present := table[key]; !present {
				return errorf("setting.tables.%s.%s is required", tableName, key)
			}
		}
	}
	if _, present := tables["operation_log_time_column"]; !present {
		return errorf("setting.tables.operation_log_time_column is required")
	}
	// APP_VERSION 是「没有构建期信息、也没有 git」时的回退值：可以缺省，
	// 但写成一个不像版本的值是配置错误，而不是安静地显示一个怪东西。
	if appVersion, present := setting["APP_VERSION"]; present && appVersion != nil {
		if !IsVersionString(appVersion) {
			return errorf(`setting.APP_VERSION must be a semantic version like "0.1.23"`)
		}
	}
	return nil
}

// ValidateLogColumns 校验 config/log_columns.yaml（R22 / §3.5）。
//
// 它是生成物，所以契约是「与其余配置一致」：同一条记录不能声明两次同一列，必须保留日志
// 自己写的列，也必须保留全部导出列 —— 不存的列既无法回顾，也无法在下一轮证明变化。
func (loader *Loader) ValidateLogColumns(
	definition map[string]any,
	setting map[string]any,
	consolidation map[string]any,
) error {
	for _, group := range []string{"before", "after", "columns"} {
		entries, ok := definition[group].([]any)
		if !ok {
			return errorf("log_columns.%s must be a list", group)
		}
		for _, entry := range entries {
			text, ok := entry.(string)
			if !ok || strings.TrimSpace(text) == "" {
				return errorf("log_columns.%s must contain non-empty strings", group)
			}
		}
	}
	columnsEntries, _ := definition["columns"].([]any)
	if len(columnsEntries) == 0 {
		return errorf("log_columns.columns must not be empty")
	}
	columns := LogColumns(definition)
	counts := map[string]int{}
	for _, name := range columns {
		counts[name]++
	}
	duplicated := []string{}
	for name, count := range counts {
		if count > 1 {
			duplicated = append(duplicated, name)
		}
	}
	if len(duplicated) > 0 {
		sort.Strings(duplicated)
		return errorf("log_columns must not declare a column twice: %s",
			strings.Join(duplicated, ", "))
	}

	dataset, err := requireMapping(consolidation["target_dataset"], "consolidation.target_dataset")
	if err != nil {
		return err
	}
	mergeRules, _ := dataset["merge_rules"].(map[string]any)
	baseSource, _ := mergeRules["base_source"].(map[string]any)
	required := []string{LogStatusColumn}
	required = append(required, LogOperationColumns...)
	required = append(required, stringList(baseSource["key"])...)
	tables, _ := setting["tables"].(map[string]any)
	if timeColumn, ok := tables["operation_log_time_column"].(string); ok && timeColumn != "" {
		required = append(required, timeColumn)
	}
	missing := []string{}
	for _, name := range required {
		if !contains(columns, name) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return errorf("log_columns is missing the columns the log itself writes: %s",
			strings.Join(missing, ", "))
	}
	exportFields, _ := dataset["fields"].(map[string]any)
	missingExport := []string{}
	for _, name := range mappingKeys(exportFields) {
		if !contains(columns, name) {
			missingExport = append(missingExport, name)
		}
	}
	if len(missingExport) > 0 {
		sort.Strings(missingExport)
		return errorf(
			"log_columns is missing export columns of consolidation.target_dataset.fields: %s",
			strings.Join(missingExport, ", "))
	}
	return nil
}
