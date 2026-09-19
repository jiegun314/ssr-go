package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/jiegun314/ssr-go/internal/config"
	"github.com/jiegun314/ssr-go/internal/consolidation"
	"github.com/jiegun314/ssr-go/internal/importer"
	"github.com/jiegun314/ssr-go/internal/paths"
	"github.com/jiegun314/ssr-go/internal/store"
)

// App 是绑定给前端的后端。一次会话一份：导入、整合、导出都作用在同一份数据库上。
type App struct {
	context context.Context
	mu      sync.Mutex

	loader   *config.Loader
	repo     *store.Repository
	importer *importer.Importer
	service  *consolidation.Service
	log      *store.OperationLog

	// logLines 是界面右下角「操作日志」里的内容（带时间戳的运行消息）。
	logLines []string
	// notices 是启动期告警（图标、任务栏身份），由 main.go 传进来（R27）。
	notices []string
	// startupError 是启动失败的原因：界面在用户点按钮时把它原样报出来，
	// 否则现场只会看到一句「配置未加载」，无从排查。
	startupError string
	// importStates 记录每个来源这一次会话的导入状态（用于状态标签与圆点）。
	importStates map[string]ImportState
	// cleanedTables 是本次启动真的被清理删掉的表（界面据此决定报不报存量）（§5.1）。
	cleanedTables []string
	busy          bool
	aboutVisible  bool
}

// ImportState 是一个来源的界面状态（对应现有界面的状态标签与圆点）。
type ImportState struct {
	Source     string `json:"source"`
	State      string `json:"state"` // empty | imported | existing | failed
	Label      string `json:"label"`
	Tooltip    string `json:"tooltip"`
	RowCount   int    `json:"rowCount"`
	ImportTime string `json:"importTime"`
}

// State 是界面启动时需要的全部状态。
type State struct {
	Version      string                 `json:"version"`
	OperationLog string                 `json:"operationLog"`
	Imports      map[string]ImportState `json:"imports"`
	Busy         bool                   `json:"busy"`
}

// NewApp 建一个尚未初始化配置的后端（startup 里才碰数据库）。
func NewApp(notices []string) *App {
	return &App{
		notices:      notices,
		logLines:     []string{},
		importStates: map[string]ImportState{},
	}
}

// startup 打开配置与数据库、跑启动清理、把启动期告警写进操作日志（§5.1）。
func (app *App) startup(ctx context.Context) {
	app.context = ctx
	app.appendLog("Application started")
	for _, notice := range app.notices {
		app.appendLog(notice)
	}
	loader, err := config.NewLoader("")
	if err != nil {
		app.fail("Configuration error: " + err.Error())
		return
	}
	if err := loader.ValidateAll(); err != nil {
		app.fail("Configuration error: " + err.Error())
		return
	}
	setting, err := loader.LoadSetting()
	if err != nil {
		app.fail("Configuration error: " + err.Error())
		return
	}
	database, _ := setting["database"].(map[string]any)
	repository, err := store.Open(loader.ResolvePath(text(database["path"])))
	if err != nil {
		app.fail("Database error: " + err.Error())
		return
	}
	app.loader = loader
	app.repo = repository
	app.importer, err = importer.NewImporter(loader, repository)
	if err != nil {
		app.fail("Configuration error: " + err.Error())
		return
	}
	configValue, err := consolidation.LoadConfig(loader)
	if err != nil {
		app.fail("Configuration error: " + err.Error())
		return
	}
	tables, _ := setting["tables"].(map[string]any)
	operationLog, _ := tables["operation_log"].(map[string]any)
	logColumns, err := loader.LoadLogColumns()
	if err != nil {
		app.fail("Configuration error: " + err.Error())
		return
	}
	app.log, err = store.OpenOperationLog(repository, store.OperationLogOptions{
		TableName:      text(operationLog["name"]),
		TimeColumn:     text(tables["operation_log_time_column"]),
		IdentityFields: configValue.IdentityFields,
		LogColumns:     logColumns,
	})
	if err != nil {
		app.fail("Database error: " + err.Error())
		return
	}
	app.service = &consolidation.Service{
		Config: configValue, Loader: loader, Repository: repository, Log: app.log,
	}
	app.resetImportStates()
	app.cleanupOnStartup(setting)
	app.reportExistingCounts()
}

// cleanupOnStartup 按配置删除暂存表（R9），并写一行运行日志。
func (app *App) cleanupOnStartup(setting map[string]any) {
	cleanup, err := app.loader.LoadCleanupOnStartup()
	if err != nil {
		app.fail("Configuration error: " + err.Error())
		return
	}
	if !cleanup {
		app.appendLog("Staging tables kept (cleanup_on_startup = false)")
		return
	}
	tableNames, err := store.CleanupTableNames(app.loader)
	if err != nil {
		app.appendLog(err.Error())
		return
	}
	dropped, failed := store.DropTables(app.repo, tableNames)
	if len(failed) > 0 {
		app.appendLog(store.FormatTableFailure(failed))
		return
	}
	app.cleanedTables = dropped
	app.appendLog("Staging tables cleaned successfully")
}

