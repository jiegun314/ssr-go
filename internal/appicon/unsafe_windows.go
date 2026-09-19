//go:build windows

package appicon

import "unsafe"

// unsafePointer 把 UTF-16 指针转成 syscall 需要的 uintptr。
func unsafePointer(pointer *uint16) unsafe.Pointer { return unsafe.Pointer(pointer) }
