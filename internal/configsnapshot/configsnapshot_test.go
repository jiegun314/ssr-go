package configsnapshot

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jiegun314/ssr-go/internal/config"
)

// TestConfigSnapshotKeepsTheLastFewVersions 固定启动快照的策略：
// 内容没变不重复建目录，变了才新建，并且只保留最近 configSnapshotKeep 份。
func TestConfigSnapshotKeepsTheLastFewVersions(t *testing.T) {
	// 自己造一个配置目录：本包测试的 CWD 是包目录，不能再依赖"仓库根"的助手。
	configDir := filepath.Join(t.TempDir(), "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	settingPath := filepath.Join(configDir, config.SettingFile)
	if err := os.WriteFile(settingPath, []byte("APP_NAME: \"test\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, config.ImportMappingFile),
		[]byte("version: \"1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	at := time.Date(2026, 9, 20, 22, 30, 0, 0, time.UTC)
	first, err := SnapshotUserConfig(configDir, at)
	if err != nil {
		t.Fatalf("首次快照失败：%v", err)
	}
	if filepath.Base(first) != "20260920-223000" {
		t.Errorf("快照目录名 = %q；want 20260920-223000", filepath.Base(first))
	}
	if _, err := os.Stat(filepath.Join(first, config.SettingFile)); err != nil {
		t.Errorf("快照里缺少配置文件：%v", err)
	}

	// 内容没变：不重复建
	if again, err := SnapshotUserConfig(configDir, at.Add(time.Minute)); err != nil {
		t.Fatal(err)
	} else if again != "" {
		t.Errorf("内容没变却又建了一份：%s", again)
	}

	// 内容变了：新建一份，并且只保留最近 5 份
	original, err := os.ReadFile(settingPath)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < configSnapshotKeep+2; index++ {
		changed := append([]byte{}, original...)
		changed = append(changed, []byte("\n# change "+string(rune('a'+index))+"\n")...)
		if err := os.WriteFile(settingPath, changed, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := SnapshotUserConfig(configDir, at.Add(time.Duration(index+1)*time.Minute)); err != nil {
			t.Fatalf("第 %d 次快照失败：%v", index+1, err)
		}
	}
	snapshots, err := existingSnapshots(filepath.Join(configDir, configSnapshotDirectory))
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != configSnapshotKeep {
		t.Errorf("保留的快照份数 = %d；want %d（%v）", len(snapshots), configSnapshotKeep, snapshots)
	}
}

// TestConfigSnapshotSkipsAnEmptyDirectory 配置目录里一份都没有时不建快照（也没什么可保的）。
func TestConfigSnapshotSkipsAnEmptyDirectory(t *testing.T) {
	snapshot, err := SnapshotUserConfig(t.TempDir(), time.Now())
	if err != nil {
		t.Fatalf("空目录快照失败：%v", err)
	}
	if snapshot != "" {
		t.Errorf("空目录不该建快照：%s", snapshot)
	}
}
