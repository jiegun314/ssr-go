package main

import (
	"regexp"
	"strings"
)

// 界面边界的错误文案口径：
//
//   - **内部错误保持英文**（便于检索、对照代码与上游库文档），操作日志里也照原样记；
//   - **给用户看的对话框文案统一走 userMessage**：认识的错误给中文说法，
//     不认识的也用中文起头并把原文附在后面（用户仍可复制原文给开发排查）。
//
// 实现要点：规则必须**整串匹配**（锚定 ^...$）并整串替换 —— 只匹配子串会把英文尾巴留下
// （例如 "database is locked (5) (SQLITE_BUSY)"）。具体规则要排在包装规则之前
// （例如 "sheet ... does not exist" 优先于 "Failed to import Excel file: ..."）。
//
// 配置类错误里保留配置自己的词汇（键名 Header row、类型 number）—— 用户改的就是那份 YAML，
// 回显原文才找得到地方。

// uiErrorRule 是一条翻译规则：整串匹配 → 用模板拼出中文说法（$1$2… 对应捕获组）。
type uiErrorRule struct {
	pattern  *regexp.Regexp
	template string
}

// uiErrorRules 按顺序匹配，第一条整串命中即用。
var uiErrorRules = []uiErrorRule{
	// 选错文件 / 文件没了 / 没权限
	{regexp.MustCompile(`^Invalid file type: (\S+)\. Must be one of (\[[^\]]*\])$`),
		"文件类型不支持：$1。请选择 $2 里的格式（Excel 工作簿）"},
	{regexp.MustCompile(`^Invalid file type: (\S+)$`),
		"文件类型不支持：$1。请选择 Excel 工作簿（.xlsx / .xls）"},
	{regexp.MustCompile(`(?is)^.*(no such file or directory|cannot find the file|The system cannot find the file).*$`),
		"文件不存在或已被移动：请确认文件还在原位置，然后重新选择"},
	{regexp.MustCompile(`(?is)^.*(permission denied|access is denied).*$`),
		"没有权限读写该文件：请关闭占用它的程序（或换一个位置）后重试"},
	// 数据库
	{regexp.MustCompile(`(?is)^.*database (?:table )?is locked.*$`),
		"数据库被占用：可能有另一个程序正在写它，请稍后重试"},
	{regexp.MustCompile(`(?is)^.*(disk I/O error|database disk image is malformed).*$`),
		"数据库文件读写失败：请确认磁盘空间与文件完好（可从 config/.backup 回退配置）"},
	// Excel：先具体（损坏 / 工作表 / 空表），再包装（导入失败）
	{regexp.MustCompile(`(?is)^.*zip: not a valid zip file.*$`),
		"这个文件不是有效的 Excel 工作簿：可能已损坏，或被改过后缀名"},
	{regexp.MustCompile(`(?is)^.*sheet .*(does not exist|not found).*$`),
		"工作簿里找不到需要的工作表：请核对配置里的 sheet_name"},
	{regexp.MustCompile(`(?is)^.*(no rows|empty workbook|row \d+ is empty).*$`),
		"这个工作簿没有可读的数据行：请确认选对了文件与工作表"},
	{regexp.MustCompile(`(?is)^.*Failed to import Excel file.*$`),
		"导入 Excel 失败：文件内容或结构与配置不符（详情见操作日志）"},
	// 导出
	{regexp.MustCompile(`(?is)^.*SameFileError.*$`),
		"源文件与目标文件是同一个文件：请选择另一个保存位置"},
	// 配置校验（回显配置自己的词汇，便于定位）
	{regexp.MustCompile(`^Unsupported ([a-z_]+) for ([^:]+): (.*)$`),
		"配置里 $2 的 $1 取值不支持：$3"},
	{regexp.MustCompile(`^([A-Z][A-Za-z ]+) must be one of ([^:]+): (.*)$`),
		"配置项 $1 只能是 $2：$3"},
	{regexp.MustCompile(`^([A-Z][A-Za-z ]+) must be a ([a-z]+)$`),
		"配置项 $1 的类型不对：需要 $2"},
	// 其它文件操作
	{regexp.MustCompile(`^Failed to (create|remove|write|read|open|copy) (.*)$`),
		"「$2」操作失败：请检查路径与权限后重试"},
}

// userMessage 把内部错误翻成给用户看的文案；nil 返回空串。
func userMessage(err error) string {
	if err == nil {
		return ""
	}
	text := strings.TrimSpace(err.Error())
	if text == "" {
		return ""
	}
	for _, rule := range uiErrorRules {
		location := rule.pattern.FindStringSubmatchIndex(text)
		if location == nil || location[0] != 0 || location[1] != len(text) {
			continue // 只认整串匹配，避免留下英文尾巴
		}
		return string(rule.pattern.ExpandString(nil, rule.template, text, location))
	}
	// 不认识的错误：已经含中文就原样显示，否则中文起头 + 原文
	if containsChinese(text) {
		return text
	}
	return "操作失败：" + text
}

// containsChinese 判断文本里是否已经有中文。
func containsChinese(text string) bool {
	for _, r := range text {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}
