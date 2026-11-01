package main

// P3：从共享盘对齐（拉取 + 校验 + 覆盖导入），以及"各端看到什么"的总览。
//
// 口径（docs/design/multiuser-workflow.md §7.2/§8）：
//   - 对齐 = 读 state.json 找最新版本 → 拉快照 → 校验 sha256/行数 → 整表覆盖导入 → 记 synced_versions；
//   - 整合前必须四份都对齐（分享盘模式 + require_before_consolidate）；
//   - RA/EAMA/UDI 只看自己来源的明细，其他来源只暴露"最后更新时间"；
//   - 待整合 = 共享盘版本 ≠ 最后一次整合的版本（所有人按可见范围看到）。

import (
	"fmt"
	"strings"
	"time"

	"github.com/jiegun314/ssr-go/internal/auth"
	"github.com/jiegun314/ssr-go/internal/consolidation"
	"github.com/jiegun314/ssr-go/internal/importer"
	"github.com/jiegun314/ssr-go/internal/publish"
	"github.com/jiegun314/ssr-go/internal/share"
	"github.com/jiegun314/ssr-go/internal/store"
)

// SourceStatus 是一个来源在"对齐/待整合"层面的状态。
//
// 对没有权限看明细的角色，除 Label / PublishedAt / CanView 外的字段都会被清空
// （后端不下发，而不是前端藏起来）。
type SourceStatus struct {
	Source        string `json:"source"`
	Label         string `json:"label"`
	CanView       bool   `json:"canView"`
	LocalVersion  string `json:"localVersion"`
	RemoteVersion string `json:"remoteVersion"`
	RemoteRows    int    `json:"remoteRows"`
	PublishedAt   string `json:"publishedAt"`
	PublishedBy   string `json:"publishedBy"`
	Aligned       bool   `json:"aligned"`
	Pending       bool   `json:"pending"`
}

// LastConsolidation 是"最后整合时间"与操作人（所有人可见）。
type LastConsolidation struct {
	At       string            `json:"at"`
	By       string            `json:"by"`
	Versions map[string]string `json:"versions"`
}

// ShareOverview 是状态栏与对齐面板的数据源。
type ShareOverview struct {
	Status            share.Status       `json:"status"`
	Sources           []SourceStatus     `json:"sources"`
	LastConsolidation *LastConsolidation `json:"lastConsolidation"`
	AutoAlign         bool               `json:"autoAlign"`
	PollSeconds       int                `json:"pollSeconds"`
	Enabled           bool               `json:"enabled"`
}

// AlignResult 是一次对齐（单份或全部）的结果。
type AlignResult struct {
	Failed   bool     `json:"failed"`
	Title    string   `json:"title"`
	Message  string   `json:"message"`
	Log      string   `json:"log"`
	Aligned  []string `json:"aligned"`
	Failures []string `json:"failures"`
}

// ShareOverview 返回四个来源的对齐/待整合状态（前端轮询用）。
func (app *App) ShareOverview() ShareOverview {
	app.mu.Lock()
	defer app.mu.Unlock()
	app.ensureSession()
	overview := ShareOverview{
		Enabled: app.shareConfig.Enabled(), AutoAlign: app.shareConfig.SyncAuto,
		PollSeconds: app.shareConfig.PollSeconds,
	}
	if overview.PollSeconds <= 0 {
		overview.PollSeconds = 30
	}
	overview.Status = app.refreshShareStatus()
	if !app.shareConfig.Enabled() {
		return overview
	}
	state, err := share.LoadState(app.shareConfig.Root)
	if err != nil {
		overview.Status.Healthy = false
		overview.Status.Detail = err.Error()
		return overview
	}
	synced, err := app.repo.SyncedVersions()
	if err != nil {
		app.appendLog(LogWarning, "读本机对齐记录失败："+err.Error())
		synced = map[string]store.SyncRecord{}
	}
	var last *share.Consolidation
	if state.LastConsolidation != nil {
		last = state.LastConsolidation
		overview.LastConsolidation = &LastConsolidation{
			At: last.At, By: last.By.Name, Versions: last.Versions,
		}
		if last.By.Role != "" {
			overview.LastConsolidation.By = last.By.Name + "（" + auth.Label(auth.Role(last.By.Role)) + "）"
		}
	}
	role := app.session.Role
	for _, source := range app.importer.Order {
		rule := app.importer.Rules[source]
		status := SourceStatus{Source: source, Label: rule.ChineseName}
		status.CanView = auth.CanReview(role, source)
		remote := state.Sources[source]
		local := synced[source]
		if remote != nil {
			status.PublishedAt = remote.PublishedAt
			status.Aligned = local.Version != "" && local.Version == remote.Version
			status.Pending = last == nil || last.Versions[source] != remote.Version
			// 明细（版本/行数/发布人）只对有权查看该来源的角色下发
			if status.CanView {
				status.RemoteVersion = remote.Version
				status.RemoteRows = remote.Rows
				status.PublishedBy = remote.PublishedBy.Name
				status.LocalVersion = local.Version
			}
		} else if status.CanView {
			status.LocalVersion = local.Version
		}
		overview.Sources = append(overview.Sources, status)
	}
	return overview
}

