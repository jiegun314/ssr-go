package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/jiegun314/ssr-go/internal/config"
)

// configSnapshotDirectory 是配置快照目录（放在配置目录下的隐藏目录里，不打扰用户）。
const configSnapshotDirectory = ".backup"

// configSnapshotKeep 是保留的快照份数：只解决"改坏了想回退"，不需要留一屋子历史。
const configSnapshotKeep = 5

// snapshotUserConfig 在启动时给当前配置留一份快照，返回新建的快照目录（没建则返回空串）。
//
// 默认文件机制只解决"配置缺失"，解决不了"配置被改坏"——所以启动时兜一层：
// 内容与最近一份快照完全相同时不重复建（避免每次启动都堆目录），超过保留份数就删最旧的。
func snapshotUserConfig(configDir string, now time.Time) (string, error) {
	if configDir == "" {
		return "", nil
	}
	current, err := readConfigFiles(configDir)
	if err != nil {
		return "", err
	}
	if len(current) == 0 {
		return "", nil
	}
	snapshotRoot := filepath.Join(configDir, configSnapshotDirectory)
	snapshots, err := existingSnapshots(snapshotRoot)
	if err != nil {
		return "", err
	}
	if len(snapshots) > 0 {
		previous, err := readConfigFiles(snapshots[len(snapshots)-1])
		if err != nil {
			return "", err
		}
		if sameConfigFiles(current, previous) {
			return "", nil
		}
	}
	target := filepath.Join(snapshotRoot, now.Format("20060102-150405"))
	if err := os.MkdirAll(target, 0o755); err != nil {
		return "", err
	}
	for name, content := range current {
		if err := os.WriteFile(filepath.Join(target, name), content, 0o644); err != nil {
			return "", err
		}
	}
	if err := pruneSnapshots(snapshotRoot, snapshots, configSnapshotKeep-1); err != nil {
		return "", err
	}
	return target, nil
}

// readConfigFiles 读配置目录里四份 YAML 的内容（缺的那份直接跳过）。
func readConfigFiles(directory string) (map[string][]byte, error) {
	files := map[string][]byte{}
	for _, name := range config.ConfigFileNames {
		content, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		files[name] = content
	}
	return files, nil
}

// existingSnapshots 列出现有快照目录，按名字（即时间戳）升序。
func existingSnapshots(snapshotRoot string) ([]string, error) {
	entries, err := os.ReadDir(snapshotRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	snapshots := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			snapshots = append(snapshots, filepath.Join(snapshotRoot, entry.Name()))
		}
	}
	sort.Strings(snapshots)
	return snapshots, nil
}

// sameConfigFiles 比较两份配置内容是否完全一致。
func sameConfigFiles(left map[string][]byte, right map[string][]byte) bool {
	if len(left) != len(right) {
		return false
	}
	for name, content := range left {
		other, ok := right[name]
		if !ok || string(other) != string(content) {
			return false
		}
	}
	return true
}

// pruneSnapshots 只保留最新的 keep 份（连同即将写入的那一份一起算）。
func pruneSnapshots(snapshotRoot string, existing []string, keep int) error {
	if keep < 0 {
		keep = 0
	}
	for len(existing) > keep {
		if err := os.RemoveAll(existing[0]); err != nil {
			return fmt.Errorf("Failed to remove old config snapshot %s: %w", existing[0], err)
		}
		existing = existing[1:]
	}
	return nil
}
