package main

import (
	"strconv"
	"strings"

	"github.com/jiegun314/ssr-go/internal/config"
	"gopkg.in/yaml.v3"
)

// 参数设定的「策略层」：哪个路径能改、用什么控件、哪些只能看。
//
// 设计口径（与用户确认的方案 B）：
//   - 控件由 YAML 自己的 tag 推断（字符串→输入框、数字→数字框、布尔→开关、null→可留空），
//     再叠加下面这几张规则表；
//   - 只读白名单挡住"改了会出事"的键（DB 字段名、样本工厂专用块、生成物、条件表达式）；
//   - 列表：列（sources.*.columns）**不允许增删与排序**（顺序即暂存表列顺序），
//     标量列表（mandatory_options / empty_values / identity_fields 等）可以增删条目；
//   - 注释原样带到前端显示（HeadComment / LineComment / FootComment 一起取）。
//
// 注意：这一层只影响"界面怎么渲染、允许改哪些路径"，不改 config 包的读取逻辑，
// 也不改任何 YAML 文件的既有内容。

// settingsRule 是一条路径规则；match 里的 "*" 匹配任意一段路径。
type settingsRule struct {
	match    string
	readOnly bool
	reason   string
	control  string // 覆盖推断出来的控件（input | number | switch | select | textarea）
	options  []string
	label    string
	note     string
	min      int
	max      int
	hasRange bool
}

// settingsPolicy 是一份配置文件的策略：整份只读、按路径的规则、按路径后缀的标签。
type settingsPolicy struct {
	fileReadOnly string
	rules        []settingsRule
	// 列表是否允许增删条目；键是列表自身的路径模式。
	listEdit map[string]bool
}

// errSettingsPathNotEditable 是写回时的拒绝原因（前端直接把它显示出来）。
type errSettingsPathNotEditable struct{ reason string }

func (err errSettingsPathNotEditable) Error() string { return err.reason }

// matchSettingsPath 用 "*" 通配匹配路径模式。
func matchSettingsPath(pattern string, path []string) bool {
	parts := strings.Split(pattern, ".")
	if len(parts) != len(path) {
		return false
	}
	for index, part := range parts {
		if part == "*" {
			continue
		}
		if part != path[index] {
			return false
		}
	}
	return true
}

// pathMatchesPrefix 判断路径是否落在某个模式之下（模式本身或它的子路径）。
func pathMatchesPrefix(pattern string, path []string) bool {
	parts := strings.Split(pattern, ".")
	if len(path) < len(parts) {
		return false
	}
	return matchSettingsPath(pattern, path[:len(parts)])
}

func settingsPathKey(path []string) string { return strings.Join(path, ".") }

