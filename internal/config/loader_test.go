package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// newTempLoader 把真实配置复制到临时目录，测试只改动副本（与 tests/conftest.py 的
// isolated_workspace 同一个思路：跑测试不会碰仓库里的 data/ 与 output/）。
func newTempLoader(t *testing.T) (*Loader, string) {
	t.Helper()
	source := filepath.Join("..", "..", "config")
	target := t.TempDir()
	for _, name := range []string{
		SettingFile, ImportMappingFile, ConsolidationMappingFile, LogColumnsFile,
	} {
		content, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			t.Fatalf("读取 %s 失败：%v", name, err)
		}
		if err := os.WriteFile(filepath.Join(target, name), content, 0o644); err != nil {
			t.Fatalf("写 %s 失败：%v", name, err)
		}
	}
	loader, err := NewLoader(target)
	if err != nil {
		t.Fatalf("NewLoader 失败：%v", err)
	}
	return loader, target
}

// patchYAML 改一份配置副本并写回，返回改动后的文档。
func patchYAML(t *testing.T, loader *Loader, name string, mutate func(map[string]any)) {
	t.Helper()
	document, err := loader.LoadYAML(name)
	if err != nil {
		t.Fatalf("读 %s 失败：%v", name, err)
	}
	mutate(document)
	rendered, err := yaml.Marshal(document)
	if err != nil {
		t.Fatalf("序列化 %s 失败：%v", name, err)
	}
	if err := os.WriteFile(loader.Resolver.ConfigFile(name), rendered, 0o644); err != nil {
		t.Fatalf("写回 %s 失败：%v", name, err)
	}
}

func expectValidateAllError(t *testing.T, loader *Loader, wanted string) string {
	t.Helper()
	err := loader.ValidateAll()
	if err == nil {
		t.Fatalf("配置校验应该失败并包含 %q", wanted)
	}
	if !strings.Contains(err.Error(), wanted) {
		t.Fatalf("错误 %q 不含 %q", err.Error(), wanted)
	}
	return err.Error()
}

func nestedMapping(t *testing.T, document map[string]any, keys ...string) map[string]any {
	t.Helper()
	current := document
	for _, key := range keys {
		next, ok := current[key].(map[string]any)
		if !ok {
			t.Fatalf("配置里 %s 不是映射", strings.Join(keys, "."))
		}
		current = next
	}
	return current
}

func TestR24ValidateAllAcceptsTheCurrentConfiguration(t *testing.T) {
	loader, _ := newTempLoader(t)

	// 这一条同时证明 YAML 的类型解析与配置一致：如果 value: 1 被读成字符串，
	// 大小比较就会因为「必须 value_type: number」而失败。
	if err := loader.ValidateAll(); err != nil {
		t.Fatalf("当前配置应当通过校验，得到：%v", err)
	}
}

func TestR24ReportsAMissingConfigurationFile(t *testing.T) {
	loader, err := NewLoader(t.TempDir())
	if err != nil {
		t.Fatalf("NewLoader 失败：%v", err)
	}
	_, err = loader.LoadSetting()
	if err == nil || !strings.Contains(err.Error(), "Configuration file not found") {
		t.Fatalf("缺少配置文件要明确报出来，得到：%v", err)
	}
}

func TestR24RequiresTheConfigurationRootToBeAMapping(t *testing.T) {
	loader, configDir := newTempLoader(t)
	if err := os.WriteFile(
		filepath.Join(configDir, SettingFile), []byte("- just\n- a list\n"), 0o644); err != nil {
		t.Fatalf("写文件失败：%v", err)
	}
	_, err := loader.LoadSetting()
	if err == nil || !strings.Contains(err.Error(), "must be a mapping") {
		t.Fatalf("根不是映射要报错，得到：%v", err)
	}
}

