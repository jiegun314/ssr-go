package main

import (
	"runtime"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/menu"
)

// TestTheMenuOnlyKeepsTheWorkingEntries 固定菜单结构：
// 删掉没有实际用途的「文件」菜单，只保留「设置 → 参数设定」「工具 → 打开目录 / 备份数据库」
// 与「关于 → 关于」；macOS 额外有系统应用菜单与编辑菜单。
// 回归点：「文件 → 打开」走的是"按文件名猜来源"的启发式路径（逐个来源试导入），
// 界面里每个来源都有自己的导入按钮，这个入口既多余又容易导错来源。
func TestTheMenuOnlyKeepsTheWorkingEntries(t *testing.T) {
	root := buildMenu(NewApp(nil))

	labels := make([]string, 0, len(root.Items))
	submenus := map[string]*menu.Menu{}
	for _, item := range root.Items {
		if item.Role == menu.AppMenuRole {
			continue // macOS 的系统应用菜单（关于/退出/服务等由系统提供）
		}
		if item.Role == menu.EditMenuRole {
			// 编辑菜单只在 macOS 出现，且整菜单都是系统 role（撤销/拷贝/粘贴…）
			if runtime.GOOS != "darwin" {
				t.Errorf("编辑菜单只应出现在 macOS，当前 GOOS=%s", runtime.GOOS)
			}
			continue
		}
		labels = append(labels, item.Label)
		submenus[item.Label] = item.SubMenu
	}

	want := []string{"设置", "工具", "关于"}
	if len(labels) != len(want) {
		t.Fatalf("菜单项 = %v；want %v", labels, want)
	}
	for index, label := range want {
		if labels[index] != label {
			t.Errorf("第 %d 个菜单 = %q；want %q", index+1, labels[index], label)
		}
	}
	if _, exists := submenus["文件"]; exists {
		t.Error("「文件」菜单应已删除")
	}
	if items := menuLabels(submenus["设置"]); len(items) != 1 || items[0] != "参数设定" {
		t.Errorf("「设置」下的菜单项 = %v；want [参数设定]", items)
	}
	tools := menuLabels(submenus["工具"])
	wantTools := []string{"打开配置目录", "打开导出目录", "打开数据目录", "", "备份数据库…"}
	if len(tools) != len(wantTools) {
		t.Fatalf("「工具」下的菜单项 = %v；want %v", tools, wantTools)
	}
	for index, label := range wantTools {
		if tools[index] != label {
			t.Errorf("「工具」第 %d 项 = %q；want %q", index+1, tools[index], label)
		}
	}
	if items := menuLabels(submenus["关于"]); len(items) != 1 || items[0] != "关于" {
		t.Errorf("「关于」下的菜单项 = %v；want [关于]", items)
	}
}

func menuLabels(root *menu.Menu) []string {
	if root == nil {
		return nil
	}
	labels := make([]string, 0, len(root.Items))
	for _, item := range root.Items {
		labels = append(labels, item.Label)
	}
	return labels
}
