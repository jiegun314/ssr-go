package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/config"
)

// TestConfigurationDocumentReturnsFourTrees 固定「参数设定」窗口的数据来源与结构：
// 四个页签对应 config/ 下的四份 YAML，每份都给出一棵结构化树（前端用 antd Tree 渲染）
// 以及原始文本（供「编辑原文」）。
func TestConfigurationDocumentReturnsFourTrees(t *testing.T) {
	workspace := t.TempDir()
	copyDirectoryForTest(t, "config", filepath.Join(workspace, "config"))
	loader, err := config.NewLoader(filepath.Join(workspace, "config"))
	if err != nil {
		t.Fatalf("读配置失败：%v", err)
	}
	app := NewApp(nil)
	app.loader = loader

	tabs := app.ConfigurationDocument()
	if len(tabs) != 6 {
		t.Fatalf("页签数量 = %d; want 6", len(tabs))
	}
	expectedTitles := []string{"基础设置", "导入映射", "整合映射", "日志列", "共享盘", "人员名单"}
	expectedKeys := []string{
		config.SettingFile, config.ImportMappingFile,
		config.ConsolidationMappingFile, config.LogColumnsFile,
		config.WorkflowFile, config.UsersFile,
	}
	for index, tab := range tabs {
		if tab.Title != expectedTitles[index] || tab.Key != expectedKeys[index] {
			t.Errorf("第 %d 个页签 = %q/%q; want %q/%q",
				index, tab.Title, tab.Key, expectedTitles[index], expectedKeys[index])
		}
		if tab.Path == "" || !strings.HasSuffix(tab.Path, tab.Key) {
			t.Errorf("%s 的路径不对：%q", tab.Key, tab.Path)
		}
		if tab.Key == config.UsersFile || tab.Key == config.WorkflowFile {
			// 这两份是可选的：不存在时给模板原文，没有树是预期的
			if tab.Raw == "" {
				t.Errorf("%s 没有给出模板原文", tab.Key)
			}
			continue
		}
		if len(tab.Tree) == 0 {
			t.Errorf("%s 没有给出结构化树", tab.Key)
		}
		if tab.Raw == "" {
			t.Errorf("%s 没有返回原文（「编辑原文」要用）", tab.Key)
		}
	}
	// 日志列是生成物，页签说明要写明
	for _, tab := range tabs {
		if tab.Key == config.LogColumnsFile && !strings.Contains(tab.Note, "生成物") {
			t.Errorf("日志列页签的说明没写「生成物」：%q", tab.Note)
		}
	}
	// 日志列的 columns 是一串"列名"（列表项的值），单独按值检查一次
	for _, tab := range tabs {
		if tab.Key != config.LogColumnsFile {
			continue
		}
		if !treeHasValue(tab.Tree, "material_code") {
			t.Errorf("日志列的树里缺少列表项 material_code")
		}
	}
	// 代表性键要出现在对应页签的树里（深度优先查找）
	expectations := map[string][]string{
		config.SettingFile:              {"export_template", "startup"},
		config.ImportMappingFile:        {"global_settings", "mandatory_status"},
		config.ConsolidationMappingFile: {"merge_rules", "duplicate_check"},
		config.LogColumnsFile:           {"before", "columns", "after"},
	}
	for _, tab := range tabs {
		for _, wanted := range expectations[tab.Key] {
			if !treeHasKey(tab.Tree, wanted) {
				t.Errorf("%s 的树里缺少 %q", tab.Key, wanted)
			}
		}
	}
}

// TestSettingsTreeCarriesKindsAndDeepNesting 固定树的节点语义：
// 容器标 map/seq 并带 Children，标量按 YAML 类型标 string/number/bool/null 并带 Value；
// 配置里最深的层级（整合映射的 duplicates.compare_by）也要在树里保留下来。
func TestSettingsTreeCarriesKindsAndDeepNesting(t *testing.T) {
	workspace := t.TempDir()
	copyDirectoryForTest(t, "config", filepath.Join(workspace, "config"))
	loader, err := config.NewLoader(filepath.Join(workspace, "config"))
	if err != nil {
		t.Fatalf("读配置失败：%v", err)
	}
	app := NewApp(nil)
	app.loader = loader
	tabs := app.ConfigurationDocument()

	kinds := map[string]bool{}
	maxDepth := 0
	var walk func(nodes []SettingsNode, depth int)
	walk = func(nodes []SettingsNode, depth int) {
		if depth > maxDepth {
			maxDepth = depth
		}
		for _, node := range nodes {
			kinds[node.Kind] = true
			switch node.Kind {
			case "map", "seq":
				if node.Value != "" {
					t.Errorf("容器节点不该带 Value：%+v", node)
				}
			default:
				if len(node.Children) > 0 {
					t.Errorf("标量节点不该带 Children：%+v", node)
				}
			}
			walk(node.Children, depth+1)
		}
	}
	for _, tab := range tabs {
		walk(tab.Tree, 1)
	}
	for _, wanted := range []string{"map", "seq", "string", "number", "bool", "null"} {
		if !kinds[wanted] {
			t.Errorf("四份配置里应该都出现过 kind=%s 的节点", wanted)
		}
	}
	if maxDepth < 6 {
		t.Fatalf("最深层级 = %d；配置里最少有第 6 层（compare_by），用例没有覆盖到这个回归点", maxDepth)
	}
	// 具体抽查：setting.yaml 的 export_template.header_row 是数字 4
	setting := tabs[0]
	if node := treeFind(setting.Tree, "header_row"); node == nil {
		t.Error("setting.yaml 的树里找不到 header_row")
	} else if node.Kind != "number" || node.Value != "4" {
		t.Errorf("header_row = %+v; want number/4", *node)
	}
}