func TestR9CleanupSwitchIsOverriddenByTheEnvironmentForOneRun(t *testing.T) {
	loader, _ := newTempLoader(t)
	for _, value := range []string{"false", "FALSE", "0", "no", "off"} {
		t.Setenv(CleanupOnStartupEnv, value)
		enabled, err := loader.LoadCleanupOnStartup()
		if err != nil || enabled {
			t.Errorf("%s 应关闭启动清理（err=%v, enabled=%v）", value, err, enabled)
		}
	}
	for _, value := range []string{"true", "TRUE", "1", "yes", "on"} {
		t.Setenv(CleanupOnStartupEnv, value)
		enabled, err := loader.LoadCleanupOnStartup()
		if err != nil || !enabled {
			t.Errorf("%s 应开启启动清理（err=%v, enabled=%v）", value, err, enabled)
		}
	}
	t.Setenv(CleanupOnStartupEnv, "maybe")
	if _, err := loader.LoadCleanupOnStartup(); err == nil ||
		!strings.Contains(err.Error(), "must be a boolean value") {
		t.Fatalf("写错的值必须报配置错误，而不是静默取默认值：%v", err)
	}
}

// TestR9CleanupSwitchDefaultsToTheConfigurationFile 固定发布包的默认行为：
// 默认**不**清理暂存表 —— 重启后导入的数据要还在（用户要求）。需要老行为时，
// 改 setting.yaml 或临时用 SSR_CLEANUP_ON_STARTUP=true 覆盖（见上一个用例）。
func TestR9CleanupSwitchDefaultsToTheConfigurationFile(t *testing.T) {
	loader, _ := newTempLoader(t)
	os.Unsetenv(CleanupOnStartupEnv)

	enabled, err := loader.LoadCleanupOnStartup()
	if err != nil {
		t.Fatalf("读配置失败：%v", err)
	}
	if enabled {
		t.Fatal("发布包的 setting.yaml 默认应为 cleanup_on_startup: false（重启不清理导入数据）")
	}
}

func TestR24RejectsANonBooleanCleanupFlag(t *testing.T) {
	loader, _ := newTempLoader(t)
	patchYAML(t, loader, SettingFile, func(document map[string]any) {
		nestedMapping(t, document, "startup")["cleanup_on_startup"] = "yes"
	})

	expectValidateAllError(t, loader, "setting.startup.cleanup_on_startup must be boolean")
}

func TestR24RejectsAnAppVersionThatIsNotAVersion(t *testing.T) {
	loader, _ := newTempLoader(t)
	patchYAML(t, loader, SettingFile, func(document map[string]any) {
		document["APP_VERSION"] = "version one"
	})

	expectValidateAllError(t, loader, "setting.APP_VERSION")
}

func TestR24AcceptsASettingWithoutAnAppVersion(t *testing.T) {
	loader, _ := newTempLoader(t)
	patchYAML(t, loader, SettingFile, func(document map[string]any) {
		delete(document, "APP_VERSION")
	})

	if err := loader.ValidateAll(); err != nil {
		t.Fatalf("APP_VERSION 是回退值，可以缺省：%v", err)
	}
}

func TestR24RequiresEveryExportTemplateKey(t *testing.T) {
	loader, _ := newTempLoader(t)
	patchYAML(t, loader, SettingFile, func(document map[string]any) {
		delete(nestedMapping(t, document, "export_template"), "sheet_name")
	})

	expectValidateAllError(t, loader, "setting.export_template.sheet_name is required")
}

func TestR22LogColumnsMustNotDeclareAColumnTwice(t *testing.T) {
	loader, _ := newTempLoader(t)
	patchYAML(t, loader, LogColumnsFile, func(document map[string]any) {
		after, _ := document["after"].([]any)
		document["after"] = append(after, LogStatusColumn)
	})

	expectValidateAllError(t, loader, "must not declare a column twice")
}

func TestR22LogColumnsMustKeepTheColumnsTheLogWrites(t *testing.T) {
	loader, _ := newTempLoader(t)
	patchYAML(t, loader, LogColumnsFile, func(document map[string]any) {
		before, _ := document["before"].([]any)
		document["before"] = before[1:] // 去掉 log_time
	})

	expectValidateAllError(t, loader,
		"missing the columns the log itself writes: log_time")
}

func TestR22LogColumnsMustKeepEveryExportColumn(t *testing.T) {
	loader, _ := newTempLoader(t)
	patchYAML(t, loader, LogColumnsFile, func(document map[string]any) {
		columns, _ := document["columns"].([]any)
		remaining := []any{}
		for _, entry := range columns {
			if entry != "Primary DI" {
				remaining = append(remaining, entry)
			}
		}
		document["columns"] = remaining
	})

	expectValidateAllError(t, loader, "missing export columns")
}

