package store

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/config"
)

func newRepository(t *testing.T) *Repository {
	t.Helper()
	repository, err := Open(filepath.Join(t.TempDir(), "test.sqlite3"))
	if err != nil {
		t.Fatalf("打开数据库失败：%v", err)
	}
	t.Cleanup(func() { repository.Close() })
	return repository
}

func newLoader(t *testing.T) *config.Loader {
	t.Helper()
	loader, err := config.NewLoader(filepath.Join("..", "..", "config"))
	if err != nil {
		t.Fatalf("读配置失败：%v", err)
	}
	return loader
}

func TestWriteReadDropTable(t *testing.T) {
	repository := newRepository(t)

	if err := repository.WriteRows(
		"repository_test",
		[]string{"material_code", "value"},
		[]Row{{"material_code": "0001", "value": "A"}},
		WriteReplace,
	); err != nil {
		t.Fatalf("写表失败：%v", err)
	}
	names, err := repository.TableNames()
	if err != nil {
		t.Fatalf("列表名失败：%v", err)
	}
	if len(names) != 1 || names[0] != "repository_test" {
		t.Fatalf("表名 = %v", names)
	}
	keyed, err := repository.KeyedRows("repository_test", "material_code")
	if err != nil {
		t.Fatalf("读表失败：%v", err)
	}
	if keyed["0001"]["value"] != "A" {
		t.Fatalf("读回的数据不对：%v", keyed)
	}
	if err := repository.DropTable("repository_test"); err != nil {
		t.Fatalf("删表失败：%v", err)
	}
	exists, err := repository.TableExists("repository_test")
	if err != nil {
		t.Fatalf("判断表是否存在失败：%v", err)
	}
	if exists {
		t.Fatal("表应当已被删掉")
	}
	// 删一张不存在的表是空操作（R25 的幂等口径）
	if err := repository.DropTable("repository_test"); err != nil {
		t.Fatalf("重复删表应当成功：%v", err)
	}
}

func TestCountRowsTreatsAMissingTableAsEmpty(t *testing.T) {
	repository := newRepository(t)

	count, err := repository.CountTableRows("repository_test")
	if err != nil || count != 0 {
		t.Fatalf("表不存在时应报 0（err=%v, count=%d）", err, count)
	}
	if err := repository.WriteRows(
		"repository_test",
		[]string{"material_code"},
		[]Row{{"material_code": "0001"}, {"material_code": "0002"}, {"material_code": "0003"}},
		WriteReplace,
	); err != nil {
		t.Fatalf("写表失败：%v", err)
	}
	if count, _ = repository.CountTableRows("repository_test"); count != 3 {
		t.Fatalf("count = %d; want 3", count)
	}
	if err := repository.WriteRows(
		"repository_test",
		[]string{"material_code"},
		[]Row{{"material_code": "0004"}},
		WriteAppend,
	); err != nil {
		t.Fatalf("追加失败：%v", err)
	}
	if count, _ = repository.CountTableRows("repository_test"); count != 4 {
		t.Fatalf("append 后 count = %d; want 4", count)
	}
	if err := repository.DropTable("repository_test"); err != nil {
		t.Fatalf("删表失败：%v", err)
	}
	if count, _ = repository.CountTableRows("repository_test"); count != 0 {
		t.Fatalf("删表后 count = %d; want 0", count)
	}
}