// reportExistingCounts 报出「本次启动没被清理删掉、且真的有记录」的来源（§5.1 第 5 步）。
func (app *App) reportExistingCounts() {
	for _, source := range app.importer.Order {
		rule := app.importer.Rules[source]
		if contains(app.cleanedTables, rule.TargetTable) {
			continue
		}
		count, err := app.repo.CountTableRows(rule.TargetTable)
		if err != nil {
			app.appendLog(fmt.Sprintf("Failed to read the existing row count of %s: %v", source, err))
			continue
		}
		if count <= 0 {
			continue
		}
		app.importStates[source] = ImportState{
			Source:   source,
			State:    "existing",
			Label:    fmt.Sprintf("现有%d条", count),
			Tooltip:  fmt.Sprintf("现有数据：%d 行（上一次运行导入的，本次尚未导入）", count),
			RowCount: count,
		}
	}
}

// resetImportStates 把四个来源复位成「尚未导入」（R25 与启动复位共用）。
func (app *App) resetImportStates() {
	for _, source := range app.importer.Order {
		app.importStates[source] = ImportState{
			Source: source, State: "empty", Tooltip: "尚未导入",
		}
	}
}

// ImportState 返回某个来源当前的状态。
func (app *App) ImportState(source string) ImportState {
	return app.importStates[source]
}

func (app *App) appendLog(message string) {
	stamp := time.Now().Format("2006-01-02 15:04:05")
	line := stamp + " - " + message
	app.logLines = append(app.logLines, line)
	// 同时打印到 stdout：发布构建没有控制台窗口，但现场从终端启动时这是唯一的线索
	// （R27 对启动告警也是这个口径）。
	fmt.Println(line)
}

// fail 记录启动失败：写进操作日志，并留下面向用户的说明。
func (app *App) fail(message string) {
	app.startupError = message
	app.appendLog(message)
}

func (app *App) logText() string {
	return strings.Join(app.logLines, "\n")
}

func text(value any) string {
	text, _ := value.(string)
	return text
}

// contains 判断表名是否在启动清理删掉的清单里。
func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// Quit 关掉应用（菜单「退出」）。
func (app *App) Quit() {
	if app.context != nil {
		runtime.Quit(app.context)
	}
}

// reserve 让下面的文件对话框与业务方法保持同一个加锁口径。
func (app *App) beginOperation() error {
	if app.busy {
		return fmt.Errorf("另一个操作正在进行，请稍候")
	}
	if app.importer == nil {
		configDir := ""
		if resolver, err := paths.New(""); err == nil {
			configDir = resolver.ConfigDir
		}
		if app.startupError != "" {
			return fmt.Errorf("%s\n（期望的配置目录：%s）", app.startupError, configDir)
		}
		return fmt.Errorf(
			"配置未加载：找不到配置目录 %s。\n"+
				"请把 config/ 与 SingleSourceReady.app 放在同一层目录后再启动"+
				"（直接双击 .app 时它同级那层就是项目根）。", configDir)
	}
	app.busy = true
	return nil
}

func (app *App) endOperation() { app.busy = false }

// selectFile 打开「Select Excel File」对话框（§6.3 的标题与过滤器）。
func (app *App) selectFile() (string, error) {
	return runtime.OpenFileDialog(app.context, runtime.OpenDialogOptions{
		Title: "Select Excel File",
		Filters: []runtime.FileFilter{
			{DisplayName: "Excel Files (*.xlsx *.xls)", Pattern: "*.xlsx;*.xls"},
		},
	})
}

// SavePlaceholder 保留给后续步骤：导出前的保存对话框。
func (app *App) saveFile(defaultName string) (string, error) {
	return runtime.SaveFileDialog(app.context, runtime.SaveDialogOptions{
		Title:           "Save Consolidation Result",
		DefaultFilename: defaultName,
		Filters: []runtime.FileFilter{
			{DisplayName: "Excel Files (*.xlsx)", Pattern: "*.xlsx"},
		},
	})
}

// OpenSourceFromDialog 是菜单「文件 → 打开」：按中文名选来源再选文件。
func (app *App) OpenSourceFromDialog() {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.beginOperation(); err != nil {
		return
	}
	defer app.endOperation()
	path, err := app.selectFile()
	if err != nil || path == "" {
		return
	}
	app.importFileFor(path)
}

// importFileFor 按文件名猜来源（菜单入口用）：表头能对上的第一个来源。
func (app *App) importFileFor(path string) {
	for _, source := range app.importer.Order {
		if _, err := app.importer.Import(source, path); err == nil {
			app.markImported(source, filepath.Base(path))
			return
		}
	}
	app.appendLog(fmt.Sprintf("Import failed: %s", path))
}

// markImported 记录导入成功后的状态与日志。
func (app *App) markImported(source string, fileName string) {
	count, err := app.importer.CountImportedRows(source)
	if err != nil {
		count = 0
	}
	importTime := time.Now().Format("2006/01/02 15:04")
	app.importStates[source] = ImportState{
		Source:     source,
		State:      "imported",
		Label:      fmt.Sprintf("导入%d条记录", count),
		Tooltip:    fmt.Sprintf("最近导入：%s，%d 行", importTime, count),
		RowCount:   count,
		ImportTime: importTime,
	}
	app.appendLog(fmt.Sprintf("%s imported successfully. Rows imported: %d",
		app.importer.Rules[source].ChineseName, count))
}

var _ = filepath.Base
