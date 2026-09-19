package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/config"
)

// 老版本应用写下的日志表（对应 Python 版 tests/test_align_log_columns.py 的 OLD_COLUMNS）。
var oldLogColumns = []string{
	"status", "Primary DI", "Catalog or Reference Number",
	"material_code", "operation", "details", "log_time",
}

// writeOlderTable 建一张老版本结构的表，operation / details 是 NULL（老行就是这样存的）。
func writeOlderTable(t *testing.T, repository *Repository, tableName string) {
	t.Helper()
	definitions := make([]string, 0, len(oldLogColumns))
	for _, name := range oldLogColumns {
		definitions = append(definitions, quoteIdentifier(name)+" VARCHAR")
	}
	if err := repository.Execute("CREATE TABLE " + quoteIdentifier(tableName) +
		" (\n    " + strings.Join(definitions, ",\n    ") + "\n)"); err != nil {
		t.Fatalf("建旧表失败：%v", err)
	}
	statement := "INSERT INTO " + quoteIdentifier(tableName) + " VALUES (?, ?, ?, ?, NULL, NULL, ?)"
	for _, row := range [][3]string{
		{"Pass", "DI-1", "SKU-1"},
		{"Pass", "DI-2", "SKU-2"},
	} {
		if err := repository.Execute(statement,
			row[0], row[1], row[2], "MAT-"+row[1][len(row[1])-1:], "2026/09/11 23:47:31"); err != nil {
			t.Fatalf("插旧行失败：%v", err)
		}
	}
}

func logColumnsOf(t *testing.T, loader *config.Loader) []string {
	t.Helper()
	columns, err := loader.LoadLogColumns()
	if err != nil {
		t.Fatalf("读日志列失败：%v", err)
	}
	return columns
}