func TestAppendAddsMissingColumns(t *testing.T) {
	repository := newRepository(t)

	if err := repository.WriteRows("append_test", []string{"material_code"},
		[]Row{{"material_code": "0001"}}, WriteReplace); err != nil {
		t.Fatalf("写表失败：%v", err)
	}
	// 追加时缺列要补列（R8），旧行的新列为空
	if err := repository.WriteRows("append_test", []string{"material_code", "extra"},
		[]Row{{"material_code": "0002", "extra": "B"}}, WriteAppend); err != nil {
		t.Fatalf("追加失败：%v", err)
	}
	columns, err := repository.TableColumns("append_test")
	if err != nil {
		t.Fatalf("读列失败：%v", err)
	}
	if strings.Join(columns, ",") != "material_code,extra" {
		t.Fatalf("列 = %v", columns)
	}
	rows, err := repository.Rows("append_test")
	if err != nil {
		t.Fatalf("读行失败：%v", err)
	}
	if len(rows) != 2 || rows[0]["extra"] != "" || rows[1]["extra"] != "B" {
		t.Fatalf("行 = %v", rows)
	}
}

func TestEveryColumnIsText(t *testing.T) {
	repository := newRepository(t)

	// 数字样子的值也必须按文本存：typeof 返回 "text" 才说明没有丢前导零的风险
	if err := repository.WriteRows("text_test", []string{"Primary DI"},
		[]Row{{"Primary DI": "0123"}}, WriteReplace); err != nil {
		t.Fatalf("写表失败：%v", err)
	}
	database, err := sql.Open("sqlite", repository.Path)
	if err != nil {
		t.Fatalf("打开数据库失败：%v", err)
	}
	defer database.Close()
	var columnType string
	if err := database.QueryRow(
		`SELECT typeof("Primary DI") FROM text_test`).Scan(&columnType); err != nil {
		t.Fatalf("读列类型失败：%v", err)
	}
	if columnType != "text" {
		t.Fatalf("列类型 = %q; want text", columnType)
	}
	var stored string
	if err := database.QueryRow(
		`SELECT "Primary DI" FROM text_test`).Scan(&stored); err != nil {
		t.Fatalf("读值失败：%v", err)
	}
	if stored != "0123" {
		t.Fatalf("前导零丢了：%q", stored)
	}
}

func TestKeyedRowsKeepsTheLastRowPerKey(t *testing.T) {
	repository := newRepository(t)

	if err := repository.WriteRows(
		"keyed_test",
		[]string{"material_code", "product_name"},
		[]Row{
			{"material_code": "MAT-1", "product_name": "first"},
			{"material_code": "MAT-1", "product_name": "last"},
		},
		WriteReplace,
	); err != nil {
		t.Fatalf("写表失败：%v", err)
	}
	keyed, err := repository.KeyedRows("keyed_test", "material_code")
	if err != nil {
		t.Fatalf("读表失败：%v", err)
	}
	if len(keyed) != 1 || keyed["MAT-1"]["product_name"] != "last" {
		t.Fatalf("重复业务键应当以最后一行为准：%v", keyed)
	}
}

func TestR8SourceTablesFollowTheConfiguration(t *testing.T) {
	loader := newLoader(t)

	tables, err := SourceTables(loader)
	if err != nil {
		t.Fatalf("读来源表定义失败：%v", err)
	}
	names := []string{}
	counts := []int{}
	for _, table := range tables {
		names = append(names, table.SourceName)
		counts = append(counts, len(table.Columns))
		if !table.ReplaceOnImport {
			t.Errorf("%s 的 replace_on_import 应为 true（当前配置）", table.SourceName)
		}
	}
	// 顺序 = 配置顺序（Python 的 dict 保留 YAML 顺序）
	if strings.Join(names, "|") != "ra_input|global_udi_input|medical_insurance_code|product_category" {
		t.Fatalf("来源顺序 = %v", names)
	}
	// 列数由 AGENTS.md §3.3 记录：25 / 42 / 35 / 6
	if len(counts) != 4 || counts[0] != 25 || counts[1] != 42 || counts[2] != 35 || counts[3] != 6 {
		t.Fatalf("列数 = %v; want [25 42 35 6]", counts)
	}
}