// AlignSource 对齐单个来源（SC/Admin）。
func (app *App) AlignSource(source string) AlignResult {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.beginOperation(); err != nil {
		return AlignResult{Failed: true, Title: "错误", Message: userMessage(err)}
	}
	defer app.endOperation()
	if err := app.authorize(auth.ActionAlign); err != nil {
		return AlignResult{Failed: true, Title: "无权限", Message: userMessage(err), Log: app.logText()}
	}
	version, err := app.alignSource(source)
	if err != nil {
		app.audit("align", source, err.Error(), "failed")
		return AlignResult{Failed: true, Title: "对齐失败", Message: userMessage(err), Log: app.logText()}
	}
	return AlignResult{
		Log: app.logText(), Aligned: []string{source},
		Title: "对齐完成", Message: fmt.Sprintf("%s已对齐到共享盘版本 %s。", app.importer.Rules[source].ChineseName, version),
	}
}

// AlignAll 把所有"落后于共享盘"的来源逐个对齐。
func (app *App) AlignAll() AlignResult {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.beginOperation(); err != nil {
		return AlignResult{Failed: true, Title: "错误", Message: userMessage(err)}
	}
	defer app.endOperation()
	if err := app.authorize(auth.ActionAlign); err != nil {
		return AlignResult{Failed: true, Title: "无权限", Message: userMessage(err), Log: app.logText()}
	}
	status := app.refreshShareStatus()
	if !status.Healthy {
		return AlignResult{Failed: true, Title: "共享盘异常",
			Message: "共享盘不可用，无法对齐：" + status.Detail, Log: app.logText()}
	}
	state, err := share.LoadState(app.shareConfig.Root)
	if err != nil {
		return AlignResult{Failed: true, Title: "对齐失败", Message: userMessage(err), Log: app.logText()}
	}
	synced, _ := app.repo.SyncedVersions()
	result := AlignResult{Log: app.logText(), Title: "对齐完成"}
	for _, source := range app.importer.Order {
		remote := state.Sources[source]
		if remote == nil {
			continue // 还没发布过的来源跳过（不阻塞其他来源）
		}
		if synced[source].Version == remote.Version {
			continue
		}
		if _, err := app.alignSource(source); err != nil {
			result.Failures = append(result.Failures, app.importer.Rules[source].ChineseName+"："+err.Error())
			app.audit("align", source, err.Error(), "failed")
			continue
		}
		result.Aligned = append(result.Aligned, source)
	}
	result.Log = app.logText()
	if len(result.Failures) > 0 {
		result.Failed = true
		result.Title = "部分对齐失败"
		result.Message = "已对齐：" + describeSources(app, result.Aligned) +
			"\n失败：" + strings.Join(result.Failures, "；")
		return result
	}
	if len(result.Aligned) == 0 {
		result.Message = "本机已与共享盘一致，不需要对齐。"
		return result
	}
	result.Message = "已对齐：" + describeSources(app, result.Aligned) + "。"
	return result
}

// alignSource 是单份对齐的实现（不加锁，调用方已持有 app.mu）。
func (app *App) alignSource(source string) (string, error) {
	if !app.shareConfig.Enabled() {
		return "", fmt.Errorf("没有配置共享盘（本地模式不需要对齐）")
	}
	rule, known := app.importer.Rules[source]
	if !known {
		return "", fmt.Errorf("未知来源：%s", source)
	}
	state, err := share.LoadState(app.shareConfig.Root)
	if err != nil {
		return "", err
	}
	remote := state.Sources[source]
	if remote == nil {
		return "", fmt.Errorf("共享盘上还没有「%s」的发布数据", rule.ChineseName)
	}
	// 拉取 + sha256/行数校验（不通过就中止，绝不降级用旧版）
	manifest, rows, columns, err := publish.Fetch(app.shareConfig.Root, source, remote.Version)
	if err != nil {
		return "", err
	}
	if err := store.ImportSourceRows(app.repo, store.SourceTable{
		SourceName: source, TableName: rule.TargetTable, Columns: columns, ReplaceOnImport: true,
	}, rows); err != nil {
		return "", err
	}
	cleanedRows := 0
	if rule.Clean != nil {
		count, err := importer.RebuildCleaned(app.repo, rule)
		if err != nil {
			return "", err
		}
		cleanedRows = count
	}
	if err := app.repo.SaveSyncedVersion(store.SyncRecord{
		Source: source, Version: remote.Version, SHA256: manifest.SHA256, Rows: len(rows),
		SyncedAt: auth.Now(), SyncedBy: app.owner().Name,
	}); err != nil {
		return "", err
	}
	app.importStates[source] = ImportState{
		Source: source, State: "existing",
		Label:   fmt.Sprintf("已对齐 %s 条记录", formatCount(len(rows))),
		Tooltip: "已对齐共享盘版本 " + remote.Version, RowCount: len(rows), ImportTime: auth.Now(),
	}
	detail := "版本=" + remote.Version + " 行数=" + fmt.Sprint(len(rows))
	if cleanedRows > 0 {
		detail += " 清洗后=" + fmt.Sprint(cleanedRows)
	}
	app.audit("align", source, detail, "ok")
	app.appendLog(LogSuccess, fmt.Sprintf("%s已对齐到共享盘版本 %s", rule.ChineseName, remote.Version))
	return remote.Version, nil
}

