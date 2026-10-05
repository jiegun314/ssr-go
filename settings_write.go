package main

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/jiegun314/ssr-go/internal/config"
	"gopkg.in/yaml.v3"
)

// 结构化写回：**只改被编辑的那几行**，其余字节（注释、空行、键序、锚点、引号风格）原样不动。
//
// 做法：先把文件解析成 yaml.Node 找到目标标量，拿它的 Line / Column 回到原文那一行，
// 只替换"值"这一段（或对列表插入 / 删除整行），最后逐行拼回去。
// 不做「结构体 → 重新序列化」——那会丢注释、重排键、展开锚点，等于改写整个文件。

// SettingsChange 是一次结构化修改。
type SettingsChange struct {
	Path   []string `json:"path"`             // 目标路径（列表项用下标）
	Action string   `json:"action,omitempty"` // set（默认）| append | remove（后两个只对标量列表）
	Index  int      `json:"index,omitempty"`  // remove 的位置
	Value  string   `json:"value"`            // 新值（字符串形态，按原类型写入）
}

// settingsEdit 是一次已经算好的行级编辑。
type settingsEdit struct {
	line    int    // 1-based
	kind    string // replace | insertAfter | remove
	text    string // replace / insertAfter 用的新行文本
	endLine int    // remove 用（含）
}

// applySettingsEdits 从后往前应用编辑，保证行号不漂移。
func applySettingsEdits(lines []string, edits []settingsEdit) ([]string, error) {
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].line > edits[j].line })
	for _, edit := range edits {
		if edit.line < 1 || edit.line > len(lines) {
			return nil, fmt.Errorf("行号越界：%d", edit.line)
		}
		switch edit.kind {
		case "replace":
			lines[edit.line-1] = edit.text
		case "insertAfter":
			lines = append(lines[:edit.line], append([]string{edit.text}, lines[edit.line:]...)...)
		case "remove":
			if edit.endLine < edit.line || edit.endLine > len(lines) {
				return nil, fmt.Errorf("行号越界：%d-%d", edit.line, edit.endLine)
			}
			lines = append(lines[:edit.line-1], lines[edit.endLine:]...)
		default:
			return nil, fmt.Errorf("未知编辑类型：%s", edit.kind)
		}
	}
	return lines, nil
}

