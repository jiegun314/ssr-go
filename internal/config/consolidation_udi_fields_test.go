package config

import (
	"path/filepath"
	"testing"
)

// TestConsolidationMappingCoversTheRequestedUDIFields 固定 UDI 导出列的增补口径：
//   - DI Data Carrier Type / Combination Product? / Kit? / 四个 Controlled by...
//     均从 global_udi_input 取数；
//   - Combination Product? 与 Kit? 按业务确认共用 if_kit_product；
//   - Catalog or Reference Number 仍取基准来源 ra_input.material_code，
//     不改成 UDI 的 product_code（当前导入结构里也没有这个 db_field）。
func TestConsolidationMappingCoversTheRequestedUDIFields(t *testing.T) {
	loader, err := NewLoader(filepath.Join("..", "..", "config"))
	if err != nil {
		t.Fatalf("装载配置失败：%v", err)
	}
	mapping, err := loader.LoadConsolidationMapping()
	if err != nil {
		t.Fatal(err)
	}
	dataset, _ := mapping["target_dataset"].(map[string]any)
	fields, _ := dataset["fields"].(map[string]any)

	wanted := []struct {
		name  string
		file  string
		field string
	}{
		{"Catalog or Reference Number", "ra_input", "material_code"},
		{"DI Data Carrier Type", "global_udi_input", "di_carrier"},
		{"Combination Product?", "global_udi_input", "if_kit_product"},
		{"Kit?", "global_udi_input", "if_kit_product"},
		{"Controlled by Expiration Date?", "global_udi_input", "if_pi_contains_expiry_date"},
		{"Controlled by Manufacturing Date?", "global_udi_input", "if_pi_contains_manufacturing_date"},
		{"Controlled by Lot or Batch Number?", "global_udi_input", "if_pi_contains_lot_number"},
		{"Controlled by Serial Number?", "global_udi_input", "if_pi_contains_serial_number"},
	}
	for _, want := range wanted {
		t.Run(want.name, func(t *testing.T) {
			raw, ok := fields[want.name]
			if !ok {
				t.Fatalf("导出列 %q 没有配置", want.name)
			}
			definition, _ := raw.(map[string]any)
			source, _ := definition["source"].(map[string]any)
			if got, _ := source["file"].(string); got != want.file {
				t.Errorf("来源文件 = %q，want %q", got, want.file)
			}
			if got, _ := source["field"].(string); got != want.field {
				t.Errorf("来源字段 = %q，want %q", got, want.field)
			}
		})
	}
}
