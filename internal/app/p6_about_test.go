package app

// 关于窗口的数据面：版本、数据/参数目录、数据库、运行环境与技术栈都要有值。

import (
	"strings"
	"testing"
)

func TestAboutExposesPathsRuntimeAndTechStack(t *testing.T) {
	app := newShareApp(t, "")
	about := app.About()
	if about.Name == "" || about.Version == "" || about.ChineseName == "" || about.Tagline == "" {
		t.Fatalf("关于的基本信息不完整：%+v", about)
	}
	if about.ConfigDir == "" || about.DataDir == "" {
		t.Errorf("配置目录/数据目录应当有值：%+v", about)
	}
	if !strings.Contains(about.Runtime, "go") || !strings.Contains(about.Runtime, "/") {
		t.Errorf("运行环境应当形如 go1.x · OS/ARCH：%q", about.Runtime)
	}
	if len(about.TechStack) < 2 {
		t.Errorf("技术栈至少要有后端与前端两行：%v", about.TechStack)
	}
	if about.Database == "" {
		// 未连接数据库的实例允许为空，但 newShareApp 已经连过库
		t.Errorf("数据库路径应当有值：%+v", about)
	}
}
