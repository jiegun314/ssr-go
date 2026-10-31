package share

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocalModeIsAlwaysHealthy(t *testing.T) {
	status := Config{}.Check()
	if status.Mode != "local" || !status.Healthy {
		t.Fatalf("没有配置 root 时应当是本地模式且健康：%+v", status)
	}
	if !strings.Contains(status.Detail, "本地模式") {
		t.Errorf("说明里应当写明本地模式：%q", status.Detail)
	}
}

func TestMissingRootIsUnhealthy(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "not-there")
	status := Config{Root: missing, ProbeTimeout: time.Second}.Check()
	if status.Healthy {
		t.Fatalf("不存在的目录应当判定不健康：%+v", status)
	}
	if !strings.Contains(status.Detail, "不可达") {
		t.Errorf("原因里应当写不可达：%q", status.Detail)
	}
}

func TestTempRootIsHealthyAndLeavesNoProbe(t *testing.T) {
	root := t.TempDir()
	status := Config{Root: root, ProbeTimeout: time.Second}.Check()
	if !status.Healthy {
		t.Fatalf("可写目录应当健康：%+v", status)
	}
	if status.Mode != "share" {
		t.Errorf("mode = %q；want share", status.Mode)
	}
	entries, err := os.ReadDir(filepath.Join(root, "state"))
	if err != nil {
		t.Fatalf("state 目录应当被创建：%v", err)
	}
	if len(entries) != 0 {
		t.Errorf("探针文件应当被清理干净，实际还剩：%v", entries)
	}
}

func TestReadOnlyRootIsUnhealthy(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o555); err != nil {
		t.Skipf("当前环境不支持改权限：%v", err)
	}
	t.Cleanup(func() { os.Chmod(root, 0o755) })
	status := Config{Root: root, ProbeTimeout: time.Second}.Check()
	if os.Geteuid() == 0 {
		t.Skip("root 用户不受只读目录限制")
	}
	if status.Healthy {
		t.Fatalf("只读目录应当判定不健康：%+v", status)
	}
}
