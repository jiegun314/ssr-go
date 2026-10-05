package main

import (
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"gopkg.in/yaml.v3"

	"github.com/jiegun314/ssr-go/internal/config"
)

// SettingsNode 是参数设定树状视图的一个节点：容器（map/seq）带 Children，
// 标量带 Kind 与 Value（前端按类型着色）。数量不落字段，前端数 Children 即可。
//
// 结构化表单用的信息也挂在这个节点上：Path（写回时定位）、Comment（原样展示的注释）、
// Control / Editable / Reason / Note（怎么渲染、能不能改、为什么不能改）。
type SettingsNode struct {
	Key      string         `json:"key"`  // 键名；列表项是下标（"0"、"1"…）
	Kind     string         `json:"kind"` // map | seq | string | number | bool | null
	Value    string         `json:"value,omitempty"`
	Children []SettingsNode `json:"children,omitempty"`

	// 结构化表单
	Path     []string `json:"path,omitempty"`     // 从文件根到这里的键路径（列表项用下标）
	Comment  string   `json:"comment,omitempty"`  // 该节点上挂着的全部注释（原样）
	Control  string   `json:"control,omitempty"`  // input | number | switch | select | none
	Options  []string `json:"options,omitempty"`  // select 的候选值
	Label    string   `json:"label,omitempty"`    // 中文标签（没有就显示键名）
	Note     string   `json:"note,omitempty"`     // 改动提示
	Editable bool     `json:"editable,omitempty"` // 标量：能不能改
	Reason   string   `json:"reason,omitempty"`   // 只读原因
	ListEdit bool     `json:"listEdit,omitempty"` // 列表：能不能增删条目
	Flow     bool     `json:"flow,omitempty"`     // 流式写法（{...} / [...]）→ 不支持行级编辑
	Alias    string   `json:"alias,omitempty"`    // 别名引用（指向的锚点名）
	Style    string   `json:"style,omitempty"`    // plain | single | double | literal | folded | flow
	Min      *int     `json:"min,omitempty"`
	Max      *int     `json:"max,omitempty"`
}

