package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/jiegun314/ssr-go/internal/buildinfo"
	"github.com/jiegun314/ssr-go/internal/consolidation"
	"github.com/jiegun314/ssr-go/internal/importer"
	"github.com/jiegun314/ssr-go/internal/store"
)

// 这一组方法通过 window.go.main.App.* 暴露给前端。

// InitialState 返回界面启动时需要的一切（版本、操作日志、四个来源的状态）。
func (app *App) InitialState() State {
	app.mu.Lock()
	defer app.mu.Unlock()
	version := ""
	if app.loader != nil {
		if setting, err := app.loader.LoadSetting(); err == nil {
			version = text(setting["APP_VERSION"])
		}
	}
	return State{
		Version:      version,
		OperationLog: app.logText(),
		Imports:      app.importStates,
		Busy:         app.busy,
	}
}

// ImportResult 是一次导入回给界面的结果。
type ImportResult struct {
	Source   string      `json:"source"`
	FileName string      `json:"fileName"`
	RowCount int         `json:"rowCount"`
	State    ImportState `json:"state"`
	Log      string      `json:"log"`
	Title    string      `json:"title"`
	Message  string      `json:"message"`
	Failed   bool        `json:"failed"`
}

// ImportSource 导入一份来源：先弹「Select Excel File」，再按 R2–R8 导入（§5.2）。
func (app *App) ImportSource(source string) ImportResult {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.beginOperation(); err != nil {
		return ImportResult{Source: source, Failed: true, Title: "Error", Message: err.Error()}
	}
	defer app.endOperation()
	path, err := app.selectFile()
	if err != nil || path == "" {
		// 用户取消文件对话框不算失败：状态与标签都不变（§5.2）
		return ImportResult{Source: source, State: app.importStates[source], Log: app.logText()}
	}
	rule := app.importer.Rules[source]
	if _, importErr := app.importer.Import(source, path); importErr != nil {
		return app.markImportFailed(source, importErr)
	}
	app.markImported(source, filepath.Base(path))
	state := app.importStates[source]
	return ImportResult{
		Source:   source,
		FileName: filepath.Base(path),
		RowCount: state.RowCount,
		State:    state,
		Log:      app.logText(),
		Title:    "Success",
		Message: fmt.Sprintf("%s imported successfully. Rows imported: %d",
			rule.ChineseName, state.RowCount),
	}
}

// markImportFailed 走两条失败通道：缺值被拒（弹窗只给汇总、明细进日志）与其他错误（§5.2）。
func (app *App) markImportFailed(source string, importErr error) ImportResult {
	app.importStates[source] = ImportState{
		Source: source, State: "failed", Label: "导入失败",
		Tooltip: "导入失败，详情见日志窗口",
	}
	result := ImportResult{Source: source, Failed: true, State: app.importStates[source]}
	var missing *importer.MissingRowValuesError
	if errors.As(importErr, &missing) {
		app.appendLog("Import failed: " + missing.Summary() + "\n" + missing.Error())
		result.Title = "数据缺失"
		result.Message = missing.Summary() + "\n详情见日志窗口。"
	} else {
		app.appendLog("Import failed:\n" + importErr.Error())
		result.Title = "Error"
		result.Message = importErr.Error()
	}
	result.Log = app.logText()
	return result
}

// ClearResult 是「清空导入数据」的结果（R25）。
type ClearResult struct {
	Log     string   `json:"log"`
	Title   string   `json:"title"`
	Message string   `json:"message"`
	Cleared []string `json:"cleared"`
	Failed  bool     `json:"failed"`
}

