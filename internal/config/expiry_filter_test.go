package config

import (
	"path/filepath"
	"testing"
)

// RA 导入的失效过滤口径：drop_expired.field 指向 RA失效日期，且该列必须已声明。
func TestRAImportHasExpiryFilter(t *testing.T) {
	loader, err := NewLoader(filepath.Join("..", "..", "config"))
	if err != nil {
		t.Fatalf("读配置失败：%v", err)
	}
	if err := loader.ValidateAll(); err != nil {
		t.Fatalf("仓库配置应当通过校验：%v", err)
	}
	document, err := loader.LoadImportMappingDocument()
	if err != nil {
		t.Fatal(err)
	}
	sources, _ := document.Value["sources"].(map[string]any)
	ra, _ := sources["ra_input"].(map[string]any)
	block, _ := ra["drop_expired"].(map[string]any)
	if block == nil {
		t.Fatal("ra_input 应当声明 drop_expired（删除失效注册证）")
	}
	if field, _ := block["field"].(string); field != "ra_expiration_date" {
		t.Errorf("drop_expired.field = %v；want ra_expiration_date", block["field"])
	}
}