// SettingsTab 是「参数设定」窗口里的一个页签：一份配置文件的标题、路径与结构化内容。
type SettingsTab struct {
	Key   string         `json:"key"`   // 文件名（配置里唯一）
	Title string         `json:"title"` // 页签标题
	Note  string         `json:"note"`  // 页签说明（例如「生成物，请勿手改」）
	Path  string         `json:"path"`  // 实际读取的文件路径
	Tree  []SettingsNode `json:"tree"`  // 结构化内容（前端用 antd Tree 渲染）
	Raw   string         `json:"raw"`   // 文件原文（供"编辑原文"使用）
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

// ConfigurationDocument 读四份 YAML，把每份解析成结构化树，
// 供菜单「设置 → 参数设定」的四个页签显示（前端负责渲染成树状视图）。
func (app *App) ConfigurationDocument() []SettingsTab {
	app.mu.Lock()
	defer app.mu.Unlock()
	tabs := settingsTabs()
	if app.loader == nil {
		for index := range tabs {
			tabs[index].Note = "配置未加载：请确认 config/ 与程序位于同一目录。"
		}
		return tabs
	}
	for index := range tabs {
		path := app.loader.Resolver.ConfigFile(tabs[index].Key)
		tabs[index].Path = path
		content, err := os.ReadFile(path)
		if err != nil {
			tabs[index].Note = fmt.Sprintf("无法读取 %s：%s", path, err.Error())
			continue
		}
		var document yaml.Node
		if err := yaml.Unmarshal(content, &document); err != nil {
			tabs[index].Note = fmt.Sprintf("解析失败：%s", err.Error())
			continue
		}
		tabs[index].Tree = settingsTree(tabs[index].Key, &document)
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

// settingsTree 把一份配置的 YAML 文档转成结构化树（带路径、注释与控件信息）。
func settingsTree(fileKey string, document *yaml.Node) []SettingsNode {
	policy := settingsPolicyFor(fileKey)
	return settingsNodes(policy, document, nil)
}

// settingsNodes 递归构建节点；path 是"从文件根到这里"的键路径。
func settingsNodes(policy settingsPolicy, node *yaml.Node, path []string) []SettingsNode {
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) == 0 {
			return nil
		}
		return settingsNodes(policy, node.Content[0], path)
	case yaml.MappingNode:
		nodes := make([]SettingsNode, 0, len(node.Content)/2)
		for index := 0; index+1 < len(node.Content); index += 2 {
			keyNode, valueNode := node.Content[index], node.Content[index+1]
			nodes = append(nodes, settingsChild(policy, keyNode, valueNode, append(path, keyNode.Value)))
		}
		return nodes
	case yaml.SequenceNode:
		nodes := make([]SettingsNode, 0, len(node.Content))
		for index, item := range node.Content {
			nodes = append(nodes, settingsChild(policy, nil, item, append(path, intToText(index))))
		}
		return nodes
	default:
		return []SettingsNode{settingsScalar(policy, nil, node, path)}
	}
}

// settingsChild 生成一个「键/下标 + 值」的节点。
func settingsChild(policy settingsPolicy, keyNode *yaml.Node, valueNode *yaml.Node, path []string) SettingsNode {
	key := path[len(path)-1]
	comment := settingsComment(keyNode, valueNode)
	switch valueNode.Kind {
	case yaml.MappingNode:
		return SettingsNode{
			Key: key, Kind: "map", Path: path, Comment: comment,
			Label:    settingsLabelFor(policy, path),
			Flow:     valueNode.Style == yaml.FlowStyle,
			Reason:   settingsContainerReason(policy, path),
			Children: settingsNodes(policy, valueNode, path),
		}
	case yaml.SequenceNode:
		return SettingsNode{
			Key: key, Kind: "seq", Path: path, Comment: comment,
			Label:    settingsLabelFor(policy, path),
			Flow:     valueNode.Style == yaml.FlowStyle,
			ListEdit: settingsListEditable(policy, path),
			Reason:   settingsContainerReason(policy, path),
			Children: settingsNodes(policy, valueNode, path),
		}
	case yaml.AliasNode:
		return SettingsNode{
			Key: key, Kind: "string", Path: path, Comment: comment,
			Value: valueNode.Value, Alias: valueNode.Value,
			Control: "none", Editable: false,
			Reason: "引用共享映射「" + valueNode.Value + "」：改请到「共享映射」里改本体（会一起生效）",
			Style:  scalarStyle(valueNode),
		}
	default:
		return settingsScalar(policy, keyNode, valueNode, path)
	}
}

// settingsScalar 是一个标量节点：类型、控件、可编辑性与只读原因都在这里定。
func settingsScalar(policy settingsPolicy, keyNode *yaml.Node, valueNode *yaml.Node, path []string) SettingsNode {
	kind := yamlScalarKind(valueNode)
	control, options, editable, reason, note := settingsControlFor(policy, path, kind, "")
	node := SettingsNode{
		Key: path[len(path)-1], Kind: kind, Value: valueNode.Value, Path: path,
		Comment:  settingsComment(keyNode, valueNode),
		Control:  control,
		Options:  options,
		Label:    settingsLabelFor(policy, path),
		Note:     note,
		Editable: editable,
		Reason:   reason,
		Style:    scalarStyle(valueNode),
	}
	if kind == "null" {
		node.Value = "null"
	}
	if min, max, ok := settingsRangeFor(policy, path); ok {
		node.Min, node.Max = &min, &max
	}
	// 值在原文里是多行写法（| / >）或流式写法时，行级编辑不安全 → 只读
	if style := node.Style; style == "literal" || style == "folded" || style == "flow" {
		node.Editable = false
		node.Control = "none"
		if node.Reason == "" {
			node.Reason = "该值在原文里是多行或流式写法：请用『编辑原文』修改"
		}
	}
	return node
}

// settingsContainerReason 给容器节点一个只读说明（容器本身不可编辑，只有叶子可编辑）。
func settingsContainerReason(policy settingsPolicy, path []string) string {
	if policy.fileReadOnly != "" {
		return policy.fileReadOnly
	}
	for _, rule := range policy.rules {
		if rule.readOnly && pathMatchesPrefix(rule.match, path) {
			return rule.reason
		}
	}
	return ""
}

// SettingsSaveResult 是一次保存回给界面的结果。
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
		return SettingsSaveResult{Failed: true, Title: "错误", Message: "配置未加载"}
	}
	allowed := map[string]bool{}
	for _, tab := range settingsTabs() {
		allowed[tab.Key] = true
	}
	if !allowed[key] {
		return SettingsSaveResult{Failed: true, Title: "错误",
			Message: "不允许修改的文件：" + key}
	}
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(content), &document); err != nil {
		return SettingsSaveResult{Failed: true, Title: "错误",
			Message: "YAML 语法错误，未保存：\n" + err.Error()}
	}
	path := app.loader.Resolver.ConfigFile(key)
	original, err := os.ReadFile(path)
	if err != nil {
		return SettingsSaveResult{Failed: true, Title: "错误", Message: err.Error()}
	}
	backup := path + ".bak"
	if err := os.WriteFile(backup, original, 0o644); err != nil {
		return SettingsSaveResult{Failed: true, Title: "错误", Message: err.Error()}
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return SettingsSaveResult{Failed: true, Title: "错误", Message: err.Error()}
	}
	// 写盘后整体校验：不通过就还原，绝不让磁盘上留下不可启动的配置
	reloaded, loadErr := config.NewLoader(app.loader.Resolver.ConfigDir)
	if loadErr == nil {
		loadErr = reloaded.ValidateAll()
	}
	if err := loadErr; err != nil {
		_ = os.WriteFile(path, original, 0o644)
		return SettingsSaveResult{Failed: true, Title: "错误",
			Message: "配置校验失败，已还原为保存前的内容：\n" + err.Error(), Backup: backup}
	}
	app.appendLog(LogSuccess, "配置已保存："+key)
	return SettingsSaveResult{
		Title: "成功",
		Message: "配置已保存：" + key + "\n原文件已备份为 " + backup +
			"\n注意：改动在重启程序后生效。",
		Backup: backup,
	}
}