// ClearImportedData 只清「从来源文件导入进来」的表：医保编码、整合结果与操作日志都不动。
func (app *App) ClearImportedData() ClearResult {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.beginOperation(); err != nil {
		return ClearResult{Failed: true, Title: "Error", Message: err.Error()}
	}
	defer app.endOperation()
	names, err := store.SourceCleanupNames(app.loader)
	if err != nil {
		return ClearResult{Failed: true, Title: "Error", Message: err.Error()}
	}
	tableNames, err := store.SourceCleanupTableNames(app.loader)
	if err != nil {
		return ClearResult{Failed: true, Title: "Error", Message: err.Error()}
	}
	if _, failed := store.DropTables(app.repo, tableNames); len(failed) > 0 {
		message := store.FormatTableFailure(failed)
		app.appendLog("Failed to clean imported data: " + message)
		// 失败时界面状态不变：数据还在库里
		return ClearResult{Log: app.logText(), Title: "Error", Message: message, Failed: true}
	}
	labels := []string{}
	for _, source := range names {
		labels = append(labels, app.importer.Rules[source].ChineseName)
		app.importStates[source] = ImportState{
			Source: source, State: "empty", Label: "已清空", Tooltip: "尚未导入",
		}
	}
	app.appendLog("Imported data cleared successfully: " + strings.Join(labels, ", "))
	message := ""
	switch len(labels) {
	case 0:
		message = "数据已清空"
	case 1:
		message = labels[0] + "数据已清空"
	default:
		message = strings.Join(labels[:len(labels)-1], ",") + "和" + labels[len(labels)-1] + "数据已清空"
	}
	return ClearResult{Log: app.logText(), Title: "Success", Message: message, Cleared: names}
}

// ConsolidateResult 是一次整合回给界面的结果（结果表按状态着色，MISSING 加粗）。
type ConsolidateResult struct {
	Log      string     `json:"log"`
	Title    string     `json:"title"`
	Message  string     `json:"message"`
	Failed   bool       `json:"failed"`
	Columns  []string   `json:"columns"`
	Rows     [][]string `json:"rows"`
	Statuses []string   `json:"statuses"`
}

// Consolidate 跑整合并回传结果表（§5.4）。
func (app *App) Consolidate() ConsolidateResult {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.beginOperation(); err != nil {
		return ConsolidateResult{Failed: true, Title: "Error", Message: err.Error()}
	}
	defer app.endOperation()
	outcome, err := app.service.Consolidate()
	if err != nil {
		app.appendLog("Consolidation failed: " + err.Error())
		return ConsolidateResult{Log: app.logText(), Title: "数据缺失", Message: err.Error(), Failed: true}
	}
	counts := map[string]int{}
	for _, row := range outcome.Rows {
		counts[row.Values["status"]]++
	}
	app.appendLog(fmt.Sprintf(
		"Consolidation completed successfully. Rows consolidated: %d. Rows missing: %d. "+
			"Rows duplicate: %d. Rows changed: %d. Conflicts: %d",
		len(outcome.Rows), counts[consolidation.StatusIncomplete],
		counts[consolidation.StatusDuplicate], len(outcome.ChangedData), len(outcome.ConflictData)))
	for _, detail := range []struct {
		name string
		list []string
	}{
		{"Missing data", outcome.MissingData},
		{"Duplicate data", outcome.DuplicateData},
		{"Changed data", outcome.ChangedData},
		{"Conflict data", outcome.ConflictData},
	} {
		if len(detail.list) > 0 {
			app.appendLog(fmt.Sprintf("%s: [%s]", detail.name, strings.Join(detail.list, ", ")))
		}
	}
	columns := append([]string{"status"}, app.service.Config.FieldNames...)
	rows := make([][]string, 0, len(outcome.Rows))
	statuses := make([]string, 0, len(outcome.Rows))
	for _, row := range outcome.Rows {
		values := make([]string, 0, len(columns))
		for _, column := range columns {
			values = append(values, row.Values[column])
		}
		rows = append(rows, values)
		statuses = append(statuses, row.Values["status"])
	}
	return ConsolidateResult{Log: app.logText(), Columns: columns, Rows: rows, Statuses: statuses}
}

// ExportResult 是一次导出回给界面的结果。
type ExportResult struct {
	Log     string `json:"log"`
	Title   string `json:"title"`
	Message string `json:"message"`
	Failed  bool   `json:"failed"`
	Path    string `json:"path"`
}

