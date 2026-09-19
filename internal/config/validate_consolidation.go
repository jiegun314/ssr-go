package config

import (
	"fmt"
	"regexp"
	"strings"
)

// ValidateConsolidationMapping 校验 consolidation_mapping.yaml（R24 / R12–R19）。
func (loader *Loader) ValidateConsolidationMapping(
	consolidation map[string]any,
	imports map[string]any,
	setting map[string]any,
) error {
	dataset, err := requireMapping(consolidation["target_dataset"], "consolidation.target_dataset")
	if err != nil {
		return err
	}
	sources, err := requireMapping(imports["sources"], "imports.sources")
	if err != nil {
		return err
	}
	mergeRules, err := requireMapping(dataset["merge_rules"],
		"consolidation.target_dataset.merge_rules")
	if err != nil {
		return err
	}
	baseSource, err := requireMapping(mergeRules["base_source"],
		"consolidation.target_dataset.merge_rules.base_source")
	if err != nil {
		return err
	}
	baseSourceName, _ := asString(baseSource["source"])
	if _, known := sources[baseSourceName]; !known {
		return errorf("Unknown base source: %s", baseSourceName)
	}
	if err := validateSourceFields(sources, baseSourceName, baseSource["key"], "base source key"); err != nil {
		return err
	}

	resultIdentity, err := requireMapping(mergeRules["result_identity"],
		"consolidation.target_dataset.merge_rules.result_identity")
	if err != nil {
		return err
	}
	identityFields, ok := stringListStrict(resultIdentity["fields"])
	if !ok || len(identityFields) == 0 {
		return errorf("Result identity fields must be a non-empty list")
	}
	availableIdentityFields := map[string]bool{}
	for _, source := range sources {
		sourceMapping, _ := source.(map[string]any)
		columns, _ := sourceMapping["columns"].([]any)
		for _, rawColumn := range columns {
			column, _ := rawColumn.(map[string]any)
			if field, ok := asString(column["db_field"]); ok {
				availableIdentityFields[field] = true
			}
		}
	}
	missingIdentity := []string{}
	for _, field := range identityFields {
		if !availableIdentityFields[field] {
			missingIdentity = append(missingIdentity, field)
		}
	}
	if len(missingIdentity) > 0 {
		return errorf("Unknown result identity field: %s", strings.Join(missingIdentity, ", "))
	}
	if _, ok := asString(resultIdentity["separator"]); !ok {
		return errorf("Result identity separator must be a string")
	}

	sourceRules, err := requireMapping(mergeRules["sources"],
		"consolidation.target_dataset.merge_rules.sources")
	if err != nil {
		return err
	}
	if len(sourceRules) != len(sources) {
		return errorf("Merge rule sources must exactly match import sources")
	}
	for name := range sourceRules {
		if _, known := sources[name]; !known {
			return errorf("Merge rule sources must exactly match import sources")
		}
	}
	allowedCardinalities := []string{"one_to_one", "one_to_many", "many_to_one"}
	for _, sourceName := range sortedKeys(sourceRules) {
		rule, err := requireMapping(sourceRules[sourceName],
			"consolidation.target_dataset.merge_rules.sources."+sourceName)
		if err != nil {
			return err
		}
		matchType, _ := asString(rule["match_type"])
		if matchType != "lookup" {
			return errorf("Unsupported match_type for %s: %s", sourceName, matchType)
		}
		cardinality, _ := asString(rule["cardinality"])
		if !contains(allowedCardinalities, cardinality) {
			return errorf("Unsupported cardinality for %s: %s", sourceName, cardinality)
		}
		match, err := requireMapping(rule["match"],
			"consolidation.target_dataset.merge_rules.sources."+sourceName+".match")
		if err != nil {
			return err
		}
		matchValues := make([]any, 0, len(match))
		for _, key := range sortedKeys(match) {
			matchValues = append(matchValues, match[key])
		}
		if err := validateSourceFields(sources, sourceName, matchValues,
			"merge match for "+sourceName); err != nil {
			return err
		}
		for _, fieldGroup := range []string{"expand_by", "row_identity"} {
			if value, present := rule[fieldGroup]; present {
				if err := validateSourceFields(sources, sourceName, value,
					fieldGroup+" for "+sourceName); err != nil {
					return err
				}
			}
		}
		if duplicatesValue, present := rule["duplicates"]; present {
			duplicates, err := requireMapping(duplicatesValue,
				"consolidation.target_dataset.merge_rules.sources."+sourceName+".duplicates")
			if err != nil {
				return err
			}
			if err := validateSourceFields(sources, sourceName, duplicates["compare_by"],
				"duplicate comparison for "+sourceName); err != nil {
				return err
			}
			for _, actionName := range []string{
				"identical_content", "different_content", "same_value", "different_value",
			} {
				if value, present := duplicates[actionName]; present {
					action, _ := asString(value)
					if action != "deduplicate" && action != "conflict" {
						return errorf("Unsupported duplicate action for %s.%s: %s",
							sourceName, actionName, action)
					}
				}
			}
			selected := "first"
			if value, present := duplicates["selected_record"]; present {
				selected, _ = asString(value)
			}
			if selected != "first" && selected != "last" {
				return errorf("Unsupported selected_record for %s: %s", sourceName, selected)
			}
		}
		if noMatchValue, present := rule["no_match"]; present {
			noMatch, err := requireMapping(noMatchValue,
				"consolidation.target_dataset.merge_rules.sources."+sourceName+".no_match")
			if err != nil {
				return err
			}
			action, _ := asString(noMatch["action"])
			if !contains(NoMatchActions, action) {
				return errorf("Unsupported no_match action for %s: %s", sourceName, action)
			}
			if action == "emit_incomplete" {
				if _, ok := asString(noMatch["value"]); !ok {
					return errorf("no_match.value is required for %s emit_incomplete", sourceName)
				}
			}
		}
	}

	outputFields, err := requireMapping(dataset["fields"], "consolidation.target_dataset.fields")
	if err != nil {
		return err
	}
	for _, outputName := range sortedKeys(outputFields) {
		definition, err := requireMapping(outputFields[outputName],
			"consolidation.target_dataset.fields."+outputName)
		if err != nil {
			return err
		}
		mandatoryStatus, _ := asString(definition["mandatory_status"])
		if !contains([]string{"required", "required_if_applicable", "user_defined", ""},
			mandatoryStatus) {
			return errorf("Unsupported mandatory_status for %s: %q", outputName, mandatoryStatus)
		}
		source, _ := definition["source"].(map[string]any)
		transform, _ := definition["transform"].(map[string]any)
		if err := validateTransform(outputName, transform); err != nil {
			return err
		}
		switch {
		case mandatoryStatus == "user_defined":
			key, _ := asString(source["key"])
			if key == "" {
				return errorf("user_defined source key is required: %s", outputName)
			}
			userDefinedData, _ := setting["user_defined_data"].(map[string]any)
			if _, known := userDefinedData[key]; !known {
				return errorf("Unknown user_defined_data key for %s: %s", outputName, key)
			}
		case len(source) > 0:
			sourceName, _ := asString(source["file"])
			sourceMapping, known := sources[sourceName]
			if !known {
				return errorf("Unknown output source: %s", sourceName)
			}
			available := map[string]bool{}
			sourceRecord, _ := sourceMapping.(map[string]any)
			columns, _ := sourceRecord["columns"].([]any)
			for _, rawColumn := range columns {
				column, _ := rawColumn.(map[string]any)
				if field, ok := asString(column["db_field"]); ok {
					available[field] = true
				}
			}
			sourceFields := []string{}
			if list, ok := source["field"].([]any); ok {
				sourceFields = stringList(list)
			} else if field, ok := asString(source["field"]); ok {
				sourceFields = []string{field}
			} else {
				sourceFields = []string{""}
			}
			missing := []string{}
			for _, field := range sourceFields {
				if !available[field] {
					missing = append(missing, field)
				}
			}
			if len(missing) > 0 {
				return errorf("Unknown output field for %s: %s", outputName, strings.Join(missing, ", "))
			}
		case mandatoryStatus == "required":
			_, hasLiteral := transform["value"]
			transformType, _ := asString(transform["type"])
			if !hasLiteral && transformType != ChangeDescriptionTransform {
				return errorf(
					"Literal transform value is required for source-less required field: %s",
					outputName)
			}
		}
	}
	if err := validateDuplicateCheck(dataset, outputFields); err != nil {
		return err
	}
	return validateChangeDescriptionTransform(dataset, outputFields)
}