func TestR24ImportMappingRejectsASourceWithoutItsRequiredKeys(t *testing.T) {
	loader, _ := newTempLoader(t)
	patchYAML(t, loader, ImportMappingFile, func(document map[string]any) {
		delete(nestedMapping(t, document, "sources", "ra_input"), "target_table")
	})

	expectValidateAllError(t, loader, "imports.sources.ra_input.target_table is required")
}

func TestR24ImportMappingRejectsDuplicateDbFields(t *testing.T) {
	loader, _ := newTempLoader(t)
	patchYAML(t, loader, ImportMappingFile, func(document map[string]any) {
		source := nestedMapping(t, document, "sources", "ra_input")
		columns, _ := source["columns"].([]any)
		source["columns"] = append(columns, map[string]any{
			"chinese_name":     "重复字段",
			"db_field":         "material_code",
			"mandatory_status": "optional",
		})
	})

	expectValidateAllError(t, loader, "Duplicate db_field")
}

func TestR24ImportMappingRejectsAnUnknownMandatoryStatus(t *testing.T) {
	loader, _ := newTempLoader(t)
	patchYAML(t, loader, ImportMappingFile, func(document map[string]any) {
		source := nestedMapping(t, document, "sources", "ra_input")
		columns, _ := source["columns"].([]any)
		first, _ := columns[0].(map[string]any)
		first["mandatory_status"] = "sometimes"
	})

	expectValidateAllError(t, loader, "Invalid mandatory_status")
}

func TestR6RejectsAConditionOnAStrictlyRequiredColumn(t *testing.T) {
	loader, _ := newTempLoader(t)
	patchYAML(t, loader, ImportMappingFile, func(document map[string]any) {
		column := conditionalColumn(t, document)
		column["mandatory_status"] = "required"
	})

	expectValidateAllError(t, loader, "only allowed when mandatory_status")
}

func TestR6RejectsAnUnknownConditionOperator(t *testing.T) {
	loader, _ := newTempLoader(t)
	patchYAML(t, loader, ImportMappingFile, func(document map[string]any) {
		column := conditionalColumn(t, document)
		column["required_when"] = []any{map[string]any{
			"column": "quantity_per_min_sales_unit", "operator": "more_than",
			"value": 1, "value_type": "number",
		}}
	})

	expectValidateAllError(t, loader, "unknown condition operator")
}

func TestR6RejectsAConditionReferencingAnUnknownColumn(t *testing.T) {
	loader, _ := newTempLoader(t)
	patchYAML(t, loader, ImportMappingFile, func(document map[string]any) {
		column := conditionalColumn(t, document)
		column["required_when"] = []any{map[string]any{
			"column": "not_a_configured_column", "operator": "is_not_empty",
		}}
	})

	expectValidateAllError(t, loader, "unknown condition column")
}

func TestR6RejectsAnOrderingConditionWithoutNumberValueType(t *testing.T) {
	loader, _ := newTempLoader(t)
	patchYAML(t, loader, ImportMappingFile, func(document map[string]any) {
		column := conditionalColumn(t, document)
		column["required_when"] = []any{map[string]any{
			"column": "quantity_per_min_sales_unit", "operator": "greater_than",
			"value": 1, "value_type": "text",
		}}
	})

	expectValidateAllError(t, loader, "requires value_type: number")
}

func TestR6AcceptsAConditionReferencingALaterColumn(t *testing.T) {
	loader, _ := newTempLoader(t)
	patchYAML(t, loader, ImportMappingFile, func(document map[string]any) {
		source := nestedMapping(t, document, "sources", "global_udi_input")
		columns, _ := source["columns"].([]any)
		first, _ := columns[0].(map[string]any)
		first["mandatory_status"] = "required_if_applicable"
		first["required_when"] = []any{map[string]any{
			"column": "quantity_per_min_sales_unit", "operator": "greater_than",
			"value": 1, "value_type": "number",
		}}
	})

	if err := loader.ValidateAll(); err != nil {
		t.Fatalf("条件可以引用同一来源里位置更靠后的列：%v", err)
	}
}

