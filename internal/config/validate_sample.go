package config

import "strings"

// ValidateSampleMapping 校验样本工厂专用的列元数据（config/sample_mapping.yaml）。
//
// 这个文件是可选的：不存在就跳过（老配置把样本元数据内联在导入映射的列里）。
// 校验口径与"文件里的内容必须真的对应得上"对齐：
//   - sources.<来源> 必须是导入映射里存在的来源；
//   - <db_field> 必须是该来源某一列的 db_field；
//   - 每条的键只能是 english_name / data_type / allowed_values，且 english_name 非空；
//   - 同一条目内 allowed_values 必须是字符串列表。
func (loader *Loader) ValidateSampleMapping(imports map[string]any) error {
	samples, err := loader.LoadSampleMapping()
	if err != nil {
		return err
	}
	if samples == nil {
		return nil
	}
	sourceSamples, err := requireMapping(samples["sources"], "sample_mapping.sources")
	if err != nil {
		return err
	}
	sources, _ := imports["sources"].(map[string]any)
	for _, sourceName := range sortedKeys(sourceSamples) {
		rawHints, _ := sourceSamples[sourceName].(map[string]any)
		label := "sample_mapping.sources." + sourceName
		source, present := sources[sourceName].(map[string]any)
		if !present {
			return errorf("%s: 导入映射里没有这个来源（excel_import_mapping.sources）", label)
		}
		columns := map[string]bool{}
		if list, ok := source["columns"].([]any); ok {
			for _, rawColumn := range list {
				column, _ := rawColumn.(map[string]any)
				if field, ok := column["db_field"].(string); ok {
					columns[field] = true
				}
			}
		}
		for _, dbField := range sortedKeys(rawHints) {
			hint, _ := rawHints[dbField].(map[string]any)
			fieldLabel := label + "." + dbField
			if !columns[dbField] {
				return errorf("%s: 该来源的列里没有这个 db_field", fieldLabel)
			}
			if len(hint) == 0 {
				return errorf("%s: 条目不能为空", fieldLabel)
			}
			for key, value := range hint {
				switch key {
				case "english_name":
					text, ok := asString(value)
					if !ok || strings.TrimSpace(text) == "" {
						return errorf("%s.english_name 必须是非空字符串", fieldLabel)
					}
				case "data_type":
					if _, ok := asString(value); !ok {
						return errorf("%s.data_type 必须是字符串", fieldLabel)
					}
				case "allowed_values":
					list, ok := value.([]any)
					if !ok {
						return errorf("%s.allowed_values 必须是列表", fieldLabel)
					}
					for _, item := range list {
						if _, ok := asString(item); !ok {
							return errorf("%s.allowed_values 里只能是字符串", fieldLabel)
						}
					}
				default:
					return errorf("%s: 不认识的键 %q（只允许 english_name / data_type / allowed_values）",
						fieldLabel, key)
				}
			}
		}
	}
	return nil
}
