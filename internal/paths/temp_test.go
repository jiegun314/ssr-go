package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInsideTempDirectoryResolvesSymlinks(t *testing.T) {
	// macOS 上 /var 是 /private/var 的符号链接：两种写法都要判为「在临时目录里」
	resolvedTemp, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		t.Skip("无法解析系统临时目录")
	}
	inside := filepath.Join(os.TempDir(), "go-build123", "exe", "ssr-core")
	if !insideTempDirectory(inside) {
		t.Errorf("%s 应当被判定为在临时目录里", inside)
	}
	alsoInside := filepath.Join(resolvedTemp, "go-build123", "ssr-core")
	if !insideTempDirectory(alsoInside) {
		t.Errorf("%s 应当被判定为在临时目录里", alsoInside)
	}
	outside := filepath.Join(string(filepath.Separator), "Users", "someone", "repo", "bin", "ssr-core")
	if insideTempDirectory(outside) {
		t.Errorf("%s 不该被判定为在临时目录里", outside)
	}
}
