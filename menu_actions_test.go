package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jiegun314/ssr-go/internal/config"
	"github.com/jiegun314/ssr-go/internal/importer"
	"github.com/jiegun314/ssr-go/internal/store"
)

// TestBackupTargetPathSitsNextToTheExportFolder 固定备份文件的落点与命名：
// <导出目录的父目录>/backup/<库名>-YYYYmmdd-HHMMSS.sqlite3 —— 库名取自配置
// （库改名备份也跟着改名），时间戳到秒，便于按时间找回。
func TestBackupTargetPathSitsNextToTheExportFolder(t *testing.T) {
	at := time.Date(2026, 9, 20, 22, 15, 30, 0, time.UTC)
	target := backupTargetPath("/app/data/udi_data.sqlite3", "/app/output/export", at)
	want := filepath.Join("/app", "output", "backup", "udi_data-20260920-221530.sqlite3")
	if target != want {
		t.Errorf("备份路径 = %q；want %q", target, want)
	}
	renamed := backupTargetPath("/app/data/其他的库.sqlite3", "/app/output/export", at)
	if filepath.Base(renamed) != "其他的库-20260920-221530.sqlite3" {
		t.Errorf("库改名后备份名没有跟随：%q", filepath.Base(renamed))
	}
}

// TestBackupDatabaseWritesAReadableSnapshot 固定备份的实现方式：
// 用 VACUUM INTO 导出（WAL 模式下直接拷文件可能漏掉未合并的写入），
// 备份结果必须是能独立打开、数据齐全的库；目标目录不存在时自动创建。
func TestBackupDatabaseWritesAReadableSnapshot(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "data", "udi_data.sqlite3")
	repository, err := store.Open(source)
	if err != nil {
		t.Fatalf("建库失败：%v", err)
	}
	if err := repository.Execute("CREATE TABLE sample (value TEXT)"); err != nil {
		t.Fatalf("建表失败：%v", err)
	}
	if err := repository.Execute("INSERT INTO sample (value) VALUES ('UDI-001')"); err != nil {
		t.Fatalf("写数据失败：%v", err)
	}

	target := filepath.Join(root, "output", "backup", "udi_data-20260920-221530.sqlite3")
	if err := backupDatabase(repository, target); err != nil {
		repository.Close()
		t.Fatalf("备份失败：%v", err)
	}
	repository.Close()

	if info, err := os.Stat(target); err != nil || info.Size() == 0 {
		t.Fatalf("备份文件不可用：%v", err)
	}
	restored, err := store.Open(target)
	if err != nil {
		t.Fatalf("打开备份失败：%v", err)
	}
	defer restored.Close()
	rows, err := restored.Query("SELECT value FROM sample")
	if err != nil {
		t.Fatalf("读备份失败：%v", err)
	}
	if len(rows) != 1 || len(rows[0]) != 1 || rows[0][0] != "UDI-001" {
		t.Errorf("备份里的数据 = %v；want 1 行 UDI-001", rows)
	}
}

// TestFileManagerCommandFollowsThePlatform 固定"打开目录"用的命令：macOS 用 open、
// Windows 用 explorer（只 Start 不等待，避免文件管理器进程卡住界面），其余兜底 xdg-open。
func TestFileManagerCommandFollowsThePlatform(t *testing.T) {
	command := fileManagerCommand("/some/dir")
	wantName := "xdg-open"
	switch runtime.GOOS {
	case "darwin":
		wantName = "open"
	case "windows":
		wantName = "explorer"
	}
	if filepath.Base(command.Path) != wantName {
		t.Errorf("命令 = %q；want %q", filepath.Base(command.Path), wantName)
	}
	if len(command.Args) != 2 || command.Args[1] != "/some/dir" {
		t.Errorf("参数 = %v；want [<命令> /some/dir]", command.Args)
	}
}

// TestQuoteSQLStringEscapesApostrophes 路径里带单引号时不能拼坏 SQL。
func TestQuoteSQLStringEscapesApostrophes(t *testing.T) {
	if got := quoteSQLString("/tmp/it's/a.sqlite3"); got != "'/tmp/it''s/a.sqlite3'" {
		t.Errorf("转义结果 = %s", got)
	}
}

// TestMissingRepositoryBackupIsReportedNotPanicked 库没就绪时备份要有明确的失败结果。
func TestMissingRepositoryBackupIsReportedNotPanicked(t *testing.T) {
	result := (&App{}).BackupDatabase()
	if !result.Failed || result.Title != "Error" {
		t.Errorf("空后端备份结果 = %+v；want Failed/Error", result)
	}
}

// TestBackupDatabaseLandsInTheConfiguredOutputFolder 端到端：路径全部来自配置 ——
// 库取 setting.yaml 的 database.path，备份目录取 folder.export 同级的 backup/。
func TestBackupDatabaseLandsInTheConfiguredOutputFolder(t *testing.T) {
	workspace := t.TempDir()
	copyDirectoryForTest(t, "config", filepath.Join(workspace, "config"))
	loader, err := config.NewLoader(filepath.Join(workspace, "config"))
	if err != nil {
		t.Fatalf("读配置失败：%v", err)
	}
	repository, err := store.Open(loader.ResolvePath("data/udi_data.sqlite3"))
	if err != nil {
		t.Fatalf("建库失败：%v", err)
	}
	defer repository.Close()
	importerService, err := importer.NewImporter(loader, repository)
	if err != nil {
		t.Fatalf("建导入服务失败：%v", err)
	}

	app := NewApp(nil)
	app.loader = loader
	app.repo = repository
	app.importer = importerService

	result := app.BackupDatabase()
	if result.Failed {
		t.Fatalf("备份失败：%s", result.Message)
	}
	wantDirectory := filepath.Join(workspace, "output", "backup")
	if filepath.Dir(result.Path) != wantDirectory {
		t.Errorf("备份目录 = %q；want %q", filepath.Dir(result.Path), wantDirectory)
	}
	if !strings.HasPrefix(filepath.Base(result.Path), "udi_data-") {
		t.Errorf("备份文件名 = %q；want udi_data-<时间戳>.sqlite3", filepath.Base(result.Path))
	}
	if _, err := os.Stat(result.Path); err != nil {
		t.Errorf("备份文件不存在：%v", err)
	}
}