// settingsPolicyFor 返回一份配置文件的策略。
func settingsPolicyFor(fileKey string) settingsPolicy {
	switch fileKey {
	case config.LogColumnsFile:
		return settingsPolicy{
			fileReadOnly: "生成物：由导出模板表头生成，请勿手改（要改请改导出模板后重新生成）",
		}

	case config.SettingFile:
		return settingsPolicy{
			rules: []settingsRule{
				{match: "APP_VERSION", note: "回退版本号：只有既没有构建期模块、又没有 git 时才用它"},
				{match: "database.path", label: "数据库文件", note: "相对路径相对程序目录；改了要重启才切换连接"},
				{match: "folder.export", label: "导出目录"},
				{match: "export_template.path", label: "导出模板文件"},
				{match: "export_template.sheet_name", label: "工作表名"},
				{match: "export_template.header_row", label: "表头行", control: "number", min: 1, max: 100000, hasRange: true},
				{match: "export_template.data_start_row", label: "数据起始行", control: "number", min: 1, max: 100000, hasRange: true},
				{match: "user_defined_data.social_credit_code", label: "统一社会信用代码"},
				{match: "user_defined_data.data_scope", label: "数据范围"},
				{match: "tables.*.name", label: "表名", note: "改表名不会迁移已有数据：旧表会留在库里"},
				{match: "tables.*.replace_on_import", label: "导入时重建"},
				{match: "tables.*.preserve_on_cleanup", label: "清理时保留"},
				{match: "tables.operation_log_time_column", label: "日志时间列"},
				{match: "startup.cleanup_on_startup", label: "启动时清空导入数据"},
			},
		}

	case config.ImportMappingFile:
		policy := settingsPolicy{
			rules: []settingsRule{
				{match: "version", readOnly: true, reason: "版本号由程序维护"},
				// 决定暂存表列名的键：改了会和已存在的表对不上
				{match: "sources.*.columns.*.db_field", readOnly: true,
					reason: "决定暂存表的列名：改动会让已有数据表对不上，请用『编辑原文』并按需重建表"},
				// 只给样本工厂用的块
				{match: "sources.*.columns.*.sample", readOnly: true,
					reason: "只给样本工厂（ssr-core gensample）使用，导入 / 整合 / 导出都不读"},
				{match: "sources.*.columns.*.sample.*", readOnly: true,
					reason: "只给样本工厂（ssr-core gensample）使用，导入 / 整合 / 导出都不读"},
				// 条件表达式：v1 只读展示
				{match: "sources.*.columns.*.required_when", readOnly: true,
					reason: "条件必填表达式：v1 只读展示（改动请用『编辑原文』）"},
				{match: "sources.*.columns.*.required_when.*", readOnly: true,
					reason: "条件必填表达式：v1 只读展示（改动请用『编辑原文』）"},
				// 列本身的字段
				{match: "sources.*.columns.*.chinese_name", label: "列名（Excel 表头精确匹配）"},
				{match: "sources.*.columns.*.mandatory_status", label: "必填状态", control: "select"},
				// 值归一：键是"原始写法"（数据），只允许改映射后的值
				{match: "shared_mappings.*.*", label: "映射为"},
				// 来源块
				{match: "sources.*.chinese_name", label: "来源中文名"},
				{match: "sources.*.display_name", label: "来源显示名"},
				{match: "sources.*.target_table", label: "暂存表名", note: "改表名不会迁移已有数据：旧表会留在库里"},
				{match: "sources.*.replace_on_import", label: "导入时重建"},
				{match: "sources.*.preserve_on_cleanup", label: "清理时保留"},
				{match: "sources.*.excel.sheet_name", label: "工作表名(样本用)"},
				{match: "sources.*.excel.chinese_header_row", label: "中文表头行", control: "number", min: 1, max: 100000, hasRange: true},
				{match: "sources.*.excel.english_header_row", label: "英文表头行", control: "number", min: 1, max: 100000, hasRange: true,
					note: "只给样本工厂用；留空表示 null"},
				{match: "sources.*.excel.required_row", label: "必填标记行", control: "number", min: 1, max: 100000, hasRange: true,
					note: "只给样本工厂用；留空表示 null"},
				{match: "sources.*.excel.data_start_row", label: "数据起始行", control: "number", min: 1, max: 100000, hasRange: true},
				{match: "global_settings.mandatory_options", label: "必填状态可选值"},
				{match: "global_settings.empty_values", label: "条件判断的空值"},
			},
			listEdit: map[string]bool{
				"global_settings.mandatory_options": true,
				"global_settings.empty_values":      true,
			},
		}
		return policy

	case config.ConsolidationMappingFile:
		return settingsPolicy{
			rules: []settingsRule{
				{match: "version", readOnly: true, reason: "版本号由程序维护"},
				{match: "target_dataset.display_name", label: "导出数据集名称"},
				{match: "target_dataset.duplicate_check.identity_fields", label: "重复判定字段"},
				{match: "target_dataset.merge_rules.base_source.source", label: "基准来源", control: "select"},
				{match: "target_dataset.merge_rules.base_source.key", label: "基准来源主键"},
				{match: "target_dataset.merge_rules.result_identity.fields", label: "结果身份字段"},
				{match: "target_dataset.merge_rules.result_identity.separator", label: "身份分隔符"},
				{match: "target_dataset.merge_rules.sources.*.match_type", label: "匹配方式", control: "select",
					options: []string{"lookup"}},
				{match: "target_dataset.merge_rules.sources.*.cardinality", label: "基数", control: "select",
					options: []string{"one_to_one", "one_to_many", "many_to_one"}},
				{match: "target_dataset.merge_rules.sources.*.match", label: "匹配键"},
				{match: "target_dataset.merge_rules.sources.*.expand_by", label: "展开字段"},
				{match: "target_dataset.merge_rules.sources.*.row_identity", label: "行身份字段"},
				{match: "target_dataset.merge_rules.sources.*.no_match.action", label: "无匹配时", control: "select",
					options: config.NoMatchActions},
				{match: "target_dataset.merge_rules.sources.*.no_match.value", label: "无匹配填充值"},
				{match: "target_dataset.merge_rules.sources.*.duplicates.*", label: "重复处理", control: "select",
					options: []string{"deduplicate", "conflict"}},
			},
			listEdit: map[string]bool{
				"target_dataset.duplicate_check.identity_fields":    true,
				"target_dataset.merge_rules.base_source.key":        true,
				"target_dataset.merge_rules.result_identity.fields": true,
				"target_dataset.merge_rules.sources.*.expand_by":    true,
				"target_dataset.merge_rules.sources.*.row_identity": true,
			},
		}
	}
	return settingsPolicy{}
}

