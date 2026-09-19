package config

import (
	"fmt"
	"strings"

	"github.com/jiegun314/ssr-go/internal/rules"
)

// ValidateImportMapping 校验 excel_import_mapping.yaml（R24）。
//
// 逐条对应 tests/test_config_loader.py 的 config 用例与 AGENTS.md 的 R5/R6/R8。
func (loader *Loader) ValidateImportMapping(imports map[string]any) error {
	globalSettings, err := requireMapping(imports["global_settings"], "imports.global_settings")
	if err != nil {
		return err
	}
	mandatoryOptions, ok := stringListStrict(globalSettings["mandatory_options"])
	if !ok || len(mandatoryOptions) == 0 {
		return errorf("imports.global_settings.mandatory_options must be a non-empty list")
	}
	sources, err := requireMapping(imports["sources"], "imports.sources")
	if err != nil {
		return err
	}
	if len(sources) == 0 {
		return errorf("imports.sources must not be empty")
	}

	tableNames := map[string]bool{}
	for _, sourceName := range sortedKeys(sources) {
		source, err := requireMapping(sources[sourceName], "imports.sources."+sourceName)
		if err != nil {
			return err
		}
		for _, key := range []string{"target_table", "excel", "columns"} {
			if _, present := source[key]; !present {
				return errorf("imports.sources.%s.%s is required", sourceName, key)
			}
		}
		tableName, _ := asString(source["target_table"])
		if tableNames[tableName] {
			return errorf("Duplicate source target table: %s", tableName)
		}
		tableNames[tableName] = true
		for _, strategy := range []string{"replace_on_import", "preserve_on_cleanup"} {
			if _, ok := source[strategy].(bool); !ok {
				return errorf("imports.sources.%s.%s must be boolean", sourceName, strategy)
			}
		}
		columns, ok := source["columns"].([]any)
		if !ok || len(columns) == 0 {
			return errorf("imports.sources.%s.columns must be a list", sourceName)
		}

		dbFields := map[string]bool{}
		// required_when 可以引用同一来源里位置更靠后的列，所以条件要等所有
		// db_field 都收集完再校验。
		type conditionalColumn struct {
			index      int
			definition map[string]any
		}
		conditional := []conditionalColumn{}
		for index, rawColumn := range columns {
			path := fmt.Sprintf("imports.sources.%s.columns[%d]", sourceName, index)
			column, err := requireMapping(rawColumn, path)
			if err != nil {
				return err
			}
			for _, key := range []string{"chinese_name", "db_field", "mandatory_status"} {
				if _, present := column[key]; !present {
					return errorf("%s.%s is required", path, key)
				}
			}
			mandatoryStatus, _ := asString(column["mandatory_status"])
			if !contains(mandatoryOptions, mandatoryStatus) {
				return errorf("Invalid mandatory_status in %s: %s", path, mandatoryStatus)
			}
			dbField, _ := asString(column["db_field"])
			if dbFields[dbField] {
				return errorf("Duplicate db_field in %s: %s", sourceName, dbField)
			}
			dbFields[dbField] = true
			if value, present := column["required_when"]; present && value != nil {
				conditional = append(conditional, conditionalColumn{index: index, definition: column})
			}
		}

		available := make([]string, 0, len(dbFields))
		for field := range dbFields {
			available = append(available, field)
		}
		for _, item := range conditional {
			path := fmt.Sprintf("imports.sources.%s.columns[%d].required_when", sourceName, item.index)
			dbField, _ := asString(item.definition["db_field"])
			mandatoryStatus, _ := asString(item.definition["mandatory_status"])
			if mandatoryStatus != "required_if_applicable" {
				return errorf(
					"%s is only allowed when mandatory_status is 'required_if_applicable' (column %s)",
					path, dbField)
			}
			if err := rules.ValidateConditionSpec(
				item.definition["required_when"], available, path, dbField,
			); err != nil {
				// 条件规则自己的错误文案直接作为配置错误抛出（与 Python 版一致）
				return errorf("%s", err.Error())
			}
		}
	}
	return nil
}

func sortedKeys(mapping map[string]any) []string {
	keys := mappingKeys(mapping)
	sortStrings(keys)
	return keys
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && strings.Compare(values[j-1], values[j]) > 0; j-- {
			values[j-1], values[j] = values[j], values[j-1]
		}
	}
}
