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
	"github.com/jiegun314/ssr-go/internal/excelio"
	"github.com/jiegun314/ssr-go/internal/fileutil"
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

// SelectImportFile 打开「Select Excel File」对话框并返回用户选中的文件（取消返回空串）。
//
// 与导入分成两步，是为了让载入图层能显示准确的阶段：打开对话框时是「正在打开文件夹」，
// 选中文件真正开始导入时才是「正在导入 Excel 数据...」。取消不算失败（§5.2）。
func (app *App) SelectImportFile(source string) string {
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.importer == nil {
		return ""
	}
	path, err := app.selectFile()
	if err != nil {
		return ""
	}
	return path
}

// ImportSource 导入一份来源：filePath 为空时什么都不做（用户取消）否则按 R2–R8 导入（§5.2）。
func (app *App) ImportSource(source string, filePath string) ImportResult {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.beginOperation(); err != nil {
		return ImportResult{Source: source, Failed: true, Title: "错误", Message: err.Error()}
	}
	defer app.endOperation()
	path := filePath
	if path == "" {
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
		// 成功反馈固定两行中文：第一行「{来源中文名}导入成功」，第二行行数
		Title:   "导入成功",
		Message: fmt.Sprintf("%s导入成功\n导入行数：%d", rule.ChineseName, state.RowCount),
	}
}

// markImportFailed 走两条失败通道：缺值被拒（弹窗只给汇总、明细进日志）与其他错误（§5.2）。
func (app *App) markImportFailed(source string, importErr error) ImportResult {
	app.importStates[source] = ImportState{
		Source: source, State: "failed", Label: "导入失败",
		Tooltip: "导入失败，详情见日志窗口",
	}
	result := ImportResult{Source: source, Failed: true, State: app.importStates[source]}
	chineseName := source
	if app.importer != nil {
		chineseName = app.importer.Rules[source].ChineseName
	}
	var missing *importer.MissingRowValuesError
	if errors.As(importErr, &missing) {
		app.appendLog(LogError, fmt.Sprintf("%s导入失败：%s\n%s",
			chineseName, missing.Summary(), missing.Error()))
		result.Title = "导入失败"
		// 汇总本身以「{来源中文名}：」开头，第一行已经写了来源名，这里去掉避免重复
		result.Message = fmt.Sprintf("%s导入失败\n失败原因：%s\n详情见日志窗口。",
			chineseName, strings.TrimPrefix(missing.Summary(), chineseName+"："))
	} else if containsCJK(importErr.Error()) {
		// 校验类错误的文案本来就是中文（表头缺失、必填列为空…），原样展示
		app.appendLog(LogError, chineseName+"导入失败：\n"+importErr.Error())
		result.Title = "导入失败"
		result.Message = fmt.Sprintf("%s导入失败\n失败原因：%s", chineseName, importErr.Error())
	} else {
		// 读文件失败这类底层错误是英文的：日志与弹窗都给中文结论，英文原文作为「技术详情」跟在后面
		app.appendLog(LogError, fmt.Sprintf(
			"%s导入失败：文件无法读取或格式不受支持（请确认是 .xlsx、未被 Excel 占用且未损坏）。\n技术详情：%s",
			chineseName, importErr.Error()))
		result.Title = "导入失败"
		result.Message = fmt.Sprintf(
			"%s导入失败\n失败原因：文件无法读取或格式不受支持（请确认是 .xlsx、未被 Excel 占用且未损坏）。\n详情见日志窗口。",
			chineseName)
	}
	result.Log = app.logText()
	return result
}

