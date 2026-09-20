// Command SingleSourceReady 是桌面入口（Wails v2）。
//
// 启动顺序是需求的一部分（R27）：先声明 Windows 任务栏身份，再建应用与窗口，最后才把
// 启动期告警交给界面写进操作日志 —— 顺序错了任务栏图标就会变成「Windows 自己猜」。
package main

import (
	"embed"
	"runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/jiegun314/ssr-go/internal/appicon"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// 图标与任务栏身份：资源优先、文件回退、失败只留痕（R27）
	notices := []string{}
	_, iconNotice := appicon.Load()
	if iconNotice != "" {
		notices = append(notices, iconNotice)
	}
	if notice := appicon.SetWindowsAppUserModelID(); notice != "" {
		notices = append(notices, notice)
	}

	application := NewApp(notices)
	applicationMenu := buildMenu(application)
	err := wails.Run(&options.App{
		Title:            "SSR - UDI数据整合平台",
		Width:            969,
		Height:           962,
		MinWidth:         969,
		MinHeight:        760, // 保证最小尺寸下 记录导出/操作日志 都完整可见
		DisableResize:    false,
		BackgroundColour: &options.RGBA{R: 240, G: 240, B: 240, A: 1},
		AssetServer:      &assetserver.Options{Assets: assets},
		Menu:             applicationMenu,
		OnStartup:        application.startup,
		Bind:             []any{application},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}

// buildMenu 复刻现有界面的菜单：设置（参数设定）、关于（关于）。
//
// 曾经的「文件」菜单已删除：它的「打开」走的是"按文件名猜来源"的启发式路径
// （四个来源的表头校验逐个试，容易把文件导进错的来源），而界面里每个来源都有自己的
// 导入按钮；「退出」在 macOS 由系统应用菜单提供、Windows 直接关窗口即可。
func buildMenu(application *App) *menu.Menu {
	root := menu.NewMenu()
	// macOS 会把第一个子菜单当作「应用菜单」：先补上系统应用菜单，
	// 我们的「设置」「关于」才会作为独立菜单出现在菜单栏里（否则第一个会变成应用菜单本身）。
	if runtime.GOOS == "darwin" {
		root.Append(menu.AppMenu())
	}
	settings := root.AddSubmenu("设置")
	settings.AddText("参数设定", nil, func(*menu.CallbackData) {
		application.ShowSettings()
	})
	about := root.AddSubmenu("关于")
	about.AddText("关于", nil, func(*menu.CallbackData) {
		application.ShowAbout()
	})
	return root
}