func TestR23AlignRewritesTheTableAndKeepsEveryRow(t *testing.T) {
	loader := newLoader(t)
	repository := newRepository(t)
	writeOlderTable(t, repository, "operation_log")
	columns := logColumnsOf(t, loader)

	report, err := AlignDatabase(repository.Path, "operation_log", columns, true)
	if err != nil {
		t.Fatalf("对齐失败：%v", err)
	}

	if report.Action != AlignAligned {
		t.Fatalf("动作 = %q; want %q", report.Action, AlignAligned)
	}
	if report.Rows != 2 {
		t.Errorf("保留行数 = %d; want 2", report.Rows)
	}
	if report.Backup == "" {
		t.Error("默认要先备份")
	} else if _, err := os.Stat(report.Backup); err != nil {
		t.Errorf("备份文件不存在：%v", err)
	}
	for _, name := range columns {
		if containsString(oldLogColumns, name) {
			continue
		}
		if !containsString(report.Added, name) {
			t.Errorf("新增列清单缺少 %s", name)
		}
	}
	if len(report.Removed) != 0 {
		t.Errorf("被删列 = %v（配置里应包含旧列）", report.Removed)
	}

	aligned, err := repository.TableColumns("operation_log")
	if err != nil {
		t.Fatalf("读列失败：%v", err)
	}
	if strings.Join(aligned, "|") != strings.Join(columns, "|") {
		t.Fatalf("对齐后的列与配置不一致")
	}
	if len(aligned) != 80 {
		t.Fatalf("列数 = %d; want 80", len(aligned))
	}
	// 老行原样保留：details 仍是 NULL，配置新增的列是空
	rows, err := repository.Query(
		`SELECT "Primary DI", "Catalog or Reference Number", material_code, details IS NULL, "Device Description" FROM operation_log ORDER BY "Primary DI"`)
	if err != nil {
		t.Fatalf("读行失败：%v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("行数 = %d", len(rows))
	}
	if strings.Join(rows[0], "|") != "DI-1|SKU-1|MAT-1|1|" {
		t.Fatalf("第一行 = %v", rows[0])
	}
	if strings.Join(rows[1], "|") != "DI-2|SKU-2|MAT-2|1|" {
		t.Fatalf("第二行 = %v", rows[1])
	}
}

func TestR23AligningAnAlignedTableWritesNothing(t *testing.T) {
	loader := newLoader(t)
	repository := newRepository(t)
	writeOlderTable(t, repository, "operation_log")
	columns := logColumnsOf(t, loader)
	if _, err := AlignDatabase(repository.Path, "operation_log", columns, true); err != nil {
		t.Fatalf("第一次对齐失败：%v", err)
	}

	report, err := AlignDatabase(repository.Path, "operation_log", columns, true)
	if err != nil {
		t.Fatalf("第二次对齐失败：%v", err)
	}

	if report.Action != AlignAlreadyAligned {
		t.Fatalf("动作 = %q; want %q", report.Action, AlignAlreadyAligned)
	}
	if report.Backup != "" {
		t.Errorf("已经对齐时不该备份：%q", report.Backup)
	}
	if report.Rows != 2 {
		t.Errorf("行数 = %d; want 2", report.Rows)
	}
}

func TestR23AlignCreatesTheMissingTable(t *testing.T) {
	loader := newLoader(t)
	repository := newRepository(t)
	columns := logColumnsOf(t, loader)

	report, err := AlignDatabase(repository.Path, "operation_log", columns, true)
	if err != nil {
		t.Fatalf("对齐失败：%v", err)
	}

	if report.Action != AlignCreated {
		t.Fatalf("动作 = %q; want %q", report.Action, AlignCreated)
	}
	if report.Backup != "" {
		t.Errorf("建表不需要备份：%q", report.Backup)
	}
	aligned, err := repository.TableColumns("operation_log")
	if err != nil {
		t.Fatalf("读列失败：%v", err)
	}
	if strings.Join(aligned, "|") != strings.Join(columns, "|") {
		t.Fatalf("新建的表列与配置不一致")
	}
}

func TestR23AlignRefusesToDropAConstraint(t *testing.T) {
	loader := newLoader(t)
	repository := newRepository(t)
	if err := repository.Execute(
		`CREATE TABLE operation_log ("Primary DI" VARCHAR UNIQUE, operation VARCHAR, log_time VARCHAR)`); err != nil {
		t.Fatalf("建表失败：%v", err)
	}
	if err := repository.Execute(
		`INSERT INTO operation_log VALUES ('DI-1', 'import', '2026/09/11 23:47:31')`); err != nil {
		t.Fatalf("插行失败：%v", err)
	}
	columns := logColumnsOf(t, loader)

	_, err := AlignDatabase(repository.Path, "operation_log", columns, true)

	if err == nil || !strings.Contains(err.Error(), "constraint") {
		t.Fatalf("带约束的表必须被拒绝，得到：%v", err)
	}
	// 表保持原样：列没变、行还在
	existing, err := repository.TableColumns("operation_log")
	if err != nil {
		t.Fatalf("读列失败：%v", err)
	}
	if strings.Join(existing, "|") != "Primary DI|operation|log_time" {
		t.Fatalf("表被改动了：%v", existing)
	}
	count, err := repository.CountTableRows("operation_log")
	if err != nil || count != 1 {
		t.Fatalf("行数 = %d（err=%v）", count, err)
	}
}

func TestR23AlignDoesNotLeaveAStagingTableBehind(t *testing.T) {
	loader := newLoader(t)
	repository := newRepository(t)
	writeOlderTable(t, repository, "operation_log")
	if _, err := AlignDatabase(repository.Path, "operation_log", logColumnsOf(t, loader), false); err != nil {
		t.Fatalf("对齐失败：%v", err)
	}
	names, err := repository.TableNames()
	if err != nil {
		t.Fatalf("列表名失败：%v", err)
	}
	for _, name := range names {
		if strings.HasSuffix(name, "__aligned") {
			t.Fatalf("临时表没有清理：%v", names)
		}
	}
	// 不备份时不应产生备份文件
	entries, err := filepath.Glob(filepath.Join(filepath.Dir(repository.Path), "*-backup-*"))
	if err != nil {
		t.Fatalf("找备份失败：%v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("--no-backup 不该产生备份：%v", entries)
	}
}
