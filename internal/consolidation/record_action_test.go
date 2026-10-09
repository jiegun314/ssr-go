package consolidation

import (
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/changedetect"
	"github.com/jiegun314/ssr-go/internal/config"
	"github.com/jiegun314/ssr-go/internal/store"
)

// 这一组测试固定修正后的业务口径（见 consolidation_mapping.yaml 的 record_action）：
//
//   - Choose Action 只能是 Add 或 Modify，不可能为空；
//   - 判定键 = duplicate_check.identity_fields（Catalog or Reference Number + Primary DI）：
//     操作日志里没有同一身份的最新记录 → Add；已经有 → Modify；
//   - Change Description 在 Modify 时列出差异项，形如「Product Name/Generic Name变更」，
//     多项按**导出列顺序**用全角分号连接；Add 行留空。

// TestRecordActionIsAddWithoutStoredRecordsAndModifyWithThem 固定 Add / Modify 的判定：
// 第一次整合（库里没有任何历史记录）→ 全部 Add 且变更描述为空；
// 记进操作日志后再整合 → 同一身份变成 Modify（哪怕数据完全一致）。
func TestRecordActionIsAddWithoutStoredRecordsAndModifyWithThem(t *testing.T) {
	service, outcome, _ := consolidated(t)

	if len(outcome.Rows) == 0 {
		t.Fatal("样本没有产出任何结果行")
	}
	for _, row := range outcome.Rows {
		if got := row.Values["Choose Action"]; got != "Add" {
			t.Errorf("%s：库里没有历史记录时 Choose Action = %q; want Add", row.Key, got)
		}
		if got := row.Values["Change Description"]; got != "" {
			t.Errorf("%s：Add 行的变更描述应当为空，实际 %q", row.Key, got)
		}
	}

	// 只有 Ready 行会被写进操作日志（等价于导出成功）
	recorded := map[string]bool{}
	for _, row := range outcome.Rows {
		if row.Values["status"] == StatusReady {
			recorded[row.Key] = true
		}
	}
	recordReady(t, service, outcome)
	second, err := service.Consolidate()
	if err != nil {
		t.Fatalf("第二次整合失败：%v", err)
	}
	for _, row := range second.Rows {
		if !recorded[row.Key] {
			// 第一次就没进导出的行：库里没有它的身份 → 仍然是 Add，状态不变
			if row.Values["Choose Action"] != "Add" {
				t.Errorf("%s：没有历史记录的行 Choose Action = %q; want Add",
					row.Key, row.Values["Choose Action"])
			}
			continue
		}
		if got := row.Values["Choose Action"]; got != "Modify" {
			t.Errorf("%s：身份已在库里时 Choose Action = %q; want Modify", row.Key, got)
		}
		// 数据没变 → 判为重复、不进导出；变更描述保持为空
		if row.Values["status"] != StatusDuplicate {
			t.Errorf("%s：数据未变时状态 = %q; want Duplicate", row.Key, row.Values["status"])
		}
		if row.Values["Change Description"] != "" {
			t.Errorf("%s：无差异时变更描述应当为空，实际 %q", row.Key, row.Values["Change Description"])
		}
	}
	if len(recorded) == 0 {
		t.Fatal("第一次整合没有任何 Ready 行")
	}
}

// TestChangeDescriptionListsEveryChangedColumnInExportOrder 固定多项变更的写法：
// 「列名变更」+ 全角分号，顺序按导出列顺序（Product Name/Generic Name 先于 Device Description）。
func TestChangeDescriptionListsEveryChangedColumnInExportOrder(t *testing.T) {
	service, outcome, repository := consolidated(t)
	recordReady(t, service, outcome)

	// 故意先改后面的列、再改前面的列：输出顺序必须是导出列顺序，不是改动顺序
	changeSourceValue(t, repository, "ra_input_staging", "000MAT-001", "product_description", "描述也改了")
	changeSourceValue(t, repository, "ra_input_staging", "000MAT-001", "product_name", "名称改了")

	second, err := service.Consolidate()
	if err != nil {
		t.Fatalf("第二次整合失败：%v", err)
	}
	seen := 0
	for _, row := range second.Rows {
		if row.Values[service.Config.BaseKeyField] != "000MAT-001" {
			continue
		}
		seen++
		if row.Values["status"] != StatusReady {
			t.Errorf("%s：有差异时状态 = %q; want Ready", row.Key, row.Values["status"])
		}
		if row.Values["Choose Action"] != "Modify" {
			t.Errorf("%s：Choose Action = %q; want Modify", row.Key, row.Values["Choose Action"])
		}
		want := "Product Name/Generic Name变更；Device Description变更"
		if got := row.Values["Change Description"]; got != want {
			t.Errorf("%s：变更描述 = %q; want %q", row.Key, got, want)
		}
	}
	if seen == 0 {
		t.Fatal("没有找到 000MAT-001 的结果行")
	}
}

