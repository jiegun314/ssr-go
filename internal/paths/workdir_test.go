package paths

import (
	"os"
	"path/filepath"
	"testing"
)

// 源码运行（go run）时，工作目录里的 config/ 优先于可执行文件所在目录，
// 否则项目根会被算到 Go 构建缓存里。
func TestWorkingDirectoryConfigWinsForASourceRun(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	t.Chdir(root)
	t.Setenv(ConfigEnvironmentVariable, "")
	os.Unsetenv(ConfigEnvironmentVariable)

	resolver, err := New("")
	if err != nil {
		t.Fatalf("New 失败：%v", err)
	}
	if resolver.ConfigDir != configDir {
		t.Fatalf("ConfigDir = %q; want %q", resolver.ConfigDir, configDir)
	}
	if resolver.ProjectRoot != root {
		t.Fatalf("ProjectRoot = %q; want %q", resolver.ProjectRoot, root)
	}
}

// 环境变量仍然优先于工作目录。
func TestEnvironmentStillWinsOverTheWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	other := t.TempDir()
	if err := os.MkdirAll(filepath.Join(other, "config"), 0o755); err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	t.Chdir(root)
	t.Setenv(ConfigEnvironmentVariable, filepath.Join(other, "config"))

	resolver, err := New("")
	if err != nil {
		t.Fatalf("New 失败：%v", err)
	}
	if resolver.ConfigDir != filepath.Join(other, "config") {
		t.Fatalf("ConfigDir = %q; want 环境变量指定的那一个", resolver.ConfigDir)
	}
}
