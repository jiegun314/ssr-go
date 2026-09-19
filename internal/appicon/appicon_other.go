//go:build !windows

package appicon

// setAppUserModelID 在其他平台上没有这个概念：不做事，也不留告警（R27）。
func setAppUserModelID() string { return "" }
