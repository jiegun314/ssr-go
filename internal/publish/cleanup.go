package publish

// 旧版本清理（P5，Admin 维护）：只删"共享盘上确定可以丢"的旧快照与旧整合目录。
//
// 保护规则（任何一条命中就保留）：
//   - 每个来源各自保留最新 keep 个版本；
//   - state.sources 当前指向的版本、以及 last_consolidation 用到的版本永远保留；
//   - consolidated/ 保留最新 keep 个目录 + last_consolidation.id。

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Cleanup 删除旧版本，返回被删除的相对路径。
func Cleanup(
	root string,
	keep int,
	protectedVersions map[string]map[string]bool,
	protectedConsolidation string,
) ([]string, error) {
	if keep < 1 {
		keep = 5
	}
	deleted := []string{}

	// 1) published/<部门>/<版本>.{csv,manifest.json,xlsx}
	publishedRoot := filepath.Join(root, "published")
	departments, err := os.ReadDir(publishedRoot)
	if err != nil && !os.IsNotExist(err) {
		return deleted, fmt.Errorf("读 published 目录失败：%w", err)
	}
	for _, department := range departments {
		if !department.IsDir() {
			continue
		}
		dir := filepath.Join(publishedRoot, department.Name())
		entries, err := os.ReadDir(dir)
		if err != nil {
			return deleted, fmt.Errorf("读 %s 失败：%w", dir, err)
		}
		versions := map[string]bool{}
		for _, entry := range entries {
			name := entry.Name()
			if strings.HasSuffix(name, ".csv") {
				versions[strings.TrimSuffix(name, ".csv")] = true
			}
		}
		ordered := make([]string, 0, len(versions))
		for version := range versions {
			ordered = append(ordered, version)
		}
		sort.Sort(sort.Reverse(sort.StringSlice(ordered)))
		keepSet := map[string]bool{}
		for index, version := range ordered {
			if index < keep {
				keepSet[version] = true
			}
		}
		for version := range protectedVersions[department.Name()] {
			keepSet[version] = true
		}
		for _, version := range ordered {
			if keepSet[version] {
				continue
			}
			for _, extension := range []string{".csv", ".manifest.json", ".xlsx"} {
				path := filepath.Join(dir, version+extension)
				if err := os.Remove(path); err != nil {
					if os.IsNotExist(err) {
						continue
					}
					return deleted, fmt.Errorf("删除 %s 失败：%w", path, err)
				}
				deleted = append(deleted, relativePath(root, path))
			}
		}
	}

	// 2) consolidated/<id>/
	consolidatedRoot := filepath.Join(root, "consolidated")
	entries, err := os.ReadDir(consolidatedRoot)
	if err != nil && !os.IsNotExist(err) {
		return deleted, fmt.Errorf("读 consolidated 目录失败：%w", err)
	}
	ids := []string{}
	for _, entry := range entries {
		if entry.IsDir() && isConsolidationID(entry.Name()) {
			ids = append(ids, entry.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	for index, id := range ids {
		if index < keep || id == protectedConsolidation {
			continue
		}
		path := filepath.Join(consolidatedRoot, id)
		if err := os.RemoveAll(path); err != nil {
			return deleted, fmt.Errorf("删除 %s 失败：%w", path, err)
		}
		deleted = append(deleted, relativePath(root, path))
	}
	return deleted, nil
}

// isConsolidationID 判断目录名是否是整合 id（YYYYMMDDTHHMMSS）。
func isConsolidationID(name string) bool {
	if len(name) != len("20060102T150405") || name[8] != 'T' {
		return false
	}
	for index, char := range name {
		if index == 8 {
			continue
		}
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func relativePath(root string, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(relative)
}
