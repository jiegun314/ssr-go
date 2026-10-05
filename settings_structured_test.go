package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/config"
)

// diffLines 返回两份文本的差异行（用于"只改一行"的硬契约）。
func diffLines(before, after string) []string {
	oldLines := strings.Split(before, "\n")
	newLines := strings.Split(after, "\n")
	if len(oldLines) != len(newLines) {
		return []string{"行数不同"}
	}
	changed := []string{}
	for index := range oldLines {
		if oldLines[index] != newLines[index] {
			changed = append(changed, oldLines[index]+" → "+newLines[index])
		}
	}
	return changed
}

// TestStructuredSaveTouchesOnlyTheEditedLine 是结构化写回的硬契约：
// 改一个标量之后，配置文件除那一行以外**逐字节不变**（注释、空行、键序、引号风格都不动）。
func TestStructuredSaveTouchesOnlyTheEditedLine(t *testing.T) {
	app, configDir := newSettingsApp(t)
	path := filepath.Join(configDir, config.SettingFile)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读原文件失败：%v", err)
	}

	result := app.SaveSettingsValues(config.SettingFile, []SettingsChange{
		{Path: []string{"startup", "cleanup_on_startup"}, Value: "true"},
	})
	if result.Failed {
		t.Fatalf("保存失败：%s", result.Message)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读回失败：%v", err)
	}
	changed := diffLines(string(before), string(after))
	if len(changed) != 1 {
		t.Fatalf("应当只改动一行，实际 %d 行：%v", len(changed), changed)
	}
	if !strings.Contains(changed[0], "cleanup_on_startup: false") ||
		!strings.Contains(changed[0], "cleanup_on_startup: true") {
		t.Fatalf("改动行不对：%s", changed[0])
	}
	if !strings.Contains(string(after), "# 启动行为：默认 false") {
		t.Error("注释必须原样保留")
	}
	if _, err := os.Stat(result.Backup); err != nil {
		t.Errorf("应当留下 .bak 备份：%v", err)
	}
}

// TestStructuredSaveEditsInsideBigMappingWithoutReformatting 在最大的那份配置里改一个值：
// 除了目标行，其它 1000 多行（含 160 行注释与 20 处锚点引用）必须一个字节都不变。
func TestStructuredSaveEditsInsideBigMappingWithoutReformatting(t *testing.T) {
	app, configDir := newSettingsApp(t)
	path := filepath.Join(configDir, config.ImportMappingFile)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读原文件失败：%v", err)
	}
	original := string(before)

	result := app.SaveSettingsValues(config.ImportMappingFile, []SettingsChange{
		{Path: []string{"sources", "ra_input", "chinese_name"}, Value: "RA信息（改）"},
	})
	if result.Failed {
		t.Fatalf("保存失败：%s", result.Message)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读回失败：%v", err)
	}
	changed := diffLines(original, string(after))
	if len(changed) != 1 {
		t.Fatalf("应当只改动一行，实际 %d 行：%v", len(changed), changed)
	}
	if strings.Count(string(after), "*") != strings.Count(original, "*") {
		t.Error("别名引用必须保持原样（不能被展开）")
	}
	if strings.Count(string(after), "&") != strings.Count(original, "&") {
		t.Error("锚点定义必须保持原样")
	}
	if !strings.Contains(string(after), "# 导入映射：四个来源的 Excel 怎么读进来") {
		t.Error("头部注释必须保留")
	}
}

