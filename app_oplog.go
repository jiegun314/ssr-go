package main

// 操作日志的全量同步（用户反馈：记录导出所有人可见，所以日志要全量同步）。
//
// 口径：
//   - 本机 operation_log 与共享盘 <root>/log/operation_log.csv 取并集（整行去重）；
//   - 并集里本机没有的行写回本机，共享盘没有的行发布到共享盘；
//   - 全程在 state.lock 里做，避免两台机器同时导出时丢行；
//   - 同步失败只记日志，不影响导出/回顾本身。

import (
	"errors"
	"fmt"
	"os"

	"github.com/jiegun314/ssr-go/internal/oplogsync"
	"github.com/jiegun314/ssr-go/internal/share"
	"github.com/jiegun314/ssr-go/internal/store"
)

// syncOperationLog 把本机日志与共享盘合并一次（调用方持有 app.mu）。
func (app *App) syncOperationLog() error {
	if !app.shareConfig.Enabled() || app.log == nil || app.repo == nil {
		return nil
	}
	columns := app.log.TableColumns()
	root := app.shareConfig.Root
	err := share.WithStateLock(root, func() error {
		shared, pullErr := oplogsync.Pull(root)
		if pullErr != nil && !errors.Is(pullErr, os.ErrNotExist) {
			return pullErr
		}
		localRows, err := app.repo.Rows(app.log.TableName)
		if err != nil {
			return err
		}
		local := matrixFromRows(columns, localRows)
		merged, _ := oplogsync.Merge(shared.Rows, local)
		// 并集只写"回顾表"（「记录导出」读它）；本机日志保持原样，
		// 否则未变判定会把别人导出过的记录当成"本机已导出"，导致整份数据被判重复。
		existing, existsErr := app.repo.Rows(store.ReviewLogTable)
		if existsErr != nil {
			existing = nil
		}
		currentReview := matrixFromRows(columns, existing)
		if !sameMatrix(merged, currentReview) {
			if err := app.repo.WriteRows(store.ReviewLogTable, columns, rowsFromMatrix(columns, merged), store.WriteReplace); err != nil {
				return err
			}
			app.appendLog(LogSuccess, fmt.Sprintf("记录导出已同步（共 %d 条）", len(merged)))
		}
		if !sameMatrix(merged, shared.Rows) {
			manifest, err := oplogsync.Publish(root, columns, merged, app.owner().Name)
			if err != nil {
				return err
			}
			app.appendLog(LogSuccess, fmt.Sprintf("操作日志已同步到共享盘（共 %d 条）", manifest.Rows))
		}
		return nil
	})
	if err != nil {
		app.appendLog(LogWarning, "操作日志同步失败："+err.Error())
		return err
	}
	return nil
}

// matrixFromRows 按列顺序把 store.Row 摊平成矩阵。
func matrixFromRows(columns []string, rows []store.Row) [][]string {
	matrix := make([][]string, 0, len(rows))
	for _, row := range rows {
		values := make([]string, 0, len(columns))
		for _, column := range columns {
			values = append(values, row[column])
		}
		matrix = append(matrix, values)
	}
	return matrix
}

// rowsFromMatrix 把矩阵还原成按列的 store.Row。
func rowsFromMatrix(columns []string, matrix [][]string) []store.Row {
	rows := make([]store.Row, 0, len(matrix))
	for _, values := range matrix {
		row := store.Row{}
		for index, column := range columns {
			if index < len(values) {
				row[column] = values[index]
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func sameMatrix(left, right [][]string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if len(left[index]) != len(right[index]) {
			return false
		}
		for column := range left[index] {
			if left[index][column] != right[index][column] {
				return false
			}
		}
	}
	return true
}

// reviewLogRows 读「记录导出」的数据：共享盘模式读并集表（没有就回退本机），本地模式读本机。
func (app *App) reviewLogRows(start string, end string) ([]store.Row, error) {
	table := app.log.TableName
	if app.shareConfig.Enabled() {
		if exists, err := app.repo.TableExists(store.ReviewLogTable); err == nil && exists {
			table = store.ReviewLogTable
		}
	}
	rows, err := app.repo.Rows(table)
	if err != nil {
		return nil, err
	}
	timeColumn := app.log.TimeColumn
	filtered := make([]store.Row, 0, len(rows))
	for _, row := range rows {
		stamp := row[timeColumn]
		if start != "" && stamp < start {
			continue
		}
		if end != "" && stamp > end {
			continue
		}
		filtered = append(filtered, row)
	}
	return filtered, nil
}