// requireAligned 是整合前的对齐门禁（分享盘模式 + require_before_consolidate 才生效）。
func (app *App) requireAligned() error {
	if !app.shareConfig.Enabled() || !app.shareConfig.RequireBeforeConsolidate {
		return nil
	}
	state, err := share.LoadState(app.shareConfig.Root)
	if err != nil {
		return err
	}
	synced, err := app.repo.SyncedVersions()
	if err != nil {
		return err
	}
	missing := []string{}
	for _, source := range app.importer.Order {
		remote := state.Sources[source]
		if remote == nil {
			missing = append(missing, app.importer.Rules[source].ChineseName+"（还没发布）")
			continue
		}
		if synced[source].Version != remote.Version {
			missing = append(missing, app.importer.Rules[source].ChineseName)
		}
	}
	if len(missing) > 0 {
		app.audit("consolidate", "", "未对齐："+strings.Join(missing, "、"), "denied")
		return fmt.Errorf("还有 %d 份数据未对齐：%s。请先在「数据对齐」里对齐，再整合。",
			len(missing), strings.Join(missing, "、"))
	}
	return nil
}

// recordConsolidation 把本次整合用的版本写进 state（"待整合"标志据此清除）。
func (app *App) recordConsolidation(counts map[string]int) {
	if !app.shareConfig.Enabled() {
		return
	}
	synced, err := app.repo.SyncedVersions()
	if err != nil {
		app.appendLog(LogWarning, "读本机对齐记录失败："+err.Error())
		return
	}
	versions := map[string]string{}
	for _, source := range app.importer.Order {
		if record, ok := synced[source]; ok && record.Version != "" {
			versions[source] = record.Version
		}
	}
	now := auth.Now()
	// 目录名只用安全字符（consolidated/<id>/）
	id := time.Now().Format("20060102T150405")
	err = share.WithStateLock(app.shareConfig.Root, func() error {
		state, err := share.LoadState(app.shareConfig.Root)
		if err != nil {
			return err
		}
		state.LastConsolidation = &share.Consolidation{
			ID: id, At: now, By: app.owner(), Versions: versions,
			Counts: share.Counts{
				Ready:     counts[consolidation.StatusReady],
				Missing:   counts[consolidation.StatusIncomplete],
				Duplicate: counts[consolidation.StatusDuplicate],
				Conflict:  counts[consolidation.StatusConflict],
			},
			Email: share.EmailStatus{Status: "disabled"},
		}
		state.Generation++
		return state.Save(app.shareConfig.Root)
	})
	if err != nil {
		app.appendLog(LogWarning, "写共享盘整合记录失败："+err.Error())
		app.audit("consolidate", "", "写共享盘状态失败："+err.Error(), "failed")
		return
	}
	app.audit("consolidate", "", "已记录整合版本 "+now, "ok")
}

// refreshShareStatus 做一次共享盘检查（不写审计；RecheckShare 才写）。
func (app *App) refreshShareStatus() share.Status {
	if app.session != nil && !app.shareConfig.Enabled() {
		return share.Status{Mode: "local", Healthy: true, Detail: "本地模式：未配置共享盘", CheckedAt: auth.Now()}
	}
	status := app.shareConfig.Check()
	app.shareStatus = status
	if app.session != nil {
		app.session.ShareReady = status.Healthy
		app.session.ShareDetail = status.Detail
	}
	return status
}

// describeSources 把来源键渲染成"RA信息、UDI团队信息"。
func describeSources(app *App, sources []string) string {
	labels := make([]string, 0, len(sources))
	for _, source := range sources {
		if rule, ok := app.importer.Rules[source]; ok {
			labels = append(labels, rule.ChineseName)
			continue
		}
		labels = append(labels, source)
	}
	return strings.Join(labels, "、")
}

// shareState 读共享盘状态（共享盘未配置时返回空状态）。
func (app *App) shareState() (*share.State, error) {
	if !app.shareConfig.Enabled() {
		return &share.State{Sources: map[string]*share.SourceVersion{}}, nil
	}
	return share.LoadState(app.shareConfig.Root)
}
