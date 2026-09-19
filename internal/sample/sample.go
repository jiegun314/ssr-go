// Package sample 生成手工走流程用的样本工作簿（对应 scripts/build_sample_data.py +
// tests/sample_data.py）。
//
// 工厂是**配置驱动**的：表名、中文表头行、英文表头行、必填标记行、数据起始行与列清单
// 全部来自 excel_import_mapping.yaml，所以改映射不需要改这里。四个来源共用的值来自
// profile()，一份样本可以完整跑通 导入 → 整合 → 导出。
package sample

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/xuri/excelize/v2"

	"github.com/jiegun314/ssr-go/internal/config"
)

// DefaultRows 与 Python 版一致：5 个产品代码。
const DefaultRows = 5

// DI 前缀让不同来源、不同角色的标识互不相同。
const (
	diPrefixPrimary   = "0691234"
	diPrefixSecondary = "0691239"
	diPrefixUseUnit   = "0691235"
)

// baseRecord 是四个来源必须一致的值（产品代码、注册证号、医保编码、DI、数量）。
var baseRecord = map[string]any{
	"material_code":                          "000MAT-001",
	"registration_filing_certificate_number": "000REG-001",
	"medical_insurance_code":                 "000INS-001",
	"serial_number":                          "000SER-001",
	"device_identifier_min_sales_unit":       "06912345678901",
	"device_identifier_use_unit":             "06912345678902",
	"quantity_per_min_sales_unit":            "1",
}

// columnValues 是只有一个列会用的值。
var columnValues = map[string]any{
	"udi_issuing_agency_name":              "GS1",
	"product_name":                         "Test device",
	"device_generic_name":                  "Test device",
	"product_description":                  "Test description",
	"product_type":                         "Consumable",
	"if_one_time_use":                      "Yes",
	"if_sterilized":                        "Yes",
	"if_requires_sterilization_before_use": "Yes",
	"is_identifier_consistent_with_registered_product": "Yes",
	"if_consistent_with_registration_di":               "Yes",
	"if_direct_marking":                                "No",
	"if_dm_identical_to_min_sales_unit_di":             "No",
	"data_maintenance_type":                            "New",
	"change_description":                               "Initial test record",
}

// typeValues 是按声明类型兜底的值。
var typeValues = map[string]any{
	"number":  1,
	"integer": 1,
	"date":    "2026/01/01",
}

// Overrides 是一行的覆盖项：键可以是 db_field，也可以是中文列名。
type Overrides map[string]any

// Profile 是四个来源共用的一条业务记录（键 = db_field）。
func Profile(index int) Overrides {
	return Overrides{
		"material_code":                          fmt.Sprintf("000MAT-%03d", index),
		"registration_filing_certificate_number": fmt.Sprintf("000REG-%03d", index),
		"medical_insurance_code":                 fmt.Sprintf("000INS-%03d", index),
		"serial_number":                          fmt.Sprintf("000SER-%03d", index),
		"device_identifier_min_sales_unit":       fmt.Sprintf("%s%06d", diPrefixPrimary, index),
		"device_identifier_use_unit":             fmt.Sprintf("%s%06d", diPrefixUseUnit, index),
		"quantity_per_min_sales_unit":            "1",
	}
}

// WriteSampleSets 写出 valid/ 与 invalid-conditions/ 两套样本。
//
// 返回值是 invalid-conditions 的 UDI 文件路径与第一条被拒行的行号。
func WriteSampleSets(
	outputRoot string,
	loader *config.Loader,
	count int,
) (string, int, error) {
	document, err := loader.LoadImportMappingDocument()
	if err != nil {
		return "", 0, err
	}
	sources, _ := document.Value["sources"].(map[string]any)
	order := document.Keys("sources")

	rowsBySource := sourceRows(sources, count)
	udiRows := globalUdiRows(count)
	layout, _ := sources["global_udi_input"].(map[string]any)
	excel, _ := layout["excel"].(map[string]any)
	dataStartRow := intValue(excel["data_start_row"])
	edgeCases, err := edgeCaseRows(sources, udiRows, dataStartRow)
	if err != nil {
		return "", 0, err
	}
	rowsBySource["global_udi_input"] = append(append([]Overrides{}, udiRows...), edgeCases...)
	if err := writeSourceSet(filepath.Join(outputRoot, "valid"), sources, order, rowsBySource); err != nil {
		return "", 0, err
	}

	firstOffendingRow := dataStartRow + len(udiRows) + len(edgeCases)
	rowsBySource["global_udi_input"] = append(
		rowsBySource["global_udi_input"], offendingRows(count)...)
	if err := writeSourceSet(
		filepath.Join(outputRoot, "invalid-conditions"), sources, order, rowsBySource); err != nil {
		return "", 0, err
	}
	return filepath.Join(outputRoot, "invalid-conditions", "global_udi_input.xlsx"),
		firstOffendingRow, nil
}