// TestChooseActionIsNeverEmpty 固定"不可能为空"：所有行（含冲突行）都要有 Add 或 Modify。
func TestChooseActionIsNeverEmpty(t *testing.T) {
	service, outcome, repository := consolidated(t)
	recordReady(t, service, outcome)
	changeSourceValue(t, repository, "ra_input_staging", "000MAT-002", "product_name", "改过的名称")

	second, err := service.Consolidate()
	if err != nil {
		t.Fatalf("第二次整合失败：%v", err)
	}
	for _, row := range second.Rows {
		action := strings.TrimSpace(row.Values["Choose Action"])
		if action != "Add" && action != "Modify" {
			t.Errorf("%s：Choose Action = %q; want Add 或 Modify", row.Key, action)
		}
	}
}

// TestEmptyChangeDescriptionsSafetyNet 固定安全网本身：动作是 Modify 却没有描述时能被点出来。
func TestEmptyChangeDescriptionsSafetyNet(t *testing.T) {
	rule := &changedetect.Rule{
		Field:                "Change Description",
		ActionField:          "Choose Action",
		ActionOnNewRecord:    "Add",
		ActionOnStoredRecord: "Modify",
		IdentityFields:       []string{"Catalog or Reference Number", "Primary DI"},
	}
	detector := changedetect.NewService(rule, rule.IdentityFields, nil)
	rows := []changedetect.Row{
		{Key: "a", Values: store.Row{"status": StatusReady, "Choose Action": "Modify", "Change Description": ""}},
		{Key: "b", Values: store.Row{"status": StatusReady, "Choose Action": "Modify", "Change Description": "Package Type变更"}},
		{Key: "c", Values: store.Row{"status": StatusReady, "Choose Action": "Add", "Change Description": ""}},
		// 重复行（与历史完全一致）：不进导出，没有描述是正常的 → 不该报警
		{Key: "d", Values: store.Row{"status": StatusDuplicate, "Choose Action": "Modify", "Change Description": ""}},
		{Key: "e", Values: store.Row{"status": StatusIncomplete, "Choose Action": "Modify", "Change Description": ""}},
	}
	got := detector.EmptyChangeDescriptions(rows)
	if len(got) != 1 || got[0] != "a" {
		t.Errorf("安全网点出的行 = %v; want [a]", got)
	}
	// 没有配置动作列时安全网不生效（不误报）
	plain := changedetect.NewService(&changedetect.Rule{Field: "Change Description"}, nil, nil)
	if offending := plain.EmptyChangeDescriptions(rows); len(offending) != 0 {
		t.Errorf("没有动作列时不该有安全网结果：%v", offending)
	}
}

// TestRecordActionConfigIsRequired 固定配置侧的硬要求：
// record_action 列必须声明 mandatory_status: required，且两个文案非空、身份引用 duplicate_check。
func TestRecordActionConfigIsRequired(t *testing.T) {
	loader, _, _ := newWorkspace(t)
	rule, err := loader.LoadChangeDescriptionRule()
	if err != nil {
		t.Fatalf("解析派生列失败：%v", err)
	}
	if rule == nil || rule.ActionField != "Choose Action" {
		t.Fatalf("没有解析出动作列：%+v", rule)
	}
	if rule.ActionOnNewRecord != "Add" || rule.ActionOnStoredRecord != "Modify" {
		t.Errorf("动作文案 = (%q, %q); want (Add, Modify)",
			rule.ActionOnNewRecord, rule.ActionOnStoredRecord)
	}
	if rule.Field != "Change Description" {
		t.Errorf("变更描述列 = %q; want Change Description", rule.Field)
	}
	if !strings.Contains(rule.OnOtherChange["template"].(string), "变更") {
		t.Errorf("变更描述模板 = %v; want 中文后缀「变更」", rule.OnOtherChange["template"])
	}
	if rule.OnOtherChange["separator"] != "；" {
		t.Errorf("分隔符 = %v; want 全角分号", rule.OnOtherChange["separator"])
	}
	if strings.Join(rule.IdentityFields, "|") != "Catalog or Reference Number|Primary DI" {
		t.Errorf("身份字段 = %v; want duplicate_check 的那一对", rule.IdentityFields)
	}
	_ = config.SettingFile
}