// TestStructuredSaveKeepsSharedMappingAnchorsIntact 改共享映射本体的值：
// 只动那一行，20 处 *别名 引用保持原样（引用处本身不可编辑）。
func TestStructuredSaveKeepsSharedMappingAnchorsIntact(t *testing.T) {
	app, configDir := newSettingsApp(t)
	path := filepath.Join(configDir, config.ImportMappingFile)
	before, _ := os.ReadFile(path)
	original := string(before)
	// shared_mappings.udi_yes_no 里的 "N/A": "否" —— 只改映射后的值
	result := app.SaveSettingsValues(config.ImportMappingFile, []SettingsChange{
		{Path: []string{"shared_mappings", "udi_yes_no", "N/A"}, Value: "不适用"},
	})
	if result.Failed {
		t.Fatalf("保存失败：%s", result.Message)
	}
	after, _ := os.ReadFile(path)
	changed := diffLines(original, string(after))
	if len(changed) != 1 || !strings.Contains(changed[0], `"不适用"`) {
		t.Fatalf("应当只改映射值那一行，实际：%v", changed)
	}
	if strings.Count(string(after), "*udi_yes_no") != strings.Count(original, "*udi_yes_no") {
		t.Error("引用共享映射的别名必须保持原样")
	}

	// 引用处（别名）不允许直接编辑
	refused := app.SaveSettingsValues(config.ImportMappingFile, []SettingsChange{
		{Path: []string{"sources", "ra_input", "columns", "9", "value_mapping"}, Value: "x"},
	})
	if !refused.Failed || !strings.Contains(refused.Message, "共享映射") {
		t.Fatalf("共享映射的引用处应当被拒绝：%+v", refused)
	}
}

// TestStructuredSaveRefusesDangerousPaths 只读白名单：DB 字段名、样本工厂专用块、
// 条件表达式、生成物（日志列）都不允许在这里改。
func TestStructuredSaveRefusesDangerousPaths(t *testing.T) {
	app, configDir := newSettingsApp(t)
	importPath := filepath.Join(configDir, config.ImportMappingFile)
	before, _ := os.ReadFile(importPath)

	cases := []struct {
		name    string
		key     string
		path    []string
		keyword string
	}{
		{"DB 字段名", config.ImportMappingFile,
			[]string{"sources", "ra_input", "columns", "0", "db_field"}, "暂存表"},
		{"样本工厂专用块", config.ImportMappingFile,
			[]string{"sources", "ra_input", "columns", "0", "sample", "english_name"}, "样本工厂"},
		{"条件表达式", config.ImportMappingFile,
			[]string{"sources", "global_udi_input", "columns", "9", "required_when"}, "只读"},
		{"日志列生成物", config.LogColumnsFile, []string{"before", "0"}, "生成物"},
	}
	for _, item := range cases {
		result := app.SaveSettingsValues(item.key, []SettingsChange{{Path: item.path, Value: "x"}})
		if !result.Failed {
			t.Errorf("%s 应当被拒绝，实际成功：%+v", item.name, result)
			continue
		}
		if !strings.Contains(result.Message, item.keyword) {
			t.Errorf("%s 的拒绝原因里应当提到 %q，实际：%s", item.name, item.keyword, result.Message)
		}
	}
	after, _ := os.ReadFile(importPath)
	if string(before) != string(after) {
		t.Error("被拒绝的修改不许动文件")
	}
}

// TestStructuredSaveRestoresOnInvalidValue 非法值（语法/校验不过）必须还原文件并报错。
func TestStructuredSaveRestoresOnInvalidValue(t *testing.T) {
	app, configDir := newSettingsApp(t)
	path := filepath.Join(configDir, config.SettingFile)
	before, _ := os.ReadFile(path)

	// 数字框写入非数字：在渲染阶段就被挡下
	bad := app.SaveSettingsValues(config.SettingFile, []SettingsChange{
		{Path: []string{"export_template", "header_row"}, Value: "第4行"},
	})
	if !bad.Failed || !strings.Contains(bad.Message, "数字") {
		t.Fatalf("非数字应当被拒绝：%+v", bad)
	}
	// 通过结构校验但整体校验不过：例如把清理开关写成非布尔（这里走整体校验路径）
	invalid := app.SaveSettingsValues(config.SettingFile, []SettingsChange{
		{Path: []string{"database", "path"}, Value: ""},
	})
	if invalid.Failed {
		if after, _ := os.ReadFile(path); string(after) != string(before) {
			t.Error("校验失败后必须还原文件")
		}
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Errorf("非法修改不该留在盘上：\n%s", diffLines(string(before), string(after)))
	}
}

