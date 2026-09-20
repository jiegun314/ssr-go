package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/config"
)

// TestMissingConfigFilesAreRestoredFromDefaults 固定升级不覆盖用户配置的做法：
// 发布包只带 config/defaults/*.yaml，正式名文件缺失时由程序从默认文件生成一份。
func TestMissingConfigFilesAreRestoredFromDefaults(t *testing.T) {
	workspace := t.TempDir()
	copyDirectoryForTest(t, "config", filepath.Join(workspace, "config", config.DefaultsDirectory))
	if err := os.RemoveAll(filepath.Join(workspace, "config", config.SettingFile)); err != nil {
		t.Fatal(err)
	}

	loader, err := config.NewLoader(filepath.Join(workspace, "config"))
	if err != nil {
		t.Fatalf("装载配置失败：%v", err)
	}
	if len(loader.Bootstrapped) != len(config.ConfigFileNames) {
		t.Fatalf("补齐的文件 = %v；want 四份都补齐", loader.Bootstrapped)
	}
	created := filepath.Join(workspace, "config", config.SettingFile)
	content, err := os.ReadFile(created)
	if err != nil {
		t.Fatalf("没有生成 %s：%v", config.SettingFile, err)
	}
	expected, err := os.ReadFile(filepath.Join(workspace, "config", config.DefaultsDirectory, config.SettingFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != string(expected) {
		t.Error("生成的配置内容与默认文件不一致")
	}
	// 四份齐全之后必须能整体校验通过（否则等于还原了一份坏的配置）
	if err := loader.ValidateAll(); err != nil {
		t.Errorf("补齐后整体校验失败：%v", err)
	}
	// 第二次装载不应该再重复生成
	again, err := config.NewLoader(filepath.Join(workspace, "config"))
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Bootstrapped) != 0 {
		t.Errorf("第二次装载又生成了 %v", again.Bootstrapped)
	}
}

// TestBrokenConfigFilesAreNeverOverwrittenByDefaults 最关键的一条：
// 文件存在但读不懂时**不能**用默认值覆盖（那等于把用户改坏的内容悄悄换成默认行为），
// 要保持 R24 的口径——报错、不启动。
func TestBrokenConfigFilesAreNeverOverwrittenByDefaults(t *testing.T) {
	workspace := t.TempDir()
	copyDirectoryForTest(t, "config", filepath.Join(workspace, "config"))
	copyDirectoryForTest(t, "config", filepath.Join(workspace, "config", config.DefaultsDirectory))
	broken := filepath.Join(workspace, "config", config.SettingFile)
	original := []byte("APP_NAME: \"用户改坏的配置\"\n  bad indentation: [\n")
	if err := os.WriteFile(broken, original, 0o644); err != nil {
		t.Fatal(err)
	}

	loader, err := config.NewLoader(filepath.Join(workspace, "config"))
	if err != nil {
		t.Fatalf("装载配置失败：%v", err)
	}
	if len(loader.Bootstrapped) != 0 {
		t.Errorf("坏文件不该被当成缺失补掉：%v", loader.Bootstrapped)
	}
	after, err := os.ReadFile(broken)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Error("用户的配置文件被覆盖了")
	}
	if err := loader.ValidateAll(); err == nil {
		t.Error("坏配置必须报错（R24），不能静默启动")
	}
}

// TestMissingConfigWithoutDefaultsStillFails 没有默认文件可补时保持原行为：
// LoadYAML 报"配置找不到"，程序不启动。
func TestMissingConfigWithoutDefaultsStillFails(t *testing.T) {
	workspace := t.TempDir()
	configDir := filepath.Join(workspace, "config")
	copyDirectoryForTest(t, "config", configDir)
	if err := os.Remove(filepath.Join(configDir, config.LogColumnsFile)); err != nil {
		t.Fatal(err)
	}
	loader, err := config.NewLoader(configDir)
	if err != nil {
		t.Fatalf("装载配置失败：%v", err)
	}
	if len(loader.Bootstrapped) != 0 {
		t.Errorf("没有默认文件时不该声称补齐了：%v", loader.Bootstrapped)
	}
	if _, err := loader.LoadLogColumns(); err == nil {
		t.Fatal("缺文件时必须报错")
	} else if !strings.Contains(err.Error(), "Configuration file not found") {
		t.Errorf("错误文案变了：%v", err)
	}
}
