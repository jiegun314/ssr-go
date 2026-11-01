package main

import (
	"errors"
	"fmt"
	"path/filepath"
	stdruntime "runtime"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/jiegun314/ssr-go/internal/auth"
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
		Session:      app.sessionInfo(),
		Roles:        loginRoles(),
		Username:     auth.CurrentUsername(),
		SingleUser:   app.singleUser,
		Share:        app.shareStatus,
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
	// PublishedVersion 是发布到共享盘的版本号（本地模式为空）。
	PublishedVersion string `json:"publishedVersion"`
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
	if err := app.authorizeSource(source, true); err != nil {
		app.appendLog(LogWarning, "导入被拒绝："+err.Error())
		return ""
	}
	if err := app.requireShare("导入/发布"); err != nil {
		app.appendLog(LogError, "导入被拒绝："+err.Error())
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
		return ImportResult{Source: source, Failed: true, Title: "错误", Message: userMessage(err)}
	}
	defer app.endOperation()
	if err := app.authorizeSource(source, true); err != nil {
		app.audit("deny", source, "导入", "denied")
		return ImportResult{Source: source, Failed: true, Title: "无权限", Message: userMessage(err)}
	}
	if err := app.requireShare("导入/发布"); err != nil {
		return ImportResult{Source: source, Failed: true, Title: "共享盘异常", Message: userMessage(err), Log: app.logText()}
	}
	path := filePath
	if path == "" {
		// 用户取消文件对话框不算失败：状态与标签都不变（§5.2）
		return ImportResult{Source: source, State: app.importStates[source], Log: app.logText()}
	}
	rule := app.importer.Rules[source]
	if _, importErr := app.importer.Import(source, path); importErr != nil {
		app.audit("import", source, "导入失败："+importErr.Error(), "failed")
		return app.markImportFailed(source, importErr)
	}
	app.audit("import", source, "导入成功："+filepath.Base(path), "ok")
	app.markImported(source, filepath.Base(path))
	state := app.importStates[source]

	// 配了共享盘就自动发布（整张表 + manifest + state.json）；失败要明确告诉用户"只在本机"
	published := ""
	if app.shareConfig.Enabled() {
		version, publishErr := app.publishSource(source, path)
		if publishErr != nil {
			app.audit("publish", source, "发布失败："+publishErr.Error(), "failed")
			app.appendLog(LogError, "发布到共享盘失败："+publishErr.Error())
			// 数据在本机，但没发布：状态标签要如实写，避免看起来"已经导入并生效"
			app.importStates[source] = ImportState{
				Source: source, State: "failed", Label: "已导入，未发布",
				Tooltip:  "数据只在本机；发布失败，请检查共享盘后重新导入",
				RowCount: state.RowCount, ImportTime: state.ImportTime,
			}
			return ImportResult{
				Source: source, FileName: filepath.Base(path), RowCount: state.RowCount,
				State: app.importStates[source],
				Log:   app.logText(),
				Title: "导入成功，发布失败",
				Message: fmt.Sprintf("%s导入成功（只保存在本机），但发布到共享盘失败。\n"+
					"请检查共享盘后重新导入；技术详情见日志窗口。", rule.ChineseName),
				Failed: true,
			}
		}
		published = version
		app.audit("publish", source, "版本="+version, "ok")
		app.appendLog(LogSuccess, fmt.Sprintf("%s已发布到共享盘：%s", rule.ChineseName, version))
	}

	return ImportResult{
		Source:   source,
		FileName: filepath.Base(path),
		RowCount: state.RowCount,
		State:    state,
		Log:      app.logText(),
		// 成功反馈固定两行中文：第一行「{来源中文名}导入成功」，第二行行数
		Title:            "导入成功",
		Message:          fmt.Sprintf("%s导入成功\n导入行数：%d", rule.ChineseName, state.RowCount),
		PublishedVersion: published,
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
		return ClearResult{Failed: true, Title: "错误", Message: userMessage(err)}
	}
	defer app.endOperation()
	if err := app.authorize(auth.ActionClearData); err != nil {
		return ClearResult{Failed: true, Title: "无权限", Message: userMessage(err), Log: app.logText()}
	}
	names, err := store.SourceCleanupNames(app.loader)
	if err != nil {
		return ClearResult{Failed: true, Title: "错误", Message: userMessage(err)}
	}
	tableNames, err := store.SourceCleanupTableNames(app.loader)
	if err != nil {
		return ClearResult{Failed: true, Title: "错误", Message: userMessage(err)}
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
	// Conflicts 是单元格级的冲突提示（AGENTS.md §11）：与 Rows 同形同序
	// （同一行、同一列下标 = Columns），空串 = 该格不冲突。
	Conflicts [][]string `json:"conflicts"`
	// Notice 是"整合期间有新版本发布"之类的提示（空串 = 无）。
	Notice string `json:"notice"`
	// RowConflicts 是行级冲突原因（与 Rows 同形同序，空串 = 该行不冲突）：
	// 冲突字段没映射到导出列时，用它告诉用户"这一行为什么冲突"。
	RowConflicts []string `json:"rowConflicts"`
}

// Consolidate 跑整合并回传结果表（§5.4）。
// consolidateOnce 是整合主体（调用方已加锁、已校验权限与对齐）。
func (app *App) consolidateOnce(force bool) ConsolidateResult {
	outcome, err := app.service.ConsolidateWith(force)
	if err != nil {
		app.audit("consolidate", "", "整合失败："+err.Error(), "failed")
		app.appendLog(LogError, "数据整合失败："+err.Error())
		return ConsolidateResult{Log: app.logText(), Title: "数据缺失", Message: userMessage(err), Failed: true}
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
	app.audit("consolidate", "", "整合完成："+strings.Join(summary, "，"), "ok")
	app.recordConsolidation(counts)
	// 安全网：动作是 Modify 却没有任何变更描述 —— 正常不会出现（完全一致的行会被判为
	// 「重复」且不进导出），真出现时留一条警告便于排查，不改行状态。
	if warnings := outcome.ChangeDescriptionWarnings; len(warnings) > 0 {
		shown := warnings
		if len(shown) > 10 {
			shown = shown[:10]
		}
		detail := strings.Join(shown, "、")
		if len(warnings) > len(shown) {
			detail += fmt.Sprintf(" 等 %d 行", len(warnings))
		}
		app.appendLog(LogWarning, fmt.Sprintf(
			"有 %d 行判定为 Modify 但没有变更描述（已按原样导出，请核对）：%s",
			len(warnings), detail))
	}
	columns := append([]string{"status"}, app.service.Config.FieldNames...)
	rows := make([][]string, 0, len(outcome.Rows))
	statuses := make([]string, 0, len(outcome.Rows))
	conflicts := make([][]string, 0, len(outcome.Rows))
	for index, row := range outcome.Rows {
		values := make([]string, 0, len(columns))
		for _, column := range columns {
			values = append(values, row.Values[column])
		}
		rows = append(rows, values)
		statuses = append(statuses, row.Values["status"])
		// 提示矩阵与 Rows 同形同序（同一行、同一列下标）：首列 status 没有提示，补一个空串
		conflicts = append(conflicts, append([]string{""}, outcome.ConflictNotes[index]...))
	}
	return ConsolidateResult{
		Log: app.logText(), Columns: columns, Rows: rows,
		Statuses: statuses, Conflicts: conflicts, RowConflicts: outcome.RowConflicts,
	}
}

// ExportResult 是一次导出回给界面的结果。
type ExportResult struct {
	Log     string `json:"log"`
	Title   string `json:"title"`
	Message string `json:"message"`
	Failed  bool   `json:"failed"`
	Path    string `json:"path"`
	// SharedPath 是归档到共享盘 consolidated/<id>/ 的相对路径（本地模式为空）。
	SharedPath string `json:"sharedPath"`
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
	if err := app.authorize(auth.ActionExport); err != nil {
		app.appendLog(LogWarning, "导出被拒绝："+err.Error())
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
		return ExportResult{Failed: true, Title: "导出失败", Message: userMessage(err)}
	}
	defer app.endOperation()
	if err := app.authorize(auth.ActionExport); err != nil {
		return ExportResult{Failed: true, Title: "无权限", Message: userMessage(err), Log: app.logText()}
	}
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
		return ExportResult{Log: app.logText(), Title: "导出失败", Message: userMessage(err), Failed: true}
	}
	// 用户路径与导出目录里的副本重合时跳过复制（R20，否则会抛 SameFileError）
	if filepath.Clean(target) != filepath.Clean(exported.FilePath) {
		if err := fileutil.CopyFile(exported.FilePath, target); err != nil {
			app.appendLog(LogError, "导出失败："+err.Error())
			return ExportResult{Log: app.logText(), Title: "导出失败", Message: userMessage(err), Failed: true}
		}
	}
	if err := app.service.RecordConsolidationResult(now); err != nil {
		app.appendLog(LogError, "记录整合结果失败："+err.Error())
	} else if syncErr := app.syncOperationLog(); syncErr != nil {
		app.appendLog(LogWarning, "操作日志同步失败（下次打开记录导出时会重试）："+syncErr.Error())
	}
	app.appendLog(LogSuccess, "整合结果已导出到 "+target)
	app.audit("export", target, "整合结果", "ok")
	sharedPath := ""
	archiveName := filepath.Base(target)
	if archiveName == "" || archiveName == "." {
		archiveName = filepath.Base(exported.FilePath)
	}
	if app.shareConfig.Enabled() {
		path, archiveErr := app.archiveExport(exported.FilePath, archiveName)
		if archiveErr != nil {
			app.appendLog(LogWarning, "归档到共享盘失败："+archiveErr.Error())
			app.audit("export", target, "归档失败："+archiveErr.Error(), "failed")
		} else if path != "" {
			sharedPath = path
			app.appendLog(LogSuccess, "已归档到共享盘："+path)
			app.audit("export", path, "已归档", "ok")
		}
	}
	message := "整合结果已导出。"
	if sharedPath != "" {
		message += "\n已归档到共享盘：" + sharedPath
	} else if app.shareConfig.Enabled() {
		message += "\n共享盘不可用或还没有整合记录，结果未归档（文件已保存在本机）。"
	}
	return ExportResult{
		Log: app.logText(), Title: "成功", Message: message, Path: target, SharedPath: sharedPath,
	}
}

// ReviewLog 按时间区间读操作日志，并像来源回顾一样分页（§5.6）。
//
// 运行日志只在第 1 页写一行，与现状实现的开窗时机一致。
func (app *App) ReviewLog(start string, end string, page int, pageSize int, query string) ReviewResult {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.beginOperation(); err != nil {
		return ReviewResult{Failed: true, Title: "错误", Message: userMessage(err)}
	}
	defer app.endOperation()
	if pageSize <= 0 {
		pageSize = 100
	}
	if page < 1 {
		page = 1
	}
	// 记录导出所有人可见：读之前先把本机日志与共享盘合并成全量
	if err := app.syncOperationLog(); err != nil {
		app.appendLog(LogWarning, "操作日志同步失败（本次显示本机数据）："+err.Error())
	}
	rows, err := app.reviewLogRows(start, end)
	if err != nil {
		return ReviewResult{Log: app.logText(), Failed: true, Title: "错误", Message: userMessage(err)}
	}
	columns := app.log.TableColumns()
	rows = filterReviewRows(rows, columns, query)
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

// AboutInfo 是「关于」窗口的数据：版本、目录/数据库、运行环境与技术栈。
type AboutInfo struct {
	Name        string   `json:"name"`
	Tagline     string   `json:"tagline"`
	ChineseName string   `json:"chineseName"`
	Version     string   `json:"version"`
	Detail      string   `json:"detail"`
	DataDir     string   `json:"dataDir"`
	ConfigDir   string   `json:"configDir"`
	Database    string   `json:"database"`
	Runtime     string   `json:"runtime"`
	TechStack   []string `json:"techStack"`
}

// About 是「关于」窗口需要的内容（版本、路径、运行环境、技术栈，§6.4）。
func (app *App) About() AboutInfo {
	version := ""
	if app.loader != nil {
		if setting, err := app.loader.LoadSetting(); err == nil {
			version = text(setting["APP_VERSION"])
		}
	}
	info := buildinfo.ReadBuildInfo(buildinfo.Options{AppVersion: version})
	about := AboutInfo{
		Name:        "SingleSourceReady",
		Tagline:     "Single Source Ready · UDI Data Consolidation Platform",
		ChineseName: "UDI 数据整合与单源数据准备平台",
		Version:     "版本 " + info.Version,
		Detail:      info.Detail(),
		Runtime:     stdruntime.Version() + " · " + stdruntime.GOOS + "/" + stdruntime.GOARCH,
		TechStack: []string{
			"Go + Wails v2 + SQLite（modernc，纯 Go、无 CGO）",
			"Excelize 读写 Excel；四份 YAML 配置驱动",
			"前端 React 18 + Ant Design 5 + Vite",
		},
	}
	if app.loader != nil {
		about.ConfigDir = app.loader.Resolver.ConfigDir
		about.DataDir = filepath.Dir(app.loader.Resolver.ConfigDir)
	}
	about.Database = app.wiredDatabasePath
	return about
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
func (app *App) ReviewSource(source string, page int, pageSize int, query string) ReviewResult {
	app.mu.Lock()
	err := app.authorizeSource(source, false)
	app.mu.Unlock()
	if err != nil {
		return ReviewResult{Failed: true, Title: "无权限", Message: userMessage(err)}
	}
	return app.reviewPaged(source, page, pageSize, query)
}

// ReviewCleanedSource 是「查看清洗后数据」：同一个来源的清洗表（只有声明了 clean 的来源才有）。
//
// 与 ReviewSource 共用同一套分页与回顾窗口，只是读的表与列不同：
// 数据回顾看原文表，这里看整合实际读取的清洗表，方便核对合并结果。
func (app *App) ReviewCleanedSource(source string, page int, pageSize int, query string) ReviewResult {
	app.mu.Lock()
	err := app.authorizeSource(source, false)
	app.mu.Unlock()
	if err != nil {
		return ReviewResult{Failed: true, Title: "无权限", Message: userMessage(err)}
	}
	return app.reviewPaged(source+cleanedReviewSuffix, page, pageSize, query)
}

// reviewPaged 是来源回顾的分页主体；reviewKey 可以带「清洗后」后缀。
func (app *App) reviewPaged(reviewKey string, page int, pageSize int, query string) ReviewResult {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.beginOperation(); err != nil {
		return ReviewResult{Failed: true, Title: "错误", Message: userMessage(err)}
	}
	defer app.endOperation()
	if pageSize <= 0 {
		pageSize = 100
	}
	if page < 1 {
		page = 1
	}
	columns, rows, err := app.reviewData(reviewKey)
	if err != nil {
		return ReviewResult{Log: app.logText(), Failed: true, Title: "错误", Message: userMessage(err)}
	}
	rows = filterMatrix(rows, query)
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
	label, cleaned := reviewLabel(app.importer.Rules, reviewKey)
	if page == 1 {
		app.appendLog(LogInfo, fmt.Sprintf("%s数据回顾完成：共 %d 行", label, len(rows)))
	}
	title := label + "数据回顾"
	if cleaned {
		title += "（清洗后）"
	}
	return ReviewResult{
		Log: app.logText(), Title: title,
		Columns: columns, Rows: rows[begin:finish],
		Total: len(rows), Page: page, PageSize: pageSize, PageCount: pageCount,
	}
}

// filterReviewRows 按关键字过滤（任一列的文本包含即命中，不区分大小写）。
func filterReviewRows(rows []store.Row, columns []string, query string) []store.Row {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return rows
	}
	filtered := make([]store.Row, 0, len(rows))
	for _, row := range rows {
		for _, column := range columns {
			if strings.Contains(strings.ToLower(row[column]), query) {
				filtered = append(filtered, row)
				break
			}
		}
	}
	return filtered
}

// filterMatrix 是 [][]string 版本的同一口径（回顾窗口的表格行）。
func filterMatrix(rows [][]string, query string) [][]string {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return rows
	}
	filtered := make([][]string, 0, len(rows))
	for _, row := range rows {
		for _, value := range row {
			if strings.Contains(strings.ToLower(value), query) {
				filtered = append(filtered, row)
				break
			}
		}
	}
	return filtered
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
	base, cleaned := splitReviewKey(fileType)
	rule, known := app.importer.Rules[base]
	if !known {
		return nil, nil, fmt.Errorf("Invalid file type: %s", fileType)
	}
	// 清洗后的视图：读清洗表、列也按清洗表（去掉 drop 的列）
	tableName := rule.TargetTable
	definitions := rule.Columns
	if cleaned {
		if rule.Clean == nil {
			return nil, nil, fmt.Errorf("Source %s has no cleaned table", base)
		}
		tableName = rule.Clean.TargetTable
		definitions = importer.CleanColumns(rule)
	}
	rows, err := app.repo.Rows(tableName)
	if err != nil {
		return nil, nil, err
	}
	columns := make([]string, 0, len(definitions))
	for _, column := range definitions {
		columns = append(columns, column.ChineseName)
	}
	table := make([][]string, 0, len(rows))
	for _, row := range rows {
		values := make([]string, 0, len(definitions))
		for _, column := range definitions {
			values = append(values, row[column.DBField])
		}
		table = append(table, values)
	}
	return columns, table, nil
}

// cleanedReviewSuffix 是「查看清洗后数据」在回顾键上的后缀（ra_input::cleaned）。
const cleanedReviewSuffix = "::cleaned"

// splitReviewKey 拆出「来源键 + 是否清洗后」。
func splitReviewKey(reviewKey string) (string, bool) {
	if strings.HasSuffix(reviewKey, cleanedReviewSuffix) {
		return strings.TrimSuffix(reviewKey, cleanedReviewSuffix), true
	}
	return reviewKey, false
}

// reviewLabel 是回顾窗口的标题前缀（来源中文名，清洗后加后缀）。
func reviewLabel(rules map[string]*importer.Rule, reviewKey string) (string, bool) {
	base, cleaned := splitReviewKey(reviewKey)
	label := base
	if rule, known := rules[base]; known {
		label = rule.ChineseName
	}
	return label, cleaned
}

// SelectReviewExportTarget 打开回顾窗口的「Save Imported Data」对话框。
//
// 与现状实现一致：默认文件名 `{fileType}_imported_data.xlsx`、过滤器 Excel Files (*.xlsx)；
// 记录导出的回顾用 fileType = operation_log，于是默认名是 operation_log_imported_data.xlsx。
func (app *App) SelectReviewExportTarget(fileType string) ExportTarget {
	app.mu.Lock()
	if base, _ := splitReviewKey(fileType); base != "operation_log" {
		if err := app.authorizeSource(base, false); err != nil {
			app.appendLog(LogWarning, "回顾导出被拒绝："+err.Error())
			app.mu.Unlock()
			return ExportTarget{}
		}
	}
	app.mu.Unlock()
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.importer == nil {
		return ExportTarget{}
	}
	defaultName := reviewFileName(fileType)
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

// reviewFileName 是回顾导出的默认文件名；清洗后视图用 _cleaned 而不是 "::"（文件名里不能有冒号）。
func reviewFileName(fileType string) string {
	if base, cleaned := splitReviewKey(fileType); cleaned {
		return base + "_cleaned_imported_data.xlsx"
	}
	return fileType + "_imported_data.xlsx"
}

// ExportReviewData 把回顾窗口里的全部数据写成 Excel（保存位置由上一步选定）。
//
// 成功会写一行运行日志（回顾数据已导出到 {路径}）并弹提示，失败弹「导出失败」。
func (app *App) ExportReviewData(fileType string, target string) ExportResult {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.beginOperation(); err != nil {
		return ExportResult{Failed: true, Title: "导出失败", Message: userMessage(err)}
	}
	defer app.endOperation()
	if base, _ := splitReviewKey(fileType); base != "operation_log" {
		if err := app.authorizeSource(base, false); err != nil {
			return ExportResult{Failed: true, Title: "无权限", Message: userMessage(err), Log: app.logText()}
		}
	}
	if target == "" {
		return ExportResult{Log: app.logText()}
	}
	columns, rows, err := app.reviewData(fileType)
	if err != nil {
		return ExportResult{Log: app.logText(), Title: "导出失败", Message: userMessage(err), Failed: true}
	}
	if err := excelio.WriteReviewWorkbook(target, columns, rows); err != nil {
		return ExportResult{Log: app.logText(), Title: "导出失败", Message: userMessage(err), Failed: true}
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
