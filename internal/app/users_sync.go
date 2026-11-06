package app

// 人员名单同步（P1 延伸）：Admin 保存 users.yaml 后发布到共享盘；其它电脑在启动/登录时
// 按 manifest 校验并拉取。最后保存的 Admin 版本为准；共享盘不可用时保持本机并等下次登录重试。

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jiegun314/ssr-go/internal/auth"
	"github.com/jiegun314/ssr-go/internal/config"
	"github.com/jiegun314/ssr-go/internal/usersync"
	"gopkg.in/yaml.v3"
)

const usersMarkerName = "users.sync.json"

type usersMarker struct {
	SHA256  string `json:"sha256"`
	Version string `json:"version,omitempty"`
	// Pending = 本机保存过但还没同步到共享盘（下一次 Admin 登录时重试发布）。
	Pending bool `json:"pending,omitempty"`
}

func (app *App) usersConfigPath() string {
	if app.loader == nil {
		return ""
	}
	return app.loader.Resolver.ConfigFile(config.UsersFile)
}

func (app *App) usersMarkerPath() string {
	if app.loader == nil {
		return ""
	}
	return filepath.Join(app.loader.Resolver.ConfigDir, usersMarkerName)
}

func (app *App) readUsersMarker() (usersMarker, bool) {
	path := app.usersMarkerPath()
	if path == "" {
		return usersMarker{}, false
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return usersMarker{}, false
	}
	var marker usersMarker
	if err := json.Unmarshal(content, &marker); err != nil {
		return usersMarker{}, false
	}
	return marker, true
}

func (app *App) writeUsersMarker(marker usersMarker) {
	path := app.usersMarkerPath()
	if path == "" {
		return
	}
	content, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return
	}
	if err := writeLocalAtomic(path, append(content, '\n')); err != nil {
		app.appendLog(LogWarning, "人员名单同步标记写入失败："+err.Error())
	}
}

