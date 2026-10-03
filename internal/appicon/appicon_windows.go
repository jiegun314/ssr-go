//go:build windows

package appicon

import (
	"golang.org/x/sys/windows"
)

// setAppUserModelID 在 Windows 上显式声明任务栏身份，失败时返回告警文案（R27）。
func setAppUserModelID() string {
	shell32 := windows.NewLazySystemDLL("shell32.dll")
	procedure := shell32.NewProc("SetCurrentProcessExplicitAppUserModelID")
	id, err := windows.UTF16PtrFromString(AppUserModelID)
	if err != nil {
		return "设置 Windows 任务栏身份失败（" + AppUserModelID + "）：" + err.Error()
	}
	if _, _, callErr := procedure.Call(uintptr(unsafePointer(id))); callErr != nil && callErr.Error() != "The operation completed successfully." {
		return "设置 Windows 任务栏身份失败（" + AppUserModelID + "）：" + callErr.Error()
	}
	return ""
}
