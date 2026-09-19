package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jiegun314/ssr-go/internal/config"
	"github.com/jiegun314/ssr-go/internal/excelio"
	"github.com/jiegun314/ssr-go/internal/sample"
	"github.com/jiegun314/ssr-go/internal/store"
)

// logColumnsComment 与 Python 版 scripts/build_log_columns.py 的 COMMENT 逐字一致。
//
// 这里刻意保留 Python 的示例命令：config/ 是本仓库与 Python 仓库共享的行为来源，
// 两边的 log_columns.yaml 必须逐字节相同（Go 的等价命令见 README）。
const logColumnsComment = `# Columns of one stored operation log record.
#
# The log keeps one column per exported column of the target export template, in
# the order of its header row, so a stored record can be read back and reviewed
# like the exported file. The file is generated, not maintained: change the
# template or setting.yaml and run
#
#     python scripts/build_log_columns.py
#
# instead of editing the column list by hand.
#
# before / after list the columns the template does not define. A stored record
# needs them anyway - when it was logged, what happened, and how it was
# consolidated - so they surround the exported columns. The order below is the
# order the log table stores and the log review shows; a column without a value
# stays empty.
`

// LogColumnsDefinition 是 config/log_columns.yaml 的内容。
type LogColumnsDefinition struct {
	Version string
	Before  []string
	Columns []string
	After   []string
}

// BuildLogColumns 从导出模板的表头行生成日志列定义（R22 / §3.5）。
func BuildLogColumns(loader *config.Loader) (LogColumnsDefinition, error) {
	setting, err := loader.LoadSetting()
	if err != nil {
		return LogColumnsDefinition{}, err
	}
	consolidation, err := loader.LoadConsolidationMapping()
	if err != nil {
		return LogColumnsDefinition{}, err
	}
	exportTemplate, _ := setting["export_template"].(map[string]any)
	tables, _ := setting["tables"].(map[string]any)
	timeColumn, _ := tables["operation_log_time_column"].(string)
	dataset, _ := consolidation["target_dataset"].(map[string]any)
	mergeRules, _ := dataset["merge_rules"].(map[string]any)
	baseSource, _ := mergeRules["base_source"].(map[string]any)

	templatePath := loader.ResolvePath(textValue(exportTemplate["path"]))
	sheetName := textValue(exportTemplate["sheet_name"])
	headerRow := intValue(exportTemplate["header_row"])
	titles, err := readTemplateTitles(templatePath, sheetName, headerRow)
	if err != nil {
		return LogColumnsDefinition{}, err
	}
	after := []string{}
	for _, key := range stringValues(baseSource["key"]) {
		after = append(after, key)
	}
	after = append(after, config.LogOperationColumns...)
	return LogColumnsDefinition{
		Version: "1.0",
		Before:  []string{timeColumn, config.LogStatusColumn},
		Columns: titles,
		After:   after,
	}, nil
}

// readTemplateTitles 读表头行的列名（折叠空白、跳过空标题、拒绝重复标题）。
func readTemplateTitles(templatePath string, sheetName string, headerRow int) ([]string, error) {
	sheet, err := excelio.ReadSheetByName(templatePath, sheetName)
	if err != nil {
		return nil, err
	}
	titles := []string{}
	for _, value := range sheet.HeaderRow(headerRow) {
		normalized := excelio.NormalizeHeaderTitle(value)
		if normalized == "" {
			continue
		}
		titles = append(titles, normalized)
	}
	counts := map[string]int{}
	for _, title := range titles {
		counts[title]++
	}
	duplicates := []string{}
	for title, count := range counts {
		if count > 1 {
			duplicates = append(duplicates, title)
		}
	}
	if len(duplicates) > 0 {
		sortStrings(duplicates)
		return nil, fmt.Errorf("Header row %d of %s repeats titles: %s",
			headerRow, sheetName, strings.Join(duplicates, ", "))
	}
	return titles, nil
}

