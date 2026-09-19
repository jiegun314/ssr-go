// Package validation 承载整合前的前置检查（R10）。
//
// 注意：Python 版的 services/data_validation_service.py 里还有一个
// ImportDataValidation 老接口，它对 required_if_applicable 用了 truthy 判断，与真正的
// 导入校验不一致；AGENTS.md §7.2 / §10.1 #7 明确说**不要翻译**那个类，这里只有来源完备性。
package validation

import (
	"strings"

	"github.com/jiegun314/ssr-go/internal/config"
	"github.com/jiegun314/ssr-go/internal/store"
)

// MissingSourceDataError：整合前发现来源表缺表或为空。
type MissingSourceDataError struct {
	SourceNames []string
}

func (errorValue *MissingSourceDataError) Error() string {
	return "数据整合未执行：以下原数据表为空：\n- " +
		strings.Join(errorValue.SourceNames, "\n- ")
}

// SourceCompleteness 检查四张来源表是否都存在且都有数据。
type SourceCompleteness struct {
	Loader     *config.Loader
	Repository *store.Repository
}

// CheckTableCompleteness 报告缺表；缺表时错误里写着「…不存在」。
func (validation *SourceCompleteness) CheckTableCompleteness() error {
	document, err := validation.Loader.LoadImportMappingDocument()
	if err != nil {
		return err
	}
	sources, _ := document.Value["sources"].(map[string]any)
	existing, err := validation.Repository.TableNames()
	if err != nil {
		return err
	}
	missing := []string{}
	missingTables := map[string]bool{}
	for _, sourceName := range document.Keys("sources") {
		source, _ := sources[sourceName].(map[string]any)
		tableName, _ := source["target_table"].(string)
		if !containsString(existing, tableName) {
			missing = append(missing, sourceName)
			missingTables[tableName] = true
		}
	}
	if len(missing) == 0 {
		return nil
	}
	descriptions := []string{}
	for _, sourceName := range document.Keys("sources") {
		source, _ := sources[sourceName].(map[string]any)
		tableName, _ := source["target_table"].(string)
		if !missingTables[tableName] {
			continue
		}
		descriptions = append(descriptions, describeSource(sourceName, source, tableName)+"不存在）")
	}
	return &MissingSourceDataError{SourceNames: descriptions}
}

// EmptySourceNames 返回没有记录的来源（表存在但一条都没有）。
func (validation *SourceCompleteness) EmptySourceNames() ([]string, error) {
	document, err := validation.Loader.LoadImportMappingDocument()
	if err != nil {
		return nil, err
	}
	sources, _ := document.Value["sources"].(map[string]any)
	empty := []string{}
	for _, sourceName := range document.Keys("sources") {
		source, _ := sources[sourceName].(map[string]any)
		tableName, _ := source["target_table"].(string)
		count, err := validation.Repository.CountTableRows(tableName)
		if err != nil {
			return nil, err
		}
		if count == 0 {
			empty = append(empty, describeSource(sourceName, source, tableName)+"）")
		}
	}
	return empty, nil
}

// CheckSourceDataAvailable 报告空来源；有空来源时整合不执行。
func (validation *SourceCompleteness) CheckSourceDataAvailable() error {
	empty, err := validation.EmptySourceNames()
	if err != nil {
		return err
	}
	if len(empty) > 0 {
		return &MissingSourceDataError{SourceNames: empty}
	}
	return nil
}

// describeSource 是错误文案里的一个来源：`{来源键}（{中文名}，表：{表名}`（缺表时补「不存在」）。
func describeSource(sourceName string, source map[string]any, tableName string) string {
	chineseName, _ := source["chinese_name"].(string)
	if chineseName == "" {
		chineseName, _ = source["display_name"].(string)
	}
	return sourceName + "（" + chineseName + "，表：" + tableName
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
