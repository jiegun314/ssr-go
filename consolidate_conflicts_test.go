package main

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/config"
	"github.com/jiegun314/ssr-go/internal/consolidation"
	"github.com/jiegun314/ssr-go/internal/store"
)

// TestConsolidateReturnsAlignedConflictCells 固定界面接口的单元格冲突契约（AGENTS.md §11 第 4 步）：
// Conflicts 与 Rows 同形同序（同一行、同一列下标 = Columns），
// 只有冲突的格有提示，其余都是空串（首列 status 永远是空串）。
func TestConsolidateReturnsAlignedConflictCells(t *testing.T) {
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, "config")
	copyDirectoryForTest(t, "config", configDir)
	importEverySource(t, configDir, filepath.Join(workspace, "data", "udi_data.sqlite3"))
	t.Setenv(config.CleanupOnStartupEnv, "")
	app := startAppForTest(t, configDir)

	// 造一条冲突：同一 material_code + DI 的两行只有数量不同（R14）
	original := ""
	table := app.service.Config.SourceTables["global_udi_input"]
	rewriteTableRows(t, app.service.Repository, table, func(rows []store.Row) []store.Row {
		original = rows[0]["quantity_per_min_sales_unit"]
		conflictRow := store.Row{}
		for column, value := range rows[0] {
			conflictRow[column] = value
		}
		conflictRow["quantity_per_min_sales_unit"] = "999"
		return append(rows, conflictRow)
	})

	result := app.Consolidate()
	if result.Failed {
		t.Fatalf("整合失败：%s", result.Message)
	}
	if len(result.Conflicts) != len(result.Rows) {
		t.Fatalf("Conflicts 有 %d 行; want %d（与 Rows 同序）",
			len(result.Conflicts), len(result.Rows))
	}

	// Columns = ["status", ...FieldNames]，提示矩阵与它逐列对齐
	column := -1
	for index, name := range result.Columns {
		if name == "Quantity of Device" {
			column = index
		}
	}
	if column < 0 {
		t.Fatalf("导出列 Quantity of Device 不在 Columns 里：%v", result.Columns)
	}
	candidates := []string{original, "999"}
	sort.Strings(candidates)
	marked := 0
	for index, notes := range result.Conflicts {
		if len(notes) != len(result.Columns) {
			t.Fatalf("第 %d 行的提示有 %d 格; want %d（与 Columns 同形同序）",
				index, len(notes), len(result.Columns))
		}
		for position, note := range notes {
			if position == column && result.Statuses[index] == consolidation.StatusConflict {
				if !strings.Contains(note, strings.Join(candidates, ", ")) {
					t.Errorf("第 %d 行 %q 的提示 = %q; want 含 %q",
						index, result.Columns[position], note, strings.Join(candidates, ", "))
				}
				marked++
				continue
			}
			if note != "" {
				t.Errorf("第 %d 行 %q 不该有提示：%q", index, result.Columns[position], note)
			}
		}
	}
	if marked != 1 {
		t.Fatalf("被标记的冲突格 = %d; want 1", marked)
	}
}

// rewriteTableRows 读出整张表、按 mutate 改写后写回（列顺序不变）。
func rewriteTableRows(
	t *testing.T,
	repository *store.Repository,
	tableName string,
	mutate func(rows []store.Row) []store.Row,
) {
	t.Helper()
	columns, err := repository.TableColumns(tableName)
	if err != nil {
		t.Fatalf("读列失败：%v", err)
	}
	rows, err := repository.Rows(tableName)
	if err != nil {
		t.Fatalf("读行失败：%v", err)
	}
	if err := repository.WriteRows(tableName, columns, mutate(rows), store.WriteReplace); err != nil {
		t.Fatalf("写回失败：%v", err)
	}
}
