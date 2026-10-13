package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSampleHintsLiveInTheirOwnFileAndAreMergedBack 固定样本元数据的拆分口径：
//
//   - 导入映射里不再有 sample 子块（它只剩运行期真正读的键）；
//   - 元数据在 config/sample_mapping.yaml；
//   - loader 读导入映射时把条目**合并回列的 sample 子块** —— internal/sample 因此一行不用改；
//   - 老配置（把元数据内联在列里、或干脆没有这个文件）照样能用。
func TestSampleHintsLiveInTheirOwnFileAndAreMergedBack(t *testing.T) {
	loader, err := NewLoader(filepath.Join("..", "..", "config"))
	if err != nil {
		t.Fatal(err)
	}
	samples, err := loader.LoadSampleMapping()
	if err != nil {
		t.Fatalf("读样本元数据失败：%v", err)
	}
	if samples == nil {
		t.Fatal("仓库里应当有 config/sample_mapping.yaml")
	}
	sourceSamples, _ := samples["sources"].(map[string]any)
	if len(sourceSamples) != 4 {
		t.Errorf("样本元数据的来源数 = %d；want 4", len(sourceSamples))
	}

	imports, err := loader.LoadImportMapping()
	if err != nil {
		t.Fatal(err)
	}
	sources, _ := imports["sources"].(map[string]any)
	for _, sourceName := range []string{"ra_input", "global_udi_input", "medical_insurance_code", "product_category"} {
		source, _ := sources[sourceName].(map[string]any)
		columns, _ := source["columns"].([]any)
		if len(columns) == 0 {
			t.Fatalf("%s 没有列", sourceName)
		}
		merged := 0
		for _, rawColumn := range columns {
			column, _ := rawColumn.(map[string]any)
			hint, _ := column["sample"].(map[string]any)
			if len(hint) == 0 {
				continue
			}
			merged++
			if name, _ := hint["english_name"].(string); name == "" {
				t.Errorf("%s.%v 的 english_name 丢了", sourceName, column["db_field"])
			}
			if _, ok := hint["data_type"].(string); !ok {
				t.Errorf("%s.%v 的 data_type 丢了", sourceName, column["db_field"])
			}
		}
		if merged != len(columns) {
			t.Errorf("%s 合并回来的样本条目 %d 个；want %d（每列都有）",
				sourceName, merged, len(columns))
		}
	}

	// 抽查一条：原样搬过来的英文表头
	raInput, _ := sources["ra_input"].(map[string]any)
	columns, _ := raInput["columns"].([]any)
	first, _ := columns[0].(map[string]any)
	hint, _ := first["sample"].(map[string]any)
	if got, _ := hint["english_name"].(string); got != "Registration Filing Certificate Number" {
		t.Errorf("第一条列的 english_name = %q；want Registration Filing Certificate Number", got)
	}
}

// TestSampleHintsAreOptionalForOldConfigs 固定向后兼容：没有这个文件时一切照常
// （老配置把样本元数据内联在列里，或干脆不用样本工厂）。
func TestSampleHintsAreOptionalForOldConfigs(t *testing.T) {
	workspace := t.TempDir()
	copyConfigDirForTest(t, filepath.Join("..", "..", "config"), workspace)
	if err := removeFileForTest(filepath.Join(workspace, SampleMappingFile)); err != nil {
		t.Fatal(err)
	}
	loader, err := NewLoader(workspace)
	if err != nil {
		t.Fatalf("装载失败：%v", err)
	}
	if err := loader.ValidateAll(); err != nil {
		t.Fatalf("没有样本元数据文件时应当照常通过校验：%v", err)
	}
	imports, err := loader.LoadImportMapping()
	if err != nil {
		t.Fatal(err)
	}
	sources, _ := imports["sources"].(map[string]any)
	raInput, _ := sources["ra_input"].(map[string]any)
	columns, _ := raInput["columns"].([]any)
	first, _ := columns[0].(map[string]any)
	if _, present := first["sample"]; present {
		t.Error("没有元数据文件时不该凭空造出 sample 子块")
	}
}

// TestSampleMappingFallsBackToDefaultsOnAFreshInstall 固定"全新安装"这条路径：
// 启动补齐只处理四份行为配置，样本元数据不进那一套 —— 正式名文件不存在时，
// loader 必须回退读 config/defaults/sample_mapping.yaml（只读，不生成正式名文件），
// 否则新装环境的样本工厂会拿不到任何元数据。
func TestSampleMappingFallsBackToDefaultsOnAFreshInstall(t *testing.T) {
	workspace := t.TempDir()
	// 只放 defaults（模拟发布包解压后的样子：正式名文件一个都没有）
	defaults := filepath.Join(workspace, "config", DefaultsDirectory)
	if err := os.MkdirAll(defaults, 0o755); err != nil {
		t.Fatal(err)
	}
	copyConfigDirForTest(t, filepath.Join("..", "..", "config"), defaults)

	loader, err := NewLoader(filepath.Join(workspace, "config"))
	if err != nil {
		t.Fatalf("装载失败：%v", err)
	}
	if err := loader.ValidateAll(); err != nil {
		t.Fatalf("全新安装校验失败：%v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "config", SampleMappingFile)); !os.IsNotExist(err) {
		t.Error("样本元数据是可选文件，不该被启动补齐生成正式名文件")
	}
	imports, err := loader.LoadImportMapping()
	if err != nil {
		t.Fatal(err)
	}
	sources, _ := imports["sources"].(map[string]any)
	total, merged := 0, 0
	for _, rawSource := range sources {
		source, _ := rawSource.(map[string]any)
		columns, _ := source["columns"].([]any)
		for _, rawColumn := range columns {
			column, _ := rawColumn.(map[string]any)
			total++
			if hint, _ := column["sample"].(map[string]any); len(hint) > 0 {
				merged++
			}
		}
	}
	if total == 0 || merged != total {
		t.Errorf("全新安装时样本元数据合并 %d/%d；want 全部合并", merged, total)
	}
}