// Export 弹保存对话框并把 Ready 行写进模板副本（R20/R21）。
func (app *App) Export() ExportResult {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.beginOperation(); err != nil {
		return ExportResult{Failed: true, Title: "Export Error", Message: err.Error()}
	}
	defer app.endOperation()
	now := time.Now()
	defaultName := app.service.DefaultExportFileName(now)
	target, err := app.saveFile(defaultName)
	if err != nil || target == "" {
		// 用户取消：什么都不做
		return ExportResult{Log: app.logText()}
	}
	exported, err := app.service.ExportConsolidationResult(defaultName, now)
	if err != nil {
		app.appendLog("Export failed: " + err.Error())
		return ExportResult{Log: app.logText(), Title: "Export Error", Message: err.Error(), Failed: true}
	}
	// 用户路径与导出目录里的副本重合时跳过复制（R20，否则会抛 SameFileError）
	if filepath.Clean(target) != filepath.Clean(exported.FilePath) {
		if err := copyFile(exported.FilePath, target); err != nil {
			app.appendLog("Export failed: " + err.Error())
			return ExportResult{Log: app.logText(), Title: "Export Error", Message: err.Error(), Failed: true}
		}
	}
	if err := app.service.RecordConsolidationResult(now); err != nil {
		app.appendLog(err.Error())
	}
	app.appendLog("Consolidation result exported successfully to " + target)
	app.appendLog("Exported file path: " + target)
	return ExportResult{
		Log: app.logText(), Title: "Success",
		Message: "Consolidation result exported successfully.", Path: target,
	}
}

// ReviewResult 是数据回顾 / 日志回顾回给界面的内容。
type ReviewResult struct {
	Log     string     `json:"log"`
	Title   string     `json:"title"`
	Message string     `json:"message"`
	Failed  bool       `json:"failed"`
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

// ReviewSource 读一个来源暂存表的全部数据，列名用中文表头（§5.3）。
//
// 医保编码来源同样可以回顾 —— 这是 #3 的修复：现状实现里那个分支被注释掉了。
func (app *App) ReviewSource(source string) ReviewResult {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.beginOperation(); err != nil {
		return ReviewResult{Failed: true, Title: "Error", Message: err.Error()}
	}
	defer app.endOperation()
	rule := app.importer.Rules[source]
	rows, err := app.repo.Rows(rule.TargetTable)
	if err != nil {
		return ReviewResult{Log: app.logText(), Failed: true, Title: "Error", Message: err.Error()}
	}
	columns := make([]string, 0, len(rule.Columns))
	for _, column := range rule.Columns {
		columns = append(columns, column.ChineseName)
	}
	table := make([][]string, 0, len(rows))
	for _, row := range rows {
		values := make([]string, 0, len(rule.Columns))
		for _, column := range rule.Columns {
			values = append(values, row[column.DBField])
		}
		table = append(table, values)
	}
	app.appendLog(fmt.Sprintf(
		"Data review completed successfully for %s. Rows reviewed: %d",
		rule.ChineseName, len(table)))
	return ReviewResult{
		Log: app.logText(), Title: "Data Review - " + source, Columns: columns, Rows: table,
	}
}

// ReviewLog 按时间区间读操作日志（§5.6）。
func (app *App) ReviewLog(start string, end string) ReviewResult {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.beginOperation(); err != nil {
		return ReviewResult{Failed: true, Title: "Error", Message: err.Error()}
	}
	defer app.endOperation()
	rows, err := app.log.ReadByTime(start, end)
	if err != nil {
		return ReviewResult{Log: app.logText(), Failed: true, Title: "Error", Message: err.Error()}
	}
	table := make([][]string, 0, len(rows))
	for _, row := range rows {
		values := make([]string, 0, len(app.log.LogColumns))
		for _, column := range app.log.LogColumns {
			values = append(values, row[column])
		}
		table = append(table, values)
	}
	app.appendLog(fmt.Sprintf(
		"Operation log review completed successfully for time range %s to %s. Rows reviewed: %d",
		start, end, len(table)))
	return ReviewResult{
		Log: app.logText(), Title: "Operation Log Review - " + start + " to " + end,
		Columns: app.log.TableColumns(), Rows: table,
	}
}

// About 是「关于」窗口需要的内容（版本 + tooltip 明细，§6.4）。
func (app *App) About() map[string]string {
	version := ""
	if app.loader != nil {
		if setting, err := app.loader.LoadSetting(); err == nil {
			version = text(setting["APP_VERSION"])
		}
	}
	info := buildinfo.ReadBuildInfo(buildinfo.Options{AppVersion: version})
	return map[string]string{
		"name":    "SingleSourceReady",
		"version": "Version: " + info.Version,
		"detail":  info.Detail(),
	}
}

// ShowAbout 由菜单「关于」调用：把版本信息推给前端弹窗。
func (app *App) ShowAbout() {
	if app.context == nil {
		return
	}
	runtime.EventsEmit(app.context, "show-about", app.About())
}
