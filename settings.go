package main

import (
	"fmt"
	"html"
	"os"
	"strconv"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"gopkg.in/yaml.v3"

	"github.com/jiegun314/ssr-go/internal/config"
)

// SettingsTab 是「参数设定」窗口里的一个页签：一份配置文件的标题、路径与渲染后的内容。
type SettingsTab struct {
	Key   string `json:"key"`   // 文件名（配置里唯一）
	Title string `json:"title"` // 页签标题
	Note  string `json:"note"`  // 页签说明（例如「生成物，请勿手改」）
	Path  string `json:"path"`  // 实际读取的文件路径
	HTML  string `json:"html"`  // 按 Markdown 风格渲染的结构化内容
	Raw   string `json:"raw"`   // 文件原文（供"编辑原文"使用）
}

// settingsTabSpec 是四个页签的展示信息（顺序即页签顺序）。
func settingsTabs() []SettingsTab {
	return []SettingsTab{
		{Key: config.SettingFile, Title: "基础设置", Note: "路径、表名、导出模板与启动策略"},
		{Key: config.ImportMappingFile, Title: "导入映射", Note: "来源、列定义、必填与条件必填、值归一"},
		{Key: config.ConsolidationMappingFile, Title: "整合映射", Note: "匹配规则、去重规则、导出列与变更描述"},
		{Key: config.LogColumnsFile, Title: "日志列", Note: "生成物：由导出模板表头生成，请勿手改"},
	}
}

// ConfigurationDocument 读四份 YAML 并渲染成 Markdown 风格的结构化内容，
// 供菜单「设置 → 参数设定」的四个页签显示。
func (app *App) ConfigurationDocument() []SettingsTab {
	app.mu.Lock()
	defer app.mu.Unlock()
	tabs := settingsTabs()
	if app.loader == nil {
		for index := range tabs {
			tabs[index].HTML = "<p class='empty'>配置未加载：请确认 config/ 与程序位于同一目录。</p>"
		}
		return tabs
	}
	for index := range tabs {
		path := app.loader.Resolver.ConfigFile(tabs[index].Key)
		tabs[index].Path = path
		content, err := os.ReadFile(path)
		if err != nil {
			tabs[index].HTML = fmt.Sprintf(
				"<p class='empty'>无法读取 %s：%s</p>", html.EscapeString(path), html.EscapeString(err.Error()))
			continue
		}
		var document yaml.Node
		if err := yaml.Unmarshal(content, &document); err != nil {
			tabs[index].HTML = fmt.Sprintf(
				"<p class='empty'>解析失败：%s</p>", html.EscapeString(err.Error()))
			continue
		}
		tabs[index].HTML = yamlNodeToHTML(&document, 0)
		tabs[index].Raw = string(content)
	}
	return tabs
}

// ShowSettings 由菜单「设置 → 参数设定」调用：请前端打开参数设定窗口。
func (app *App) ShowSettings() {
	if app.context == nil {
		return
	}
	runtime.EventsEmit(app.context, "show-settings", nil)
}

// yamlNodeToHTML 把 YAML 节点渲染成**树状展开结构**（可折叠、按类型着色）：
//
//	<details class="tree-node" open>
//	  <summary class="tree-row"><span class="tree-key">excel</span>
//	    <span class="tree-meta">{5}</span></summary>
//	  <div class="tree-children">…子节点…</div>
//	</details>
//
// 用原生 <details>/<summary> 而不是自己写 JS：折叠/展开由浏览器负责，键盘也能操作，
// 缩进靠嵌套的 .tree-children 自然累加，深度没有上限。
// 容器节点标注子项数量（映射 {n}、列表 [n]），标量按类型着色（string/number/bool/null）。
func yamlNodeToHTML(node *yaml.Node, depth int) string {
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) == 0 {
			return ""
		}
		return yamlNodeToHTML(node.Content[0], depth)
	case yaml.MappingNode:
		var builder strings.Builder
		for index := 0; index+1 < len(node.Content); index += 2 {
			builder.WriteString(yamlEntryToHTML(node.Content[index].Value, node.Content[index+1], depth))
		}
		return builder.String()
	case yaml.SequenceNode:
		var builder strings.Builder
		for index, item := range node.Content {
			builder.WriteString(yamlEntryToHTML(strconv.Itoa(index), item, depth))
		}
		return builder.String()
	default:
		return treeValueRow("", node, depth)
	}
}

