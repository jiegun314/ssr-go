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
			// accepted_values：这一列只接受这些**原始写法**（空单元格不归它管）。
			// 必须是至少一项的非空字符串列表 —— 写成空列表等于没配，容易误以为已生效。
			if value, present := column["accepted_values"]; present && value != nil {
				accepted, ok := stringListStrict(value)
				if !ok || len(accepted) == 0 {
					return errorf(
						"%s.accepted_values must be a non-empty list of strings", path)
				}
				for _, item := range accepted {
					if strings.TrimSpace(item) == "" {
						return errorf("%s.accepted_values 里不能有空字符串", path)
					}
				}
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

		// 失效过滤（可选）：drop_expired.field 必须是本来源已声明的列
		if raw, present := source["drop_expired"]; present && raw != nil {
			block, err := requireMapping(raw, "imports.sources."+sourceName+".drop_expired")
			if err != nil {
				return err
			}
			field, _ := asString(block["field"])
			if field == "" {
				return errorf("imports.sources.%s.drop_expired.field is required", sourceName)
			}
			if !dbFields[field] {
				return errorf(
					"imports.sources.%s.drop_expired.field references unknown column: %s",
					sourceName, field)
			}
		}

		// 清洗表（可选）：只允许已声明的列，表名不能与其它表撞车
		if cleanValue, present := source["clean"]; present && cleanValue != nil {
			cleanTable, err := validateCleanRule(sourceName, source, cleanValue, available)
			if err != nil {
				return err
			}
			if tableNames[cleanTable] {
				return errorf("Duplicate source clean table: %s", cleanTable)
			}
			tableNames[cleanTable] = true
		}
	}
	return nil
}

// validateCleanRule 校验「导入后生成清洗表」的规则（来源的 clean 块）。
//
// 口径：表名独立且不同于原文表；分组/去重/最新日期/丢弃/唯一/连接列都必须是该来源
// 已声明的 db_field；连接与唯一列不能被 drop（它们是输出列）；conflict 只收 latest / error。
func validateCleanRule(sourceName string, source map[string]any, raw any, available []string) (string, error) {
	label := "imports.sources." + sourceName + ".clean"
	clean, err := requireMapping(raw, label)
	if err != nil {
		return "", err
	}
	tableName, _ := asString(clean["target_table"])
	if tableName == "" {
		return "", errorf("%s.target_table is required", label)
	}
	if rawTable, _ := asString(source["target_table"]); tableName == rawTable {
		return "", errorf("%s.target_table must differ from target_table", label)
	}
	groupBy, ok := stringListStrict(clean["group_by"])
	if !ok || len(groupBy) == 0 {
		return "", errorf("%s.group_by must be a non-empty list of db_field", label)
	}
	dedupe, err := requireMapping(clean["dedupe"], label+".dedupe")
	if err != nil {
		return "", err
	}
	dedupeBy, ok := stringListStrict(dedupe["by"])
	if !ok || len(dedupeBy) == 0 {
		return "", errorf("%s.dedupe.by must be a non-empty list of db_field", label)
	}
	latestBy, _ := asString(dedupe["latest_by"])
	if latestBy == "" {
		return "", errorf("%s.dedupe.latest_by is required", label)
	}
	conflict, _ := asString(clean["conflict"])
	if conflict == "" {
		conflict = "latest"
	}
	if conflict != "latest" && conflict != "error" {
		return "", errorf("%s.conflict must be latest or error", label)
	}

	drop, ok := stringListStrict(clean["drop"])
	if !ok && clean["drop"] != nil {
		return "", errorf("%s.drop must be a list of db_field", label)
	}
	unique, ok := stringListStrict(clean["unique"])
	if !ok && clean["unique"] != nil {
		return "", errorf("%s.unique must be a list of db_field", label)
	}
	joinComma, ok := stringListStrict(clean["join_comma"])
	if !ok && clean["join_comma"] != nil {
		return "", errorf("%s.join_comma must be a list of db_field", label)
	}
	joinBackslash, ok := stringListStrict(clean["join_backslash"])
	if !ok && clean["join_backslash"] != nil {
		return "", errorf("%s.join_backslash must be a list of db_field", label)
	}

	for key, fields := range map[string][]string{
		"group_by": groupBy, "dedupe.by": dedupeBy, "drop": drop,
		"unique": unique, "join_comma": joinComma, "join_backslash": joinBackslash,
	} {
		for _, field := range fields {
			if !contains(available, field) {
				return "", errorf("%s.%s: unknown db_field %s", label, key, field)
			}
		}
	}
	if !contains(available, latestBy) {
		return "", errorf("%s.dedupe.latest_by: unknown db_field %s", label, latestBy)
	}
	// 连接/唯一列是输出列，不能被 drop 掉（否则规则等于没生效）
	for key, fields := range map[string][]string{
		"unique": unique, "join_comma": joinComma, "join_backslash": joinBackslash,
		"group_by": groupBy,
	} {
		for _, field := range fields {
			if contains(drop, field) {
				return "", errorf("%s.%s: %s is dropped, cannot be used", label, key, field)
			}
		}
	}
	// 一列只能有一种合并口径
	for _, field := range joinComma {
		if contains(joinBackslash, field) {
			return "", errorf("%s: %s cannot be both join_comma and join_backslash", label, field)
		}
	}
	return tableName, nil
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
