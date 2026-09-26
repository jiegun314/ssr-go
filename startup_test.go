package main

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/config"
	"github.com/jiegun314/ssr-go/internal/importer"
	"github.com/jiegun314/ssr-go/internal/store"
)

// copyDirectoryForTest 把配置目录复制进临时工作区。
func copyDirectoryForTest(t *testing.T, source string, target string) {
	t.Helper()
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, content, 0o644)
	})
	if err != nil {
		t.Fatalf("复制 %s 失败：%v", source, err)
	}
}

// TestStartupKeepsEveryImportedSource 固定「默认不清理」的口径（用户要求）：
// 发布包的 setting.yaml 是 cleanup_on_startup: false，所以重启后四张来源表都原样保留、
// 行数不变、界面状态走 existing，日志里写明「保留」，而不是「清理成功」。
func TestStartupKeepsEveryImportedSource(t *testing.T) {
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, "config")
	copyDirectoryForTest(t, "config", configDir)
	databasePath := filepath.Join(workspace, "data", "udi_data.sqlite3")
	importEverySource(t, configDir, databasePath)

	// 明确不设置覆盖变量，让开关取配置里的默认值
	t.Setenv(config.CleanupOnStartupEnv, "")
	app := startAppForTest(t, configDir)

	check, err := store.Open(databasePath)
	if err != nil {
		t.Fatalf("重新打开数据库失败：%v", err)
	}
	defer check.Close()

	for table, want := range map[string]int{
		"ra_input_staging":         6,
		"global_udi_input_staging": 9,
		"medical_insurance_code":   6,
		"product_category_staging": 6,
	} {
		count, err := check.CountTableRows(table)
		if err != nil {
			t.Fatalf("统计 %s 行数失败：%v", table, err)
		}
		if count != want {
			t.Errorf("%s 行数 = %d; want %d（默认不该清理）", table, count, want)
		}
	}
	for _, source := range []string{
		"ra_input", "global_udi_input", "medical_insurance_code", "product_category",
	} {
		if state := app.importStates[source]; state.State != "existing" {
			t.Errorf("%s 的界面状态 = %q; want existing（数据还在）", source, state.State)
		}
	}
	log := app.logText()
	if !strings.Contains(log, "Staging tables kept (cleanup_on_startup = false)") {
		t.Errorf("启动日志缺少「保留」那行：%s", log)
	}
	if strings.Contains(log, "Staging tables cleaned successfully") {
		t.Errorf("默认不该执行清理：%s", log)
	}
	if strings.Contains(log, "Configuration error") || strings.Contains(log, "Database error") {
		t.Errorf("启动日志里有错误：%s", log)
	}
}

// TestCleanupOnStartupStillWorksWhenExplicitlyEnabled 机制本身还在：
// 用 SSR_CLEANUP_ON_STARTUP=true 覆盖一次运行，非保留来源表会被删掉、
// medical_insurance_code 按 preserve_on_cleanup 保留 —— 这就是原来的行为，
// 现在只是默认关掉了。
func TestCleanupOnStartupStillWorksWhenExplicitlyEnabled(t *testing.T) {
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, "config")
	copyDirectoryForTest(t, "config", configDir)
	databasePath := filepath.Join(workspace, "data", "udi_data.sqlite3")
	importEverySource(t, configDir, databasePath)

	t.Setenv(config.CleanupOnStartupEnv, "true")
	app := startAppForTest(t, configDir)

	check, err := store.Open(databasePath)
	if err != nil {
		t.Fatalf("重新打开数据库失败：%v", err)
	}
	defer check.Close()

	for _, table := range []string{
		"ra_input_staging", "global_udi_input_staging", "product_category_staging",
	} {
		exists, err := check.TableExists(table)
		if err != nil {
			t.Fatalf("检查 %s 失败：%v", table, err)
		}
		if exists {
			t.Errorf("%s 在显式开启清理时应当被删掉", table)
		}
	}
	if count, err := check.CountTableRows("medical_insurance_code"); err != nil || count != 6 {
		t.Errorf("医保表行数 = %d (err=%v); want 6（preserve_on_cleanup: true）", count, err)
	}
	if log := app.logText(); !strings.Contains(log, "Staging tables cleaned successfully") {
		t.Errorf("启动日志缺少清理成功那行：%s", log)
	}
}

// importEverySource 造出「上一次运行留下的数据」：四个来源各导入一次。
func importEverySource(t *testing.T, configDir string, databasePath string) {
	t.Helper()
	loader, err := config.NewLoader(configDir)
	if err != nil {
		t.Fatalf("读配置失败：%v", err)
	}
	repository, err := store.Open(databasePath)
	if err != nil {
		t.Fatalf("打开数据库失败：%v", err)
	}
	importService, err := importer.NewImporter(loader, repository)
	if err != nil {
		t.Fatalf("构造导入器失败：%v", err)
	}
	for _, source := range importService.Order {
		if _, err := importService.Import(source,
			filepath.Join("testdata", "sample-valid", source+".xlsx")); err != nil {
			t.Fatalf("导入 %s 失败：%v", source, err)
		}
	}
	repository.Close()
}

// startAppForTest 让应用按给定配置目录启动一次（清理开关由环境变量决定）。
func startAppForTest(t *testing.T, configDir string) *App {
	t.Helper()
	t.Setenv("UDI_CONFIG_DIR", configDir)
	app := NewApp(nil)
	app.startup(context.Background())
	return app
}
