package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigDirectoryFollowsTheEnvironmentVariable(t *testing.T) {
	// §4.3：UDI_CONFIG_DIR 决定配置目录，project_root 是它的父目录
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	t.Setenv(ConfigEnvironmentVariable, configDir)

	resolver, err := New("")
	if err != nil {
		t.Fatalf("New 失败：%v", err)
	}
	if resolver.ConfigDir != configDir {
		t.Errorf("ConfigDir = %q; want %q", resolver.ConfigDir, configDir)
	}
	if resolver.ProjectRoot != root {
		t.Errorf("ProjectRoot = %q; want %q", resolver.ProjectRoot, root)
	}
	if got := resolver.Resolve("data/test.sqlite3"); got != filepath.Join(root, "data", "test.sqlite3") {
		t.Errorf("相对路径应相对 project_root：%q", got)
	}
	if got := resolver.Resolve("/tmp/absolute.sqlite3"); got != "/tmp/absolute.sqlite3" {
		t.Errorf("绝对路径原样使用：%q", got)
	}
}

func TestExplicitConfigurationDirectoryWins(t *testing.T) {
	t.Setenv(ConfigEnvironmentVariable, filepath.Join(t.TempDir(), "from-env"))
	explicit := t.TempDir()

	resolver, err := New(explicit)
	if err != nil {
		t.Fatalf("New 失败：%v", err)
	}

	if resolver.ConfigDir != explicit {
		t.Errorf("显式参数优先：%q", resolver.ConfigDir)
	}
}
