package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Document 是一份 YAML 配置的两种视图：
//
//   - Value：解码后的映射，供逐键校验使用（与 Python 的 dict 一致）；
//   - Root：原始的 yaml.Node，用来读取**键的声明顺序**。
//
// 顺序不是装饰：启动清理删表的顺序、导出列的顺序、日志列的顺序都来自配置声明顺序，
// Python 的 dict 保留 YAML 顺序，Go 的 map 不保留，所以必须显式带上节点树。
type Document struct {
	Root  *yaml.Node
	Value map[string]any
}

// LoadDocument 读取一份 YAML，同时给出映射与键序。
func (loader *Loader) LoadDocument(fileName string) (*Document, error) {
	path := loader.Resolver.ConfigFile(fileName)
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errorf("Configuration file not found: %s", path)
		}
		return nil, errorf("Configuration file not readable: %s: %v", path, err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(content, &root); err != nil {
		return nil, errorf("Invalid YAML in %s: %v", path, err)
	}
	var decoded any
	if err := yaml.Unmarshal(content, &decoded); err != nil {
		return nil, errorf("Invalid YAML in %s: %v", path, err)
	}
	value, ok := decoded.(map[string]any)
	if !ok {
		return nil, errorf("Configuration root must be a mapping: %s", path)
	}
	return &Document{Root: &root, Value: value}, nil
}

// Keys 返回 path 指向的映射的键，顺序与文件里声明的一致。
// path 为空表示文档根。找不到或不是映射时返回 nil。
func (document *Document) Keys(path ...string) []string {
	node := document.mappingNode(path...)
	if node == nil {
		return nil
	}
	keys := make([]string, 0, len(node.Content)/2)
	for index := 0; index+1 < len(node.Content); index += 2 {
		keys = append(keys, node.Content[index].Value)
	}
	return keys
}

// MappingAt 按声明顺序读出 path 指向的映射，值是解码后的 Go 值。
func (document *Document) MappingAt(path ...string) map[string]any {
	node := document.mappingNode(path...)
	if node == nil {
		return nil
	}
	value := map[string]any{}
	if err := node.Decode(&value); err != nil {
		return nil
	}
	return value
}

func (document *Document) mappingNode(path ...string) *yaml.Node {
	node := document.Root
	if node != nil && node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		node = node.Content[0]
	}
	for _, key := range path {
		if node == nil || node.Kind != yaml.MappingNode {
			return nil
		}
		next := (*yaml.Node)(nil)
		for index := 0; index+1 < len(node.Content); index += 2 {
			if node.Content[index].Value == key {
				next = node.Content[index+1]
				break
			}
		}
		node = next
	}
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	return node
}

// LoadSettingDocument 返回 setting.yaml 的键序视图。
func (loader *Loader) LoadSettingDocument() (*Document, error) {
	return loader.LoadDocument(SettingFile)
}

// LoadImportMappingDocument 返回 excel_import_mapping.yaml 的键序视图。
func (loader *Loader) LoadImportMappingDocument() (*Document, error) {
	return loader.LoadDocument(ImportMappingFile)
}

// LoadConsolidationMappingDocument 返回 consolidation_mapping.yaml 的键序视图。
func (loader *Loader) LoadConsolidationMappingDocument() (*Document, error) {
	return loader.LoadDocument(ConsolidationMappingFile)
}
