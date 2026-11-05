package app

// P5：管理员的运行状态、审计查看与旧版本清理。

import (
	"fmt"
	"strings"

	"github.com/jiegun314/ssr-go/internal/auth"
	"github.com/jiegun314/ssr-go/internal/consolidation"
	"github.com/jiegun314/ssr-go/internal/publish"
	"github.com/jiegun314/ssr-go/internal/share"
)

// AuditColumns 是审计表格的列（与 store.AuditRecord 的字段一一对应）。
var AuditColumns = []string{"时间", "操作人", "角色", "用户名", "机器", "动作", "对象", "说明", "结果"}

// ReviewAudit 分页读审计（SC/Admin；最新的在前）。
func (app *App) ReviewAudit(page int, pageSize int, query string) ReviewResult {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.beginOperation(); err != nil {
		return ReviewResult{Failed: true, Title: "错误", Message: userMessage(err)}
	}
	defer app.endOperation()
	if err := app.authorize(auth.ActionViewAuditAny); err != nil {
		return ReviewResult{Failed: true, Title: "无权限", Message: userMessage(err), Log: app.logText()}
	}
	if pageSize <= 0 {
		pageSize = 100
	}
	if page < 1 {
		page = 1
	}
	exists, existsErr := app.repo.TableExists("audit_log")
	if existsErr != nil {
		return ReviewResult{Failed: true, Title: "错误", Message: userMessage(existsErr), Log: app.logText()}
	}
	rows := []map[string]string{}
	if exists {
		loaded, err := app.repo.AuditRows()
		if err != nil {
			return ReviewResult{Failed: true, Title: "错误", Message: userMessage(err), Log: app.logText()}
		}
		rows = loaded
	}
	table := make([][]string, 0, len(rows))
	for index := len(rows) - 1; index >= 0; index-- {
		row := rows[index]
		table = append(table, []string{
			row["at"], row["actor"], row["role"], row["username"], row["machine"],
			row["action"], row["target"], row["detail"], row["result"],
		})
	}
	table = filterMatrix(table, query)
	pageCount := (len(table) + pageSize - 1) / pageSize
	if pageCount == 0 {
		pageCount = 1
	}
	if page > pageCount {
		page = pageCount
	}
	begin := (page - 1) * pageSize
	finish := begin + pageSize
	if finish > len(table) {
		finish = len(table)
	}
	return ReviewResult{
		Log: app.logText(), Title: "审计日志", Columns: AuditColumns,
		Rows: table[begin:finish], Total: len(table), Page: page, PageSize: pageSize, PageCount: pageCount,
	}
}

// CleanupResult 是旧版本清理的结果。
type CleanupResult struct {
	Failed  bool     `json:"failed"`
	Title   string   `json:"title"`
	Message string   `json:"message"`
	Log     string   `json:"log"`
	Deleted []string `json:"deleted"`
}

// CleanupShareVersions 清理共享盘上的旧版本（保留最新 keep 个 + 当前/最后整合用到的版本）。
func (app *App) CleanupShareVersions(keep int) CleanupResult {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.beginOperation(); err != nil {
		return CleanupResult{Failed: true, Title: "错误", Message: userMessage(err)}
	}
	defer app.endOperation()
	if err := app.authorize(auth.ActionManageShare); err != nil {
		return CleanupResult{Failed: true, Title: "无权限", Message: userMessage(err), Log: app.logText()}
	}
	if !app.shareEnabled() {
		return CleanupResult{Failed: true, Title: "未使用共享盘",
			Message: "当前是本地模式（或没有配置共享盘），没有可清理的共享盘版本。", Log: app.logText()}
	}
	if status := app.refreshShareStatus(); !status.Healthy {
		return CleanupResult{Failed: true, Title: "共享盘异常",
			Message: "共享盘不可用，暂不能清理。请检查连接后点状态栏的「重新检查」。", Log: app.logText()}
	}
	if keep < 1 {
		keep = 5
	}
	state, err := share.LoadState(app.shareConfig.Root)
	if err != nil {
		return CleanupResult{Failed: true, Title: "清理失败", Message: userMessage(err), Log: app.logText()}
	}
	protected := map[string]map[string]bool{}
	add := func(source string, version string) {
		if source == "" || version == "" {
			return
		}
		department := share.Department(source)
		if protected[department] == nil {
			protected[department] = map[string]bool{}
		}
		protected[department][version] = true
	}
	for source, item := range state.Sources {
		if item != nil {
			add(source, item.Version)
		}
	}
	lastID := ""
	if state.LastConsolidation != nil {
		lastID = state.LastConsolidation.ID
		for source, version := range state.LastConsolidation.Versions {
			add(source, version)
		}
	}
	deleted, err := publish.Cleanup(app.shareConfig.Root, keep, protected, lastID)
	if err != nil {
		app.audit("cleanup", "", err.Error(), "failed")
		return CleanupResult{Failed: true, Title: "清理失败", Message: userMessage(err), Deleted: deleted, Log: app.logText()}
	}
	counts := map[string]int{}
	for _, path := range deleted {
		counts[strings.SplitN(path, "/", 2)[0]]++
	}
	app.audit("cleanup", "", fmt.Sprintf("保留 %d 份，删除 %d 个条目", keep, len(deleted)), "ok")
	app.appendLog(LogSuccess, fmt.Sprintf("已清理共享盘旧版本：保留最近 %d 份，删除 %d 个条目", keep, len(deleted)))
	message := fmt.Sprintf("保留每个来源最近 %d 份与最后整合用到的版本；删除 %d 个条目。", keep, len(deleted))
	if len(deleted) == 0 {
		message = "没有需要清理的旧版本。"
	}
	return CleanupResult{Log: app.logText(), Title: "清理完成", Message: message, Deleted: deleted}
}

// 让 "整合状态计数" 的键与 consolidation 包保持一处来源（避免魔法字符串）。
var _ = consolidation.StatusReady