// CopyToClipboard 把文本写进系统剪贴板：日志详情弹窗的「复制」走这条路。
//
// Wails 的 webview 不一定把应用页面当成安全上下文，浏览器 Clipboard API 可能被拒；
// 运行时自带的剪贴板不受这个限制，所以前端优先调它，失败再退回浏览器兜底方案。
func (app *App) CopyToClipboard(text string) bool {
	app.mu.Lock()
	ctx := app.context
	app.mu.Unlock()
	if ctx == nil {
		return false
	}
	return runtime.ClipboardSetText(ctx, text) == nil
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
		return ClearResult{Failed: true, Title: "错误", Message: err.Error()}
	}
	defer app.endOperation()
	names, err := store.SourceCleanupNames(app.loader)
	if err != nil {
		return ClearResult{Failed: true, Title: "错误", Message: err.Error()}
	}
	tableNames, err := store.SourceCleanupTableNames(app.loader)
	if err != nil {
		return ClearResult{Failed: true, Title: "错误", Message: err.Error()}
	}
	if _, failed := store.DropTables(app.repo, tableNames); len(failed) > 0 {
		message := store.FormatTableFailure(failed)
		app.appendLog(LogError, "清空导入数据失败："+message)
		// 失败时界面状态不变：数据还在库里
		return ClearResult{Log: app.logText(), Title: "错误", Message: message, Failed: true}
	}
	labels := []string{}
	for _, source := range names {
		labels = append(labels, app.importer.Rules[source].ChineseName)
		app.importStates[source] = ImportState{
			Source: source, State: "empty", Label: "已清空", Tooltip: "尚未导入",
		}
	}
	app.appendLog(LogSuccess, "导入数据已清空："+strings.Join(labels, "、"))
	message := ""
	switch len(labels) {
	case 0:
		message = "数据已清空"
	case 1:
		message = labels[0] + "数据已清空"
	default:
		message = strings.Join(labels[:len(labels)-1], ",") + "和" + labels[len(labels)-1] + "数据已清空"
	}
	return ClearResult{Log: app.logText(), Title: "成功", Message: message, Cleared: names}
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
		return ConsolidateResult{Failed: true, Title: "错误", Message: err.Error()}
	}
	defer app.endOperation()
	outcome, err := app.service.Consolidate()
	if err != nil {
		app.appendLog(LogError, "数据整合失败："+err.Error())
		return ConsolidateResult{Log: app.logText(), Title: "数据缺失", Message: err.Error(), Failed: true}
	}
	counts := map[string]int{}
	for _, row := range outcome.Rows {
		counts[row.Values["status"]]++
	}
	// 只给汇总：合格 / 缺失 / 重复 三个行数（有变更、冲突时再补两个计数），
	// 不逐条展开缺失、重复的明细 —— 具体哪些记录有问题，看结果表按状态筛选即可。
	summary := []string{
		fmt.Sprintf("合格 %d 行", counts[consolidation.StatusReady]),
		fmt.Sprintf("缺失 %d 行", counts[consolidation.StatusIncomplete]),
		fmt.Sprintf("重复 %d 行", counts[consolidation.StatusDuplicate]),
	}
	if changed := len(outcome.ChangedData); changed > 0 {
		summary = append(summary, fmt.Sprintf("变更 %d 行", changed))
	}
	if conflicts := len(outcome.ConflictData); conflicts > 0 {
		summary = append(summary, fmt.Sprintf("冲突 %d 行", conflicts))
	}
	app.appendLog(LogSuccess, "数据整合完成："+strings.Join(summary, "，"))
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

// ExportTarget 是保存对话框的结果：默认文件名（= 导出目录里那份副本的名字，R20）
// 与用户选中的路径；Target 为空表示用户取消。
type ExportTarget struct {
	DefaultName string `json:"defaultName"`
	Target      string `json:"target"`
}

// SelectExportTarget 打开「Save Consolidation Result」对话框并返回选择结果。
//
// 与导出分成两步，让载入图层显示准确阶段：对话框阶段是「正在选择保存位置」，
// 真正写文件时才是「正在导出文件...」。
func (app *App) SelectExportTarget() ExportTarget {
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.importer == nil {
		return ExportTarget{}
	}
	defaultName := app.service.DefaultExportFileName(time.Now())
	target, err := app.saveFile(defaultName)
	if err != nil {
		return ExportTarget{DefaultName: defaultName}
	}
	return ExportTarget{DefaultName: defaultName, Target: target}
}

