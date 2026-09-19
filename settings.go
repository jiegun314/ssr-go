package main

import (
	"fmt"
	"html"
	"os"
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

// yamlNodeToHTML 把 YAML 节点渲染成 Markdown 风格的结构化 HTML：
// 映射与列表的键变成标题，标量变成「键 / 值」行或条目。
//
// 每个带层级类的元素都会带上本层的缩进基准 --md-indent（见 indentStyle）：
// CSS 只负责消费这个变量，所以缩进没有深度上限 —— 之前用 md-d0…md-d5 六个
// 固定类，深度 ≥6 的元素取不到基准值，会退回 0 并顶到最左边。
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
			key := node.Content[index].Value
			value := node.Content[index+1]
			if value.Kind == yaml.MappingNode || value.Kind == yaml.SequenceNode {
				level := depth + 3
				if level > 6 {
					level = 6
				}
				fmt.Fprintf(&builder, "<h%d class=\"md-h md-d%d\"%s>%s</h%d>\n",
					level, depth, indentStyle(depth), html.EscapeString(key), level)
				builder.WriteString(yamlNodeToHTML(value, depth+1))
				continue
			}
			fmt.Fprintf(&builder,
				"<div class=\"md-kv md-d%d\"%s><span class=\"md-k\">%s</span>"+
					"<span class=\"md-v\">%s</span></div>\n",
				depth, indentStyle(depth), html.EscapeString(key), html.EscapeString(value.Value))
		}
		return builder.String()
	case yaml.SequenceNode:
		var builder strings.Builder
		fmt.Fprintf(&builder, "<ul class=\"md-list md-d%d\"%s>\n", depth, indentStyle(depth))
		for _, item := range node.Content {
			if item.Kind == yaml.MappingNode || item.Kind == yaml.SequenceNode {
				builder.WriteString("<li class=\"md-item-block\">")
				builder.WriteString(yamlNodeToHTML(item, depth+1))
				builder.WriteString("</li>\n")
				continue
			}
			builder.WriteString("<li class=\"md-d" + fmt.Sprint(depth) + "\">" +
				html.EscapeString(item.Value) + "</li>\n")
		}
		builder.WriteString("</ul>\n")
		return builder.String()
	default:
		return "<div class=\"md-kv\"><span class=\"md-v\">" +
			html.EscapeString(node.Value) + "</span></div>\n"
	}
}

// indentStyle 输出本层的缩进基准：每深入一层多 12px。
// 基准值写在元素上（而不是靠 CSS 里 md-d0…md-d5 的固定清单），
// 这样任意深度的配置都能正确缩进。
func indentStyle(depth int) string {
	if depth < 0 {
		depth = 0
	}
	return fmt.Sprintf(" style=\"--md-indent:%dpx\"", depth*12)
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