// TestStructuredSaveEditsScalarLists 标量列表可以增删条目，且只动列表那几行。
func TestStructuredSaveEditsScalarLists(t *testing.T) {
	app, configDir := newSettingsApp(t)
	path := filepath.Join(configDir, config.ImportMappingFile)
	before, _ := os.ReadFile(path)
	original := string(before)

	appended := app.SaveSettingsValues(config.ImportMappingFile, []SettingsChange{
		{Path: []string{"global_settings", "empty_values"}, Action: "append", Value: "null值"},
	})
	if appended.Failed {
		t.Fatalf("追加列表项失败：%s", appended.Message)
	}
	afterAppend, _ := os.ReadFile(path)
	if !strings.Contains(string(afterAppend), `- "null值"`) {
		t.Errorf("追加的列表项没写进去：\n%s", string(afterAppend))
	}
	if lines := diffLines(original, string(afterAppend)); len(lines) != 1 {
		t.Errorf("追加只应当多一行：%v", lines)
	}

	removed := app.SaveSettingsValues(config.ImportMappingFile, []SettingsChange{
		{Path: []string{"global_settings", "empty_values"}, Action: "remove", Index: 5}, // 刚追加的那一项
	})
	if removed.Failed {
		t.Fatalf("删除列表项失败：%s", removed.Message)
	}
	afterRemove, _ := os.ReadFile(path)
	if strings.Contains(string(afterRemove), `- "null值"`) {
		t.Errorf("删除没生效：\n%s", string(afterRemove))
	}

	// 列（columns）不允许增删：顺序即暂存表列顺序
	refused := app.SaveSettingsValues(config.ImportMappingFile, []SettingsChange{
		{Path: []string{"sources", "ra_input", "columns"}, Action: "append", Value: "新列"},
	})
	if !refused.Failed || !strings.Contains(refused.Message, "不允许增删") {
		t.Fatalf("列列表应当禁止增删：%+v", refused)
	}
}

// TestStructuredSaveHotReloadsTheConfiguration 保存成功后立即热重载：
// importer 的规则要换成新值，不需要重启程序。
func TestStructuredSaveHotReloadsTheConfiguration(t *testing.T) {
	app, _ := newSettingsApp(t)
	// 热重载会重建 importer / 整合服务 / 操作日志，需要数据库与外键字段：直接跑一次装配
	if _, err := app.wireConfiguration(app.loader); err != nil {
		t.Fatalf("初次装配失败：%v", err)
	}
	before := app.importer.Rules["ra_input"].ChineseName

	result := app.SaveSettingsValues(config.ImportMappingFile, []SettingsChange{
		{Path: []string{"sources", "ra_input", "chinese_name"}, Value: "RA信息X"},
	})
	if result.Failed {
		t.Fatalf("保存失败：%s", result.Message)
	}
	if !strings.Contains(result.Message, "热重载") {
		t.Errorf("保存成功提示里应当说明已热重载：%s", result.Message)
	}
	if got := app.importer.Rules["ra_input"].ChineseName; got != "RA信息X" {
		t.Errorf("热重载后规则没更新：%q（原 %q）", got, before)
	}
	if got := app.service.Config.SourceRules; got == nil {
		t.Error("整合服务应当已经用新配置重建")
	}
	// 日志里留一行"已保存并热重载"
	if !strings.Contains(app.logText(), "配置已保存并热重载") {
		t.Errorf("日志里应当记一行热重载：%s", app.logText())
	}
}

// findSettingsNode 在树里按路径找节点（测试用）。
func findSettingsNode(nodes []SettingsNode, path ...string) *SettingsNode {
	for index := range nodes {
		node := &nodes[index]
		if len(node.Path) == len(path) {
			match := true
			for position, part := range path {
				if node.Path[position] != part {
					match = false
					break
				}
			}
			if match {
				return node
			}
		}
		if found := findSettingsNode(node.Children, path...); found != nil {
			return found
		}
	}
	return nil
}

