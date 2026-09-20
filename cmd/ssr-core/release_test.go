package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReleaseArchiveLeavesTheDatabaseBehind 固定「发布压缩包不带数据库」的口径：
// 用户升级时通常是解压覆盖旧目录，包里带一个空库就会把用户已有的库冲掉。
// 数据库（连它的 WAL / SHM / journal）必须被排除，其余内容照样打包。
func TestReleaseArchiveLeavesTheDatabaseBehind(t *testing.T) {
	releaseRoot := filepath.Join(t.TempDir(), "SingleSourceReady")
	writeReleaseFile(t, filepath.Join(releaseRoot, "VERSION"), "version 0.0.0\n")
	writeReleaseFile(t, filepath.Join(releaseRoot, "config", "setting.yaml"), "database: {path: data/udi_data.sqlite3}\n")
	writeReleaseFile(t, filepath.Join(releaseRoot, "data", "template", "template.xlsx"), "template")
	writeReleaseFile(t, filepath.Join(releaseRoot, "data", "udi_data.sqlite3"), "db")
	writeReleaseFile(t, filepath.Join(releaseRoot, "data", "udi_data.sqlite3-wal"), "wal")
	writeReleaseFile(t, filepath.Join(releaseRoot, "output", "export", ".keep"), "")

	skip := releaseArchiveSkipSet("data/udi_data.sqlite3")
	if len(skip) != 4 {
		t.Fatalf("跳过清单应有 4 项（本体 + wal/shm/journal），实际 %d：%v", len(skip), skip)
	}
	archive, err := zipRelease(releaseRoot, "out.zip", skip)
	if err != nil {
		t.Fatalf("打包失败：%v", err)
	}
	reader, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatalf("读压缩包失败：%v", err)
	}
	defer reader.Close()

	names := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		names = append(names, file.Name)
		if strings.Contains(file.Name, "udi_data.sqlite3") {
			t.Errorf("压缩包里不该出现数据库：%s", file.Name)
		}
	}
	joined := strings.Join(names, "\n")
	for _, wanted := range []string{"VERSION", "config/setting.yaml", "data/template/template.xlsx", "output/export/"} {
		if !strings.Contains(joined, wanted) {
			t.Errorf("压缩包里缺少 %s（现有：%v）", wanted, names)
		}
	}
}

// TestReleaseArchiveSkipSetFollowsTheConfiguredDatabasePath 库路径来自配置，
// 不能写死文件名：改 setting.yaml 的 database.path 之后跳过清单要跟着走。
func TestReleaseArchiveSkipSetFollowsTheConfiguredDatabasePath(t *testing.T) {
	skip := releaseArchiveSkipSet("data/other.sqlite3")
	if !skip["data/other.sqlite3"] || !skip["data/other.sqlite3-wal"] {
		t.Errorf("跳过清单没有跟随配置的库路径：%v", skip)
	}
	if skip["data/udi_data.sqlite3"] {
		t.Error("跳过清单不应写死默认库名")
	}
	if empty := releaseArchiveSkipSet(""); len(empty) != 0 {
		t.Errorf("配置里没有库路径时不该跳过任何文件：%v", empty)
	}
}

func writeReleaseFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写文件失败：%v", err)
	}
}
