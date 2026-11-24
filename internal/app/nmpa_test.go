package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/nmpa"
	"github.com/xuri/excelize/v2"
)

func seedNMPADevice(app *App, device nmpa.Device) {
	if err := app.nmpaService.Store.ReplaceAll([]nmpa.Device{device}, nil); err != nil {
		panic(err)
	}
}

func TestNMPAReferenceLibraryIsReadableBySupplyChainOnly(t *testing.T) {
	app := newUsersApp(t)
	if result := app.Login("ra", ""); result.Failed {
		t.Fatalf("RA 登录失败：%+v", result)
	}
	if _, err := app.NMPAStatus(); err == nil || !strings.Contains(err.Error(), "没有这个操作的权限") {
		t.Fatalf("RA 读取药监参考库应被拒绝，err=%v", err)
	}
	if _, err := app.NMPADetail("DI-1"); err == nil || !strings.Contains(err.Error(), "没有这个操作的权限") {
		t.Fatalf("RA 读取药监详情应被拒绝，err=%v", err)
	}
	if result := app.NMPACheck(); !strings.Contains(result.Error, "没有这个操作的权限") {
		t.Fatalf("RA 检查药监更新应被拒绝：%+v", result)
	}
	if result := app.Logout(); result.Session != nil {
		t.Fatalf("退出后仍有会话：%+v", result.Session)
	}
	if result := app.Login("supply_chain", ""); result.Failed {
		t.Fatalf("SC 登录失败：%+v", result)
	}
	if _, err := app.NMPAStatus(); err != nil {
		t.Fatalf("SC 读取药监参考库应允许，err=%v", err)
	}
	if _, err := app.NMPASearch("", 1, 20); err != nil {
		t.Fatalf("SC 查询药监参考库应允许，err=%v", err)
	}
	if result := app.NMPACheck(); !strings.Contains(result.Error, "没有这个操作的权限") {
		t.Fatalf("SC 检查药监更新应被拒绝：%+v", result)
	}
}

func TestNMPASummaryAndDetailExposeTheFullDevice(t *testing.T) {
	app := newUsersApp(t)
	device := nmpa.Device{
		PrimaryDI: "DI-1", DeviceGenericName: "通用名", ProductName: "", Specification: "规格-A",
		RegistrantName: "注册人", RegistrationFilingCertificateNo: "国械注准1", ProductCategory: "骨科",
		VersionStatus: "新增",
		Packages:      []nmpa.Package{{PackageDI: "PKG-1", PackageType: "盒", LowerLevelPackageQuantity: "2", LowerLevelPackageIdentifier: "PKG-0"}},
		Contacts:      []nmpa.Contact{{Fax: "021-1", Email: "a@example.com", Phone: "123"}},
	}
	seedNMPADevice(app, device)
	if result := app.Login("supply_chain", ""); result.Failed {
		t.Fatalf("SC 登录失败：%+v", result)
	}
	result, err := app.NMPASearch("", 1, 20)
	if err != nil {
		t.Fatalf("查询失败：%v", err)
	}
	if len(result.Columns) != 8 {
		t.Fatalf("摘要列数 = %d；want 8", len(result.Columns))
	}
	if len(result.Rows) != 1 || result.Rows[0][1] != "通用名" {
		t.Fatalf("产品名称应回退到通用名称：%+v", result.Rows)
	}
	detail, err := app.NMPADetail("DI-1")
	if err != nil {
		t.Fatalf("详情失败：%v", err)
	}
	fieldCount := 0
	for _, group := range detail.Groups {
		fieldCount += len(group.Fields)
	}
	if fieldCount != len(nmpa.DeviceColumns) {
		t.Fatalf("详情字段数 = %d；want %d", fieldCount, len(nmpa.DeviceColumns))
	}
	if detail.DisplayName != "通用名" || len(detail.Packages) != 1 || len(detail.Contacts) != 1 {
		t.Fatalf("详情内容不对：%+v", detail)
	}
}

func TestNMPAExportWritesFullWorkbookForSupplyChain(t *testing.T) {
	app := newUsersApp(t)
	seedNMPADevice(app, nmpa.Device{
		PrimaryDI: "DI-9", DeviceGenericName: "融合器", Specification: "规格-9",
		Packages: []nmpa.Package{{PackageDI: "PKG-9", PackageType: "盒"}},
		Contacts: []nmpa.Contact{{Email: "sc@example.com"}},
	})
	if result := app.Login("supply_chain", ""); result.Failed {
		t.Fatalf("SC 登录失败：%+v", result)
	}
	target := filepath.Join(t.TempDir(), "nmpa.xlsx")
	result := app.NMPAExport("", target)
	if result.Failed {
		t.Fatalf("导出应允许 SC：%+v", result)
	}
	info, err := os.Stat(target)
	if err != nil || info.Size() == 0 {
		t.Fatalf("导出文件未生成：%v / %+v", err, info)
	}
	if result.Path != target || !strings.Contains(result.Message, "全字段") {
		t.Fatalf("导出返回不对：%+v", result)
	}
	workbook, err := excelize.OpenFile(target)
	if err != nil {
		t.Fatalf("打开导出文件失败：%v", err)
	}
	defer workbook.Close()
	names := workbook.GetSheetList()
	if strings.Join(names, ",") != "设备,包装,联系人" {
		t.Fatalf("工作表 = %v；want 设备,包装,联系人", names)
	}
	rows, err := workbook.GetRows("设备")
	if err != nil || len(rows) != 2 || len(rows[0]) != len(nmpa.DeviceColumns) {
		t.Fatalf("设备表头/行数不对：%v / %+v", err, rows)
	}
}
