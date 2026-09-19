package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInsideTempDirectoryResolvesSymlinks(t *testing.T) {
	// 用真实创建的目录测试：macOS 上 /var 是 /private/var 的符号链接，两种写法都要判为
	// 「在临时目录里」。路径必须真实存在，否则 EvalSymlinks 无法把 /var 解析成 /private/var。
	nested := filepath.Join(os.TempDir(), "ssr-paths-test", "exe")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	t.Cleanup(func() { os.RemoveAll(filepath.Join(os.TempDir(), "ssr-paths-test")) })

	executable := filepath.Join(nested, "ssr-core")
	if err := os.WriteFile(executable, []byte("x"), 0o644); err != nil {
		t.Fatalf("写文件失败：%v", err)
	}
	if !insideTempDirectory(executable) {
		t.Errorf("%s 应当被判定为在临时目录里", executable)
	}

	resolvedTemp, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		t.Skip("无法解析系统临时目录")
	}
	alsoInside := filepath.Join(resolvedTemp, "ssr-paths-test", "exe", "ssr-core")
	if !insideTempDirectory(alsoInside) {
		t.Errorf("%s 应当被判定为在临时目录里", alsoInside)
	}

	outside := filepath.Join(string(filepath.Separator), "Users", "someone", "repo", "bin", "ssr-core")
	if insideTempDirectory(outside) {
		t.Errorf("%s 不该被判定为在临时目录里", outside)
	}
}
