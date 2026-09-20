package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jiegun314/ssr-go/internal/store"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// BackupResult 是一次「备份数据库」的结果：与导入/导出同一套 Title/Message/Failed 口径，
// 菜单动作没有返回值给前端，所以同时通过 show-message 事件弹窗。
type BackupResult struct {
	Title   string `json:"title"`
	Message string `json:"message"`
	Failed  bool   `json:"failed"`
	Path    string `json:"path"`
}

// OpenConfigDirectory 打开四份 YAML 所在目录（内审/RA 同事经常要直接看规则）。
func (app *App) OpenConfigDirectory() {
	app.openDirectory("配置目录", app.configDirectory(), false)
}

// OpenExportDirectory 打开导出目录（setting.yaml 的 folder.export）。
func (app *App) OpenExportDirectory() {
	app.openDirectory("导出目录", app.exportDirectory(), true)
}

// OpenDataDirectory 打开数据库所在目录（备份/换机器时要用）。
func (app *App) OpenDataDirectory() {
	app.openDirectory("数据目录", filepath.Dir(app.databasePath()), true)
}

// BackupDatabase 把当前库整库导出成一份带时间戳的快照，放在「导出目录」同级的 backup/ 下。
//
// 用 SQLite 的 VACUUM INTO 而不是拷文件：库跑在 WAL 模式下，直接拷 .sqlite3
// 可能漏掉还没合并进主库的写入；VACUUM INTO 出来的是一份一致、已整理的完整快照。
func (app *App) BackupDatabase() BackupResult {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.beginOperation(); err != nil {
		return BackupResult{Title: "Error", Message: err.Error(), Failed: true}
	}
	defer app.endOperation()
	if app.repo == nil {
		return app.backupFailed("配置或数据库尚未就绪，无法备份。")
	}
	target := backupTargetPath(app.databasePath(), app.exportDirectory(), time.Now())
	if err := backupDatabase(app.repo, target); err != nil {
		return app.backupFailed(err.Error())
	}
	message := fmt.Sprintf("数据库已备份到：\n%s", target)
	app.appendLog("Database backed up to " + target)
	app.refreshLogPanel()
	app.notify("Success", message)
	return BackupResult{Title: "Success", Message: message, Path: target}
}

func (app *App) backupFailed(reason string) BackupResult {
	message := "Failed to back up database: " + reason
	app.appendLog(message)
	app.refreshLogPanel()
	app.notify("Error", message)
	return BackupResult{Title: "Error", Message: message, Failed: true}
}

// backupDatabase 是备份的可测核心：把 repository 指向的库导出到 target。
func backupDatabase(repository *store.Repository, target string) error {
	if repository == nil {
		return fmt.Errorf("数据库尚未就绪")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("Failed to create backup folder: %w", err)
	}
	return repository.Execute("VACUUM INTO " + quoteSQLString(target))
}

// backupTargetPath 是备份文件路径：<output>/backup/<库名>-YYYYmmdd-HHMMSS.sqlite3。
// 名字取配置里的库文件名（去掉扩展名），这样库改名了备份也跟着改名。
func backupTargetPath(databasePath string, exportDirectory string, now time.Time) string {
	base := strings.TrimSuffix(filepath.Base(databasePath), filepath.Ext(databasePath))
	if base == "" || base == "." {
		base = "udi_data"
	}
	return filepath.Join(filepath.Dir(exportDirectory), "backup",
		fmt.Sprintf("%s-%s.sqlite3", base, now.Format("20060102-150405")))
}

// quoteSQLString 把路径写进 SQL 字面量（VACUUM INTO 的目标不接受绑定参数）。
func quoteSQLString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// fileManagerCommand 返回"在系统文件管理器里打开这个目录"的命令。
// 本项目只发 macOS 与 Windows，Linux 分支只是兜底。
func fileManagerCommand(path string) *exec.Cmd {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", path)
	case "windows":
		return exec.Command("explorer", path)
	default:
		return exec.Command("xdg-open", path)
	}
}

// openDirectory 在文件管理器里打开目录；目录不存在时按需创建（导出/数据目录属于运行期产物）。
func (app *App) openDirectory(title string, path string, create bool) {
	if app.loader == nil || path == "" || path == "." {
		app.notify("Error", "配置尚未加载，无法打开"+title+"。")
		return
	}
	if create {
		if err := os.MkdirAll(path, 0o755); err != nil {
			app.failToOpenDirectory(title, path, err)
			return
		}
	}
	if _, err := os.Stat(path); err != nil {
		app.failToOpenDirectory(title, path, err)
		return
	}
	// 只用 Start 不等待：文件管理器是长驻进程，等它会卡住界面
	if err := fileManagerCommand(path).Start(); err != nil {
		app.failToOpenDirectory(title, path, err)
		return
	}
	app.appendLog(fmt.Sprintf("%s opened: %s", title, path))
	app.refreshLogPanel()
}

func (app *App) failToOpenDirectory(title string, path string, err error) {
	message := fmt.Sprintf("Failed to open %s (%s): %v", title, path, err)
	app.appendLog(message)
	app.refreshLogPanel()
	app.notify("Error", message)
}

// configDirectory 是四份 YAML 所在目录。
func (app *App) configDirectory() string {
	if app.loader == nil {
		return ""
	}
	return app.loader.Resolver.ConfigDir
}

// databasePath 是配置里数据库文件的绝对路径。
func (app *App) databasePath() string {
	return app.resolveSettingPath("database", "path")
}

// exportDirectory 是配置里的导出目录（folder.export）。
func (app *App) exportDirectory() string {
	return app.resolveSettingPath("folder", "export")
}

// resolveSettingPath 取 setting.yaml 里 <section>.<key> 的路径并解析成绝对路径。
func (app *App) resolveSettingPath(section string, key string) string {
	if app.loader == nil {
		return ""
	}
	setting, err := app.loader.LoadSetting()
	if err != nil {
		return ""
	}
	values, _ := setting[section].(map[string]any)
	return app.loader.ResolvePath(text(values[key]))
}

// refreshLogPanel 把操作日志推给界面：菜单动作没有返回值通道，只能走事件。
func (app *App) refreshLogPanel() {
	if app.context == nil {
		return
	}
	wruntime.EventsEmit(app.context, "log-updated", strings.Join(app.logLines, "\n"))
}

// notify 让界面弹一个提示框（菜单动作没有返回值通道）。
func (app *App) notify(title string, message string) {
	if app.context == nil {
		return
	}
	wruntime.EventsEmit(app.context, "show-message", map[string]any{
		"title":   title,
		"message": message,
	})
}
