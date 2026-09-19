package consolidation

import (
	"archive/zip"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/jiegun314/ssr-go/internal/excelio"
)

// prepareTemplate 把真实导出模板放到临时工作区里（配置的模板路径指向那里）。
func prepareTemplate(t *testing.T, service *Service) {
	t.Helper()
	source := filepath.Join("..", "..", "data", "template", "SingleSource-DCF-Row-China-UDI.xlsx")
	if err := os.MkdirAll(filepath.Dir(service.Config.TemplatePath), 0o755); err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	content, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("读模板失败：%v", err)
	}
	if err := os.WriteFile(service.Config.TemplatePath, content, 0o644); err != nil {
		t.Fatalf("写模板失败：%v", err)
	}
}

func zipPartNames(t *testing.T, path string) map[string]bool {
	t.Helper()
	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("读 zip 失败：%v", err)
	}
	defer archive.Close()
	names := map[string]bool{}
	for _, entry := range archive.File {
		names[entry.Name] = true
	}
	return names
}

func TestR20R21ExportWritesTextAndKeepsTheTemplate(t *testing.T) {
	service, _, _ := consolidated(t)
	prepareTemplate(t, service)
	moment := time.Date(2026, 9, 19, 8, 0, 0, 0, time.Local)

	result, err := service.ExportConsolidationResult("", moment)
	if err != nil {
		t.Fatalf("导出失败：%v", err)
	}

	// 文件命名：模板名 + 时间戳，落在导出目录里（R20）
	if !regexp.MustCompile(`^SingleSource-DCF-Row-China-UDI-20260919_080000\.xlsx$`).
		MatchString(result.FileName) {
		t.Errorf("文件名 = %q", result.FileName)
	}
	if filepath.Dir(result.FilePath) != service.Config.ExportFolder {
		t.Errorf("导出目录 = %q; want %q", filepath.Dir(result.FilePath), service.Config.ExportFolder)
	}
	if result.WrittenRows != 8 {
		t.Errorf("导出行数 = %d; want 8", result.WrittenRows)
	}
	if len(result.MissingHeaders) != 0 {
		t.Errorf("缺列 = %v", result.MissingHeaders)
	}

	// 逐格内容：第一行第一个单元格是 Primary DI，且前导零还在（按文本写，R21）
	sheet, err := excelio.ReadFirstSheet(result.FilePath)
	if err != nil {
		t.Fatalf("读回导出文件失败：%v", err)
	}
	if got := sheet.Cell(service.Config.DataStartRow, 1); got != "0691234000001" {
		t.Errorf("第 6 行 A 列 = %q; want 0691234000001", got)
	}

	// 口径②：模板里的部件必须都还在（这正是 excelize 相对 openpyxl 的改进）
	templateParts := zipPartNames(t, service.Config.TemplatePath)
	exportParts := zipPartNames(t, result.FilePath)
	for name := range templateParts {
		if !exportParts[name] {
			t.Errorf("导出文件丢了模板部件：%s", name)
		}
	}
	for _, name := range []string{
		"docMetadata/LabelInfo.xml", "xl/media/image1.png",
		"xl/drawings/drawing1.xml", "xl/printerSettings/printerSettings1.bin",
	} {
		if !templateParts[name] {
			t.Fatalf("模板里应有 %s", name)
		}
		if !exportParts[name] {
			t.Errorf("导出文件应保留 %s", name)
		}
	}
}

func TestR20ExportingWithoutReadyRowsFailsAndDoesNotWriteTheLog(t *testing.T) {
	service, outcome, _ := consolidated(t)
	prepareTemplate(t, service)
	recordReady(t, service, outcome)
	// 再整合一次：同样的结果全部变成 Duplicate，于是没有可导出的行
	if _, err := service.Consolidate(); err != nil {
		t.Fatalf("第二次整合失败：%v", err)
	}

	_, err := service.ExportConsolidationResult("", time.Now())

	if err == nil || err.Error() !=
		"No data to export, please check the consolidation result." {
		t.Fatalf("报错应是 Python 版原文，得到：%v", err)
	}
	rows, err := service.Log.Repository.Rows(service.Log.TableName)
	if err != nil {
		t.Fatalf("读日志失败：%v", err)
	}
	if len(rows) != 8 {
		t.Fatalf("没有可导出行时不该写日志：%d 条", len(rows))
	}
}

func TestR22RecordingTheSameResultTwiceStoresOneRecord(t *testing.T) {
	service, outcome, _ := consolidated(t)
	prepareTemplate(t, service)
	moment := time.Date(2026, 9, 19, 8, 0, 0, 0, time.Local)
	if _, err := service.ExportConsolidationResult("", moment); err != nil {
		t.Fatalf("导出失败：%v", err)
	}
	if err := service.RecordConsolidationResult(moment); err != nil {
		t.Fatalf("写日志失败：%v", err)
	}
	// 暂存行仍是 Ready，再记录一次也不该追加第二条
	if err := service.RecordConsolidationResult(moment); err != nil {
		t.Fatalf("写日志失败：%v", err)
	}

	rows, err := service.Log.Repository.Rows(service.Log.TableName)
	if err != nil {
		t.Fatalf("读日志失败：%v", err)
	}
	if len(rows) != 8 {
		t.Fatalf("同一份结果只写一条记录：%d 条", len(rows))
	}
	if rows[0]["Catalog or Reference Number"] != "000MAT-001" {
		t.Errorf("日志第一行 = %v", rows[0])
	}
	_ = outcome
}