// nodeAtPath 按路径找到 YAML 节点（映射按键名、序列按下标）。
func nodeAtPath(root *yaml.Node, path []string) (*yaml.Node, error) {
	current := root
	if current.Kind == yaml.DocumentNode {
		if len(current.Content) == 0 {
			return nil, fmt.Errorf("空文档")
		}
		current = current.Content[0]
	}
	for index, segment := range path {
		switch current.Kind {
		case yaml.MappingNode:
			found := false
			for pair := 0; pair+1 < len(current.Content); pair += 2 {
				if current.Content[pair].Value == segment {
					current = current.Content[pair+1]
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("找不到键：%s", strings.Join(path[:index+1], "."))
			}
		case yaml.SequenceNode:
			position, err := strconv.Atoi(segment)
			if err != nil || position < 0 || position >= len(current.Content) {
				return nil, fmt.Errorf("列表下标越界：%s", strings.Join(path[:index+1], "."))
			}
			current = current.Content[position]
		default:
			return nil, fmt.Errorf("路径过深：%s", strings.Join(path[:index+1], "."))
		}
	}
	return current, nil
}

// scalarSpan 算出一个标量在它那一行里占用的 [start,end)（0-based 字节区间）。
//
// 只处理"单行标量"：块状（| / >）与流式（{...} / [...] 内部）都不在行级编辑范围内，
// 调用方会先用 Style 把它们挡掉。
func scalarSpan(line string, column int, node *yaml.Node) (int, int, error) {
	start := column - 1
	if start < 0 || start > len(line) {
		return 0, 0, fmt.Errorf("列号越界：%d", column)
	}
	if node.Style == yaml.LiteralStyle || node.Style == yaml.FoldedStyle {
		return 0, 0, fmt.Errorf("多行写法不支持行级编辑")
	}
	// 引号形式：扫到配对的收尾引号
	if start < len(line) && (line[start] == '"' || line[start] == '\'') {
		quote := line[start]
		for index := start + 1; index < len(line); index++ {
			if line[index] == '\\' && quote == '"' {
				index++
				continue
			}
			if line[index] == quote {
				return start, index + 1, nil
			}
		}
		return 0, 0, fmt.Errorf("引号没有闭合")
	}
	// 普通标量：扫到行尾或收尾标记（注释、流式逗号/括号）
	for index := start; index < len(line); index++ {
		char := line[index]
		if char == ',' || char == '}' || char == ']' {
			return start, index, nil
		}
		if char == '#' && index > start && (line[index-1] == ' ' || line[index-1] == '\t') {
			return start, index, nil
		}
	}
	return start, len(line), nil
}

// renderScalar 把用户输入渲染成该节点原本风格的一段 YAML 文本。
func renderScalar(node *yaml.Node, kind string, value string) (string, error) {
	switch kind {
	case "number":
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return "", fmt.Errorf("数字不能为空")
		}
		if _, err := strconv.ParseFloat(trimmed, 64); err != nil {
			return "", fmt.Errorf("不是合法数字：%s", trimmed)
		}
		return trimmed, nil
	case "bool":
		trimmed := strings.TrimSpace(value)
		if trimmed != "true" && trimmed != "false" {
			return "", fmt.Errorf("布尔值只能是 true / false")
		}
		return trimmed, nil
	case "null":
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) == "null" {
			return "null", nil
		}
		return "", fmt.Errorf("该值只能是 null（留空）")
	}
	// 字符串：按原风格处理引号；没有风格时交给 yaml 自己决定要不要加引号
	text := value
	switch node.Style {
	case yaml.DoubleQuotedStyle:
		escaped := strings.ReplaceAll(text, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
		escaped = strings.ReplaceAll(escaped, "\n", "\\n")
		return "\"" + escaped + "\"", nil
	case yaml.SingleQuotedStyle:
		return "'" + strings.ReplaceAll(text, "'", "''") + "'", nil
	case yaml.LiteralStyle, yaml.FoldedStyle:
		return "", fmt.Errorf("多行写法不支持行级编辑")
	}
	if text == "" {
		return `""`, nil
	}
	// 用 yaml 编码一个字符串，拿到"需要时自动加引号"的写法
	encoded, err := yaml.Marshal(text)
	if err != nil {
		return "", err
	}
	rendered := strings.TrimRight(string(encoded), "\n")
	if strings.Contains(rendered, "\n") {
		return "", fmt.Errorf("值里不能有换行（请用『编辑原文』）")
	}
	return rendered, nil
}

