package importer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jiegun314/ssr-go/internal/numfmt"
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

// TypeCount 是一种违规类型的汇总：类型说明 + 命中它的行数（同一行只算一次）。
type TypeCount struct {
	Description string
	Rows        int
}

// Error 只给「错误类型 + 该类型的行数」，不再逐行展开明细。
//
// 报错行数可能上千：逐行明细会让日志与悬浮提示（原生 title）又长又慢，
// 前端渲染几百 KB 的文本节点时 hover 会卡住甚至出不来。具体到哪些行，
// 前端本来就能按日志级别筛选、按结果表筛选查看，这里只留可读的汇总。
func (errorValue *MissingRowValuesError) Error() string {
	lines := []string{errorValue.countLine()}
	for _, count := range errorValue.TypeCounts() {
		lines = append(lines, fmt.Sprintf("- %s：%s 行", count.Description, numfmt.Format(count.Rows)))
	}
	lines = append(lines, errorValue.Hint)
	return fmt.Sprintf("%s：%s：\n%s",
		errorValue.SourceChineseName,
		errorValue.Heading,
		strings.Join(lines, "\n"),
	)
}

// TypeCounts 按「列」归类违规：同一列的行数合在一起，行数多的排前面。
//
// 条件必填也只按列归类，不把每行各不相同的条件取值带进类型说明 ——
// 否则类型数会跟行数一个量级，又变成"逐行明细"了。
func (errorValue *MissingRowValuesError) TypeCounts() []TypeCount {
	type bucket struct {
		description string
		rows        map[int]bool
	}
	order := make([]string, 0, len(errorValue.Violations))
	buckets := make(map[string]*bucket, len(errorValue.Violations))
	for _, violation := range errorValue.Violations {
		key := violation.DBField
		current, ok := buckets[key]
		if !ok {
			current = &bucket{
				description: fmt.Sprintf("%s（%s）为空", violation.ChineseName, violation.DBField),
				rows:        map[int]bool{},
			}
			buckets[key] = current
			order = append(order, key)
		}
		current.rows[violation.RowNumber] = true
	}
	counts := make([]TypeCount, 0, len(order))
	for _, key := range order {
		current := buckets[key]
		counts = append(counts, TypeCount{Description: current.description, Rows: len(current.rows)})
	}
	sort.SliceStable(counts, func(i, j int) bool { return counts[i].Rows > counts[j].Rows })
	return counts
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
	return errorValue.SourceChineseName + "：" + errorValue.countLine()
}

// countLine 是弹窗与日志共用的计数行（不带来源名），数字一律带千分位。
func (errorValue *MissingRowValuesError) countLine() string {
	return fmt.Sprintf("有 %s 行数据导入失败（%s 个单元格为空）。",
		numfmt.Format(len(errorValue.FailedRows())), numfmt.Format(len(errorValue.Violations)))
}
