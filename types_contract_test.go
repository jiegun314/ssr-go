package main

import (
	"strings"
	"testing"
)

// TestTheFrontendTypesComeFromTheGeneratedModels 固定前端类型的来源口径：
// Go 返回结构一律从 Wails 生成的 frontend/wailsjs/go/models.ts 派生，
// 不许在 frontend/src/types.ts 里再手写一遍字段（两处必然漂移）。
//
// 需要"收窄"的地方允许用 Omit + 交叉类型补联合类型（例如 state 只有四种、
// SettingsNode.kind 只有六种），但字段清单必须来自生成模型。
func TestTheFrontendTypesComeFromTheGeneratedModels(t *testing.T) {
	types := readFrontendSource(t, "src/types.ts")

	if !strings.Contains(types, `from "../wailsjs/go/models"`) {
		t.Fatal("types.ts 应当从生成的 models.ts 派生类型")
	}
	// 生成模型里有的结构，不许手写字面量
	for _, name := range []string{
		"ImportState", "InitialState", "ImportResult", "ClearResult", "ConsolidateResult",
		"ExportResult", "ExportTarget", "ReviewResult", "SettingsNode", "SettingsChange",
		"SettingsTab", "SettingsSaveResult",
	} {
		if strings.Contains(types, "export type "+name+" = {\n") {
			t.Errorf("%s 应当从生成模型派生，不要再手写字段清单", name)
		}
	}
	// 允许收窄的两个联合类型：kind 与 action（穷尽判断依赖它们）
	for _, narrowed := range []string{
		`kind: "map" | "seq" | "string" | "number" | "bool" | "null";`,
		`action?: "set" | "append" | "remove";`,
		`state: "empty" | "imported" | "existing" | "failed";`,
	} {
		if !strings.Contains(types, narrowed) {
			t.Errorf("前端自己的联合类型应当保留：%s", narrowed)
		}
	}
	// 生成的类上带 convertValues 方法，必须剥掉，否则纯对象字面量无法满足类型
	if !strings.Contains(types, `type Data<T> = Omit<T, "convertValues">;`) {
		t.Error("应当用 Data<> 去掉生成类上的 convertValues 方法")
	}

	// 只有 Go 侧没有对应结构体的两个类型留在前端手写
	for _, handwritten := range []string{"export type AboutInfo = {", "export type SourceDefinition = {"} {
		if !strings.Contains(types, handwritten) {
			t.Errorf("前端自有类型应当保留在手写区：%s", handwritten)
		}
	}
}