// TestSettingsTreeCarriesCommentsControlsAndReadOnlyReasons 固定结构化表单的"读"这一侧：
// 树里必须带路径、注释、控件类型、可编辑性与只读原因 —— 前端才能渲染出分块表单。
func TestSettingsTreeCarriesCommentsControlsAndReadOnlyReasons(t *testing.T) {
	app, _ := newSettingsApp(t)
	tabs := app.ConfigurationDocument()
	if len(tabs) != 4 {
		t.Fatalf("应当有四个页签，实际 %d", len(tabs))
	}
	byKey := map[string]SettingsTab{}
	for _, tab := range tabs {
		byKey[tab.Key] = tab
	}

	// 基础设置：开关 + 数字 + 注释 + 依赖
	setting := byKey[config.SettingFile]
	cleanup := findSettingsNode(setting.Tree, "startup", "cleanup_on_startup")
	if cleanup == nil {
		t.Fatal("树里应当有 startup.cleanup_on_startup")
	}
	if cleanup.Control != "switch" || !cleanup.Editable {
		t.Errorf("启动清理开关应当渲染成开关且可编辑：%+v", cleanup)
	}
	// 注释挂在它所属的键上：setting.yaml 的"启动行为"整段注释挂在 startup 上
	startup := findSettingsNode(setting.Tree, "startup")
	if startup == nil || startup.Comment == "" {
		t.Errorf("容器节点上应当带注释（前端要显示全部注释）：%+v", startup)
	}
	if !strings.Contains(startup.Comment, "启动行为") {
		t.Errorf("注释内容不对：%q", startup.Comment)
	}
	headerRow := findSettingsNode(setting.Tree, "export_template", "header_row")
	if headerRow == nil || headerRow.Control != "number" || headerRow.Min == nil || *headerRow.Min != 1 {
		t.Errorf("表头行应当是带最小值的数字控件：%+v", headerRow)
	}

	// 导入映射：只读白名单 + 别名 + 列列表不可增删
	importTab := byKey[config.ImportMappingFile]
	dbField := findSettingsNode(importTab.Tree, "sources", "ra_input", "columns", "0", "db_field")
	if dbField == nil || dbField.Editable || dbField.Reason == "" {
		t.Errorf("DB 字段名必须只读且给出原因：%+v", dbField)
	}
	sample := findSettingsNode(importTab.Tree, "sources", "ra_input", "columns", "0", "sample")
	if sample == nil || sample.Editable || sample.Reason == "" {
		t.Errorf("样本工厂专用块必须只读：%+v", sample)
	}
	columns := findSettingsNode(importTab.Tree, "sources", "ra_input", "columns")
	if columns == nil || columns.ListEdit {
		t.Errorf("列列表不允许增删条目：%+v", columns)
	}
	emptyValues := findSettingsNode(importTab.Tree, "global_settings", "empty_values")
	if emptyValues == nil || !emptyValues.ListEdit {
		t.Errorf("标量列表应当允许增删条目：%+v", emptyValues)
	}
	mapping := findSettingsNode(importTab.Tree, "shared_mappings", "udi_yes_no", "N/A")
	if mapping == nil || !mapping.Editable {
		t.Errorf("共享映射本体的值应当可编辑：%+v", mapping)
	}
	aliasNode := findSettingsNode(importTab.Tree, "sources", "ra_input", "columns", "9", "value_mapping")
	if aliasNode == nil || aliasNode.Alias == "" || aliasNode.Editable || aliasNode.Reason == "" {
		t.Errorf("共享映射的引用处应当只读并指向锚点名：%+v", aliasNode)
	}

	// 日志列：整份只读
	logTab := byKey[config.LogColumnsFile]
	if len(logTab.Tree) == 0 || logTab.Tree[0].Editable {
		t.Error("日志列是生成物：整份应当只读")
	}
	if logTab.Tree[0].Reason == "" {
		t.Error("只读时应当给出原因")
	}

	// 整合映射：枚举来自验证器（match_type / cardinality / no_match.action）
	consolidationTab := byKey[config.ConsolidationMappingFile]
	cardinality := findSettingsNode(consolidationTab.Tree, "target_dataset", "merge_rules", "sources", "ra_input", "cardinality")
	if cardinality == nil || cardinality.Control != "select" || len(cardinality.Options) != 3 {
		t.Errorf("基数应当是三个候选值的下拉：%+v", cardinality)
	}
}