// planSettingsChange 把一次结构化修改翻译成行级编辑。
func planSettingsChange(raw []byte, document *yaml.Node, policy settingsPolicy, change SettingsChange) (settingsEdit, error) {
	target, err := nodeAtPath(document, change.Path)
	if err != nil {
		return settingsEdit{}, err
	}
	if change.Action == "" || change.Action == "set" {
		if target.Kind == yaml.AliasNode {
			return settingsEdit{}, errSettingsPathNotEditable{
				reason: "引用共享映射「" + target.Value + "」：改请到「共享映射」里改本体（会一起生效）"}
		}
		// 先看策略：只读子树（条件表达式、样本块、生成物…）给出的是"为什么不能改"
		if _, _, editable, reason, _ := settingsControlFor(policy, change.Path, yamlScalarKind(target), ""); !editable {
			if reason == "" {
				reason = "该值不可编辑"
			}
			return settingsEdit{}, errSettingsPathNotEditable{reason: reason}
		}
		if target.Kind != yaml.ScalarNode {
			return settingsEdit{}, fmt.Errorf("%s 不是可编辑的值", strings.Join(change.Path, "."))
		}
		if target.Line <= 0 {
			return settingsEdit{}, fmt.Errorf("找不到原文行号")
		}
		lines := strings.Split(string(raw), "\n")
		if target.Line > len(lines) {
			return settingsEdit{}, fmt.Errorf("行号越界")
		}
		line := lines[target.Line-1]
		start, end, spanErr := scalarSpan(line, target.Column, target)
		if spanErr != nil {
			return settingsEdit{}, spanErr
		}
		rendered, renderErr := renderScalar(target, yamlScalarKind(target), change.Value)
		if renderErr != nil {
			return settingsEdit{}, renderErr
		}
		return settingsEdit{
			line: target.Line, kind: "replace",
			text: line[:start] + rendered + line[end:],
		}, nil
	}

	// 列表增删：只允许标量列表，而且要求列表本身是块状写法
	parentPath := change.Path
	parent, err := nodeAtPath(document, parentPath)
	if err != nil {
		return settingsEdit{}, err
	}
	if parent.Kind != yaml.SequenceNode {
		return settingsEdit{}, fmt.Errorf("%s 不是列表", strings.Join(parentPath, "."))
	}
	if !settingsListEditable(policy, parentPath) {
		return settingsEdit{}, errSettingsPathNotEditable{reason: "该列表不允许增删条目（顺序与结构由程序约定）"}
	}
	if parent.Style == yaml.FlowStyle || len(parent.Content) == 0 {
		return settingsEdit{}, fmt.Errorf("该列表是流式写法或空列表：请用『编辑原文』修改")
	}
	last := parent.Content[len(parent.Content)-1]
	if last.Style == yaml.LiteralStyle || last.Style == yaml.FoldedStyle {
		return settingsEdit{}, fmt.Errorf("列表项是多行写法：请用『编辑原文』修改")
	}
	lines := strings.Split(string(raw), "\n")

	switch change.Action {
	case "append":
		rendered, renderErr := renderScalar(last, yamlScalarKind(last), change.Value)
		if renderErr != nil {
			return settingsEdit{}, renderErr
		}
		if last.Line <= 0 || last.Line > len(lines) {
			return settingsEdit{}, fmt.Errorf("行号越界")
		}
		itemLine := lines[last.Line-1]
		dash := strings.Index(itemLine, "- ")
		if dash < 0 {
			return settingsEdit{}, fmt.Errorf("列表项不是块状写法：请用『编辑原文』修改")
		}
		return settingsEdit{
			line: last.Line, kind: "insertAfter",
			text: itemLine[:dash] + "- " + rendered,
		}, nil
	case "remove":
		if change.Index < 0 || change.Index >= len(parent.Content) {
			return settingsEdit{}, fmt.Errorf("列表下标越界")
		}
		if len(parent.Content) == 1 {
			return settingsEdit{}, fmt.Errorf("列表至少保留一项")
		}
		item := parent.Content[change.Index]
		if item.Line <= 0 {
			return settingsEdit{}, fmt.Errorf("找不到原文行号")
		}
		// 只删单行项：多行项（带子结构）不允许在这里删
		endLine := item.Line
		if next := change.Index + 1; next < len(parent.Content) && parent.Content[next].Line > 0 {
			endLine = parent.Content[next].Line - 1
		} else {
			for endLine+1 <= len(lines) {
				trimmed := strings.TrimSpace(lines[endLine])
				if trimmed == "" || strings.HasPrefix(trimmed, "#") {
					break
				}
				if strings.Contains(lines[endLine], ":") && !strings.HasPrefix(trimmed, "- ") {
					break
				}
				endLine++
			}
		}
		if endLine > item.Line {
			return settingsEdit{}, fmt.Errorf("该列表项跨多行：请用『编辑原文』删除")
		}
		return settingsEdit{line: item.Line, endLine: item.Line, kind: "remove"}, nil
	}
	return settingsEdit{}, fmt.Errorf("未知操作：%s", change.Action)
}

