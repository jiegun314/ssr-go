package main

import (
	"io"
	"os"
)

// copyFile 把导出目录里的副本复制到用户选的路径（R20）。
// 两处路径相同的情况由调用方先判断并跳过，否则会抛 SameFileError。
func copyFile(source string, target string) error {
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