// RenderLogColumns 序列化成配置文件的内容（与 Python 的 PyYAML 输出逐字节一致）。
func RenderLogColumns(definition LogColumnsDefinition) string {
	var builder strings.Builder
	builder.WriteString(logColumnsComment)
	builder.WriteString("version: " + renderLogColumnsScalar(definition.Version) + "\n")
	writeLogColumnsList(&builder, "before", definition.Before)
	writeLogColumnsList(&builder, "columns", definition.Columns)
	writeLogColumnsList(&builder, "after", definition.After)
	return builder.String()
}

func writeLogColumnsList(builder *strings.Builder, key string, values []string) {
	builder.WriteString(key + ":\n")
	for _, value := range values {
		builder.WriteString("  - " + renderLogColumnsScalar(value) + "\n")
	}
}

var (
	numberLike    = regexp.MustCompile(`^[-+]?(\d+\.?\d*|\.\d+)([eE][-+]?\d+)?$`)
	reservedWords = map[string]bool{
		"true": true, "false": true, "yes": true, "no": true, "on": true,
		"off": true, "null": true, "~": true, "y": true, "n": true,
	}
)

// renderLogColumnsScalar 复刻 PyYAML 的标量写法：该引号的引单引号，其余保持裸串。
func renderLogColumnsScalar(value string) string {
	if needsQuoting(value) {
		return "'" + strings.ReplaceAll(value, "'", "''") + "'"
	}
	return value
}

func needsQuoting(value string) bool {
	if value == "" || strings.TrimSpace(value) != value {
		return true
	}
	if numberLike.MatchString(value) || reservedWords[strings.ToLower(value)] {
		return true
	}
	if strings.ContainsAny(value, "\n\t\r") {
		return true
	}
	if strings.Contains(value, ": ") || strings.Contains(value, " #") {
		return true
	}
	return strings.ContainsAny(value[:1], "-?:,[]{}#&*!|>'\"%@`")
}

func runGenLogColumnsFlags(arguments []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("genlogcolumns", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configDir := flags.String("config", "", "配置目录（默认取 UDI_CONFIG_DIR）")
	checkOnly := flags.Bool("check", false, "只校验 config/log_columns.yaml 与模板是否一致，不写文件")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	loader, err := config.NewLoader(*configDir)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	definition, err := BuildLogColumns(loader)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	rendered := RenderLogColumns(definition)
	path := loader.Resolver.ConfigFile(config.LogColumnsFile)
	if *checkOnly {
		existing, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return 1
		}
		if string(existing) != rendered {
			fmt.Fprintf(stderr,
				"%s 与模板表头不一致，请运行 genlogcolumns 重新生成\n", path)
			return 1
		}
		fmt.Fprintf(stdout, "%s 与模板表头一致\n", path)
		return 0
	}
	if err := os.WriteFile(path, []byte(rendered), 0o644); err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Wrote %s\n", path)
	return 0
}

