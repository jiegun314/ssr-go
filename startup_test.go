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

// TestStartupKeepsTheMedicalInsuranceSource 是针对「启动后医保代码信息消失」报告的回归测试。
//
// 配置里 medical_insurance_code 的 preserve_on_cleanup 是 true（R9），所以：
//   - 启动清理之后它必须还在库里（行数不变）；
//   - 界面状态要走 existing（绿点 + 现有N条），而不是 empty；
//   - 另外三张来源表则被清掉。
func TestStartupKeepsTheMedicalInsuranceSource(t *testing.T) {
	workspace := t.TempDir()
	copyDirectoryForTest(t, "config", filepath.Join(workspace, "config"))
	configDir := filepath.Join(workspace, "config")
	databasePath := filepath.Join(workspace, "data", "udi_data.sqlite3")

	// 先用导入服务把医保来源导进去（模拟「上一次运行留下的数据」）
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
	if _, err := importService.Import("medical_insurance_code",
		filepath.Join("testdata", "sample-valid", "medical_insurance_code.xlsx")); err != nil {
		t.Fatalf("导入医保来源失败：%v", err)
	}
	repository.Close()

	// 让应用按这份配置启动
	t.Setenv("UDI_CONFIG_DIR", configDir)
	app := NewApp(nil)
	app.startup(context.Background())

	check, err := store.Open(databasePath)
	if err != nil {
		t.Fatalf("重新打开数据库失败：%v", err)
	}
	defer check.Close()

	count, err := check.CountTableRows("medical_insurance_code")
	if err != nil {
		t.Fatalf("统计行数失败：%v", err)
	}
	if count != 6 {
		t.Errorf("医保表行数 = %d; want 6（启动清理不该删这张表）", count)
	}
	state := app.importStates["medical_insurance_code"]
	if state.State != "existing" {
		t.Errorf("医保来源界面状态 = %q; want existing", state.State)
	}
	if state.Label != "现有6条" {
		t.Errorf("医保来源状态标签 = %q; want 现有6条", state.Label)
	}
	for _, table := range []string{
		"ra_input_staging", "global_udi_input_staging", "product_category_staging",
	} {
		exists, err := check.TableExists(table)
		if err != nil {
			t.Fatalf("检查 %s 失败：%v", table, err)
		}
		if exists {
			t.Errorf("%s 应当被启动清理删掉", table)
		}
	}
	log := app.logText()
	if !strings.Contains(log, "Staging tables cleaned successfully") {
		t.Errorf("启动日志缺少清理成功那行：%s", log)
	}
	if strings.Contains(log, "Configuration error") || strings.Contains(log, "Database error") {
		t.Errorf("启动日志里有错误：%s", log)
	}
}
