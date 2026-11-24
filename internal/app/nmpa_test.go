package app

import (
	"strings"
	"testing"
)

func TestNMPAReferenceLibraryIsAdminOnly(t *testing.T) {
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
}
