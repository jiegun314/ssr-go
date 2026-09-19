package excelio

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

// NormalizeHeaderTitle 把模板里的表头单元格变成配置使用的列名。
//
// 模板里的列名可能跨两行写（换行）或带尾随空格，而配置写在一行里；把连续空白折叠成
// 单个空格，两种写法就指向同一列（R21 第 2 条，§12.4）。
func NormalizeHeaderTitle(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

// BuildExportFileName 是导出文件的名字：模板名 + 导出时刻（R20）。
func BuildExportFileName(templateFile string, now time.Time) string {
	extension := filepath.Ext(templateFile)
	stem := strings.TrimSuffix(filepath.Base(templateFile), extension)
	return stem + "-" + now.Format("20060102_150405") + extension
}

// Template 是导出模板的位置与形状。
type Template struct {
	Path         string
	SheetName    string
	HeaderRow    int
	DataStartRow int
	// ExportFolder 是导出目录：每次导出都会在这里留下带时间戳的副本（R20）。
	ExportFolder string
}

// ExportResult 是一次导出的结果。
type ExportResult struct {
	FileName       string
	FilePath       string
	WrittenRows    int
	MissingHeaders []string
}

// Export 把 Ready 行的值写进模板的副本。
//
// 复刻 services/excel_service.py::export_to_singlesource_template：
// 先**复制模板**（不是新建工作簿，模板里的说明行、格式、图片、敏感度标签、打印设置都要
// 留着），再按表头行建「列名 → 列号」映射，最后按**文本**写值（前导零不能丢）。
// 配置里有、模板表头里没有的列会被跳过并记进 MissingHeaders（R21 第 5 条）。
func Export(
	template Template,
	rows []map[string]string,
	fileName string,
	now time.Time,
) (ExportResult, error) {
	if fileName == "" {
		fileName = BuildExportFileName(template.Path, now)
	}
	if err := os.MkdirAll(template.ExportFolder, 0o755); err != nil {
		return ExportResult{}, fmt.Errorf("Failed to export consolidation result: %w", err)
	}
	outputPath := filepath.Join(template.ExportFolder, fileName)
	if err := copyFile(template.Path, outputPath); err != nil {
		return ExportResult{}, fmt.Errorf("Failed to export consolidation result: %w", err)
	}
	workbook, err := excelize.OpenFile(outputPath)
	if err != nil {
		return ExportResult{}, fmt.Errorf("Failed to export consolidation result: %w", err)
	}
	defer workbook.Close()
	if index, err := workbook.GetSheetIndex(template.SheetName); err != nil || index < 0 {
		return ExportResult{}, fmt.Errorf(
			"Failed to export consolidation result: worksheet %s does not exist",
			template.SheetName)
	}
	headerMap, err := headerColumns(workbook, template.SheetName, template.HeaderRow)
	if err != nil {
		return ExportResult{}, err
	}
	textStyle, err := workbook.NewStyle(&excelize.Style{CustomNumFmt: stringPointer("@")})
	if err != nil {
		return ExportResult{}, fmt.Errorf("Failed to export consolidation result: %w", err)
	}
	missing := map[string]bool{}
	for offset, record := range rows {
		rowNumber := template.DataStartRow + offset
		for field, value := range record {
			column, known := headerMap[field]
			if !known {
				missing[field] = true
				continue
			}
			cell, err := excelize.CoordinatesToCellName(column, rowNumber)
			if err != nil {
				return ExportResult{}, fmt.Errorf("Failed to export consolidation result: %w", err)
			}
			// 按文本写：SetCellStr 不做类型推断，数字串不会变成数字（前导零不丢）
			if err := workbook.SetCellStr(template.SheetName, cell, value); err != nil {
				return ExportResult{}, fmt.Errorf("Failed to export consolidation result: %w", err)
			}
			if err := workbook.SetCellStyle(template.SheetName, cell, cell, textStyle); err != nil {
				return ExportResult{}, fmt.Errorf("Failed to export consolidation result: %w", err)
			}
		}
	}
	if err := workbook.Save(); err != nil {
		return ExportResult{}, fmt.Errorf("Failed to export consolidation result: %w", err)
	}
	missingHeaders := make([]string, 0, len(missing))
	for field := range missing {
		missingHeaders = append(missingHeaders, field)
	}
	sort.Strings(missingHeaders)
	return ExportResult{
		FileName:       fileName,
		FilePath:       outputPath,
		WrittenRows:    len(rows),
		MissingHeaders: missingHeaders,
	}, nil
}

// headerColumns 读表头行，返回「折叠空白后的列名 → 列号」。空标题跳过。
func headerColumns(workbook *excelize.File, sheetName string, headerRow int) (map[string]int, error) {
	rows, err := workbook.GetRows(sheetName)
	if err != nil {
		return nil, fmt.Errorf("Failed to export consolidation result: %w", err)
	}
	columns := map[string]int{}
	if headerRow-1 >= len(rows) {
		return columns, nil
	}
	for position, title := range rows[headerRow-1] {
		normalized := NormalizeHeaderTitle(title)
		if normalized == "" {
			continue
		}
		if _, taken := columns[normalized]; !taken {
			columns[normalized] = position + 1
		}
	}
	return columns, nil
}

func copyFile(source string, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.Create(target)
	if err != nil {
		return err
	}
	defer output.Close()
	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	return output.Sync()
}

func stringPointer(value string) *string { return &value }
