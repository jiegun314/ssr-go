package main

// 前端契约测试的公共助手（源码读取、CSS 规则抽取、主题常量、对比度计算）。
//
// 前端是 React + antd + Lucide（源码在 frontend/src，构建产物在 frontend/dist）。
// 这里的契约测试读**源码**而不是打包产物：产物里的类名会被压缩，源码才是可维护的契约面。
// 少数几条（内嵌资源、提示字样）同时检查产物，因为那才是真正发给用户的东西。
//
// 两套口径：
//   - 行为契约（日志拖动、声音、弹窗尺寸、图标语义、导出门禁……）—— 与改版前一致；
//   - 设计契约 —— 现在是 **antd 默认值 + 强生红**：字号 / 圆角 / 间距 / 控件高度都不许再出现
//     PySide6 原型带过来的手工值（4px 小圆角、红底白字标题条、13px 基准字号、36px 日期框……）。

import (
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func readFrontendSource(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("frontend", name))
	if err != nil {
		t.Fatalf("读前端源码 %s 失败：%v", name, err)
	}
	return string(raw)
}

// frontendSourceText 把前端源码全部拼起来，用于"整个前端都不许出现某字样"这类检查。
func frontendSourceText(t *testing.T) string {
	t.Helper()
	var builder strings.Builder
	walk := func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		builder.Write(content)
		builder.WriteString("\n")
		return nil
	}
	if err := filepath.WalkDir(filepath.Join("frontend", "src"), walk); err != nil {
		t.Fatalf("遍历前端源码失败：%v", err)
	}
	for _, name := range []string{"index.html", "package.json", "vite.config.ts"} {
		if content, err := os.ReadFile(filepath.Join("frontend", name)); err == nil {
			builder.Write(content)
		}
	}
	return builder.String()
}

// themeConstant 读 theme.ts 里 `export const NAME = "#RRGGBB"` 这种简单常量（返回小写）。
func themeConstant(t *testing.T, theme, declaration string) string {
	t.Helper()
	pattern := regexp.MustCompile(regexp.QuoteMeta(declaration) + `\s*=\s*"(#[0-9A-Fa-f]{6})"`)
	match := pattern.FindStringSubmatch(theme)
	if match == nil {
		t.Fatalf("没有从 theme.ts 里解析出 %s", declaration)
	}
	return strings.ToLower(match[1])
}

// cssRule 与 cssBlock 相同，但要求选择器出现在行首：
// 否则 ".log-card {" 会命中 ".left > .log-card {" 这种带父选择器的规则。
func cssRule(t *testing.T, source, selector string) string {
	t.Helper()
	return cssBlock(t, source, "\n"+selector)
}

// cssBlock 取出 `selector` 开头那一对花括号之间的内容（只用于读源码里的常量）。
func cssBlock(t *testing.T, source, selector string) string {
	t.Helper()
	start := strings.Index(source, selector)
	if start < 0 {
		t.Fatalf("样式中找不到选择器 %q", selector)
	}
	rest := source[start:]
	end := strings.Index(rest, "}")
	if end < 0 {
		t.Fatalf("选择器 %q 的样式块没有收尾", selector)
	}
	return rest[:end]
}

// themeColors 从 theme.ts 里读出 `export const NAME = {...}` 中的 {名字: #RRGGBB}（全部小写）。
func themeColors(t *testing.T, theme, declaration string) map[string]string {
	t.Helper()
	block := cssBlock(t, theme, declaration)
	pattern := regexp.MustCompile(`(\w+):\s*"(#[0-9A-Fa-f]{6})"`)
	colors := map[string]string{}
	for _, match := range pattern.FindAllStringSubmatch(block, -1) {
		colors[match[1]] = strings.ToLower(match[2])
	}
	if len(colors) == 0 {
		t.Fatalf("没有从 %s 里解析出任何颜色", declaration)
	}
	return colors
}

// contrastOnWhite 算 #RRGGBB 在白底上的 WCAG 对比度。
func contrastOnWhite(t *testing.T, hex string) float64 {
	t.Helper()
	value := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(hex)), "#")
	if len(value) != 6 {
		t.Fatalf("颜色 %q 不是 #RRGGBB", hex)
	}
	luminance := func(part string) float64 {
		number, err := strconv.ParseUint(part, 16, 8)
		if err != nil {
			t.Fatalf("颜色 %q 解析失败：%v", hex, err)
		}
		channel := float64(number) / 255
		if channel <= 0.03928 {
			return channel / 12.92
		}
		return math.Pow((channel+0.055)/1.055, 2.4)
	}
	lum := 0.2126*luminance(value[0:2]) + 0.7152*luminance(value[2:4]) + 0.0722*luminance(value[4:6])
	return 1.05 / (lum + 0.05)
}
