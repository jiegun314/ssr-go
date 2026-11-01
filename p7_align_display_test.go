package main

// 显示口径（用户反馈）：
//   - SC 对齐后应当立刻回传最新的来源状态（前端据此显示"已对齐"而不是"尚未导入"）；
//   - Admin 拥有全部权限（上传所有来源 + 对齐 + 整合 + 导出 + 管理共享盘）。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/auth"
)

func TestAlignResultCarriesImportStates(t *testing.T) {
	shareRoot := filepath.Join(t.TempDir(), "share")
	if err := os.MkdirAll(shareRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	publisher := newShareApp(t, shareRoot)
	publishSamples(t, publisher)

	consumer := newShareApp(t, shareRoot)
	result := consumer.AlignAll()
	if result.Failed {
		t.Fatalf("对齐失败：%+v", result)
	}
	if len(result.Imports) != 4 {
		t.Fatalf("对齐结果应当带回四个来源的最新状态，实际 %d", len(result.Imports))
	}
	for _, source := range []string{"ra_input", "global_udi_input", "medical_insurance_code", "product_category"} {
		state, present := result.Imports[source]
		if !present || state.State != "existing" {
			t.Errorf("%s 对齐后状态应为 existing：%+v", source, state)
		}
		if !strings.Contains(state.Label, "已对齐") {
			t.Errorf("%s 的标签应当显示已对齐：%q", source, state.Label)
		}
	}
}

func TestAdminHasAllPermissions(t *testing.T) {
	for _, action := range []auth.Action{
		auth.ActionImportRA, auth.ActionImportEAMA, auth.ActionImportUDI,
		auth.ActionAlign, auth.ActionConsolidate, auth.ActionExport, auth.ActionClearData,
		auth.ActionViewSettings, auth.ActionEditSettings, auth.ActionManageShare,
		auth.ActionReviewAny, auth.ActionViewAuditAny,
	} {
		if !auth.Can(auth.RoleAdmin, action) {
			t.Errorf("Admin 应当拥有 %s 权限", action)
		}
	}
}
