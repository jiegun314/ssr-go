package importer

import (
	"fmt"
	"sort"
	"strings"
)

// Violation 是一个「单元格为空」的违规：哪一行、哪一列，条件必填还带命中的条件。
type Violation struct {
	RowNumber   int
	ChineseName string
	DBField     string
	Condition   string
}

// MissingRequiredColumnsError：表头缺列（必填列，或必须出现才能判断条件的条件必填列）。
type MissingRequiredColumnsError struct {
	SourceChineseName string
	MissingColumns    []Column
	Heading           string
	Hint              string
}

// NewMissingRequiredColumnsError 是「缺必填列」的默认口径。
func NewMissingRequiredColumnsError(sourceChineseName string, missing []Column) *MissingRequiredColumnsError {
	return &MissingRequiredColumnsError{
		SourceChineseName: sourceChineseName,
		MissingColumns:    missing,
		Heading:           "导入文件缺少必填列",
		Hint:              "请检查 Excel 表头是否与导入配置一致。",
	}
}

// NewMissingConditionalColumnsError 是「缺条件必填列」的口径：值可以为空，但列必须在。
func NewMissingConditionalColumnsError(sourceChineseName string, missing []Column) *MissingRequiredColumnsError {
	return &MissingRequiredColumnsError{
		SourceChineseName: sourceChineseName,
		MissingColumns:    missing,
		Heading:           "导入文件缺少条件必填列",
		Hint: "条件必填列必须出现在 Excel 表头中（单元格值可以为空），" +
			"否则无法判断 required_when 条件是否成立。",
	}
}

func (errorValue *MissingRequiredColumnsError) Error() string {
	lines := make([]string, 0, len(errorValue.MissingColumns))
	for _, column := range errorValue.MissingColumns {
		lines = append(lines, fmt.Sprintf("- %s (%s)", column.ChineseName, column.DBField))
	}
	return fmt.Sprintf("%s：%s：\n%s\n%s",
		errorValue.SourceChineseName,
		errorValue.Heading,
		strings.Join(lines, "\n"),
		errorValue.Hint,
	)
}

// MissingRowValuesError 是「数据行缺值」的拒绝：弹窗只给汇总，逐行明细进日志。
//
// `violations` 每个单元格一条；`Summary` 是弹窗那一行，`Error()` 是写进日志的明细。
type MissingRowValuesError struct {
	SourceChineseName string
	Violations        []Violation
	Heading           string
	Hint              string
	conditional       bool
}

// NewMissingRequiredValuesError：必填列在数据行上为空。
func NewMissingRequiredValuesError(sourceChineseName string, violations []Violation) *MissingRowValuesError {
	return &MissingRowValuesError{
		SourceChineseName: sourceChineseName,
		Violations:        violations,
		Heading:           "导入文件存在必填列为空",
		Hint:              "请补齐这些单元格后重新导入，本次文件未被读取。",
	}
}

// NewMissingConditionalValuesError：条件成立而条件必填列为空。
func NewMissingConditionalValuesError(sourceChineseName string, violations []Violation) *MissingRowValuesError {
	return &MissingRowValuesError{
		SourceChineseName: sourceChineseName,
		Violations:        violations,
		Heading:           "导入文件存在条件必填列为空",
		Hint:              "请补充这些列的值，或修正对应的条件列。",
		conditional:       true,
	}
}

func (errorValue *MissingRowValuesError) Error() string {
	lines := make([]string, 0, len(errorValue.Violations))
	for _, violation := range errorValue.Violations {
		lines = append(lines, errorValue.describe(violation))
	}
	return fmt.Sprintf("%s：%s：\n%s\n%s",
		errorValue.SourceChineseName,
		errorValue.Heading,
		strings.Join(lines, "\n"),
		errorValue.Hint,
	)
}

// describe 是一条明细行：条件必填会带上命中的条件。
func (errorValue *MissingRowValuesError) describe(violation Violation) string {
	base := fmt.Sprintf("- 第 %d 行：%s（%s）为空",
		violation.RowNumber, violation.ChineseName, violation.DBField)
	if errorValue.conditional {
		return base + "，因为 " + violation.Condition
	}
	return base
}

// FailedRows 是至少有一个空单元格的数据行号，按文件顺序去重（一行多个空格子只算一行）。
func (errorValue *MissingRowValuesError) FailedRows() []int {
	seen := map[int]bool{}
	rows := []int{}
	for _, violation := range errorValue.Violations {
		if seen[violation.RowNumber] {
			continue
		}
		seen[violation.RowNumber] = true
		rows = append(rows, violation.RowNumber)
	}
	sort.Ints(rows)
	return rows
}

// Summary 是弹窗里的那一行：只说有几行失败、几个单元格为空。
func (errorValue *MissingRowValuesError) Summary() string {
	return fmt.Sprintf("%s：有 %d 行数据导入失败（%d 个单元格为空）。",
		errorValue.SourceChineseName, len(errorValue.FailedRows()), len(errorValue.Violations))
}
