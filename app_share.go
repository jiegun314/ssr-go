package main

// P4：整合改造——共享盘整合租约（防两人同时整合）、结果归档到 consolidated/<id>/、
// 以及"整合期间有人发布"的三种策略（warn / auto_rerun / block）。
//
// 口径见 docs/design/multiuser-workflow.md §7.3/§10：
//   - 整合前已校验"四份都已对齐"（P3），这里再加一层共享盘租约；
//   - 整合开始记录四个版本，结束后再读一次共享盘：版本变了按策略处理；
//   - 导出成功后把文件复制到 consolidated/<id>/ 并写 summary.json，
//     再把相对路径写回 state.last_consolidation.result。

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jiegun314/ssr-go/internal/auth"
	"github.com/jiegun314/ssr-go/internal/fileutil"
	"github.com/jiegun314/ssr-go/internal/share"
)

// Consolidate 跑整合：权限 → 对齐门禁 → 共享盘租约 → 整合主体 → 期间新版本策略。
func (app *App) Consolidate() ConsolidateResult {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.beginOperation(); err != nil {
		return ConsolidateResult{Failed: true, Title: "错误", Message: userMessage(err)}
	}
	defer app.endOperation()
	if err := app.authorize(auth.ActionConsolidate); err != nil {
		return ConsolidateResult{Failed: true, Title: "无权限", Message: userMessage(err), Log: app.logText()}
	}
	if err := app.requireAligned(); err != nil {
		return ConsolidateResult{Failed: true, Title: "数据未对齐", Message: userMessage(err), Log: app.logText()}
	}
	if app.shareConfig.Enabled() {
		if err := app.blockOnRecentPublish(); err != nil {
			return ConsolidateResult{Failed: true, Title: "刚有新版本发布", Message: userMessage(err), Log: app.logText()}
		}
	}
	lockID := ""
	if app.shareConfig.Enabled() {
		lockID = "consolidate-" + time.Now().Format("20060102T150405")
		if err := share.AcquireConsolidation(app.shareConfig.Root, lockID, app.owner()); err != nil {
			return ConsolidateResult{Failed: true, Title: "无法整合", Message: userMessage(err), Log: app.logText()}
		}
		defer share.ReleaseConsolidation(app.shareConfig.Root, lockID)
		app.audit("consolidate", "", "获取整合租约 "+lockID, "ok")
	}
	startVersions := app.shareVersions()
	result := app.consolidateOnce()
	if result.Failed || !app.shareConfig.Enabled() {
		return result
	}
	if !sameVersions(startVersions, app.shareVersions()) {
		switch app.shareConfig.OnNewVersion {
		case "block":
			result.Notice = "整合期间有新版本发布（block 策略）：本次结果只包含整合开始时的版本，请重新对齐后再整合。"
		case "auto_rerun":
			app.alignChangedSources()
			retry := app.consolidateOnce()
			retry.Notice = "整合期间有新版本发布，已自动对齐并重新整合一次。"
			return retry
		default:
			result.Notice = "整合期间有新版本发布，建议重新对齐后再整合一次。"
		}
		app.appendLog(LogWarning, result.Notice)
		app.audit("consolidate", "", result.Notice, "warning")
	}
	return result
}

// shareVersions 读共享盘上四个来源的当前版本（共享盘未配置时返回 nil）。
func (app *App) shareVersions() map[string]string {
	if !app.shareConfig.Enabled() {
		return nil
	}
	state, err := share.LoadState(app.shareConfig.Root)
	if err != nil {
		return nil
	}
	versions := map[string]string{}
	for source, item := range state.Sources {
		if item != nil && item.Version != "" {
			versions[source] = item.Version
		}
	}
	return versions
}

// sameVersions 判断两次读到的版本集合是否一致。
func sameVersions(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for source, version := range left {
		if right[source] != version {
			return false
		}
	}
	return true
}

// blockOnRecentPublish 是 block 策略的整合前检查：10 分钟内有发布就先别整合。
func (app *App) blockOnRecentPublish() error {
	if app.shareConfig.OnNewVersion != "block" {
		return nil
	}
	state, err := share.LoadState(app.shareConfig.Root)
	if err != nil {
		return nil
	}
	recent := []string{}
	for _, source := range app.importer.Order {
		item := state.Sources[source]
		if item == nil || item.PublishedAt == "" {
			continue
		}
		published, parseErr := time.ParseInLocation("2006-01-02 15:04:05", item.PublishedAt, time.Local)
		if parseErr != nil {
			continue
		}
		if time.Since(published) < 10*time.Minute {
			recent = append(recent, app.importer.Rules[source].ChineseName+"（"+item.PublishedAt+"）")
		}
	}
	if len(recent) == 0 {
		return nil
	}
	return fmt.Errorf("最近 10 分钟内有新版本发布：%s。请确认数据稳定后再整合（当前策略为 block）。",
		strings.Join(recent, "、"))
}

// alignChangedSources 把"落后于共享盘"的来源再对齐一次（auto_rerun 用）。
func (app *App) alignChangedSources() {
	synced, err := app.repo.SyncedVersions()
	if err != nil {
		return
	}
	state, err := share.LoadState(app.shareConfig.Root)
	if err != nil {
		return
	}
	for _, source := range app.importer.Order {
		remote := state.Sources[source]
		if remote == nil || synced[source].Version == remote.Version {
			continue
		}
		if _, err := app.alignSource(source); err != nil {
			app.appendLog(LogWarning, "自动重对齐失败："+err.Error())
		}
	}
}

// archiveExport 把导出的文件复制到 consolidated/<id>/ 并写 summary.json，
// 再把相对路径写回 state.last_consolidation.result。
func (app *App) archiveExport(localPath string, name string) (string, error) {
	if !app.shareConfig.Enabled() {
		return "", nil
	}
	state, err := share.LoadState(app.shareConfig.Root)
	if err != nil {
		return "", err
	}
	last := state.LastConsolidation
	if last == nil || last.ID == "" {
		return "", fmt.Errorf("还没有整合记录（先点「整合」再导出）")
	}
	directory := filepath.Join(app.shareConfig.Root, "consolidated", last.ID)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", fmt.Errorf("创建结果目录失败：%w", err)
	}
	destination := filepath.Join(directory, name)
	if err := fileutil.CopyFile(localPath, destination); err != nil {
		return "", fmt.Errorf("复制结果文件失败：%w", err)
	}
	summary := map[string]any{
		"id": last.ID, "at": last.At, "by": last.By, "versions": last.Versions,
		"counts": last.Counts, "file": name, "email": map[string]string{"status": "disabled"},
	}
	content, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return "", err
	}
	temp := filepath.Join(directory, "summary.json.tmp")
	if err := os.WriteFile(temp, append(content, '\n'), 0o644); err != nil {
		return "", fmt.Errorf("写 summary.json 失败：%w", err)
	}
	if err := os.Rename(temp, filepath.Join(directory, "summary.json")); err != nil {
		os.Remove(temp)
		return "", fmt.Errorf("写 summary.json 失败：%w", err)
	}
	relative := filepath.ToSlash(filepath.Join("consolidated", last.ID, name))
	err = share.WithStateLock(app.shareConfig.Root, func() error {
		current, err := share.LoadState(app.shareConfig.Root)
		if err != nil {
			return err
		}
		if current.LastConsolidation == nil || current.LastConsolidation.ID != last.ID {
			return fmt.Errorf("整合记录已变化，请重新整合后再导出")
		}
		current.LastConsolidation.Result = relative
		current.Generation++
		return current.Save(app.shareConfig.Root)
	})
	if err != nil {
		return "", err
	}
	return relative, nil
}