// SaveSettingsValues 应用一批结构化修改：只改被编辑的行，然后走与「编辑原文」同一条
// 安全通道（备份 → 写盘 → 整体校验 → 失败还原），成功后热重载配置。
func (app *App) SaveSettingsValues(key string, changes []SettingsChange) SettingsSaveResult {
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
		return SettingsSaveResult{Failed: true, Title: "错误", Message: "不允许修改的文件：" + key}
	}
	if len(changes) == 0 {
		return SettingsSaveResult{Title: "成功", Message: "没有需要保存的改动。"}
	}
	path := app.loader.Resolver.ConfigFile(key)
	raw, err := os.ReadFile(path)
	if err != nil {
		return SettingsSaveResult{Failed: true, Title: "错误", Message: err.Error()}
	}
	var document yaml.Node
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return SettingsSaveResult{Failed: true, Title: "错误", Message: "YAML 解析失败：" + err.Error()}
	}
	policy := settingsPolicyFor(key)
	edits := make([]settingsEdit, 0, len(changes))
	for _, change := range changes {
		edit, planErr := planSettingsChange(raw, &document, policy, change)
		if planErr != nil {
			return SettingsSaveResult{Failed: true, Title: "无法保存",
				Message: fmt.Sprintf("%s：%s", strings.Join(change.Path, "."), planErr.Error())}
		}
		edits = append(edits, edit)
	}
	// 同一行只允许一次编辑，避免互相覆盖
	seen := map[int]bool{}
	for _, edit := range edits {
		if seen[edit.line] {
			return SettingsSaveResult{Failed: true, Title: "无法保存",
				Message: "同一行出现了多处修改，请分开保存。"}
		}
		seen[edit.line] = true
	}
	lines, err := applySettingsEdits(strings.Split(string(raw), "\n"), edits)
	if err != nil {
		return SettingsSaveResult{Failed: true, Title: "无法保存", Message: err.Error()}
	}
	updated := strings.Join(lines, "\n")
	if updated == string(raw) {
		return SettingsSaveResult{Title: "成功", Message: "没有实际变化。"}
	}
	// 语法自检：改完必须还能解析
	var check yaml.Node
	if err := yaml.Unmarshal([]byte(updated), &check); err != nil {
		return SettingsSaveResult{Failed: true, Title: "无法保存",
			Message: "改完的 YAML 解析不过，未写盘：\n" + err.Error()}
	}
	backup := path + ".bak"
	if err := os.WriteFile(backup, raw, 0o644); err != nil {
		return SettingsSaveResult{Failed: true, Title: "错误", Message: err.Error()}
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return SettingsSaveResult{Failed: true, Title: "错误", Message: err.Error()}
	}
	reloaded, loadErr := config.NewLoader(app.loader.Resolver.ConfigDir)
	if loadErr == nil {
		loadErr = reloaded.ValidateAll()
	}
	if err := loadErr; err != nil {
		_ = os.WriteFile(path, raw, 0o644)
		return SettingsSaveResult{Failed: true, Title: "校验失败",
			Message: "配置校验失败，已还原为保存前的内容：\n" + err.Error(), Backup: backup}
	}
	app.appendLog(LogSuccess, "配置已保存并热重载："+key)
	hot, hotErr := app.wireConfiguration(reloaded)
	if hotErr != nil {
		return SettingsSaveResult{Failed: true, Title: "已保存但重载失败",
			Message: "文件已保存，但热重载失败（重启后生效）：\n" + hotErr.Error(), Backup: backup}
	}
	message := "配置已保存：" + key + "\n原文件已备份为 " + backup
	if hot != "" {
		message += "\n" + hot
	} else {
		message += "\n改动已热重载，立即生效。"
	}
	return SettingsSaveResult{Title: "成功", Message: message, Backup: backup}
}