// validateTransform 要求声明的 transform 类型是字段循环真正认识的那种。
//
// 未知类型以前会掉进 "direct" 分支，于是拼错一个词就写出错误的值而不是报配置错误。
func validateTransform(outputName string, transform map[string]any) error {
	rawType, present := transform["type"]
	if !present || rawType == nil {
		return nil
	}
	transformType, _ := asString(rawType)
	allowed := append([]string{}, TransformTypes...)
	allowed = append(allowed, ChangeDescriptionTransform)
	if !contains(allowed, transformType) {
		return errorf("Unsupported transform type for %s: %q", outputName, transformType)
	}
	return nil
}

// validateDuplicateCheck 校验身份字段（R19）。
func validateDuplicateCheck(dataset map[string]any, exportFields map[string]any) error {
	duplicateCheck, err := requireMapping(dataset["duplicate_check"],
		"consolidation.target_dataset.duplicate_check")
	if err != nil {
		return err
	}
	// 「是不是列表」与「元素是不是非空字符串」是两个不同的检查，错误文案也不同
	identityEntries, isList := duplicateCheck["identity_fields"].([]any)
	if !isList || len(identityEntries) == 0 {
		return errorf(
			"consolidation.target_dataset.duplicate_check.identity_fields must be a non-empty list")
	}
	identityFields := make([]string, 0, len(identityEntries))
	for _, entry := range identityEntries {
		field, ok := entry.(string)
		if !ok || field == "" {
			return errorf(
				"consolidation.target_dataset.duplicate_check.identity_fields must contain non-empty strings")
		}
		identityFields = append(identityFields, field)
	}
	seen := map[string]bool{}
	for _, field := range identityFields {
		if seen[field] {
			return errorf(
				"consolidation.target_dataset.duplicate_check.identity_fields must not contain duplicates")
		}
		seen[field] = true
	}
	missing := []string{}
	for _, field := range identityFields {
		if _, known := exportFields[field]; !known {
			missing = append(missing, field)
		}
	}
	if len(missing) > 0 {
		return errorf(
			"consolidation.target_dataset.duplicate_check.identity_fields must also be export columns in consolidation.target_dataset.fields: %s",
			strings.Join(missing, ", "))
	}
	return nil
}

