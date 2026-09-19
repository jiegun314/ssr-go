package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/jiegun314/ssr-go/internal/config"
	"github.com/jiegun314/ssr-go/internal/consolidation"
	"github.com/jiegun314/ssr-go/internal/excelio"
	"github.com/jiegun314/ssr-go/internal/importer"
	"github.com/jiegun314/ssr-go/internal/store"
)

// workspace 把一次命令需要的部件装在一起：配置、数据库、导入器、整合服务、日志。
type workspace struct {
	Loader     *config.Loader
	Repository *store.Repository
	Importer   *importer.Importer
	Service    *consolidation.Service
	Log        *store.OperationLog
}

func openWorkspace(configDir string) (*workspace, error) {
	loader, err := config.NewLoader(configDir)
	if err != nil {
		return nil, err
	}
	if err := loader.ValidateAll(); err != nil {
		return nil, fmt.Errorf("配置校验失败：%w", err)
	}
	setting, err := loader.LoadSetting()
	if err != nil {
		return nil, err
	}
	database, _ := setting["database"].(map[string]any)
	databasePath, _ := database["path"].(string)
	repository, err := store.Open(loader.ResolvePath(databasePath))
	if err != nil {
		return nil, err
	}
	importService, err := importer.NewImporter(loader, repository)
	if err != nil {
		repository.Close()
		return nil, err
	}
	configValue, err := consolidation.LoadConfig(loader)
	if err != nil {
		repository.Close()
		return nil, err
	}
	tables, _ := setting["tables"].(map[string]any)
	operationLog, _ := tables["operation_log"].(map[string]any)
	tableName, _ := operationLog["name"].(string)
	timeColumn, _ := tables["operation_log_time_column"].(string)
	logColumns, err := loader.LoadLogColumns()
	if err != nil {
		repository.Close()
		return nil, err
	}
	log, err := store.OpenOperationLog(repository, store.OperationLogOptions{
		TableName:      tableName,
		TimeColumn:     timeColumn,
		IdentityFields: configValue.IdentityFields,
		LogColumns:     logColumns,
	})
	if err != nil {
		repository.Close()
		return nil, err
	}
	return &workspace{
		Loader:     loader,
		Repository: repository,
		Importer:   importService,
		Service: &consolidation.Service{
			Config:     configValue,
			Loader:     loader,
			Repository: repository,
			Log:        log,
		},
		Log: log,
	}, nil
}

func (workspace *workspace) Close() {
	if workspace.Repository != nil {
		workspace.Repository.Close()
	}
}

// --- 子命令实现 ---

