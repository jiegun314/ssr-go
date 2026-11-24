package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNMPAReferenceLibraryIsReadableBySupplyChainOnly(t *testing.T) {
	app := newUsersApp(t)
	if result := app.Login("ra", ""); result.Failed {
		t.Fatalf("RA 登录失败：%+v", result)
	}
	if _, err := app.NMPAStatus(); err == nil || !strings.Contains(err.Error(), "没有这个操作的权限") {
		t.Fatalf("RA 读取药监参考库应被拒绝，err=%v", err)
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

func TestNMPAExportWritesWorkbookForSupplyChain(t *testing.T) {
	app := newUsersApp(t)
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
	if result.Path != target || !strings.Contains(result.Message, "药监 UDI 记录") {
		t.Fatalf("导出返回不对：%+v", result)
	}
}