func TestR12RejectsAnUnsupportedCardinality(t *testing.T) {
	loader, _ := newTempLoader(t)
	patchYAML(t, loader, ConsolidationMappingFile, func(document map[string]any) {
		rule := nestedMapping(t, document, "target_dataset", "merge_rules",
			"sources", "global_udi_input")
		rule["cardinality"] = "invalid"
	})

	expectValidateAllError(t, loader, "Unsupported cardinality")
}

func TestR12RejectsAnUnknownFieldInTheMergeMatch(t *testing.T) {
	loader, _ := newTempLoader(t)
	patchYAML(t, loader, ConsolidationMappingFile, func(document map[string]any) {
		rule := nestedMapping(t, document, "target_dataset", "merge_rules",
			"sources", "ra_input")
		rule["match"] = map[string]any{"material_code": "missing_field"}
	})

	expectValidateAllError(t, loader, "Unknown field in merge match")
}

func TestR13RejectsAnUnknownNoMatchAction(t *testing.T) {
	loader, _ := newTempLoader(t)
	patchYAML(t, loader, ConsolidationMappingFile, func(document map[string]any) {
		rule := nestedMapping(t, document, "target_dataset", "merge_rules",
			"sources", "medical_insurance_code")
		noMatch := rule["no_match"].(map[string]any)
		noMatch["action"] = "ignore"
	})

	expectValidateAllError(t, loader, "Unsupported no_match action")
}

func TestR14RejectsAnUnknownDuplicateAction(t *testing.T) {
	loader, _ := newTempLoader(t)
	patchYAML(t, loader, ConsolidationMappingFile, func(document map[string]any) {
		rule := nestedMapping(t, document, "target_dataset", "merge_rules",
			"sources", "global_udi_input")
		duplicates := rule["duplicates"].(map[string]any)
		duplicates["different_content"] = "overwrite"
	})

	expectValidateAllError(t, loader, "Unsupported duplicate action")
}

func TestR15RejectsAnUnknownTransformType(t *testing.T) {
	loader, _ := newTempLoader(t)
	patchYAML(t, loader, ConsolidationMappingFile, func(document map[string]any) {
		field := nestedMapping(t, document, "target_dataset", "fields", "Primary DI")
		field["transform"] = map[string]any{"type": "lookup"}
	})

	expectValidateAllError(t, loader, "Unsupported transform type")
}

func TestR15RejectsAnUnknownOutputSource(t *testing.T) {
	loader, _ := newTempLoader(t)
	patchYAML(t, loader, ConsolidationMappingFile, func(document map[string]any) {
		field := nestedMapping(t, document, "target_dataset", "fields", "Primary DI")
		field["source"] = map[string]any{"file": "unknown_source", "field": "material_code"}
	})

	expectValidateAllError(t, loader, "Unknown output source")
}

func TestR15RejectsAnUnknownOutputField(t *testing.T) {
	loader, _ := newTempLoader(t)
	patchYAML(t, loader, ConsolidationMappingFile, func(document map[string]any) {
		field := nestedMapping(t, document, "target_dataset", "fields", "Primary DI")
		field["source"] = map[string]any{"file": "global_udi_input", "field": "missing_field"}
	})

	expectValidateAllError(t, loader, "Unknown output field")
}

func TestR24RequiresAUserDefinedSourceKey(t *testing.T) {
	loader, _ := newTempLoader(t)
	patchYAML(t, loader, ConsolidationMappingFile, func(document map[string]any) {
		field := nestedMapping(t, document, "target_dataset", "fields", "Data Scope")
		field["source"] = map[string]any{}
	})

	expectValidateAllError(t, loader, "user_defined source key is required")
}

func TestR24RejectsAnUnknownUserDefinedKey(t *testing.T) {
	loader, _ := newTempLoader(t)
	patchYAML(t, loader, ConsolidationMappingFile, func(document map[string]any) {
		field := nestedMapping(t, document, "target_dataset", "fields", "Data Scope")
		field["source"] = map[string]any{"key": "missing_key"}
	})

	expectValidateAllError(t, loader, "Unknown user_defined_data key")
}

