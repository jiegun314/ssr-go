// Package changedetect 由「与历史记录的差异」派生变更描述（R17/R18）。
//
// 变更描述不来自任何来源表：它是整合结果与**同一身份最新一条已导出记录**之间的差异。
// 重复判定与变更描述共用同一次比较，所以一行不可能同时是「没变」和「变了」。
package changedetect

import (
	"strings"

	"github.com/jiegun314/ssr-go/internal/store"
)

// 一行相对历史记录的分类。
const (
	NewRecord       = "new"
	ModifiedRecord  = "modified"
	UnchangedRecord = "unchanged"
)

// MissingChangeDescriptionError：变更行需要描述，但来源没有填写（mode: source）。
type MissingChangeDescriptionError struct {
	MaterialCodes []string
}

func (errorValue *MissingChangeDescriptionError) Error() string {
	return "变更描述缺失：以下记录发生了变更，但来源没有填写变更描述：\n- " +
		strings.Join(errorValue.MaterialCodes, "\n- ")
}

// Result 是一行与历史记录比较的结论。
type Result struct {
	Kind           string
	ChangedColumns []string
	Description    string
}

// IsDuplicate 表示这一行没有差异，已经按原样导出过。
func (result Result) IsDuplicate() bool { return result.Kind == UnchangedRecord }

// Rule 是变更描述规则（ConfigLoader.load_change_description_rule 的 Go 版）。
type Rule struct {
	Field          string
	Fields         []string
	Labels         map[string]string
	IdentityFields []string
	CompareFields  any
	IgnoreFields   []string
	OnNewRecord    string
	OnNoChange     string
	OnOtherChange  map[string]any

	// ActionField 是"记录动作"派生列（数据要求：只能是 Add 或 Modify，不可能为空）。
	// 身份命中历史记录 → ActionOnStoredRecord，否则 ActionOnNewRecord。
	ActionField          string
	ActionOnNewRecord    string
	ActionOnStoredRecord string
}

// Row 是一行整合结果：Key 是 result_identity，Values 是列 → 值。
type Row struct {
	Key    string
	Values store.Row
}

// Service 拿最新的历史记录比较整合结果。
//
// 没有配置变更描述列时，比较退化成一件事：身份命中就标 Duplicate（不重复导出）。
type Service struct {
	Rule           *Rule
	IdentityFields []string
	// Latest 是「身份 → 最新一条存储记录」。
	Latest map[string]store.Row
}

// NewService 用历史记录构造比较器。
func NewService(
	rule *Rule,
	identityFields []string,
	latest []store.Row,
) *Service {
	service := &Service{
		Rule:           rule,
		IdentityFields: identityFields,
		Latest:         map[string]store.Row{},
	}
	for _, row := range latest {
		service.Latest[service.identityKey(row)] = row
	}
	return service
}

// Field 返回接收变更描述的导出列，没有配置时返回空。
func (service *Service) Field() string {
	if service.Rule == nil {
		return ""
	}
	return service.Rule.Field
}

// ActionField 返回接收记录动作的导出列，没有配置时返回空。
func (service *Service) ActionField() string {
	if service.Rule == nil {
		return ""
	}
	return service.Rule.ActionField
}

// EmptyChangeDescriptions 返回"动作是 Modify 但变更描述为空"的**可导出行**键。
//
// 只看会被导出的行（status = Ready）：与历史记录完全一致的行已判为 Duplicate（不进导出），
// 缺失行也不进导出 —— 它们没有变更描述是正常的，不该被点出来。
// 于是正常情况下一行都不会命中；真命中说明有值得排查的情况，交给调用方记一条警告。
func (service *Service) EmptyChangeDescriptions(rows []Row) []string {
	if service.Rule == nil || service.Rule.Field == "" || service.Rule.ActionField == "" {
		return nil
	}
	offending := []string{}
	for _, row := range rows {
		if row.Values["status"] != "Ready" {
			continue // 重复 / 缺失 / 冲突：不进导出，没有描述是正常的
		}
		if row.Values[service.Rule.ActionField] != service.Rule.ActionOnStoredRecord {
			continue
		}
		if Normalize(row.Values[service.Rule.Field]) == "" {
			offending = append(offending, row.Key)
		}
	}
	return offending
}