// sourceRows 给 RA / 医保 / 产品类别 各写 count+1 行（多出一个只有这三个来源才有的产品代码）。
func sourceRows(sources map[string]any, count int) map[string][]Overrides {
	profiles := []Overrides{}
	for index := 1; index <= count; index++ {
		profiles = append(profiles, Profile(index))
	}
	raOnly := Profile(count + 1)
	rows := map[string][]Overrides{}
	for _, sourceName := range []string{"ra_input", "medical_insurance_code", "product_category"} {
		if _, present := sources[sourceName]; !present {
			continue
		}
		list := []Overrides{}
		for _, profile := range profiles {
			list = append(list, copyOverrides(profile))
		}
		list = append(list, copyOverrides(raOnly))
		rows[sourceName] = list
	}
	return rows
}

// globalUdiRows 按行覆盖数量，覆盖条件必填规则的四种情况，并在偶数行追加第二个 DI。
func globalUdiRows(count int) []Overrides {
	rows := []Overrides{}
	for index := 1; index <= count; index++ {
		record := Profile(index)
		row := copyOverrides(record)
		switch index % 4 {
		case 1:
			row["quantity_per_min_sales_unit"] = "1"
			row["device_identifier_use_unit"] = ""
		case 2:
			row["quantity_per_min_sales_unit"] = "3"
		case 3:
			row["quantity_per_min_sales_unit"] = " 2 "
		default:
			row["quantity_per_min_sales_unit"] = "多套"
			row["device_identifier_use_unit"] = ""
		}
		rows = append(rows, row)
		if index%2 == 0 {
			second := copyOverrides(record)
			second["device_identifier_min_sales_unit"] =
				fmt.Sprintf("%s%06d", diPrefixSecondary, index)
			rows = append(rows, second)
		}
	}
	return rows
}

// edgeCaseRows 是真实工作簿里常见的脏数据：完全重复的一行（整合时去重），
// 以及 DI 写成数字的一行（丢掉前导零，整合后多出一行结果）。
func edgeCaseRows(
	sources map[string]any,
	udiRows []Overrides,
	firstRowIndex int,
) ([]Overrides, error) {
	source, _ := sources["global_udi_input"].(map[string]any)
	resolved, err := buildRow("global_udi_input", source, firstRowIndex, udiRows[0])
	if err != nil {
		return nil, err
	}
	duplicate := copyOverrides(resolved)
	numeric := copyOverrides(resolved)
	diColumn := columnChineseName(source, "device_identifier_min_sales_unit")
	value := resolved[diColumn]
	number, err := strconv.Atoi(fmt.Sprint(value))
	if err != nil {
		return nil, fmt.Errorf("DI %v 不是数字，无法生成脏数据行", value)
	}
	numeric[diColumn] = number
	return []Overrides{duplicate, numeric}, nil
}

// offendingRows 是条件成立（数量 > 1）但使用单元产品标识为空的两行。
func offendingRows(count int) []Overrides {
	first := copyOverrides(Profile(1))
	first["quantity_per_min_sales_unit"] = "2"
	first["device_identifier_use_unit"] = ""
	second := copyOverrides(Profile(count + 1))
	second["quantity_per_min_sales_unit"] = "2.5"
	second["device_identifier_use_unit"] = "   "
	return []Overrides{first, second}
}

// BuildRows 把覆盖项解析成完整的「中文列名 → 值」。
func buildRows(sourceName string, source map[string]any, rows []Overrides) ([]map[string]any, error) {
	excel, _ := source["excel"].(map[string]any)
	dataStartRow := intValue(excel["data_start_row"])
	built := make([]map[string]any, 0, len(rows))
	for offset, overrides := range rows {
		row, err := buildRow(sourceName, source, dataStartRow+offset, overrides)
		if err != nil {
			return nil, err
		}
		built = append(built, row)
	}
	return built, nil
}

// buildRow 返回一行的「中文列名 → 值」：覆盖项优先，其余用默认值。
func buildRow(
	sourceName string,
	source map[string]any,
	rowIndex int,
	overrides Overrides,
) (map[string]any, error) {
	row := map[string]any{}
	columns, _ := source["columns"].([]any)
	for _, rawColumn := range columns {
		column, _ := rawColumn.(map[string]any)
		chineseName := textValue(column["chinese_name"])
		dbField := textValue(column["db_field"])
		if value, present := overrides[dbField]; present {
			row[chineseName] = value
			continue
		}
		if value, present := overrides[chineseName]; present {
			row[chineseName] = value
			continue
		}
		row[chineseName] = defaultValue(column, sourceName, rowIndex)
	}
	return row, nil
}