// settingsControlFor 推断一个标量用什么控件，并给出可编辑性与说明。
func settingsControlFor(policy settingsPolicy, path []string, kind string, alias string) (control string, options []string, editable bool, reason string, note string) {
	if policy.fileReadOnly != "" {
		return "none", nil, false, policy.fileReadOnly, ""
	}
	// 别名（锚点引用）永远只读：改它要改共享映射本体
	if alias != "" {
		return "none", nil, false, "引用共享映射「" + alias + "」：改请到「共享映射」里改本体（会一起生效）", ""
	}
	for _, rule := range policy.rules {
		if !matchSettingsPath(rule.match, path) {
			continue
		}
		if rule.readOnly {
			return "none", nil, false, rule.reason, ""
		}
		control = rule.control
		options = rule.options
		note = rule.note
	}
	if control == "" {
		switch kind {
		case "number":
			control = "number"
		case "bool":
			control = "switch"
		case "null":
			control = "input" // 留空即 null
		default:
			control = "input"
		}
	}
	return control, options, true, "", note
}

// settingsLabelFor 取路径的中文标签（没有就留空，前端退回显示键名）。
func settingsLabelFor(policy settingsPolicy, path []string) string {
	for _, rule := range policy.rules {
		if rule.label != "" && matchSettingsPath(rule.match, path) {
			return rule.label
		}
	}
	return ""
}

// settingsRangeFor 取数值范围（只对 *_row 这类计数有意义）。
func settingsRangeFor(policy settingsPolicy, path []string) (int, int, bool) {
	for _, rule := range policy.rules {
		if rule.hasRange && matchSettingsPath(rule.match, path) {
			return rule.min, rule.max, true
		}
	}
	return 0, 0, false
}

// settingsListEditable 判断某个列表是否允许增删条目。
func settingsListEditable(policy settingsPolicy, path []string) bool {
	if policy.fileReadOnly != "" {
		return false
	}
	for pattern, allowed := range policy.listEdit {
		if allowed && matchSettingsPath(pattern, path) {
			return true
		}
	}
	return false
}

// settingsComment 汇总一个节点上挂着的所有注释（键上的 + 值上的，已去掉行首 "#"）。
func settingsComment(nodes ...*yaml.Node) string {
	parts := []string{}
	seen := map[string]bool{}
	for _, node := range nodes {
		if node == nil {
			continue
		}
		for _, text := range []string{node.HeadComment, node.LineComment, node.FootComment} {
			text = strings.TrimRight(text, "\n")
			if strings.TrimSpace(text) == "" || seen[text] {
				continue
			}
			seen[text] = true
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

// scalarStyle 把 YAML 的展示风格映射成前端认识的取值（决定能不能行级编辑）。
func scalarStyle(node *yaml.Node) string {
	switch node.Style {
	case yaml.SingleQuotedStyle:
		return "single"
	case yaml.DoubleQuotedStyle:
		return "double"
	case yaml.LiteralStyle:
		return "literal"
	case yaml.FoldedStyle:
		return "folded"
	case yaml.FlowStyle:
		return "flow"
	case 0:
		return "plain"
	default:
		return "other"
	}
}

// yamlScalarKind 是标量在界面上的类型（与既有树视图一致）。
func yamlScalarKind(node *yaml.Node) string {
	switch strings.TrimPrefix(node.Tag, "!!") {
	case "int", "float":
		return "number"
	case "bool":
		return "bool"
	case "null":
		return "null"
	default:
		return "string"
	}
}

// intToText 给列表项下标用。
func intToText(value int) string { return strconv.Itoa(value) }
