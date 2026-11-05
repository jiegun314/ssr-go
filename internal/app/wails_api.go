package app

// Wails 入口需要的导出包装：main.go 只负责窗口与资源，应用逻辑都在本包。
//
// 测试仍在包内直接调用 startup / buildMenu（保持原有口径）；这里只暴露 Wails 需要的两个入口。

import (
	"context"
	"runtime"

	"github.com/wailsapp/wails/v2/pkg/menu"
)

// Startup 是 Wails 的 OnStartup 回调。
func (app *App) Startup(ctx context.Context) {
	app.startup(ctx)
}

// BuildMenu 是给 main.go 用的菜单构建入口（内部实现仍是 buildMenu，测试直接调用它）。
func BuildMenu(application *App) *menu.Menu {
	return buildMenu(application)
}

// buildMenu 复刻现有界面的菜单：文件（退出）、编辑（仅 macOS）、设置（参数设定）、
// 工具（打开常用目录 / 备份数据库）、关于。
func buildMenu(application *App) *menu.Menu {
	root := menu.NewMenu()
	if runtime.GOOS == "darwin" {
		root.Append(menu.AppMenu())
		root.Append(menu.EditMenu())
	}
	file := root.AddSubmenu("文件")
	file.AddText("退出", nil, func(*menu.CallbackData) {
		application.Quit()
	})
	settings := root.AddSubmenu("设置")
	settings.AddText("参数设定", nil, func(*menu.CallbackData) {
		application.ShowSettings()
	})
	tools := root.AddSubmenu("工具")
	tools.AddText("打开配置目录", nil, func(*menu.CallbackData) {
		application.OpenConfigDirectory()
	})
	tools.AddText("打开导出目录", nil, func(*menu.CallbackData) {
		application.OpenExportDirectory()
	})
	tools.AddText("打开数据目录", nil, func(*menu.CallbackData) {
		application.OpenDataDirectory()
	})
	tools.AddSeparator()
	tools.AddText("备份数据库…", nil, func(*menu.CallbackData) {
		application.BackupDatabase()
	})
	about := root.AddSubmenu("关于")
	about.AddText("关于", nil, func(*menu.CallbackData) {
		application.ShowAbout()
	})
	return root
}
