package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/config"
	"github.com/jiegun314/ssr-go/internal/importer"
	"github.com/jiegun314/ssr-go/internal/store"
)

// TestImportFeedbackIsChineseAndTwoLines 固定导入弹窗的反馈口径（用户要求）：
//   - 成功：标题「导入成功」，正文两行 —— 第一行「{来源中文名}导入成功」，
//     第二行「导入行数：{N}」；
//   - 失败：标题「导入失败」，第一行「{来源中文名}导入失败」，之后是失败原因
//     （缺值类还会附一行「详情见日志窗口」）。
func TestImportFeedbackIsChineseAndTwoLines(t *testing.T) {
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, "config")
	copyDirectoryForTest(t, "config", configDir)

	loader, err := config.NewLoader(configDir)
	if err != nil {
		t.Fatalf("读配置失败：%v", err)
	}
	repository, err := store.Open(filepath.Join(workspace, "data", "udi_data.sqlite3"))
	if err != nil {
		t.Fatalf("打开数据库失败：%v", err)
	}
	defer repository.Close()
	importService, err := importer.NewImporter(loader, repository)
	if err != nil {
		t.Fatalf("构造导入器失败：%v", err)
	}
	app := NewApp(nil)
	app.loader = loader
	app.repo = repository
	app.importer = importService

	// 成功：两行中文
	success := app.ImportSource("ra_input", filepath.Join("testdata", "sample-valid", "ra_input.xlsx"))
	if success.Failed {
		t.Fatalf("样例文件应当导入成功：%+v", success)
	}
	if success.Title != "导入成功" {
		t.Errorf("成功弹窗标题 = %q; want 导入成功", success.Title)
	}
	lines := strings.Split(success.Message, "\n")
	if len(lines) != 2 {
		t.Fatalf("成功弹窗正文应当正好两行，实际 %d 行：%q", len(lines), success.Message)
	}
	if lines[0] != "RA信息导入成功" {
		t.Errorf("第一行 = %q; want RA信息导入成功", lines[0])
	}
	if want := fmt.Sprintf("导入行数：%d", success.RowCount); lines[1] != want {
		t.Errorf("第二行 = %q; want %q", lines[1], want)
	}

	// 失败（条件必填被拒）：第一行写明来源 + 失败，第二行起是原因
	failed := app.ImportSource("global_udi_input",
		filepath.Join("testdata", "sample-invalid-conditions", "global_udi_input.xlsx"))
	if !failed.Failed {
		t.Fatalf("条件必填的样本应当被拒：%+v", failed)
	}
	if failed.Title != "导入失败" {
		t.Errorf("失败弹窗标题 = %q; want 导入失败", failed.Title)
	}
	failedLines := strings.Split(failed.Message, "\n")
	if len(failedLines) < 2 {
		t.Fatalf("失败弹窗正文至少两行，实际 %q", failed.Message)
	}
	if failedLines[0] != "UDI团队信息导入失败" {
		t.Errorf("失败第一行 = %q; want UDI团队信息导入失败", failedLines[0])
	}
	if !strings.HasPrefix(failedLines[1], "失败原因：") {
		t.Errorf("失败第二行应当以「失败原因：」开头，实际 %q", failedLines[1])
	}
	// 汇总原本以「{来源中文名}：」开头，第一行已经写了来源名，不该再重复
	if strings.HasPrefix(failedLines[1], "失败原因：UDI团队信息：") {
		t.Errorf("失败原因里重复了来源名：%q", failedLines[1])
	}
}
