// Package appicon 负责窗口/任务栏图标与 Windows 任务栏身份（R27）。
//
// 三条必须保留的行为：
//   - 图标**优先从资源读**（Go 用 //go:embed 把 ico 编进二进制），exe 同级的
//     resource/logo.ico 只是回退；
//   - 启动时**先声明** AppUserModelID，再建窗口；
//   - 任何一步失败都**不阻断启动**，只留一条启动告警（由界面写进操作日志）。
package appicon

import (
	_ "embed"
	"os"
	"path/filepath"
)

// AppUserModelID 让 Windows 按这个身份找任务栏按钮（快捷方式要用同一个 id 才归组）。
const AppUserModelID = "bioMerieux.SingleSourceReady"

// EmbeddedIcon 是编进二进制的图标（等价于 Python 版的 Qt 资源 :/app/logo.ico）。
//
//go:embed icon.ico
var EmbeddedIcon []byte

// EmbeddedIconDescription 是告警文案里对资源那份图标的称呼（与 Python 版一致）。
const EmbeddedIconDescription = "the embedded icon"

// IconPath 返回可用的图标文件路径：优先写出内置资源，失败时回退 exe 同级的
// resource/logo.ico。返回的第二个值是启动告警（空串表示没有告警）。
func IconPath() (string, string) {
	if len(EmbeddedIcon) > 0 {
		directory, err := os.UserCacheDir()
		if err == nil {
			target := filepath.Join(directory, "SingleSourceReady", "logo.ico")
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err == nil {
				if err := os.WriteFile(target, EmbeddedIcon, 0o644); err == nil {
					return target, ""
				}
			}
		}
	}
	fallback := FallbackIconPath()
	if _, err := os.Stat(fallback); err == nil {
		return fallback, "Application icon loaded from " + fallback +
			" (" + EmbeddedIconDescription + " unavailable)"
	}
	return "", "Failed to load the application icon from " + EmbeddedIconDescription +
		" and " + fallback + "; the taskbar icon may be missing"
}

// FallbackIconPath 是 exe 同级目录里的图标（源码运行时是仓库根目录）。
func FallbackIconPath() string {
	executable, err := os.Executable()
	if err != nil {
		return filepath.Join("resource", "logo.ico")
	}
	return filepath.Join(filepath.Dir(executable), "resource", "logo.ico")
}

// Load 是入口使用的组合：返回图标路径与告警。
func Load() (string, string) { return IconPath() }

// SetWindowsAppUserModelID 在 Windows 上声明任务栏身份；其他平台是空操作。
// 返回空串表示成功（或平台不需要），返回非空串是启动告警（不阻断启动）。
func SetWindowsAppUserModelID() string { return setAppUserModelID() }