func runImportFlags(arguments []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("import", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configDir := flags.String("config", "", "配置目录（默认取 UDI_CONFIG_DIR）")
	source := flags.String("source", "", "来源键：ra_input | global_udi_input | medical_insurance_code | product_category")
	file := flags.String("file", "", "要导入的 Excel 文件")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	return runImportCommand(*configDir, *source, *file, stdout)
}

func runConsolidateFlags(arguments []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("consolidate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configDir := flags.String("config", "", "配置目录（默认取 UDI_CONFIG_DIR）")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	return runConsolidateCommand(*configDir, stdout)
}

func runExportFlags(arguments []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("export", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configDir := flags.String("config", "", "配置目录（默认取 UDI_CONFIG_DIR）")
	fileName := flags.String("file", "", "导出文件名（默认 = 模板名 + 时间戳，见 R20）")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	return runExportCommand(*configDir, *fileName, stdout)
}

func runSnapshotFlags(arguments []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("snapshot", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configDir := flags.String("config", "", "配置目录（默认取 UDI_CONFIG_DIR）")
	inputDir := flags.String("input", "data/input/sample/valid", "四份来源 Excel 所在目录")
	outDir := flags.String("out", "baseline-go", "快照输出目录")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	return runSnapshotCommand(*configDir, *inputDir, *outDir, stdout)
}

func runImportCommand(configDir string, source string, file string, stdout io.Writer) int {
	if source == "" || file == "" {
		fmt.Fprintln(os.Stderr, "用法：ssr-core import --source <来源键> --file <Excel 路径>")
		return 2
	}
	space, err := openWorkspace(configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	defer space.Close()
	result, err := space.Importer.Import(source, file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "%s imported successfully. Rows imported: %d\n",
		result.FileName, result.RowCount)
	return 0
}

func runConsolidateCommand(configDir string, stdout io.Writer) int {
	space, err := openWorkspace(configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	defer space.Close()
	outcome, err := space.Service.Consolidate()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	counts := statusCounts(outcome)
	fmt.Fprintf(stdout,
		"Consolidation completed successfully. Rows consolidated: %d. Rows missing: %d. "+
			"Rows duplicate: %d. Rows changed: %d. Conflicts: %d\n",
		len(outcome.Rows), counts[consolidation.StatusIncomplete],
		counts[consolidation.StatusDuplicate], len(outcome.ChangedData),
		len(outcome.ConflictData))
	return 0
}

func runExportCommand(configDir string, fileName string, stdout io.Writer) int {
	space, err := openWorkspace(configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	defer space.Close()
	now := time.Now()
	result, err := space.Service.ExportConsolidationResult(fileName, now)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	if err := space.Service.RecordConsolidationResult(now); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Consolidation result exported successfully to %s\n", result.FilePath)
	fmt.Fprintf(stdout, "Exported file path: %s\n", result.FilePath)
	return 0
}

func statusCounts(outcome consolidation.Outcome) map[string]int {
	counts := map[string]int{
		consolidation.StatusReady:      0,
		consolidation.StatusIncomplete: 0,
		consolidation.StatusDuplicate:  0,
		consolidation.StatusConflict:   0,
	}
	for _, row := range outcome.Rows {
		counts[row.Values["status"]]++
	}
	return counts
}

// --- snapshot：产出与 Python 侧 baseline 逐格可比的行为快照 ---

func runSnapshotCommand(
	configDir string,
	inputDir string,
	outDir string,
	stdout io.Writer,
) int {
	space, err := openWorkspace(configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	defer space.Close()
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}

	// 1. 导入四份来源并按配置列序落成 TSV
	for _, sourceName := range space.Importer.Order {
		path := filepath.Join(inputDir, sourceName+".xlsx")
		if _, err := space.Importer.Import(sourceName, path); err != nil {
			fmt.Fprintf(os.Stderr, "导入 %s 失败：%v\n", sourceName, err)
			return 1
		}
		rule := space.Importer.Rules[sourceName]
		columns := []string{}
		for _, column := range rule.Columns {
			columns = append(columns, column.DBField)
		}
		rows, err := space.Repository.Rows(rule.TargetTable)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 1
		}
		cells := make([][]string, 0, len(rows))
		for _, row := range rows {
			values := make([]string, 0, len(columns))
			for _, column := range columns {
				values = append(values, row[column])
			}
			cells = append(cells, values)
		}
		if err := writeTSV(filepath.Join(outDir, "来源表_"+sourceName+".tsv"), columns, cells); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "来源表_%s.tsv：%d 行\n", sourceName, len(cells))
	}

	// 2. 整合 → 整合结果.tsv + 计数断言.json
	outcome, err := space.Service.Consolidate()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	resultColumns := append([]string{"status"}, space.Service.Config.FieldNames...)
	resultColumns = append(resultColumns, space.Service.Config.BaseKeyField)
	resultRows := make([][]string, 0, len(outcome.Rows))
	identities := make([]string, 0, len(outcome.Rows))
	for _, row := range outcome.Rows {
		values := make([]string, 0, len(resultColumns))
		for _, column := range resultColumns {
			values = append(values, row.Values[column])
		}
		resultRows = append(resultRows, values)
		identities = append(identities, row.Key)
	}
	if err := writeTSV(filepath.Join(outDir, "整合结果.tsv"), resultColumns, resultRows); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	counts := statusCounts(outcome)
	assertion := countsAssertion{
		StatusCounts: statusCountsJSON{
			Ready:      counts[consolidation.StatusReady],
			Incomplete: counts[consolidation.StatusIncomplete],
			Duplicate:  counts[consolidation.StatusDuplicate],
			Conflict:   counts[consolidation.StatusConflict],
		},
		Rows:                    len(outcome.Rows),
		MissingData:             emptyIfNil(outcome.MissingData),
		DuplicateData:           emptyIfNil(outcome.DuplicateData),
		ChangedData:             emptyIfNil(outcome.ChangedData),
		ConflictData:            emptyIfNil(outcome.ConflictData),
		ResultIdentitiesInOrder: identities,
	}
	if err := writeJSON(filepath.Join(outDir, "计数断言.json"), assertion); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}

	// 3. 导出 → 逐格 dump + 部件清单；随后记录导出结果 → 操作日志
	exportResult, err := space.Service.ExportConsolidationResult("", time.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	if err := dumpExportCells(
		exportResult.FilePath,
		space.Service.Config.SheetName,
		space.Service.Config.DataStartRow,
		filepath.Join(outDir, "导出文件_逐格.tsv"),
	); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	if err := dumpZipParts(
		space.Service.Config.TemplatePath,
		exportResult.FilePath,
		filepath.Join(outDir, "导出文件_部件清单.tsv"),
	); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	now := time.Now()
	if err := space.Service.RecordConsolidationResult(now); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	if err := dumpOperationLog(
		space.Log,
		filepath.Join(outDir, "操作日志.json"),
	); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	fmt.Fprintf(stdout,
		"整合：%d 行（Ready %d / Incomplete %d / Duplicate %d / Conflict %d）；导出 %d 行，缺列 %d\n",
		len(outcome.Rows), counts[consolidation.StatusReady],
		counts[consolidation.StatusIncomplete], counts[consolidation.StatusDuplicate],
		counts[consolidation.StatusConflict], exportResult.WrittenRows,
		len(exportResult.MissingHeaders))
	fmt.Fprintf(stdout, "快照目录：%s\n", outDir)
	return 0
}

// --- JSON 结构（字段顺序必须与 Python 的快照一致） ---

type statusCountsJSON struct {
	Ready      int `json:"Ready"`
	Incomplete int `json:"Incomplete"`
	Duplicate  int `json:"Duplicate"`
	Conflict   int `json:"Conflict"`
}

type countsAssertion struct {
	StatusCounts            statusCountsJSON `json:"status_counts"`
	Rows                    int              `json:"rows"`
	MissingData             []string         `json:"missing_data"`
	DuplicateData           []string         `json:"duplicate_data"`
	ChangedData             []string         `json:"changed_data"`
	ConflictData            []string         `json:"conflict_data"`
	ResultIdentitiesInOrder []string         `json:"result_identities_in_order"`
}

// --- 输出工具 ---

// tsvCell 与 Python 快照脚本的转义规则一致（先反斜杠，再 \t \r \n）。
func tsvCell(value string) string {
	replaced := strings.ReplaceAll(value, "\\", "\\\\")
	replaced = strings.ReplaceAll(replaced, "\t", "\\t")
	replaced = strings.ReplaceAll(replaced, "\r", "\\r")
	return strings.ReplaceAll(replaced, "\n", "\\n")
}

func writeTSV(path string, header []string, rows [][]string) error {
	lines := []string{strings.Join(header, "\t")}
	for _, row := range rows {
		cells := make([]string, 0, len(row))
		for _, cell := range row {
			cells = append(cells, tsvCell(cell))
		}
		lines = append(lines, strings.Join(cells, "\t"))
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// writeJSON 用 Python json.dumps(..., ensure_ascii=False, indent=2) + "\n" 的等价口径。
func writeJSON(path string, value any) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func emptyIfNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// dumpExportCells 导出「sheet / 行 / 列字母 / 文本」，只含数据区里有值的单元格。
func dumpExportCells(path string, sheetName string, dataStartRow int, output string) error {
	sheet, err := excelio.ReadFirstSheet(path)
	if err != nil {
		return err
	}
	lines := []string{"sheet\trow\tcolumn\ttext"}
	for rowIndex := dataStartRow; rowIndex <= len(sheet.Rows); rowIndex++ {
		row := sheet.Rows[rowIndex-1]
		for columnIndex := 1; columnIndex <= len(row); columnIndex++ {
			text := row[columnIndex-1]
			if text == "" {
				continue
			}
			letter, err := excelizeColumnName(columnIndex)
			if err != nil {
				return err
			}
			lines = append(lines, fmt.Sprintf("%s\t%d\t%s\t%s",
				sheetName, rowIndex, letter, tsvCell(text)))
		}
	}
	return os.WriteFile(output, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// dumpZipParts 比较模板与导出文件的 zip 部件（§9.1 的「有损」证据）。
func dumpZipParts(templatePath string, exportPath string, output string) error {
	templateParts, err := zipPartSizes(templatePath)
	if err != nil {
		return err
	}
	exportParts, err := zipPartSizes(exportPath)
	if err != nil {
		return err
	}
	names := map[string]bool{}
	for name := range templateParts {
		names[name] = true
	}
	for name := range exportParts {
		names[name] = true
	}
	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	rows := [][]string{}
	for _, name := range sorted {
		inTemplate, hasTemplate := templateParts[name]
		inExport, hasExport := exportParts[name]
		change := "新增"
		switch {
		case hasTemplate && hasExport:
			if inTemplate == inExport {
				change = "保留"
			} else {
				change = "保留(大小变化)"
			}
		case hasTemplate:
			change = "丢失"
		}
		rows = append(rows, []string{
			name,
			yesNo(hasTemplate),
			yesNo(hasExport),
			sizeText(inTemplate, hasTemplate),
			sizeText(inExport, hasExport),
			change,
		})
	}
	return writeTSV(output, []string{"部件", "模板内", "导出文件内", "模板字节", "导出字节", "变化"}, rows)
}

// dumpOperationLog 写日志快照：列序 = log_columns.yaml，log_time 换成占位符。
func dumpOperationLog(log *store.OperationLog, output string) error {
	rows, err := log.Repository.Rows(log.TableName)
	if err != nil {
		return err
	}
	records := make([]orderedRecord, 0, len(rows))
	for _, row := range rows {
		record := orderedRecord{Keys: log.LogColumns, Values: map[string]string{}}
		for _, column := range log.LogColumns {
			if column == log.TimeColumn {
				record.Values[column] = "<log_time>"
				continue
			}
			record.Values[column] = row[column]
		}
		records = append(records, record)
	}
	return writeJSON(output, records)
}

// orderedRecord 让 JSON 的键顺序等于配置的列顺序（Go 的 map 会按字母序排）。
type orderedRecord struct {
	Keys   []string
	Values map[string]string
}

func (record orderedRecord) MarshalJSON() ([]byte, error) {
	var builder strings.Builder
	builder.WriteByte('{')
	for index, key := range record.Keys {
		if index > 0 {
			builder.WriteByte(',')
		}
		encodedKey, err := marshalNoHTMLEscape(key)
		if err != nil {
			return nil, err
		}
		encodedValue, err := marshalNoHTMLEscape(record.Values[key])
		if err != nil {
			return nil, err
		}
		builder.Write(encodedKey)
		builder.WriteByte(':')
		builder.Write(encodedValue)
	}
	builder.WriteByte('}')
	return []byte(builder.String()), nil
}

// marshalNoHTMLEscape 与 Python 的 json.dumps 一样不转义 < > &。
func marshalNoHTMLEscape(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buffer.Bytes(), "\n"), nil
}

// --- 文件与小工具 ---

func excelizeColumnName(number int) (string, error) {
	return excelize.ColumnNumberToName(number)
}

func zipPartSizes(path string) (map[string]int, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer archive.Close()
	sizes := map[string]int{}
	for _, entry := range archive.File {
		sizes[entry.Name] = int(entry.UncompressedSize64)
	}
	return sizes, nil
}

func yesNo(present bool) string {
	if present {
		return "是"
	}
	return "否"
}

func sizeText(size int, present bool) string {
	if !present {
		return ""
	}
	return strconv.Itoa(size)
}
