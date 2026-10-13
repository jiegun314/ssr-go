package importer

import (
	"errors"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// TestAcceptedValuesAndConditionalSterilization 覆盖"取值白名单 + 条件必填"这一对配置能力：
//
//	使用前是否需要进行灭菌：必填，且只接受 是 / 否（accepted_values，校验**原始写法**）
//	灭菌方式：填"是"时必填（required_when，条件里的"是"按 value_mapping 归一化成 Yes 后比较）
func TestAcceptedValuesAndConditionalSterilization(t *testing.T) {
	const flagColumn = "使用前是否需要进行灭菌"
	const methodColumn = "灭菌方式"

	cases := []struct {
		name       string
		flag       string
		method     string
		wantError  string // "" 表示应当导入成功
		wantDetail string
	}{
		{name: "填是 + 有灭菌方式 → 通过", flag: "是", method: "环氧乙烷"},
		{name: "填是 + 灭菌方式为空 → 条件必填拦下", flag: "是", method: "",
			wantError: "条件必填", wantDetail: "灭菌方式（sterilization_method）为空"},
		{name: "填否 + 灭菌方式为空 → 通过", flag: "否", method: ""},
		{name: "填非法写法 → 取值不合法拦下", flag: "或许", method: "环氧乙烷",
			wantError: "取值不合法", wantDetail: "只接受：是 / 否"},
		{name: "填 Y（映射认识、白名单不认识）→ 拦下", flag: "Y", method: "环氧乙烷",
			wantError: "取值不合法", wantDetail: "只接受：是 / 否"},
		{name: "填 是的（多一个字）→ 拦下", flag: "是的", method: "环氧乙烷",
			wantError: "取值不合法"},
		{name: "填是（带首尾空白）→ 通过", flag: " 是 ", method: "环氧乙烷"},
		{name: "留空 → 先被必填拦下（不是白名单）", flag: "", method: "环氧乙烷",
			wantError: "必填列", wantDetail: "为空"},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			instance, _, _ := newTestImporter(t)
			rule := instance.Rules["ra_input"]
			path := writeVariant(t, instance, "ra_input",
				func(file *excelize.File, sheet string, _ int, indexes map[string]int) {
					setCell(t, file, sheet, indexes, rule.DataStartRow, flagColumn, item.flag)
					setCell(t, file, sheet, indexes, rule.DataStartRow, methodColumn, item.method)
				})

			_, err := instance.Import("ra_input", path)
			if item.wantError == "" {
				if err != nil {
					t.Fatalf("应当导入成功，实际：%v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("应当被拦下（%s），实际导入成功", item.wantError)
			}
			var invalid *InvalidCellValuesError
			var missing *MissingRowValuesError
			switch {
			case errors.As(err, &invalid):
				if item.wantError != "取值不合法" {
					t.Fatalf("错误类型不对：%v", err)
				}
				if item.wantDetail != "" && !strings.Contains(err.Error(), item.wantDetail) {
					t.Errorf("明细里应当有 %q，实际：%s", item.wantDetail, err.Error())
				}
				if !strings.Contains(invalid.Summary(), "取值不合法") {
					t.Errorf("弹窗汇总应当说明取值不合法：%s", invalid.Summary())
				}
			case errors.As(err, &missing):
				if item.wantError != "条件必填" && item.wantError != "必填列" {
					t.Fatalf("错误类型不对（期望 %s）：%v", item.wantError, err)
				}
				if item.wantDetail != "" && !strings.Contains(err.Error(), item.wantDetail) {
					t.Errorf("明细里应当有 %q，实际：%s", item.wantDetail, err.Error())
				}
				if item.wantError == "条件必填" && !missing.conditional {
					t.Error("应当是条件必填类型的缺值错误")
				}
			default:
				t.Fatalf("错误类型不对：%T %v", err, err)
			}
		})
	}
}
