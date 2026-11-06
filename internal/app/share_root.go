package app

// 共享盘位置：每台机器的路径可能不同，所以所有角色都可以设置；
// 其余配置（含人员名单）仍然只有 Admin 能改。

import (
	"fmt"
	"strings"

	"github.com/jiegun314/ssr-go/internal/auth"
	"github.com/jiegun314/ssr-go/internal/config"
	"github.com/jiegun314/ssr-go/internal/share"
)

// ShareRootInfo 是「共享盘位置」弹窗需要的当前值。
type ShareRootInfo struct {
	Root    string `json:"root"`
	Mode    string `json:"mode"`
	Healthy bool   `json:"healthy"`
	Detail  string `json:"detail"`
}

// GetShareRoot 读取当前共享盘位置与状态（所有登录角色都可调用）。
func (app *App) GetShareRoot() ShareRootInfo {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.authorize(auth.ActionSetShareRoot); err != nil {
		return ShareRootInfo{Detail: userMessage(err)}
	}
	return ShareRootInfo{
		Root:    app.shareConfig.Root,
		Mode:    app.shareStatus.Mode,
		Healthy: app.shareStatus.Healthy,
		Detail:  app.shareStatus.Detail,
	}
}

// SaveShareRoot 只改 workflow.yaml 的 share.root，其它内容（注释/键序/其它配置）原样保留。
func (app *App) SaveShareRoot(root string) SettingsSaveResult {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.authorize(auth.ActionSetShareRoot); err != nil {
		return SettingsSaveResult{Failed: true, Title: "无权限", Message: userMessage(err)}
	}
	root = strings.TrimSpace(root)
	lower := strings.ToLower(root)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return SettingsSaveResult{Failed: true, Title: "地址不可用",
			Message: "SharePoint/OneDrive 的在线地址不能直接使用。\n" +
				"请先用 OneDrive 把该文件夹同步到本机，再填同步后的本地绝对路径" +
				"（例如 C:\\Users\\你\\公司\\SSR - Documents\\SSRShare）。"}
	}
	if !share.ValidRoot(root) {
		return SettingsSaveResult{Failed: true, Title: "地址不可用",
			Message: "共享盘位置必须是本地绝对路径或 UNC 共享路径；留空表示本地模式。"}
	}
	// 复用参数设定的安全通道：备份 → 只改这一行 → 整体校验 → 失败还原 → 热重载
	result := app.saveSettingsValuesLocked(config.WorkflowFile, []SettingsChange{
		{Path: []string{"share", "root"}, Value: root},
	})
	if result.Failed {
		return result
	}
	status := app.checkShare()
	app.audit("settings", "share.root", fmt.Sprintf("root=%s 状态=%s", root, status.Detail), "ok")
	app.appendLog(LogInfo, "共享盘位置已更新："+root+"（"+status.Detail+"）")
	result.Message = strings.TrimSpace(result.Message) + "\n共享盘状态：" + status.Detail
	return result
}