// validateChangeDescriptionTransform 校验派生列自己的规则（R18）。
func validateChangeDescriptionTransform(dataset map[string]any, exportFields map[string]any) error {
	derived := []string{}
	for _, name := range sortedKeys(exportFields) {
		definition, _ := exportFields[name].(map[string]any)
		transform, _ := definition["transform"].(map[string]any)
		transformType, _ := asString(transform["type"])
		if transformType == ChangeDescriptionTransform {
			derived = append(derived, name)
		}
	}
	if len(derived) == 0 {
		return nil
	}
	if len(derived) > 1 {
		return errorf("Only one derived change description is supported: %s",
			strings.Join(derived, ", "))
	}
	fieldName := derived[0]
	definition, _ := exportFields[fieldName].(map[string]any)
	transform, _ := definition["transform"].(map[string]any)
	label := "consolidation.target_dataset.fields." + fieldName
	transformLabel := label + ".transform"

	duplicateCheck, _ := dataset["duplicate_check"].(map[string]any)
	identityFields := stringList(duplicateCheck["identity_fields"])
	if contains(identityFields, fieldName) {
		return errorf(
			"%s is derived from the change comparison, so it can not be part of consolidation.target_dataset.duplicate_check.identity_fields",
			label)
	}
	if mandatoryStatus, _ := asString(definition["mandatory_status"]); mandatoryStatus == "user_defined" {
		return errorf("%s is derived, so mandatory_status can not be 'user_defined'", label)
	}
	identityReference := ChangeIdentityReference
	if value, present := transform["identity_fields"]; present {
		identityReference, _ = asString(value)
	}
	if identityReference != ChangeIdentityReference {
		return errorf("%s.identity_fields must be '%s'",
			transformLabel, ChangeIdentityReference)
	}
	compareFields := any("all")
	if value, present := transform["compare_fields"]; present {
		compareFields = value
	}
	if text, ok := compareFields.(string); !ok || text != "all" {
		if err := requireExportFieldNames(transformLabel+".compare_fields", compareFields, exportFields, false); err != nil {
			return err
		}
	}
	if err := requireExportFieldNames(transformLabel+".ignore_fields",
		transform["ignore_fields"], exportFields, true); err != nil {
		return err
	}
	for _, key := range []string{"on_new_record", "on_no_change"} {
		value, present := transform[key]
		if !present || value == nil {
			continue
		}
		if _, ok := asString(value); !ok {
			return errorf("%s.%s must be a string", transformLabel, key)
		}
	}
	onOtherChange, err := requireMapping(transform["on_other_change"],
		transformLabel+".on_other_change")
	if err != nil {
		return err
	}
	mode := "auto"
	if value, present := onOtherChange["mode"]; present {
		mode, _ = asString(value)
	}
	if !contains(ChangeDescriptionModes, mode) {
		return errorf("%s.on_other_change.mode must be one of %s: %q",
			transformLabel, strings.Join(ChangeDescriptionModes, ", "), mode)
	}
	source, _ := definition["source"].(map[string]any)
	if mode == "auto" {
		if len(source) > 0 {
			return errorf(
				"%s.on_other_change.mode is 'auto', so the source of %s has to stay empty; use mode 'source' to keep the text of a source field",
				transformLabel, label)
		}
		return validateChangeDescriptionTemplate(transformLabel, onOtherChange)
	}
	if len(source) == 0 {
		return errorf(
			"%s.on_other_change.mode is 'source', so %s has to declare the file and field that hold the text",
			transformLabel, label)
	}
	if mandatoryStatus, _ := asString(definition["mandatory_status"]); mandatoryStatus == "" {
		return errorf(
			"%s: mandatory_status '' ignores the declared source, so the text of the source field would never be read",
			label)
	}
	missingAction := "error"
	if value, present := onOtherChange["missing_action"]; present {
		missingAction, _ = asString(value)
	}
	if !contains(MissingDescriptionActions, missingAction) {
		return errorf("%s.on_other_change.missing_action must be one of %s: %q",
			transformLabel, strings.Join(MissingDescriptionActions, ", "), missingAction)
	}
	return nil
}

