package importer

import (
	"strings"
	"testing"
)

// TestMissingRowValuesErrorStaysShortForHugeFiles 固定「报错行数很大时也只能给汇总」：
// 一万个违规单元格，消息仍然只有几行、几百字节 —— 否则前端 hover（原生 title）会卡住。
func TestMissingRowValuesErrorStaysShortForHugeFiles(t *testing.T) {
	violations := make([]Violation, 0, 10000)
	for row := 1; row <= 5000; row++ {
		violations = append(violations,
			Violation{RowNumber: row, ChineseName: "产品代码", DBField: "material_code"},
			Violation{RowNumber: row, ChineseName: "使用单元产品标识",
				DBField: "device_identifier_use_unit", Condition: "数量 > 1"},
		)
	}
	errValue := NewMissingConditionalValuesError("UDI团队信息", violations)
	message := errValue.Error()
	if lines := strings.Count(message, "\n") + 1; lines > 6 {
		t.Fatalf("汇总最多几行，实际 %d 行：\n%s", lines, message)
	}
	if len(message) > 600 {
		t.Fatalf("汇总要短（< 600 字节），实际 %d 字节", len(message))
	}
	if strings.Contains(message, "第 1 行") || strings.Contains(message, "第 5000 行") {
		t.Fatalf("不该再逐行展开：\n%s", message)
	}
	for _, wanted := range []string{"- 产品代码（material_code）为空：5,000 行",
		"- 使用单元产品标识（device_identifier_use_unit）为空：5,000 行",
		"有 5,000 行数据导入失败（10,000 个单元格为空）"} {
		if !strings.Contains(message, wanted) {
			t.Fatalf("汇总缺少 %q：\n%s", wanted, message)
		}
	}
}
