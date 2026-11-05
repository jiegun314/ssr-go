package app

// 测试的工作目录固定为仓库根：历史上这些用例都在根目录的 package main 里跑，
// 路径（config/、testdata/、frontend/…）都是相对仓库根的；搬家后在这里还原同样的口径。

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMain(m *testing.M) {
	if _, file, _, ok := runtime.Caller(0); ok {
		root := filepath.Join(filepath.Dir(file), "..", "..")
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			_ = os.Chdir(root)
		}
	}
	os.Exit(m.Run())
}
