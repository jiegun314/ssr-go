package app

// 共享盘地址必须在「参数设定」里能改（用户明确要求）。
// 没有 workflow.yaml 时给模板，保存即创建；模板里 share.root 留空 = 本地模式。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/config"
)

func TestShareSettingsTabEditsTheShareRoot(t *testing.T) {
	app := newShareApp(t, "")
	var shareTab SettingsTab
	for _, tab := range app.ConfigurationDocument() {
		if tab.Key == config.WorkflowFile {
			shareTab = tab
		}
	}
	if shareTab.Key == "" {
		t.Fatal("参数设定里应当有「共享盘」页签")
	}
	if !strings.Contains(shareTab.Raw, "share:") || !strings.Contains(shareTab.Raw, "root:") {
		t.Fatalf("模板里应当有 share.root：%q", shareTab.Raw)
	}
	if !strings.Contains(shareTab.Note, "本地模式") {
		t.Errorf("说明里应当提示当前是本地模式：%q", shareTab.Note)
	}

	// 填一个本地目录保存：文件被创建，且配置校验通过
	root := filepath.Join(t.TempDir(), "share")
	content := strings.Replace(shareTab.Raw, "  root: ''", "  root: '"+root+"'", 1)
	if saved := app.SaveConfigurationFile(config.WorkflowFile, content); saved.Failed {
		t.Fatalf("保存共享盘配置失败：%+v", saved)
	}
	path := filepath.Join(app.loader.Resolver.ConfigDir, config.WorkflowFile)
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("保存后应当生成 workflow.yaml：%v", err)
	}
	if !strings.Contains(string(written), root) {
		t.Errorf("保存的 share.root 不对：%s", written)
	}
	// 相对路径必须被校验拦下并还原
	bad := strings.Replace(shareTab.Raw, "  root: ''", "  root: 'relative/share'", 1)
	if saved := app.SaveConfigurationFile(config.WorkflowFile, bad); !saved.Failed {
		t.Fatal("相对路径的 share.root 应当被拒绝")
	}
	restored, _ := os.ReadFile(path)
	if strings.Contains(string(restored), "relative/share") {
		t.Error("校验失败后应当还原为保存前的内容")
	}
}