func TestR8ImportUsesTheDeclaredPolicy(t *testing.T) {
	loader := newLoader(t)
	repository := newRepository(t)
	tables, err := SourceTables(loader)
	if err != nil {
		t.Fatalf("读来源表定义失败：%v", err)
	}
	table := tables[0]
	if !table.ReplaceOnImport {
		t.Skip("这条用例针对 replace_on_import: true 的来源")
	}

	if err := ImportSourceRows(repository, table, []Row{{table.Columns[0]: "0001"}}); err != nil {
		t.Fatalf("导入失败：%v", err)
	}
	if err := ImportSourceRows(repository, table, []Row{{table.Columns[0]: "0002"}}); err != nil {
		t.Fatalf("导入失败：%v", err)
	}
	count, _ := repository.CountTableRows(table.TableName)
	if count != 1 {
		t.Fatalf("replace_on_import: true 时每次导入替换整表，count = %d", count)
	}

	appendTable := table
	appendTable.TableName = "append_source"
	appendTable.ReplaceOnImport = false
	if err := ImportSourceRows(repository, appendTable, []Row{{table.Columns[0]: "0001"}}); err != nil {
		t.Fatalf("导入失败：%v", err)
	}
	if err := ImportSourceRows(repository, appendTable, []Row{{table.Columns[0]: "0002"}}); err != nil {
		t.Fatalf("导入失败：%v", err)
	}
	if count, _ = repository.CountTableRows("append_source"); count != 2 {
		t.Fatalf("追加导入不去重，count = %d; want 2", count)
	}
}

func TestR9CleanupTableNamesFollowTheConfiguration(t *testing.T) {
	loader := newLoader(t)

	names, err := CleanupTableNames(loader)
	if err != nil {
		t.Fatalf("读清理清单失败：%v", err)
	}
	if strings.Join(names, "|") !=
		"ra_input_staging|global_udi_input_staging|product_category_staging|consolidation_staging" {
		t.Fatalf("启动清理清单 = %v", names)
	}
	sourceNames, err := SourceCleanupNames(loader)
	if err != nil {
		t.Fatalf("读来源清理清单失败：%v", err)
	}
	if strings.Join(sourceNames, "|") != "ra_input|global_udi_input|product_category" {
		t.Fatalf("来源清单 = %v", sourceNames)
	}
}

func TestR25CleaningImportedSourcesDropsOnlySourceTables(t *testing.T) {
	loader := newLoader(t)
	repository := newRepository(t)
	tables, err := SourceTables(loader)
	if err != nil {
		t.Fatalf("读来源表定义失败：%v", err)
	}
	// 建出全部来源表 + 两张应用表
	for _, table := range tables {
		if err := repository.WriteRows(table.TableName, table.Columns, nil, WriteReplace); err != nil {
			t.Fatalf("建表失败：%v", err)
		}
	}
	for _, name := range []string{"consolidation_staging", "operation_log"} {
		if err := repository.WriteRows(name, []string{"material_code"}, nil, WriteReplace); err != nil {
			t.Fatalf("建表失败：%v", err)
		}
	}

	cleanupTables, err := SourceCleanupTableNames(loader)
	if err != nil {
		t.Fatalf("读来源清理清单失败：%v", err)
	}
	dropped, failed := DropTables(repository, cleanupTables)

	if len(failed) != 0 {
		t.Fatalf("删表失败：%v", failed)
	}
	if strings.Join(dropped, "|") !=
		"ra_input_staging|global_udi_input_staging|product_category_staging" {
		t.Fatalf("被删的表 = %v", dropped)
	}
	remaining, err := repository.TableNames()
	if err != nil {
		t.Fatalf("列表名失败：%v", err)
	}
	// 医保编码表（preserve_on_cleanup: true）与两张应用表都还在
	for _, name := range []string{"medical_insurance_code", "consolidation_staging", "operation_log"} {
		found := false
		for _, remainingName := range remaining {
			if remainingName == name {
				found = true
			}
		}
		if !found {
			t.Errorf("%s 不该被「清空导入数据」删掉（剩下的表：%v）", name, remaining)
		}
	}
}