func TestR24RequiresALiteralValueForASourceLessRequiredField(t *testing.T) {
	loader, _ := newTempLoader(t)
	patchYAML(t, loader, ConsolidationMappingFile, func(document map[string]any) {
		field := nestedMapping(t, document, "target_dataset", "fields", "Choose Action")
		field["source"] = map[string]any{}
		field["transform"] = map[string]any{"type": "direct"}
	})

	expectValidateAllError(t, loader, "Literal transform value is required")
}

func TestR24RejectsAnUnsupportedOutputMandatoryStatus(t *testing.T) {
	loader, _ := newTempLoader(t)
	patchYAML(t, loader, ConsolidationMappingFile, func(document map[string]any) {
		field := nestedMapping(t, document, "target_dataset", "fields", "Data Scope")
		field["mandatory_status"] = "fixed"
	})

	expectValidateAllError(t, loader, "Unsupported mandatory_status")
}

func TestR19IdentityFieldsMustBeANonEmptyListOfExportColumns(t *testing.T) {
	cases := []struct {
		name    string
		fields  any
		message string
	}{
		{"empty", []any{}, "must be a non-empty list"},
		{"non string", []any{"Primary DI", 42}, "must contain non-empty strings"},
		{"duplicate", []any{"Primary DI", "Primary DI"}, "must not contain duplicates"},
		{"not an export column", []any{"Primary DI", "Not An Export Column"}, "Not An Export Column"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			loader, _ := newTempLoader(t)
			patchYAML(t, loader, ConsolidationMappingFile, func(document map[string]any) {
				duplicateCheck := nestedMapping(t, document, "target_dataset", "duplicate_check")
				duplicateCheck["identity_fields"] = testCase.fields
			})
			expectValidateAllError(t, loader, testCase.message)
		})
	}
}

func TestR24RequiresADuplicateCheckSection(t *testing.T) {
	loader, _ := newTempLoader(t)
	patchYAML(t, loader, ConsolidationMappingFile, func(document map[string]any) {
		delete(nestedMapping(t, document, "target_dataset"), "duplicate_check")
	})

	expectValidateAllError(t, loader, "duplicate_check must be a mapping")
}

func TestR18ChangeDescriptionRulesAreValidated(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(field map[string]any)
		message string
	}{
		{
			"template is required",
			func(field map[string]any) {
				field["transform"] = map[string]any{
					"type":            "change_description",
					"on_other_change": map[string]any{"mode": "auto"},
				}
			},
			"template is required",
		},
		{
			"template without a placeholder",
			func(field map[string]any) {
				transform := field["transform"].(map[string]any)
				onOtherChange := transform["on_other_change"].(map[string]any)
				onOtherChange["template"] = "changed"
			},
			"has to use one of",
		},
		{
			"auto mode with a source",
			func(field map[string]any) {
				field["source"] = map[string]any{"file": "ra_input", "field": "change_description"}
			},
			"has to stay empty",
		},
		{
			"source mode without a source",
			func(field map[string]any) {
				transform := field["transform"].(map[string]any)
				transform["on_other_change"] = map[string]any{"mode": "source"}
			},
			"has to declare the file and field",
		},
		{
			"unknown missing action",
			func(field map[string]any) {
				field["source"] = map[string]any{"file": "ra_input", "field": "change_description"}
				transform := field["transform"].(map[string]any)
				transform["on_other_change"] = map[string]any{
					"mode": "source", "missing_action": "skip",
				}
			},
			"missing_action must be one of",
		},
		{
			"unknown mode",
			func(field map[string]any) {
				transform := field["transform"].(map[string]any)
				onOtherChange := transform["on_other_change"].(map[string]any)
				onOtherChange["mode"] = "guess"
			},
			"on_other_change.mode must be one of",
		},
		{
			"wrong identity reference",
			func(field map[string]any) {
				transform := field["transform"].(map[string]any)
				transform["identity_fields"] = "identity_fields"
			},
			"identity_fields must be 'duplicate_check'",
		},
		{
			"compare fields must be export columns",
			func(field map[string]any) {
				transform := field["transform"].(map[string]any)
				transform["compare_fields"] = []any{"Not A Column"}
			},
			"is not an export column",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			loader, _ := newTempLoader(t)
			patchYAML(t, loader, ConsolidationMappingFile, func(document map[string]any) {
				field := nestedMapping(t, document, "target_dataset", "fields", "Change Description")
				testCase.mutate(field)
			})
			expectValidateAllError(t, loader, testCase.message)
		})
	}
}

