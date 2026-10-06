// Package fileutil 是文件级的通用小工具（与业务无关，可被 App 与命令行共用）。
package fileutil

import (
	"io"
	"os"
)

// CopyFile 把导出目录里的副本复制到用户选的路径（R20）。
// 两处路径相同的情况由调用方先判断并跳过，否则会抛 SameFileError。
//
// 走流式复制并 fsync：用户选的目标可能是移动硬盘或网络盘，写完就拔/就关时要落盘。
func CopyFile(source string, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.Create(target)
	if err != nil {
		return err
	}
	defer output.Close()
	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	return output.Sync()
}