// Apply 填好派生列与状态，返回重复与变更的结果键。
//
// Conflict 的行不参与"重复/变更"判定，但仍然要填好动作列：
// 结果表里会显示它们，Choose Action 不该是空的。
func (service *Service) Apply(rows []Row) (duplicateCodes []string, changedCodes []string, err error) {
	field := service.Field()
	duplicateCodes = []string{}
	changedCodes = []string{}
	actionField := service.ActionField()
	for _, row := range rows {
		_, stored := service.Latest[service.identityKey(row.Values)]
		if actionField != "" {
			// 数据要求：Add / Modify 二选一，永远不留空
			if stored {
				row.Values[actionField] = service.Rule.ActionOnStoredRecord
			} else {
				row.Values[actionField] = service.Rule.ActionOnNewRecord
			}
		}
		if row.Values["status"] == "Conflict" {
			continue
		}
		if field == "" {
			if stored {
				row.Values["status"] = "Duplicate"
				duplicateCodes = append(duplicateCodes, row.Key)
			}
			continue
		}
		result, err := service.Classify(row.Values)
		if err != nil {
			return nil, nil, err
		}
		row.Values[field] = result.Description
		switch {
		case result.IsDuplicate():
			row.Values["status"] = "Duplicate"
			duplicateCodes = append(duplicateCodes, row.Key)
		case result.Kind == ModifiedRecord:
			changedCodes = append(changedCodes, row.Key)
		}
	}
	return duplicateCodes, changedCodes, nil
}

// Classify 判断一行相对历史记录是什么。
func (service *Service) Classify(values store.Row) (Result, error) {
	if service.Rule == nil {
		return Result{Kind: NewRecord}, nil
	}
	snapshot, stored := service.Latest[service.identityKey(values)]
	if !stored {
		return Result{Kind: NewRecord, Description: service.Rule.OnNewRecord}, nil
	}
	changed := []string{}
	for _, column := range service.comparableColumns(snapshot) {
		if Normalize(values[column]) != Normalize(snapshot[column]) {
			changed = append(changed, column)
		}
	}
	if len(changed) == 0 {
		return Result{Kind: UnchangedRecord, Description: service.Rule.OnNoChange}, nil
	}
	description, err := service.Describe(values, changed)
	if err != nil {
		return Result{}, err
	}
	return Result{Kind: ModifiedRecord, ChangedColumns: changed, Description: description}, nil
}

// Describe 生成变更描述文本（auto 模板或来源文本）。
func (service *Service) Describe(values store.Row, changedColumns []string) (string, error) {
	onOtherChange := service.Rule.OnOtherChange
	mode := "auto"
	if configured, present := onOtherChange["mode"]; present {
		if text, ok := configured.(string); ok {
			mode = text
		}
	}
	if mode == "source" {
		return service.sourceDescription(values, onOtherChange)
	}
	template, _ := onOtherChange["template"].(string)
	separator := "; "
	if configured, present := onOtherChange["separator"]; present {
		if text, ok := configured.(string); ok {
			separator = text
		}
	}
	parts := make([]string, 0, len(changedColumns))
	for _, column := range changedColumns {
		label, known := service.Rule.Labels[column]
		if !known {
			label = column
		}
		parts = append(parts, renderTemplate(template, column, label))
	}
	return strings.Join(parts, separator), nil
}

// sourceDescription 沿用来源里用户填写的描述；没填时按 missing_action 处理。
func (service *Service) sourceDescription(values store.Row, onOtherChange map[string]any) (string, error) {
	description := Normalize(values[service.Rule.Field])
	if description != "" {
		return description, nil
	}
	missingAction := "error"
	if configured, present := onOtherChange["missing_action"]; present {
		if text, ok := configured.(string); ok {
			missingAction = text
		}
	}
	if missingAction == "empty" {
		return "", nil
	}
	return "", &MissingChangeDescriptionError{
		MaterialCodes: []string{values["material_code"]},
	}
}

// comparableColumns 是这一行与历史记录**可比**的列：
// 存储记录真的有的列（旧表少列时少比较，而不是报出无法证明的变化），去掉身份列、
// 派生列本身与 ignore_fields。
func (service *Service) comparableColumns(snapshot store.Row) []string {
	candidates := service.Rule.Fields
	if configured, isList := service.Rule.CompareFields.([]any); isList {
		candidates = make([]string, 0, len(configured))
		for _, entry := range configured {
			if text, ok := entry.(string); ok {
				candidates = append(candidates, text)
			}
		}
	}
	comparable := []string{}
	for _, column := range candidates {
		if _, stored := snapshot[column]; !stored {
			continue
		}
		if contains(service.IdentityFields, column) || column == service.Rule.Field {
			continue
		}
		// 动作列也是派生列：它由身份命中与否决定，不该与历史记录里的自己比较
		if service.Rule.ActionField != "" && column == service.Rule.ActionField {
			continue
		}
		if contains(service.Rule.IgnoreFields, column) {
			continue
		}
		comparable = append(comparable, column)
	}
	return comparable
}

func (service *Service) identityKey(values store.Row) string {
	parts := make([]string, 0, len(service.IdentityFields))
	for _, field := range service.IdentityFields {
		parts = append(parts, Normalize(values[field]))
	}
	return strings.Join(parts, "\x00")
}

// Normalize 是重复判定与变更比较共用的归一：去首尾空白，缺值读作空串。
func Normalize(value string) string { return strings.TrimSpace(value) }

// renderTemplate 替换 {column} 与 {chinese_name}。
func renderTemplate(template string, column string, label string) string {
	rendered := strings.ReplaceAll(template, "{column}", column)
	return strings.ReplaceAll(rendered, "{chinese_name}", label)
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