// TestSettingsTreeReportsEmptyCollections 空集合要能被前端识别出来（显示「（空 object/array）」）。
func TestSettingsTreeReportsEmptyCollections(t *testing.T) {
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, "config")
	copyDirectoryForTest(t, "config", configDir)
	content := "version: \"1.0\"\nglobal_settings:\n  empty_map: {}\n  empty_list: []\nsources: {}\n"
	if err := os.WriteFile(filepath.Join(configDir, config.ImportMappingFile), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	loader, err := config.NewLoader(configDir)
	if err != nil {
		t.Fatalf("读配置失败：%v", err)
	}
	app := NewApp(nil)
	app.loader = loader
	tab := app.ConfigurationDocument()[1]

	if node := treeFind(tab.Tree, "empty_map"); node == nil || node.Kind != "map" || len(node.Children) != 0 {
		t.Errorf("空映射节点 = %+v; want map/无子节点", node)
	}
	if node := treeFind(tab.Tree, "empty_list"); node == nil || node.Kind != "seq" || len(node.Children) != 0 {
		t.Errorf("空列表节点 = %+v; want seq/无子节点", node)
	}
}

// TestConfigurationDocumentReportsUnreadableFile 配置读不出来时，页签要给出说明而不是空白。
func TestConfigurationDocumentReportsUnreadableFile(t *testing.T) {
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, "config")
	copyDirectoryForTest(t, "config", configDir)
	if err := os.Remove(filepath.Join(configDir, config.LogColumnsFile)); err != nil {
		t.Fatal(err)
	}
	loader, err := config.NewLoader(configDir)
	if err != nil {
		t.Fatalf("读配置失败：%v", err)
	}
	app := NewApp(nil)
	app.loader = loader
	tabs := app.ConfigurationDocument()
	var logTab SettingsTab
	for _, tab := range tabs {
		if tab.Key == config.LogColumnsFile {
			logTab = tab
		}
	}
	if !strings.Contains(logTab.Note, "无法读取") {
		t.Errorf("读不到文件时的说明 = %q; want 含「无法读取」", logTab.Note)
	}
	if len(logTab.Tree) != 0 {
		t.Errorf("读不到文件时不该有树：%+v", logTab.Tree)
	}
}

/* ---------- 测试辅助 ---------- */

func treeHasKey(nodes []SettingsNode, key string) bool { return treeFind(nodes, key) != nil }

// treeHasValue 在标量节点的值里找（列表项的值也在这里）。
func treeHasValue(nodes []SettingsNode, value string) bool {
	for _, node := range nodes {
		if node.Value == value {
			return true
		}
		if treeHasValue(node.Children, value) {
			return true
		}
	}
	return false
}

func treeFind(nodes []SettingsNode, key string) *SettingsNode {
	for index := range nodes {
		if nodes[index].Key == key {
			return &nodes[index]
		}
		if found := treeFind(nodes[index].Children, key); found != nil {
			return found
		}
	}
	return nil
}

// TestUsersTabUsesFixedGroupKeys 固定「人员名单」页签的新结构：
// users 下是按角色命名的固定组别（RA 第一），不再是一串带下标的列表，
// 每条记录里也没有 role 字段（组别键即角色）。
func TestUsersTabUsesFixedGroupKeys(t *testing.T) {
	app, configDir := newSettingsApp(t)
	content, err := os.ReadFile(filepath.Join("config", config.UsersFile))
	if err != nil {
		t.Fatalf("读人员名单样例失败：%v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, config.UsersFile), content, 0o644); err != nil {
		t.Fatalf("写人员名单失败：%v", err)
	}
	loader, err := config.NewLoader(configDir)
	if err != nil {
		t.Fatalf("重新读配置失败：%v", err)
	}
	app.loader = loader

	var usersTab SettingsTab
	for _, tab := range app.ConfigurationDocument() {
		if tab.Key == config.UsersFile {
			usersTab = tab
			break
		}
	}
	if usersTab.Key == "" {
		t.Fatal("没有找到人员名单页签")
	}
	root := findSettingsNode(usersTab.Tree, "users")
	if root == nil || root.Kind != "map" {
		t.Fatalf("users 应当是映射：%+v", root)
	}
	want := []string{"RA", "EAMA", "UDI", "SUPPLY CHAIN", "ADMIN"}
	if len(root.Children) != len(want) {
		t.Fatalf("组别数量 = %d; want %d", len(root.Children), len(want))
	}
	for index, key := range want {
		group := root.Children[index]
		if group.Key != key || group.Kind != "map" {
			t.Errorf("第 %d 个组别 = %q/%s; want %q/map", index, group.Key, group.Kind, key)
		}
		hasName := false
		for _, child := range group.Children {
			if child.Key == "role" {
				t.Errorf("组别 %s 的记录里不应该再有 role 字段", key)
			}
			if child.Key == "name" {
				hasName = true
			}
		}
		if !hasName {
			t.Errorf("组别 %s 缺少 name 字段", key)
		}
	}
}