func TestR18ChangeDescriptionCannotBeAnIdentityField(t *testing.T) {
	loader, _ := newTempLoader(t)
	patchYAML(t, loader, ConsolidationMappingFile, func(document map[string]any) {
		duplicateCheck := nestedMapping(t, document, "target_dataset", "duplicate_check")
		fields, _ := duplicateCheck["identity_fields"].([]any)
		duplicateCheck["identity_fields"] = append(fields, "Change Description")
	})

	expectValidateAllError(t, loader, "can not be part of")
}

func TestR18TheDerivedColumnRuleIsReadFromTheConfiguration(t *testing.T) {
	loader, _ := newTempLoader(t)

	rule, err := loader.LoadChangeDescriptionRule()
	if err != nil {
		t.Fatalf("读变更描述规则失败：%v", err)
	}
	if rule == nil {
		t.Fatal("当前配置里有 Change Description 派生列")
	}
	if rule.Field != "Change Description" {
		t.Errorf("Field = %q", rule.Field)
	}
	if strings.Join(rule.IdentityFields, "|") != "Catalog or Reference Number|Primary DI" {
		t.Errorf("身份字段 = %v", rule.IdentityFields)
	}
	if rule.CompareFields != "all" {
		t.Errorf("CompareFields = %v", rule.CompareFields)
	}
	if strings.Join(rule.IgnoreFields, "|") != "Choose Action" {
		t.Errorf("IgnoreFields = %v", rule.IgnoreFields)
	}
	if rule.Labels["Change Description"] != "Change Description" {
		t.Errorf("Labels = %v", rule.Labels)
	}
	if mode, _ := rule.OnOtherChange["mode"].(string); mode != "auto" {
		t.Errorf("mode = %v", rule.OnOtherChange["mode"])
	}
}

func TestR19TheIdentityFieldsAreReadFromTheConfiguration(t *testing.T) {
	loader, _ := newTempLoader(t)

	fields, err := loader.LoadDuplicateCheckIdentityFields()
	if err != nil {
		t.Fatalf("读身份字段失败：%v", err)
	}
	if strings.Join(fields, "|") != "Catalog or Reference Number|Primary DI" {
		t.Fatalf("身份字段 = %v", fields)
	}
}

func TestR22TheLogColumnOrderCombinesTheThreeGroups(t *testing.T) {
	loader, _ := newTempLoader(t)

	definition, err := loader.LoadLogColumnsDefinition()
	if err != nil {
		t.Fatalf("读 log_columns.yaml 失败：%v", err)
	}
	columns, err := loader.LoadLogColumns()
	if err != nil {
		t.Fatalf("读日志列失败：%v", err)
	}
	before, _ := definition["before"].([]any)
	definitionColumns, _ := definition["columns"].([]any)
	after, _ := definition["after"].([]any)
	if len(columns) != len(before)+len(definitionColumns)+len(after) {
		t.Fatalf("日志列数 = %d；want %d", len(columns), len(before)+len(definitionColumns)+len(after))
	}
	if columns[0] != "log_time" || columns[len(columns)-1] != "details" {
		t.Errorf("日志列顺序不对：首=%q 尾=%q", columns[0], columns[len(columns)-1])
	}
	if len(columns) != 80 {
		t.Errorf("operation_log 共 80 列（2 + 75 + 3），得到 %d", len(columns))
	}
}

// conditionalColumn 返回 global_udi_input 里带 required_when 的那一列。
func conditionalColumn(t *testing.T, document map[string]any) map[string]any {
	t.Helper()
	source := nestedMapping(t, document, "sources", "global_udi_input")
	columns, _ := source["columns"].([]any)
	for _, rawColumn := range columns {
		column, _ := rawColumn.(map[string]any)
		if _, present := column["required_when"]; present {
			return column
		}
	}
	t.Fatal("global_udi_input 里应当有声明 required_when 的列")
	return nil
}
