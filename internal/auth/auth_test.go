package auth

import (
	"strings"
	"testing"
)

func TestPermissionMatrix(t *testing.T) {
	cases := []struct {
		role   Role
		action Action
		want   bool
	}{
		{RoleRA, ActionImportRA, true},
		{RoleRA, ActionImportEAMA, false},
		{RoleRA, ActionImportUDI, false},
		{RoleRA, ActionConsolidate, false},
		{RoleRA, ActionViewSettings, false},

		{RoleEAMA, ActionImportEAMA, true},
		{RoleEAMA, ActionImportRA, false},

		{RoleUDI, ActionImportUDI, true},
		{RoleUDI, ActionExport, false},

		{RoleSupplyChain, ActionAlign, true},
		{RoleRA, ActionAlign, false},
		{RoleSupplyChain, ActionConsolidate, true},
		{RoleSupplyChain, ActionExport, true},
		{RoleSupplyChain, ActionViewSettings, true},
		{RoleSupplyChain, ActionEditSettings, false},
		{RoleSupplyChain, ActionImportRA, false},
		{RoleSupplyChain, ActionClearData, false},

		{RoleAdmin, ActionImportRA, true},
		{RoleAdmin, ActionClearData, true},
		{RoleAdmin, ActionEditSettings, true},
		{RoleAdmin, ActionManageShare, true},
	}
	for _, item := range cases {
		if got := Can(item.role, item.action); got != item.want {
			t.Errorf("Can(%s, %s) = %v；want %v", item.role, item.action, got, item.want)
		}
	}
}

func TestActionsAreSortedAndStable(t *testing.T) {
	actions := Actions(RoleRA)
	if len(actions) != 1 || actions[0] != string(ActionImportRA) {
		t.Fatalf("RA 的动作 = %v", actions)
	}
	admin := Actions(RoleAdmin)
	for index := 1; index < len(admin); index++ {
		if admin[index-1] > admin[index] {
			t.Errorf("动作清单应当排序：%v", admin)
		}
	}
}

func TestAdminPassword(t *testing.T) {
	if VerifyAdminPassword("") {
		t.Error("空密码必须拒绝")
	}
	if VerifyAdminPassword("wrong-password") {
		t.Error("错误密码必须拒绝")
	}
	if !VerifyAdminPassword("SsrAdmin@2026") {
		t.Error("写死在程序里的 Admin 密码应当通过（见 docs/design/multiuser-workflow.md §5.1）")
	}
}

func TestParseUsers(t *testing.T) {
	document := map[string]any{"users": []any{
		map[string]any{"name": "张三", "role": "ra", "email": "a@corp.com", "note": "RA"},
		map[string]any{"name": "李四", "role": "supply_chain"},
	}}
	users, err := ParseUsers(document)
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if len(users) != 2 || users[0].Role != RoleRA || users[1].Role != RoleSupplyChain {
		t.Fatalf("解析结果不对：%+v", users)
	}

	// 不合法：重名 / 角色写错 / 空列表
	for name, bad := range map[string]map[string]any{
		"重名": {"users": []any{
			map[string]any{"name": "张三", "role": "ra"},
			map[string]any{"name": "张三", "role": "eama"},
		}},
		"角色不合法": {"users": []any{
			map[string]any{"name": "张三", "role": "boss"},
		}},
		"空列表": {"users": []any{}},
	} {
		if _, err := ParseUsers(bad); err == nil {
			t.Errorf("%s 应当被拒绝", name)
		}
	}
	if users, err := ParseUsers(nil); err != nil || users != nil {
		t.Errorf("没有 users.yaml 时应返回 nil（单机模式），实际 %+v / %v", users, err)
	}
}

func TestSourcePermissions(t *testing.T) {
	if !CanImport(RoleRA, "ra_input") {
		t.Error("RA 应当能导入 ra_input")
	}
	if CanImport(RoleRA, "global_udi_input") {
		t.Error("RA 不该能导入 UDI")
	}
	if !CanImport(RoleEAMA, "product_category") || !CanImport(RoleEAMA, "medical_insurance_code") {
		t.Error("EAMA 应当能导入产品类别与医保代码")
	}
	if CanImport(RoleUDI, "medical_insurance_code") {
		t.Error("UDI 不该能导入医保代码")
	}
	if !CanReview(RoleSupplyChain, "ra_input") {
		t.Error("SC 应当能只读查看全部来源")
	}
	if !CanReview(RoleAdmin, "global_udi_input") {
		t.Error("Admin 应当能查看全部来源")
	}
	if CanReview(RoleRA, "global_udi_input") {
		t.Error("RA 不该能查看 UDI 明细")
	}
	if label := Label(RoleRA); !strings.Contains(label, "RA") {
		t.Errorf("角色显示名不对：%q", label)
	}
}
