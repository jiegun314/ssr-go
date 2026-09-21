package main

import (
	"runtime"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/menu"
)

// TestTheMenuOnlyKeepsTheWorkingEntries 固定菜单结构：
// 「文件 → 退出」在最左（macOS 上系统应用菜单在它左边，这是系统固定的位置），
// 然后是「设置 → 参数设定」「工具 → 打开目录 / 备份数据库」「关于 → 关于」；
// macOS 额外有系统应用菜单与编辑菜单。
// 回归点：「文件」里只保留「退出」——曾经那个「打开」是按文件名猜来源的启发式路径
// （逐个来源试导入），界面里每个来源都有自己的导入按钮，既多余又容易导错来源。
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

	// 顺序：文件在最左（macOS 上系统应用菜单固定占据它左边那一格）
	want := []string{"文件", "设置", "工具", "关于"}
	if len(labels) != len(want) {
		t.Fatalf("菜单项 = %v；want %v", labels, want)
	}
	for index, label := range want {
		if labels[index] != label {
			t.Errorf("第 %d 个菜单 = %q；want %q", index+1, labels[index], label)
		}
	}
	expectedSubmenus := map[string][]string{
		"文件": {"退出"},
		"设置": {"参数设定"},
		"工具": {"打开配置目录", "打开导出目录", "打开数据目录", "", "备份数据库…"},
		"关于": {"关于"},
	}
	for label, wantItems := range expectedSubmenus {
		gotItems := menuLabels(submenus[label])
		if len(gotItems) != len(wantItems) {
			t.Errorf("「%s」下的菜单项 = %v；want %v", label, gotItems, wantItems)
			continue
		}
		for index, item := range wantItems {
			if gotItems[index] != item {
				t.Errorf("「%s」第 %d 项 = %q；want %q", label, index+1, gotItems[index], item)
			}
		}
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