// localUsersHash 是当前本机 users.yaml 的 sha256；文件不存在时返回空串。
func (app *App) localUsersHash() string {
	path := app.usersConfigPath()
	if path == "" {
		return ""
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return usersync.Hash(content)
}

func (app *App) isAdminSession() bool {
	return app.session != nil && app.session.Role == auth.RoleAdmin
}

// syncUsersFromShare 在启动/登录时调用：
//   - 本机有 pending 保存：Admin 登录时重试发布，其它情况不覆盖本机；
//   - 共享盘版本与本地一致：只更新标记；
//   - 共享盘更新且本机没有未同步改动：校验后替换本机 users.yaml 并热更新内存名单。
func (app *App) syncUsersFromShare(trigger string) {
	if app.loader == nil || !app.shareConfig.Enabled() {
		return
	}
	marker, hasMarker := app.readUsersMarker()
	if hasMarker && marker.Pending {
		if trigger == "startup" {
			return
		}
		if app.isAdminSession() {
			if version, err := app.publishUsersToShare(); err == nil {
				app.appendLog(LogSuccess, "人员名单已补同步到共享盘："+version)
			} else {
				app.appendLog(LogWarning, "人员名单补同步失败："+err.Error())
			}
			return
		}
		app.appendLog(LogWarning, "人员名单本机有未同步的改动（等待 Admin 登录后发布）")
		return
	}
	content, manifest, err := usersync.Fetch(app.shareConfig.Root)
	if err != nil {
		// 还没人发布 / 共享盘暂时不可达：保持本机名单，下次登录再试
		return
	}
	localPath := app.usersConfigPath()
	localHash := app.localUsersHash()
	if localHash == manifest.SHA256 {
		app.writeUsersMarker(usersMarker{SHA256: manifest.SHA256, Version: manifest.Version})
		return
	}
	// 本机改过、但不等于上次同步的版本：可能是 Admin 离线保存（pending 已覆盖），
	// 也可能是手工改过文件；都不覆盖，避免丢改动。
	if hasMarker && marker.SHA256 != "" && localHash != marker.SHA256 {
		app.appendLog(LogWarning, "人员名单：本机有未同步的改动，跳过共享盘更新")
		return
	}
	var document map[string]any
	if err := yaml.Unmarshal(content, &document); err != nil {
		app.appendLog(LogWarning, "共享盘人员名单解析失败，已忽略："+err.Error())
		return
	}
	users, err := auth.ParseUsers(document)
	if err != nil || len(users) == 0 {
		app.appendLog(LogWarning, "共享盘人员名单不合法，已忽略："+fmt.Sprint(err))
		return
	}
	if localPath == "" {
		return
	}
	if raw, err := os.ReadFile(localPath); err == nil {
		_ = os.WriteFile(localPath+".bak", raw, 0o644)
	}
	if err := writeLocalAtomic(localPath, content); err != nil {
		app.appendLog(LogWarning, "人员名单写入本机失败："+err.Error())
		return
	}
	app.users = users
	app.singleUser = false
	app.writeUsersMarker(usersMarker{SHA256: manifest.SHA256, Version: manifest.Version})
	app.audit("users-sync", "users.yaml", "从共享盘更新：版本="+manifest.Version, "ok")
	app.appendLog(LogSuccess, "人员名单已从共享盘更新（"+manifest.Version+"）")
}

// publishUsersToShare 把本机 users.yaml 发布到共享盘（只有 Admin 会话会调用）。
func (app *App) publishUsersToShare() (string, error) {
	if !app.isAdminSession() {
		return "", fmt.Errorf("只有 Admin 可以发布人员名单")
	}
	if !app.shareEnabled() {
		return "", fmt.Errorf("共享盘未启用")
	}
	if !app.shareStatus.Healthy {
		return "", fmt.Errorf("共享盘不可用：%s", app.shareStatus.Detail)
	}
	path := app.usersConfigPath()
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	manifest, err := usersync.Publish(app.shareConfig.Root, content, app.session.Username)
	if err != nil {
		return "", err
	}
	app.writeUsersMarker(usersMarker{SHA256: manifest.SHA256, Version: manifest.Version})
	app.audit("users-sync", "users.yaml", "发布到共享盘：版本="+manifest.Version, "ok")
	app.appendLog(LogSuccess, "人员名单已同步到共享盘："+manifest.Version)
	return manifest.Version, nil
}

// afterUsersSaved 只在 Admin 保存 users.yaml 后调用：
// 成功发布就记版本；失败/未启用共享盘则打 pending 标记，Admin 下次登录重试。
func (app *App) afterUsersSaved(result *SettingsSaveResult) {
	if app.session == nil || app.session.Role != auth.RoleAdmin {
		return
	}
	if !app.shareConfig.Enabled() {
		app.writeUsersMarker(usersMarker{SHA256: app.localUsersHash(), Pending: true})
		result.Message = strings.TrimSpace(result.Message) +
			"\n当前未启用共享盘：人员名单只保存在本机，启用后 Admin 登录时会自动同步。"
		return
	}
	version, err := app.publishUsersToShare()
	if err != nil {
		app.writeUsersMarker(usersMarker{SHA256: app.localUsersHash(), Pending: true})
		app.appendLog(LogWarning, "人员名单同步到共享盘失败："+err.Error())
		result.Message = strings.TrimSpace(result.Message) +
			"\n人员名单已保存在本机，但同步到共享盘失败（" + err.Error() + "）；Admin 下次登录会自动重试。"
		return
	}
	result.Message = strings.TrimSpace(result.Message) + "\n人员名单已同步到共享盘：" + version
}

// writeLocalAtomic 用同目录临时文件 + rename，避免热重载读到半份文件。
func writeLocalAtomic(path string, content []byte) error {
	handle, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	temp := handle.Name()
	defer os.Remove(temp)
	if _, err := handle.Write(content); err != nil {
		handle.Close()
		return err
	}
	if err := handle.Sync(); err != nil {
		handle.Close()
		return err
	}
	if err := handle.Close(); err != nil {
		return err
	}
	return os.Rename(temp, path)
}
