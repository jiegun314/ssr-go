package app

// RA 的唯一性检查口径：9 项 → 8 项，医疗器械管理类别不再参与唯一性判定。
// 只去掉"唯一"这一层约束；列本身、值归一与导出都保留。

import (
	"testing"

	"github.com/jiegun314/ssr-go/internal/config"
)

func TestRAUniquenessExcludesManagementClass(t *testing.T) {
	loader, err := config.NewLoader("config")
	if err != nil {
		t.Fatalf("读配置失败：%v", err)
	}
	document, err := loader.LoadImportMappingDocument()
	if err != nil {
		t.Fatalf("读导入映射失败：%v", err)
	}
	sources, _ := document.Value["sources"].(map[string]any)
	ra, _ := sources["ra_input"].(map[string]any)
	clean, _ := ra["clean"].(map[string]any)
	unique, _ := clean["unique"].([]any)
	seenColumn := false
	for _, item := range unique {
		if item == "medical_device_management_class" {
			t.Error("医疗器械管理类别不应再做唯一性检查（应从 clean.unique 移除）")
		}
	}
	for _, item := range unique {
		if item == "device_catalog_classification_code" {
			seenColumn = true
		}
	}
	if !seenColumn {
		t.Error("其余唯一性列不应被误删（器械目录分类编码仍要检查）")
	}
	// 列定义本身保留：唯一性检查去掉，不代表不再导入/导出这一列
	columns, _ := ra["columns"].([]any)
	found := false
	for _, raw := range columns {
		column, _ := raw.(map[string]any)
		if column["db_field"] == "medical_device_management_class" {
			found = true
		}
	}
	if !found {
		t.Error("医疗器械管理类别这一列应当保留（只去掉唯一性检查）")
	}
}