// Export 把 Ready 行写进模板副本（导出目录留副本），再复制到用户选中的路径（R20/R21）。
func (app *App) Export(fileName string, target string) ExportResult {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.beginOperation(); err != nil {
		return ExportResult{Failed: true, Title: "导出失败", Message: err.Error()}
	}
	defer app.endOperation()
	if target == "" {
		// 用户取消：什么都不做
		return ExportResult{Log: app.logText()}
	}
	now := time.Now()
	defaultName := fileName
	if defaultName == "" {
		defaultName = app.service.DefaultExportFileName(now)
	}
	exported, err := app.service.ExportConsolidationResult(defaultName, now)
	if err != nil {
		app.appendLog(LogError, "导出失败："+err.Error())
		return ExportResult{Log: app.logText(), Title: "导出失败", Message: err.Error(), Failed: true}
	}
	// 用户路径与导出目录里的副本重合时跳过复制（R20，否则会抛 SameFileError）
	if filepath.Clean(target) != filepath.Clean(exported.FilePath) {
		if err := fileutil.CopyFile(exported.FilePath, target); err != nil {
			app.appendLog(LogError, "导出失败："+err.Error())
			return ExportResult{Log: app.logText(), Title: "导出失败", Message: err.Error(), Failed: true}
		}
	}
	if err := app.service.RecordConsolidationResult(now); err != nil {
		app.appendLog(LogError, "记录整合结果失败："+err.Error())
	}
	app.appendLog(LogSuccess, "整合结果已导出到 "+target)
	return ExportResult{
		Log: app.logText(), Title: "成功",
		Message: "整合结果已导出。", Path: target,
	}
}