var templatePlaceholderPattern = regexp.MustCompile(`\{(\w+)\}`)

// validateChangeDescriptionTemplate 要求 auto 模板能说出「哪一列变了」。
func validateChangeDescriptionTemplate(
	transformLabel string,
	onOtherChange map[string]any,
) error {
	label := transformLabel + ".on_other_change.template"
	template, _ := asString(onOtherChange["template"])
	if template == "" {
		return errorf("%s is required to describe a changed column", label)
	}
	if err := checkTemplateBraces(template); err != nil {
		return errorf("%s is invalid: %v", label, err)
	}
	used := map[string]bool{}
	for _, match := range templatePlaceholderPattern.FindAllStringSubmatch(template, -1) {
		used[match[1]] = true
	}
	hasPlaceholder := false
	for _, name := range ChangeDescriptionPlaceholders {
		if used[name] {
			hasPlaceholder = true
		}
	}
	if !hasPlaceholder {
		placeholders := make([]string, 0, len(ChangeDescriptionPlaceholders))
		for _, name := range ChangeDescriptionPlaceholders {
			placeholders = append(placeholders, "{"+name+"}")
		}
		return errorf("%s has to use one of %s", label, strings.Join(placeholders, ", "))
	}
	if separator, present := onOtherChange["separator"]; present && separator != nil {
		if _, ok := asString(separator); !ok {
			return errorf("%s.on_other_change.separator must be a string", transformLabel)
		}
	}
	return nil
}

// checkTemplateBraces 拒绝像 "{column" 这样的模板（Python 版的 str.format 会在这里报错）。
func checkTemplateBraces(template string) error {
	depth := 0
	for index, character := range template {
		switch character {
		case '{':
			depth++
		case '}':
			depth--
			if depth < 0 {
				return fmt.Errorf("unexpected '}' at position %d", index)
			}
		}
	}
	if depth != 0 {
		return fmt.Errorf("unbalanced '{' in %q", template)
	}
	return nil
}

func requireExportFieldNames(
	label string,
	names any,
	exportFields map[string]any,
	allowEmpty bool,
) error {
	// Python 版写的是 `transform.get("ignore_fields") or []`：键缺省等于空列表，
	// 而空列表在 allow_empty 时是合法的。
	if names == nil {
		names = []any{}
	}
	list, ok := stringListStrict(names)
	if !ok || (len(list) == 0 && !allowEmpty) {
		return errorf("%s must be a non-empty list", label)
	}
	missing := []string{}
	for _, name := range list {
		if _, known := exportFields[name]; !known {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return errorf("%s is not an export column: %s", label, strings.Join(missing, ", "))
	}
	return nil
}

func validateSourceFields(
	sources map[string]any,
	sourceName string,
	fields any,
	description string,
) error {
	entries, isList := fields.([]any)
	if !isList || len(entries) == 0 {
		return errorf("%s must be a non-empty list", description)
	}
	sourceMapping, _ := sources[sourceName].(map[string]any)
	columns, _ := sourceMapping["columns"].([]any)
	available := map[string]bool{}
	for _, rawColumn := range columns {
		column, _ := rawColumn.(map[string]any)
		if field, ok := asString(column["db_field"]); ok {
			available[field] = true
		}
	}
	missing := []string{}
	for _, entry := range entries {
		name := fmt.Sprint(entry)
		if !available[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return errorf("Unknown field in %s: %s", description, strings.Join(missing, ", "))
	}
	return nil
}