// defaultValue 是 Python 版 default_value 的等价物。
func defaultValue(column map[string]any, sourceName string, rowIndex int) any {
	dbField := textValue(column["db_field"])
	if value, present := baseRecord[dbField]; present {
		return value
	}
	if value, present := columnValues[dbField]; present {
		return value
	}
	// 映射会把某些写法变成别的值，所以这些写法不能当样本数据；第一个没被映射的
	// allowed_value 才是安全的默认值。
	mappedAway := map[string]bool{}
	valueMapping, _ := column["value_mapping"].(map[string]any)
	for key := range valueMapping {
		mappedAway[key] = true
	}
	allowedValues, _ := column["allowed_values"].([]any)
	for _, allowed := range allowedValues {
		if text, ok := allowed.(string); ok && !mappedAway[text] {
			return text
		}
	}
	dataType := textValue(column["data_type"])
	if value, present := typeValues[dataType]; present {
		return value
	}
	return fmt.Sprintf("%s-%s-%d", sourceName, dbField, rowIndex)
}

// writeSourceSet 写一套工作簿（每个来源一份）。
func writeSourceSet(
	directory string,
	sources map[string]any,
	order []string,
	rowsBySource map[string][]Overrides,
) error {
	for _, sourceName := range order {
		source, _ := sources[sourceName].(map[string]any)
		if source == nil {
			continue
		}
		parts := []string{}
		parts = append(parts, sourceName)
		rows := rowsBySource[sourceName]
		if rows == nil {
			rows = []Overrides{{}}
		}
		path := filepath.Join(directory, sourceName+".xlsx")
		if err := writeSourceWorkbook(path, sourceName, source, rows); err != nil {
			return fmt.Errorf("写 %s 失败：%w", parts[0], err)
		}
	}
	return nil
}

// writeSourceWorkbook 写一份来源工作簿：中文表头、英文表头、必填标记行与数据行。
func writeSourceWorkbook(
	path string,
	sourceName string,
	source map[string]any,
	rows []Overrides,
) error {
	excel, _ := source["excel"].(map[string]any)
	headerRow := intValue(excel["chinese_header_row"])
	dataStartRow := intValue(excel["data_start_row"])
	englishHeaderRow, hasEnglish := optionalInt(excel["english_header_row"])
	requiredRow, hasRequired := optionalInt(excel["required_row"])
	sheetName := textValue(excel["sheet_name"])

	workbook := excelize.NewFile()
	defer workbook.Close()
	index, err := workbook.NewSheet(sheetName)
	if err != nil {
		return err
	}
	workbook.SetActiveSheet(index)
	defaultSheet := workbook.GetSheetName(0)
	if defaultSheet != sheetName {
		if err := workbook.DeleteSheet(defaultSheet); err != nil {
			return err
		}
	}
	dataRows, err := buildRows(sourceName, source, rows)
	if err != nil {
		return err
	}
	columns, _ := source["columns"].([]any)
	for columnIndex, rawColumn := range columns {
		column, _ := rawColumn.(map[string]any)
		number := columnIndex + 1
		cell, err := excelize.CoordinatesToCellName(number, headerRow)
		if err != nil {
			return err
		}
		if err := workbook.SetCellValue(sheetName, cell, textValue(column["chinese_name"])); err != nil {
			return err
		}
		if hasEnglish {
			if englishName := textValue(column["english_name"]); englishName != "" {
				englishCell, err := excelize.CoordinatesToCellName(number, englishHeaderRow)
				if err != nil {
					return err
				}
				if err := workbook.SetCellValue(sheetName, englishCell, englishName); err != nil {
					return err
				}
			}
		}
		if hasRequired {
			if status := textValue(column["mandatory_status"]); status != "" {
				requiredCell, err := excelize.CoordinatesToCellName(number, requiredRow)
				if err != nil {
					return err
				}
				if err := workbook.SetCellValue(sheetName, requiredCell, status); err != nil {
					return err
				}
			}
		}
		for offset, row := range dataRows {
			cell, err := excelize.CoordinatesToCellName(number, dataStartRow+offset)
			if err != nil {
				return err
			}
			if err := workbook.SetCellValue(
				sheetName, cell, row[textValue(column["chinese_name"])]); err != nil {
				return err
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return workbook.SaveAs(path)
}

// columnChineseName 返回某个 db_field 对应的中文表头。
func columnChineseName(source map[string]any, dbField string) string {
	columns, _ := source["columns"].([]any)
	for _, rawColumn := range columns {
		column, _ := rawColumn.(map[string]any)
		if textValue(column["db_field"]) == dbField {
			return textValue(column["chinese_name"])
		}
	}
	return dbField
}

func copyOverrides(source Overrides) Overrides {
	copied := Overrides{}
	for key, value := range source {
		copied[key] = value
	}
	return copied
}

func textValue(value any) string {
	text, _ := value.(string)
	return text
}

func intValue(value any) int {
	number, _ := value.(int)
	return number
}

// optionalInt 区分「配了行号」与「没配/null」。
func optionalInt(value any) (int, bool) {
	if value == nil {
		return 0, false
	}
	number, ok := value.(int)
	return number, ok
}

// SortedSourceNames 只用于报错信息里稳定输出来源名。
func SortedSourceNames(sources map[string]any) []string {
	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