// yamlEntryToHTML 渲染一个"键/下标 + 值"的条目：容器变成可折叠节点，标量变成一行。
func yamlEntryToHTML(label string, value *yaml.Node, depth int) string {
	key := html.EscapeString(label)
	switch value.Kind {
	case yaml.MappingNode, yaml.SequenceNode:
		// 映射的 Content 是「键、值」成对存的，子项数要除以 2；列表的 Content 就是元素。
		annotation := "{" + strconv.Itoa(len(value.Content)/2) + "}"
		child := ""
		if value.Kind == yaml.SequenceNode {
			annotation = "[" + strconv.Itoa(len(value.Content)) + "]"
		}
		child = yamlNodeToHTML(value, depth+1)
		if strings.TrimSpace(child) == "" {
			kind := "object"
			if value.Kind == yaml.SequenceNode {
				kind = "array"
			}
			child = fmt.Sprintf("<div class=\"tree-empty\">（空 %s）</div>\n", kind)
		}
		// 前两层默认展开：打开参数设定就能看到顶层键与来源/字段列表的轮廓，
		// 再深的内容点开看 —— 一千多行的配置不至于一上来铺满整屏。
		open := ""
		if depth < 2 {
			open = " open"
		}
		return fmt.Sprintf(
			"<details class=\"tree-node\" data-depth=\"%d\"%s>"+
				"<summary class=\"tree-row\"><span class=\"tree-key\">%s</span>"+
				"<span class=\"tree-meta\">%s</span></summary>"+
				"<div class=\"tree-children\">\n%s</div></details>\n",
			depth, open, key, annotation, child)
	default:
		return treeValueRow(label, value, depth)
	}
}

// treeValueRow 渲染一行标量：键 + 冒号 + 按类型着色的值。
func treeValueRow(label string, value *yaml.Node, depth int) string {
	key := html.EscapeString(label)
	kind, text := scalarKind(value)
	return fmt.Sprintf(
		"<div class=\"tree-row tree-leaf\" data-depth=\"%d\">"+
			"<span class=\"tree-key\">%s</span><span class=\"tree-sep\">:</span>"+
			"<span class=\"tree-value type-%s\">%s</span></div>\n",
		depth, key, kind, html.EscapeString(text))
}

// scalarKind 判定标量类型（决定着色）并给出显示文本。
// 判定用 YAML 自己的 tag：配置里写 true / 1 / null 的写法要分别显示成布尔 / 数字 / 空。
func scalarKind(value *yaml.Node) (string, string) {
	tag := strings.TrimPrefix(value.Tag, "!!")
	switch tag {
	case "int", "float":
		return "number", value.Value
	case "bool":
		return "bool", value.Value
	case "null":
		return "null", "null"
	default:
		return "string", value.Value
	}
}

// SettingsSaveResult 是「参数设定」里保存一份配置的结果。
type SettingsSaveResult struct {
	Title   string `json:"title"`
	Message string `json:"message"`
	Failed  bool   `json:"failed"`
	Backup  string `json:"backup"`
}

// SaveConfigurationFile 把「编辑原文」的内容原样写回配置文件。
//
// 只做两件事，不做任何规范化（注释、键序、空行、引号风格都原样保留）：
//  1. 写入前做 YAML 语法校验——解析不过直接拒绝，不落盘；
//  2. 写入后用同一份配置整体跑一次 ConfigLoader().ValidateAll()，
//     任何一项校验不过就**还原**原内容（与程序"配置错误就不启动"的口径一致）。
//
// 写入前会把原内容复制成 <文件名>.bak，便于现场回退。
func (app *App) SaveConfigurationFile(key string, content string) SettingsSaveResult {
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.loader == nil {
		return SettingsSaveResult{Failed: true, Title: "Error", Message: "配置未加载"}
	}
	allowed := map[string]bool{}
	for _, tab := range settingsTabs() {
		allowed[tab.Key] = true
	}
	if !allowed[key] {
		return SettingsSaveResult{Failed: true, Title: "Error",
			Message: "不允许修改的文件：" + key}
	}
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(content), &document); err != nil {
		return SettingsSaveResult{Failed: true, Title: "Error",
			Message: "YAML 语法错误，未保存：\n" + err.Error()}
	}
	path := app.loader.Resolver.ConfigFile(key)
	original, err := os.ReadFile(path)
	if err != nil {
		return SettingsSaveResult{Failed: true, Title: "Error", Message: err.Error()}
	}
	backup := path + ".bak"
	if err := os.WriteFile(backup, original, 0o644); err != nil {
		return SettingsSaveResult{Failed: true, Title: "Error", Message: err.Error()}
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return SettingsSaveResult{Failed: true, Title: "Error", Message: err.Error()}
	}
	// 写盘后整体校验：不通过就还原，绝不让磁盘上留下不可启动的配置
	reloaded, loadErr := config.NewLoader(app.loader.Resolver.ConfigDir)
	if loadErr == nil {
		loadErr = reloaded.ValidateAll()
	}
	if err := loadErr; err != nil {
		_ = os.WriteFile(path, original, 0o644)
		return SettingsSaveResult{Failed: true, Title: "Error",
			Message: "配置校验失败，已还原为保存前的内容：\n" + err.Error(), Backup: backup}
	}
	app.appendLog("Configuration saved: " + key)
	return SettingsSaveResult{
		Title: "Success",
		Message: "配置已保存：" + key + "\n原文件已备份为 " + backup +
			"\n注意：改动在重启程序后生效。",
		Backup: backup,
	}
}
