package main

// P2：导入成功后自动把整张表发布到共享盘，并更新 state.json。
//
// 口径（docs/design/multiuser-workflow.md §7/§9）：
//   - 只有配置了 share.root 才发布；没配置 = 本地模式（不发布、不拦操作）；
//   - 发布前复查共享盘，不健康就拒绝导入（避免"以为传上去了"）；
//   - 共享盘上的流程是"先写快照、再改状态"，失败不留半个版本。

import (
	"fmt"
	"time"

	"github.com/jiegun314/ssr-go/internal/auth"
	"github.com/jiegun314/ssr-go/internal/buildinfo"
	"github.com/jiegun314/ssr-go/internal/publish"
	"github.com/jiegun314/ssr-go/internal/share"
)

// RecheckShare 手动重新检查共享盘（状态栏的"重新检查"按钮）。
func (app *App) RecheckShare() share.Status {
	app.mu.Lock()
	defer app.mu.Unlock()
	app.ensureSession()
	status := app.checkShare()
	if status.Healthy {
		app.appendLog(LogInfo, "共享盘检查通过："+status.Detail)
	} else {
		app.appendLog(LogError, "共享盘检查失败："+status.Detail)
	}
	return status
}

// requireShare 是"要写共享盘"的动作前的闸门：本地模式直接放行；配置了共享盘就必须健康。
func (app *App) requireShare(action string) error {
	if !app.shareEnabled() {
		return nil
	}
	status := app.checkShare()
	if status.Healthy {
		return nil
	}
	// 给用户看的文案保持中文、不带底层英文错误（技术详情进审计与运行日志）
	app.audit("deny", action, "共享盘不可用："+status.Detail, "denied")
	app.appendLog(LogError, "共享盘检查失败："+status.Detail)
	return fmt.Errorf("共享盘不可用，暂不能%s。请检查共享盘连接，再点状态栏的「重新检查」。", action)
}

// publishSource 把某个来源当前的整表发布成共享盘快照，返回版本号。
func (app *App) publishSource(source string, originalPath string) (string, error) {
	rule, known := app.importer.Rules[source]
	if !known {
		return "", fmt.Errorf("未知来源：%s", source)
	}
	rows, err := app.repo.Rows(rule.TargetTable)
	if err != nil {
		return "", err
	}
	columns := make([]string, 0, len(rule.Columns))
	for _, column := range rule.Columns {
		columns = append(columns, column.DBField)
	}
	cleanedRows := 0
	if rule.Clean != nil {
		if cleaned, err := app.repo.Rows(rule.Clean.TargetTable); err == nil {
			cleanedRows = len(cleaned)
		}
	}
	// 并发发布：记下发布前 state 里这一来源的版本（有人刚发过就提示覆盖）
	var previous *share.SourceVersion
	if state, err := share.LoadState(app.shareConfig.Root); err == nil {
		previous = state.Sources[source]
	}
	snapshot, err := publish.Publish(publish.Options{
		Root:         app.shareConfig.Root,
		Source:       source,
		Columns:      columns,
		Rows:         rows,
		CleanedRows:  cleanedRows,
		OriginalPath: originalPath,
		Owner:        app.owner(),
		AppVersion:   app.appVersion(),
	})
	if err != nil {
		return "", err
	}
	if snapshot.OriginalWarning != "" {
		app.appendLog(LogWarning, snapshot.OriginalWarning)
	}
	// 10 分钟内别人刚发过同一来源：明确记一笔"覆盖"，事后可审计
	if previous != nil && previous.Version != "" && previous.Version != snapshot.Version &&
		previous.PublishedBy.Name != "" && previous.PublishedBy.Name != app.owner().Name {
		if published, err := time.ParseInLocation("2006-01-02 15:04:05", previous.PublishedAt, time.Local); err == nil &&
			time.Since(published) < 10*time.Minute {
			message := fmt.Sprintf("%s：%s 刚发布的版本 %s 已被本次发布覆盖（新版本 %s）",
				rule.ChineseName, previous.PublishedBy.Name, previous.Version, snapshot.Version)
			app.appendLog(LogWarning, message)
			app.audit("publish", source, message, "warning")
		}
	}
	return snapshot.Version, nil
}

// owner 是写进快照 manifest 与 state.json 的"发布人"。
func (app *App) owner() share.Owner {
	owner := share.Owner{
		Name: auth.CurrentUsername(), Username: auth.CurrentUsername(), Machine: auth.CurrentMachine(),
	}
	if app.session != nil {
		owner.Name = app.session.Name
		owner.Role = string(app.session.Role)
		owner.Username = app.session.Username
		owner.Machine = app.session.Machine
	}
	return owner
}

// appVersion 是写进 manifest 的程序版本（与「关于」窗口同一口径）。
func (app *App) appVersion() string {
	version := ""
	if app.loader != nil {
		if setting, err := app.loader.LoadSetting(); err == nil {
			version = text(setting["APP_VERSION"])
		}
	}
	return buildinfo.ReadBuildInfo(buildinfo.Options{AppVersion: version}).Version
}