// ReviewLog 按时间区间读操作日志，并像来源回顾一样分页（§5.6）。
//
// 运行日志只在第 1 页写一行，与现状实现的开窗时机一致。
func (app *App) ReviewLog(start string, end string, page int, pageSize int) ReviewResult {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.beginOperation(); err != nil {
		return ReviewResult{Failed: true, Title: "错误", Message: err.Error()}
	}
	defer app.endOperation()
	if pageSize <= 0 {
		pageSize = 100
	}
	if page < 1 {
		page = 1
	}
	rows, err := app.log.ReadByTime(start, end)
	if err != nil {
		return ReviewResult{Log: app.logText(), Failed: true, Title: "错误", Message: err.Error()}
	}
	columns := app.log.TableColumns()
	pageCount := (len(rows) + pageSize - 1) / pageSize
	if pageCount == 0 {
		pageCount = 1
	}
	if page > pageCount {
		page = pageCount
	}
	begin := (page - 1) * pageSize
	finish := begin + pageSize
	if finish > len(rows) {
		finish = len(rows)
	}
	table := make([][]string, 0, finish-begin)
	for _, row := range rows[begin:finish] {
		values := make([]string, 0, len(columns))
		for _, column := range columns {
			values = append(values, row[column])
		}
		table = append(table, values)
	}
	if page == 1 {
		app.appendLog(LogInfo, fmt.Sprintf(
			"记录回顾完成：时间范围 %s 至 %s，共 %d 行", start, end, len(rows)))
	}
	return ReviewResult{
		Log: app.logText(), Title: "记录回顾 · " + start + " 至 " + end,
		Columns: columns, Rows: table,
		Total: len(rows), Page: page, PageSize: pageSize, PageCount: pageCount,
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

// ReviewResult 是数据回顾 / 日志回顾回给界面的内容。两种回顾都按页返回：
// 有些来源一次导入上万行，全部塞给前端既慢又占内存，所以分页在 Go 侧做。
type ReviewResult struct {
	Log       string     `json:"log"`
	Title     string     `json:"title"`
	Message   string     `json:"message"`
	Failed    bool       `json:"failed"`
	Columns   []string   `json:"columns"`
	Rows      [][]string `json:"rows"`
	Total     int        `json:"total"`
	Page      int        `json:"page"`
	PageSize  int        `json:"pageSize"`
	PageCount int        `json:"pageCount"`
}

// ReviewSource 读一个来源暂存表的一页数据，列名用中文表头（§5.3）。
//
// 医保编码来源同样可以回顾 —— 这是 #3 的修复：现状实现里那个分支被注释掉了。
// 运行日志只在打开回顾窗口（第 1 页）时写一行，与现状实现的开窗时机一致。
func (app *App) ReviewSource(source string, page int, pageSize int) ReviewResult {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.beginOperation(); err != nil {
		return ReviewResult{Failed: true, Title: "错误", Message: err.Error()}
	}
	defer app.endOperation()
	if pageSize <= 0 {
		pageSize = 100
	}
	if page < 1 {
		page = 1
	}
	columns, rows, err := app.reviewData(source)
	if err != nil {
		return ReviewResult{Log: app.logText(), Failed: true, Title: "错误", Message: err.Error()}
	}
	pageCount := (len(rows) + pageSize - 1) / pageSize
	if pageCount == 0 {
		pageCount = 1
	}
	if page > pageCount {
		page = pageCount
	}
	begin := (page - 1) * pageSize
	finish := begin + pageSize
	if finish > len(rows) {
		finish = len(rows)
	}
	if page == 1 {
		app.appendLog(LogInfo, fmt.Sprintf(
			"%s数据回顾完成：共 %d 行", app.importer.Rules[source].ChineseName, len(rows)))
	}
	return ReviewResult{
		Log: app.logText(), Title: app.importer.Rules[source].ChineseName + "数据回顾",
		Columns: columns, Rows: rows[begin:finish],
		Total: len(rows), Page: page, PageSize: pageSize, PageCount: pageCount,
	}
}

// reviewData 返回一个回顾对象的列与全部数据。
//
// fileType 是来源键，或 operation_log —— 记录导出的回顾与来源回顾在现状实现里共用同一个
// 窗口类，所以这里也共用同一份数据与导出实现。
func (app *App) reviewData(fileType string) ([]string, [][]string, error) {
	if fileType == "operation_log" {
		rows, err := app.log.Repository.Rows(app.log.TableName)
		if err != nil {
			return nil, nil, err
		}
		columns := app.log.TableColumns()
		table := make([][]string, 0, len(rows))
		for _, row := range rows {
			values := make([]string, 0, len(columns))
			for _, column := range columns {
				values = append(values, row[column])
			}
			table = append(table, values)
		}
		return columns, table, nil
	}
	rule, known := app.importer.Rules[fileType]
	if !known {
		return nil, nil, fmt.Errorf("Invalid file type: %s", fileType)
	}
	rows, err := app.repo.Rows(rule.TargetTable)
	if err != nil {
		return nil, nil, err
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
	return columns, table, nil
}

// SelectReviewExportTarget 打开回顾窗口的「Save Imported Data」对话框。
//
// 与现状实现一致：默认文件名 `{fileType}_imported_data.xlsx`、过滤器 Excel Files (*.xlsx)；
// 记录导出的回顾用 fileType = operation_log，于是默认名是 operation_log_imported_data.xlsx。
func (app *App) SelectReviewExportTarget(fileType string) ExportTarget {
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.importer == nil {
		return ExportTarget{}
	}
	defaultName := fileType + "_imported_data.xlsx"
	target, err := runtime.SaveFileDialog(app.context, runtime.SaveDialogOptions{
		Title:           "保存导入数据",
		DefaultFilename: defaultName,
		Filters: []runtime.FileFilter{
			{DisplayName: "Excel 文件 (*.xlsx)", Pattern: "*.xlsx"},
		},
	})
	if err != nil {
		return ExportTarget{DefaultName: defaultName}
	}
	return ExportTarget{DefaultName: defaultName, Target: target}
}

// ExportReviewData 把回顾窗口里的全部数据写成 Excel（保存位置由上一步选定）。
//
// 成功会写一行运行日志（回顾数据已导出到 {路径}）并弹提示，失败弹「导出失败」。
func (app *App) ExportReviewData(fileType string, target string) ExportResult {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.beginOperation(); err != nil {
		return ExportResult{Failed: true, Title: "导出失败", Message: err.Error()}
	}
	defer app.endOperation()
	if target == "" {
		return ExportResult{Log: app.logText()}
	}
	columns, rows, err := app.reviewData(fileType)
	if err != nil {
		return ExportResult{Log: app.logText(), Title: "导出失败", Message: err.Error(), Failed: true}
	}
	if err := excelio.WriteReviewWorkbook(target, columns, rows); err != nil {
		return ExportResult{Log: app.logText(), Title: "导出失败", Message: err.Error(), Failed: true}
	}
	app.appendLog(LogSuccess, fmt.Sprintf("回顾数据已导出到 %s（%d 行）", target, len(rows)))
	return ExportResult{
		Log: app.logText(), Title: "成功", Path: target,
		Message: fmt.Sprintf("已导出 %d 行到：\n%s", len(rows), target),
	}
}

// containsCJK 判断一段文案里有没有中文字符：用来区分"校验类中文提示"与
// "底层英文技术错误"（后者在弹窗里换成中文兜底，原文仍写进操作日志）。
func containsCJK(text string) bool {
	for _, r := range text {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}