func runPrepareDBFlags(arguments []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("preparedb", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configDir := flags.String("config", "", "配置目录（默认取 UDI_CONFIG_DIR）")
	outputRoot := flags.String("output-root", "", "发布包根目录（数据库按 setting.yaml 的相对路径写在这里）")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	if *outputRoot == "" {
		fmt.Fprintln(stderr, "用法：ssr-core preparedb --output-root <发布包根目录> [--config <配置目录>]")
		return 2
	}
	loader, err := config.NewLoader(*configDir)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	setting, err := loader.LoadSetting()
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	database, _ := setting["database"].(map[string]any)
	databasePath := filepath.Join(*outputRoot, textValue(database["path"]))
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	// 发布包的空库要重新生成：已存在的文件先删掉（与 Python 版一致）
	if err := os.Remove(databasePath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	repository, err := store.Open(databasePath)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	defer repository.Close()
	tables, _ := setting["tables"].(map[string]any)
	operationLog, _ := tables["operation_log"].(map[string]any)
	identityFields, err := loader.LoadDuplicateCheckIdentityFields()
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	logColumns, err := loader.LoadLogColumns()
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	if _, err := store.OpenOperationLog(repository, store.OperationLogOptions{
		TableName:      textValue(operationLog["name"]),
		TimeColumn:     textValue(tables["operation_log_time_column"]),
		IdentityFields: identityFields,
		LogColumns:     logColumns,
	}); err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Prepared release database: %s\n", databasePath)
	return 0
}

func runGenSampleFlags(arguments []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("gensample", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configDir := flags.String("config", "", "配置目录（默认取 UDI_CONFIG_DIR）")
	output := flags.String("output", "data/input/sample", "样本输出目录")
	rows := flags.Int("rows", sample.DefaultRows, "产品代码数量")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	if *rows < 1 {
		fmt.Fprintln(stderr, "至少需要 1 行")
		return 2
	}
	loader, err := config.NewLoader(*configDir)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	outputRoot := *output
	if !filepath.IsAbs(outputRoot) {
		outputRoot = filepath.Join(loader.Resolver.ProjectRoot, outputRoot)
	}
	invalidPath, firstOffendingRow, err := sample.WriteSampleSets(outputRoot, loader, *rows)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "样本数据目录：%s\n", outputRoot)
	fmt.Fprintf(stdout, "  valid/               %d 个产品代码（另有 1 个只有 RA/医保/产品类别 的产品代码）\n", *rows)
	fmt.Fprintf(stdout, "  invalid-conditions/  条件必填被拒，UDI 文件从第 %d 行开始\n", firstOffendingRow)
	fmt.Fprintf(stdout, "  被拒文件：%s\n", invalidPath)
	return 0
}

func runAlignLogColumnsFlags(arguments []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("alignlogcolumns", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configDir := flags.String("config", "", "配置目录（默认取 UDI_CONFIG_DIR）")
	database := flags.String("database", "", "要对齐的 SQLite 文件（默认取 setting.yaml）")
	noBackup := flags.Bool("no-backup", false, "跳过备份（默认先备份）")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	loader, err := config.NewLoader(*configDir)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	setting, err := loader.LoadSetting()
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	tables, _ := setting["tables"].(map[string]any)
	operationLog, _ := tables["operation_log"].(map[string]any)
	tableName := textValue(operationLog["name"])
	columns, err := loader.LoadLogColumns()
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	if len(columns) == 0 {
		fmt.Fprintf(stderr, "配置里没有日志列：%s\n",
			loader.Resolver.ConfigFile(config.LogColumnsFile))
		return 1
	}
	databasePath := *database
	if databasePath == "" {
		database, _ := setting["database"].(map[string]any)
		databasePath = loader.ResolvePath(textValue(database["path"]))
	}
	report, err := store.AlignDatabase(databasePath, tableName, columns, !*noBackup)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "%s: %s holds %d columns\n", report.Database, report.Table, len(columns))
	switch report.Action {
	case store.AlignAlreadyAligned:
		fmt.Fprintln(stdout, "Already matches the configured columns, nothing written")
	case store.AlignCreated:
		fmt.Fprintln(stdout, "Created the missing table")
	default:
		fmt.Fprintf(stdout, "Rows kept: %d\n", report.Rows)
		if len(report.Added) > 0 {
			fmt.Fprintf(stdout, "Columns added: %s\n", strings.Join(report.Added, ", "))
		}
		if len(report.Removed) > 0 {
			fmt.Fprintf(stdout, "Columns dropped: %s\n", strings.Join(report.Removed, ", "))
		}
		if report.Backup != "" {
			fmt.Fprintf(stdout, "Backup: %s\n", report.Backup)
		} else {
			fmt.Fprintln(stdout, "No backup")
		}
	}
	return 0
}

func textValue(value any) string {
	text, _ := value.(string)
	return text
}

func intValue(value any) int {
	number, _ := value.(int)
	return number
}

func stringValues(value any) []string {
	entries, ok := value.([]any)
	if !ok {
		return nil
	}
	results := make([]string, 0, len(entries))
	for _, entry := range entries {
		results = append(results, textValue(entry))
	}
	return results
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j-1] > values[j]; j-- {
			values[j-1], values[j] = values[j], values[j-1]
		}
	}
}
