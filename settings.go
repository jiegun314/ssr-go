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
				fmt.Fprintf(&builder, "<h%d class=\"md-h\">%s</h%d>\n",
					level, html.EscapeString(key), level)
				builder.WriteString(yamlNodeToHTML(value, depth+1))
				continue
			}
			builder.WriteString("<div class=\"md-kv\"><span class=\"md-k\">" +
				html.EscapeString(key) + "</span><span class=\"md-v\">" +
				html.EscapeString(value.Value) + "</span></div>\n")
		}
		return builder.String()
	case yaml.SequenceNode:
		var builder strings.Builder
		builder.WriteString("<ul class=\"md-list\">\n")
		for _, item := range node.Content {
			if item.Kind == yaml.MappingNode || item.Kind == yaml.SequenceNode {
				builder.WriteString("<li class=\"md-item-block\">")
				builder.WriteString(yamlNodeToHTML(item, depth+1))
				builder.WriteString("</li>\n")
				continue
			}
			builder.WriteString("<li>" + html.EscapeString(item.Value) + "</li>\n")
		}
		builder.WriteString("</ul>\n")
		return builder.String()
	default:
		return "<div class=\"md-kv\"><span class=\"md-v\">" +
			html.EscapeString(node.Value) + "</span></div>\n"
	}
}
