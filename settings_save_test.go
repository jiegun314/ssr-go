package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/config"
)

// newSettingsApp 准备一个带配置副本的 App（只设置 loader，不启动数据库）。
func newSettingsApp(t *testing.T) (*App, string) {
	t.Helper()
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, "config")
	copyDirectoryForTest(t, "config", configDir)
	loader, err := config.NewLoader(configDir)
	if err != nil {
		t.Fatalf("读配置失败：%v", err)
	}
	app := NewApp(nil)
	app.loader = loader
	return app, configDir
}

func TestSaveConfigurationFileWritesRawContentAndKeepsABackup(t *testing.T) {
	app, configDir := newSettingsApp(t)
	path := filepath.Join(configDir, config.SettingFile)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读原文件失败：%v", err)
	}
	// 只改版本回退值这一行，保留其余内容与注释（真实场景就是这种小改动）
	edited := strings.Replace(string(original), `APP_VERSION: "0.2.1"`, `APP_VERSION: "0.2.9"`, 1)
	if edited == string(original) {
		t.Fatal("测试预期 APP_VERSION 行存在")
	}

	result := app.SaveConfigurationFile(config.SettingFile, edited)

	if result.Failed {
		t.Fatalf("保存失败：%s", result.Message)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读回失败：%v", err)
	}
	if string(saved) != edited {
		t.Error("写回的内容应当与提交的原文逐字一致（不做任何规范化）")
	}
	if !strings.Contains(string(saved), "# settings.yaml") {
		t.Error("注释应当保留")
	}
	backup, err := os.ReadFile(result.Backup)
	if err != nil {
		t.Fatalf("备份不存在：%v", err)
	}
	if string(backup) != string(original) {
		t.Error("备份应当是保存前的内容")
	}
}

func TestSaveConfigurationFileRejectsInvalidYAMLWithoutWriting(t *testing.T) {
	app, configDir := newSettingsApp(t)
	path := filepath.Join(configDir, config.SettingFile)
	original, _ := os.ReadFile(path)

	result := app.SaveConfigurationFile(config.SettingFile, "APP_VERSION: [unclosed\n")

	if !result.Failed || !strings.Contains(result.Message, "YAML") {
		t.Fatalf("语法错误应当被拒绝，得到：%+v", result)
	}
	saved, _ := os.ReadFile(path)
	if string(saved) != string(original) {
		t.Error("语法错误时不应改动文件")
	}
}

func TestSaveConfigurationFileRollsBackWhenValidationFails(t *testing.T) {
	app, configDir := newSettingsApp(t)
	path := filepath.Join(configDir, config.SettingFile)
	original, _ := os.ReadFile(path)
	// 语法合法但配置非法：cleanup_on_startup 必须是布尔
	edited := regexp.MustCompile(`(?m)^\s*cleanup_on_startup:\s*(true|false)\s*$`).
		ReplaceAllString(string(original), "  cleanup_on_startup: \"yes\"")
	if edited == string(original) {
		t.Fatal("测试预期 cleanup_on_startup 键存在")
	}

	result := app.SaveConfigurationFile(config.SettingFile, edited)

	if !result.Failed || !strings.Contains(result.Message, "已还原") {
		t.Fatalf("校验失败应当回滚，得到：%+v", result)
	}
	saved, _ := os.ReadFile(path)
	if string(saved) != string(original) {
		t.Error("校验失败后文件内容应当与保存前一致")
	}
}

func TestSaveConfigurationFileRejectsUnknownFiles(t *testing.T) {
	app, _ := newSettingsApp(t)

	result := app.SaveConfigurationFile("../../etc/passwd", "x")

	if !result.Failed || !strings.Contains(result.Message, "不允许修改") {
		t.Fatalf("应只允许四份配置文件，得到：%+v", result)
	}
}
