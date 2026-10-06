package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// logLinePattern 是运行日志的**行契约**：`时间 [级别] 消息`。
//
// 这一串必须与 frontend/src/log.ts 里的正则逐字一致 —— 界面按它把每条日志拆成
// 「日期列 + 信息列」两列；两边任何一处改了格式，这里的测试就会红。
const logLinePattern = `^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})\s+\[(信息|成功|警告|错误)\]\s?([\s\S]*)$`

// TestTheOperationLogLineCarriesTimeLevelAndMessage 固定行格式本身。
func TestTheOperationLogLineCarriesTimeLevelAndMessage(t *testing.T) {
	app := NewApp(nil)
	app.appendLog(LogInfo, "应用已启动")
	app.appendLog(LogSuccess, "暂存表已清理")
	app.appendLog(LogWarning, "配置快照失败：演示")
	app.appendLog(LogError, "导入失败：演示")

	pattern := regexp.MustCompile(logLinePattern)
	lines := strings.Split(app.logText(), "\n")
	if len(lines) != 4 {
		t.Fatalf("日志行数 = %d; want 4：\n%s", len(lines), app.logText())
	}
	for index, line := range lines {
		match := pattern.FindStringSubmatch(line)
		if match == nil {
			t.Fatalf("第 %d 行不符合「时间 [级别] 消息」：%q", index+1, line)
		}
		if match[3] == "" {
			t.Errorf("第 %d 行没有消息正文：%q", index+1, line)
		}
	}
	wantLabels := []string{"信息", "成功", "警告", "错误"}
	for index, label := range wantLabels {
		if !strings.Contains(lines[index], "["+label+"]") {
			t.Errorf("第 %d 行的级别标签应当是 [%s]：%q", index+1, label, lines[index])
		}
	}
}

// TestEveryLogCallDeclaresItsLevel 固定「级别在写日志的地方定下来」：
// 每个 appendLog 调用的第一个参数必须是级别（Log* 常量，或循环里带出来的 *.level），
// 界面靠它自动分类，不能靠前端猜关键词。
func TestEveryLogCallDeclaresItsLevel(t *testing.T) {
	callPattern := regexp.MustCompile(`appendLog\(([A-Za-z0-9_.]+),`)
	levelConstants := map[string]bool{
		"LogInfo": true, "LogSuccess": true, "LogWarning": true, "LogError": true,
	}
	for _, name := range []string{"app.go", "app_methods.go", "settings.go", "menu_actions.go"} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("读 %s 失败：%v", name, err)
		}
		calls := callPattern.FindAllStringSubmatch(string(raw), -1)
		if len(calls) == 0 {
			t.Errorf("%s 里没找到 appendLog 调用", name)
		}
		for _, call := range calls {
			argument := call[1]
			if levelConstants[argument] || strings.HasSuffix(argument, ".level") {
				continue
			}
			t.Errorf("%s 里有不带级别的 appendLog 调用：appendLog(%s, …)", name, argument)
		}
	}
}

// TestTheLogWindowMessagesAreChinese 固定「日志窗口里的系统信息都是中文」：
// 我们自己写的模板不许再出现这些英文原文（第三方库抛出的原始报错仍可能夹在明细里，
// 那属于诊断信息，不在这条契约内）。
func TestTheLogWindowMessagesAreChinese(t *testing.T) {
	files := []string{
		"app.go", "app_methods.go", "settings.go", "menu_actions.go",
		filepath.Join("internal", "store", "source.go"),
		filepath.Join("internal", "appicon", "appicon.go"),
		filepath.Join("internal", "appicon", "appicon_windows.go"),
	}
	legacy := []string{
		"Application started",
		"Configuration error",
		"Database error",
		"Created %s from %s",
		"Configuration snapshot saved",
		"Failed to snapshot configuration",
		"Staging tables",
		"Failed to read the existing row count",
		"Failed to drop table",
		"imported successfully. Rows imported",
		"Import failed",
		"Failed to clean imported data",
		"Imported data cleared successfully",
		"Consolidation failed",
		"Consolidation completed successfully",
		"Missing data",
		"Duplicate data",
		"Changed data",
		"Conflict data",
		"Export failed",
		"Exported file path",
		"exported successfully to",
		"Operation log review completed",
		"Data review completed",
		"Configuration saved",
		"Database backed up",
		"Failed to back up database",
		"Failed to open",
		"opened: ",
		"Application icon loaded from",
		"Failed to load the application icon",
		"taskbar identity",
	}
	for _, name := range files {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("读 %s 失败：%v", name, err)
		}
		source := string(raw)
		for _, english := range legacy {
			if strings.Contains(source, english) {
				t.Errorf("%s 里还留着英文系统信息：%q", name, english)
			}
		}
	}
}

// TestTheFrontendParsesTheSameLogFormat 固定前后端对行格式的理解一致：
// frontend/src/log.ts 里的正则必须与 Go 这边逐字相同，级别标签也是同一组中文。
func TestTheFrontendParsesTheSameLogFormat(t *testing.T) {
	script := readFrontendSource(t, "src/log.ts")

	if !strings.Contains(script, logLinePattern) {
		t.Errorf("frontend/src/log.ts 里的行正则与 Go 的行契约不一致，应当是：\n%s", logLinePattern)
	}
	// 级别标签必须逐条相等（不是"包含"）：Go 写日志、TS 解析日志，标签是唯一桥梁
	for level, label := range logLevelLabels {
		// log.ts 里写作：  info: "信息",
		expected := string(level) + `: "` + label + `",`
		if !strings.Contains(script, expected) {
			t.Errorf("frontend/src/log.ts 的级别标签与 Go 不一致，应当是：%s", expected)
		}
	}
	// 级别色只在 design-tokens.ts 定义一次，CSS 规则用 var() 引用
	tokens := readFrontendSource(t, "src/design-tokens.ts")
	styles := readFrontendSource(t, "src/styles.css")
	for level := range logLevelLabels {
		if !strings.Contains(tokens, string(level)+`: "#`) {
			t.Errorf("design-tokens.ts 里缺少 %s 的级别色", level)
		}
		if !strings.Contains(tokens, `"log-`+string(level)+`": LOG_LEVEL_COLORS.`+string(level)) {
			t.Errorf("design-tokens.ts 的 CSS_TOKENS 里缺少 log-%s 的映射", level)
		}
		for _, selector := range []string{".log-level-", ".log-filter-"} {
			if !strings.Contains(styles, selector+string(level)+" {") {
				t.Errorf("styles.css 里缺少规则 %s%s", selector, level)
			}
		}
	}
	if !strings.Contains(styles, "color: var(--log-info);") {
		t.Error("级别色应当在 styles.css 里用 var(--log-*) 引用，而不是写字面量")
	}
	for _, level := range []LogLevel{LogInfo, LogSuccess, LogWarning, LogError} {
		if !strings.Contains(script, `"`+string(level)+`"`) {
			t.Errorf("frontend/src/log.ts 里缺少级别 %q", level)
		}
	}
	// 兜底分类（行里没有级别标签时按关键词猜）也要在，且四类都有
	for _, keyword := range []string{"失败", "错误", "警告", "成功", "完成"} {
		if !strings.Contains(script, keyword) {
			t.Errorf("frontend/src/log.ts 的兜底分类缺少关键词 %q", keyword)
		}
	}
}
