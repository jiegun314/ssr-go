package consolidation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/config"
	"github.com/jiegun314/ssr-go/internal/importer"
	"github.com/jiegun314/ssr-go/internal/store"
)

// newWorkspace 复制真实配置到临时目录、开一个临时库，返回三件套。
func newWorkspace(t *testing.T) (*config.Loader, *store.Repository, *importer.Importer) {
	t.Helper()
	source := filepath.Join("..", "..", "config")
	target := t.TempDir()
	for _, name := range []string{
		config.SettingFile, config.ImportMappingFile,
		config.ConsolidationMappingFile, config.LogColumnsFile,
	} {
		content, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			t.Fatalf("读取 %s 失败：%v", name, err)
		}
		if err := os.WriteFile(filepath.Join(target, name), content, 0o644); err != nil {
			t.Fatalf("写 %s 失败：%v", name, err)
		}
	}
	loader, err := config.NewLoader(target)
	if err != nil {
		t.Fatalf("读配置失败：%v", err)
	}
	repository, err := store.Open(filepath.Join(t.TempDir(), "test.sqlite3"))
	if err != nil {
		t.Fatalf("打开数据库失败：%v", err)
	}
	t.Cleanup(func() { repository.Close() })
	importService, err := importer.NewImporter(loader, repository)
	if err != nil {
		t.Fatalf("构造导入器失败：%v", err)
	}
	return loader, repository, importService
}

// newService 组装一次整合需要的全部部件（含日志表：查询历史记录用）。
func newService(t *testing.T, loader *config.Loader, repository *store.Repository) *Service {
	t.Helper()
	configValue, err := LoadConfig(loader)
	if err != nil {
		t.Fatalf("读整合配置失败：%v", err)
	}
	setting, err := loader.LoadSetting()
	if err != nil {
		t.Fatalf("读 setting.yaml 失败：%v", err)
	}
	tables, _ := setting["tables"].(map[string]any)
	operationLog, _ := tables["operation_log"].(map[string]any)
	tableName, _ := operationLog["name"].(string)
	timeColumn, _ := tables["operation_log_time_column"].(string)
	log, err := store.OpenOperationLog(repository, store.OperationLogOptions{
		TableName:      tableName,
		TimeColumn:     timeColumn,
		IdentityFields: configValue.IdentityFields,
		LogColumns:     fieldNamesWithLogColumns(t, loader),
	})
	if err != nil {
		t.Fatalf("打开日志表失败：%v", err)
	}
	return &Service{
		Config:     configValue,
		Loader:     loader,
		Repository: repository,
		Log:        log,
	}
}

func fieldNamesWithLogColumns(t *testing.T, loader *config.Loader) []string {
	t.Helper()
	columns, err := loader.LoadLogColumns()
	if err != nil {
		t.Fatalf("读日志列失败：%v", err)
	}
	return columns
}

// TestConsolidationMatchesThePythonBaseline 是整合阶段的 golden diff（数据口径①）。
//
// 对照物 `baseline/整合结果.tsv` 由 Python 现状实现产出：status + 30 个导出列 +
// material_code，行序固定。Go 实现必须逐格一致——包括样本里刻意设计的脏数据
// （DI 写成数字丢掉前导零，因此多一行结果）与 MISSING 行。
func TestConsolidationMatchesThePythonBaseline(t *testing.T) {
	loader, repository, importService := newWorkspace(t)
	for _, sourceName := range importService.Order {
		path := filepath.Join("..", "..", "testdata", "sample-valid", sourceName+".xlsx")
		if _, err := importService.Import(sourceName, path); err != nil {
			t.Fatalf("导入 %s 失败：%v", sourceName, err)
		}
	}
	service := newService(t, loader, repository)

	outcome, err := service.Consolidate()
	if err != nil {
		t.Fatalf("整合失败：%v", err)
	}

	// §9.3 第 4 项的计数断言
	counts := map[string]int{}
	for _, row := range outcome.Rows {
		counts[row.Values["status"]]++
	}
	if counts[StatusReady] != 8 || counts[StatusIncomplete] != 1 ||
		counts[StatusDuplicate] != 0 || counts[StatusConflict] != 0 {
		t.Fatalf("状态计数 = %v; want Ready 8 / Incomplete 1", counts)
	}
	if strings.Join(outcome.MissingData, "|") != "000MAT-006::MISSING" {
		t.Fatalf("缺失行 = %v", outcome.MissingData)
	}

	golden := readTSV(t, filepath.Join("..", "..", "testdata", "baseline", "整合结果.tsv"))
	if len(golden) == 0 {
		t.Fatal("baseline 是空的")
	}
	expectedColumns := append([]string{"status"}, service.Config.FieldNames...)
	expectedColumns = append(expectedColumns, service.Config.BaseKeyField)
	if strings.Join(golden[0], "|") != strings.Join(expectedColumns, "|") {
		t.Fatalf("列不一致\nbaseline: %v\nGo:       %v", golden[0], expectedColumns)
	}
	if len(outcome.Rows) != len(golden)-1 {
		t.Fatalf("行数：Go = %d，baseline = %d", len(outcome.Rows), len(golden)-1)
	}
	for position, goldenRow := range golden[1:] {
		for index, column := range golden[0] {
			wanted := unescapeTSVCell(goldenRow[index])
			got := outcome.Rows[position].Values[column]
			if got != wanted {
				t.Errorf("第 %d 行 %q：Go = %q，baseline = %q",
					position+1, column, got, wanted)
			}
		}
	}
}

func readTSV(t *testing.T, path string) [][]string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 baseline 失败：%v", err)
	}
	text := strings.TrimRight(string(content), "\n")
	lines := strings.Split(text, "\n")
	rows := make([][]string, 0, len(lines))
	for _, line := range lines {
		rows = append(rows, strings.Split(line, "\t"))
	}
	return rows
}

func unescapeTSVCell(value string) string {
	var builder strings.Builder
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character != '\\' || index+1 >= len(value) {
			builder.WriteByte(character)
			continue
		}
		index++
		switch value[index] {
		case '\\':
			builder.WriteByte('\\')
		case 't':
			builder.WriteByte('\t')
		case 'r':
			builder.WriteByte('\r')
		case 'n':
			builder.WriteByte('\n')
		default:
			builder.WriteByte(value[index])
		}
	}
	return builder.String()
}
