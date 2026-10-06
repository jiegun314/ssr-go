package main

// 前端契约测试：展示型弹窗（尺寸、关闭方式、回顾弹窗页脚）。
//
// 公共前言与助手（源码读取、CSS 抽取、对比度）见 layout_helpers_test.go。
import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheDisplayDialogsKeepTheirFixedSize 固定两个展示型窗口（关于 / 附加窗口）：
// 都用 antd Modal（默认自带的右上角 X、遮罩可关）、尺寸固定 323×323。
func TestTheDisplayDialogsKeepTheirFixedSize(t *testing.T) {
	modals := readFrontendSource(t, "src/components/Modals.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	for _, wanted := range []string{
		`className="about-window"`,  // 关于窗口
		`className="hidden-window"`, // 附加窗口
		"maskClosable",              // 点遮罩也能关
		"width={323}",
	} {
		if !strings.Contains(modals, wanted) {
			t.Errorf("Modals.tsx 里缺少展示型窗口的契约：%s", wanted)
		}
	}
	// 关闭按钮用 antd 自带的，不再自绘
	if strings.Contains(modals, "dialog-close") {
		t.Error("展示型窗口应当用 antd 自带的关闭按钮，不再自绘 dialog-close")
	}
	for _, wanted := range []string{
		".about-window {",
		".hidden-window {",
		"width: 323px !important;",
		"height: 323px;",
		"object-fit: cover", // 附加窗口的图片要铺满
		".about-window .ant-modal-close,",
		".hidden-window .ant-modal-close",
	} {
		if !strings.Contains(styles, wanted) {
			t.Errorf("样式里缺少展示型窗口的规则：%s", wanted)
		}
	}
	// 打开窗口时不要把焦点丢给右上角的 X（那样一开窗就像"被选中"）
	if strings.Contains(modals, "autoFocus") {
		t.Error("展示型窗口的关闭按钮不应自动聚焦")
	}
}

// TestTheAboutWindowKeepsTheOriginalIntro 固定关于窗口那段说明文字：
//   - 文案与原版 319×323 的 .ui 逐字一致，& 号就是 & 本身
//     （写成 HTML 实体 &amp; 会原样显示成 "&amp;"，这是之前漏掉的转义残留）；
//   - 原版那个 label 没写 alignment（Qt 默认左对齐），所以说明段左对齐，
//     图标 / 名称 / 版本仍然居中。
func TestTheAboutWindowKeepsTheOriginalIntro(t *testing.T) {
	modals := readFrontendSource(t, "src/components/Modals.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	if strings.Contains(modals, "&amp;") {
		t.Error("前端源码里不该残留 HTML 实体 &amp;（会被原样显示出来）")
	}
	for _, wanted := range []string{
		"SS Ready provides data validation, cleansing,",
		"Developed by",
		"Greater China Supply Chain & RA Team",
		"© 2026 JJMT",
	} {
		if !strings.Contains(modals, wanted) {
			t.Errorf("关于窗口的说明文字缺少原版内容：%q", wanted)
		}
	}
	intro := cssBlock(t, styles, ".about-body .intro {")
	if !strings.Contains(intro, "white-space: pre-wrap;") {
		t.Error("说明段要保留原版的换行")
	}
	if !strings.Contains(intro, "text-align: left;") {
		t.Errorf("说明段应当左对齐，实际样式块：\n%s", intro)
	}
}

// TestAllDialogsUseAntdDefaults 固定弹窗回到 antd 默认形态：
// 五个弹窗（提示 / 回顾 / 参数设定 / 关于 / 附加窗口）全部居中；
// 提示弹窗就是 antd 默认的 Modal（标题 + 自带 X + 右对齐底栏），
// 不再有红底标题条 / 居中按钮 / 手动去掉关闭按钮那套 QDialog 布局。
func TestAllDialogsUseAntdDefaults(t *testing.T) {
	panels := readFrontendSource(t, "src/components/Panels.tsx")
	modals := readFrontendSource(t, "src/components/Modals.tsx")
	styles := readFrontendSource(t, "src/styles.css")
	main := readFrontendSource(t, "src/main.tsx")

	// 1) 五个弹窗全部居中
	if got := strings.Count(modals, "centered"); got < 4 {
		t.Errorf("回顾/参数设定/关于/附加窗口都要居中（centered），实际 %d 处", got)
	}
	if !strings.Contains(panels, "centered") {
		t.Error("提示弹窗也要居中（centered）")
	}
	// 2) 提示弹窗：antd 默认底栏（右对齐）+ 单个「确定」主按钮
	if !strings.Contains(panels, `className="app-message"`) {
		t.Error("提示弹窗要保留 app-message 这个稳定挂点")
	}
	if !strings.Contains(panels, "确定") || !strings.Contains(panels, `type="primary"`) {
		t.Error("提示弹窗要有一个「确定」主按钮")
	}
	// 3) 不许再给弹窗写红底标题条 / 居中底栏的覆盖样式
	for _, leftover := range []string{".app-message ", ".app-modal"} {
		if strings.Contains(styles, leftover) {
			t.Errorf("styles.css 里不该再出现弹窗外观覆盖：%s", leftover)
		}
	}
	// 4) 中文按钮不要自动插空格
	if !strings.Contains(main, "autoInsertSpace: false") {
		t.Error("要关掉 antd 在两个汉字之间插空格的默认行为")
	}
	// 5) 展示型窗口的内容区仍然固定 323 高、内容纵向居中
	for _, wanted := range []string{"justify-content: center", "height: 323px"} {
		if !strings.Contains(styles, wanted) {
			t.Errorf("关于窗口内容区样式缺少：%s", wanted)
		}
	}
	if !strings.Contains(styles, "margin-top: 16px") {
		t.Error("图标与下方文字之间要留出间距")
	}
}

// TestTheMessageDialogShowsStructuredDetails 固定提示弹窗的形态：
// 标题前一个状态图标（成功绿对勾 / 失败红警示），结果摊成 antd 默认的键值表
// （信息类别 / 导入数量），数量带千分位；失败时保留原来的多行原因文本。
func TestTheMessageDialogShowsStructuredDetails(t *testing.T) {
	panels := readFrontendSource(t, "src/components/Panels.tsx")
	app := readFrontendSource(t, "src/App.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	// 1) 数据类型：明细是「标签 + 值」的数组
	for _, wanted := range []string{
		"export type MessageDetail = { label: string; value: string };",
		"details?: MessageDetail[];",
		"failed?: boolean;",
	} {
		if !strings.Contains(panels, wanted) {
			t.Errorf("提示弹窗的数据类型缺少：%s", wanted)
		}
	}
	// 2) 标题里的状态图标（Lucide，不用手写 SVG）
	for _, wanted := range []string{
		"<CheckCircle2",
		"<CircleAlert",
		"message-icon-ok",
		"message-icon-failed",
		`aria-hidden="true"`,
	} {
		if !strings.Contains(panels, wanted) {
			t.Errorf("提示弹窗标题的状态图标缺少：%s", wanted)
		}
	}
	// 3) 明细用 antd 默认的 Descriptions：一列、小尺寸、无冒号
	for _, wanted := range []string{"<Descriptions", "column={1}", `size="small"`, "colon={false}", "<Typography.Text strong>"} {
		if !strings.Contains(panels, wanted) {
			t.Errorf("提示弹窗的键值表缺少：%s", wanted)
		}
	}
	// 4) 导入成功后要给出「信息类别 + 导入数量」，数量带千分位
	for _, wanted := range []string{
		`{ label: "信息类别", value: category }`,
		"  SOURCES,", // 从 bridge 引入来源表，好把 key 翻成中文类别名
		".find((item) => item.key === source)",
		`toLocaleString("zh-CN")`,
		"failed: result.failed",
	} {
		if !strings.Contains(app, wanted) {
			t.Errorf("导入结果的组装缺少：%s", wanted)
		}
	}
	// 5) 图标颜色走主题：成功用与日志「成功」同一个绿，失败用危险红
	ok := cssRule(t, styles, ".message-icon-ok {")
	if !strings.Contains(ok, "color: #389e0d;") {
		t.Errorf("成功图标应当是状态绿，实际块：\n%s", ok)
	}
	failed := cssRule(t, styles, ".message-icon-failed {")
	if !strings.Contains(failed, "color: #cf1322;") {
		t.Errorf("失败图标应当是危险红，实际块：\n%s", failed)
	}
	label := cssRule(t, styles, ".message-details .ant-descriptions-item-label {")
	if !strings.Contains(label, "color: rgba(0, 0, 0, 0.45);") {
		t.Errorf("键值表的标签应当是次级灰，实际块：\n%s", label)
	}
}

// TestTheReviewModalKeepsTheFooterLayout 固定回顾窗口的底栏：
// 导出在最左、翻页居中、关闭在最右（同一行），每页 100 行。
func TestTheReviewModalKeepsTheFooterLayout(t *testing.T) {
	modals := readFrontendSource(t, "src/components/Modals.tsx")
	if !strings.Contains(modals, `justify="center"`) {
		t.Error("翻页组件应当在底栏居中")
	}
	if !strings.Contains(modals, "<Pagination") || !strings.Contains(modals, "pageSize={result?.pageSize ?? 100}") {
		t.Error("回顾窗口要分页，默认每页 100 行")
	}
	if !strings.Contains(modals, "导出") || !strings.Contains(modals, "关闭") {
		t.Error("底栏要有导出与关闭按钮")
	}
}

// TestTheReviewModalGrowsWithTheWindowKeepingTheGaps 固定回顾窗口（数据回顾 / 记录导出）的尺寸口径：
// 窗口变大时弹窗跟着变大，四周留白始终等于**最小窗口**下的那一份，不再把宽度卡在 1100。
//
// 留白来自最小窗口的实测：主窗口最窄 969、弹窗宽 80vw = 775，两侧各 97；
// 弹窗高 131 + 56vh = 556（WebView 内容区最小高度就是 760），上下各 102。
func TestTheReviewModalGrowsWithTheWindowKeepingTheGaps(t *testing.T) {
	modals := readFrontendSource(t, "src/components/Modals.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	if !strings.Contains(modals, "const REVIEW_GAP_X = 97;") || !strings.Contains(modals, "const REVIEW_GAP_Y = 102;") {
		t.Error("留白常量应当写死成最小窗口下的实测值（97 / 102）")
	}
	// 宽度必须走 width 属性：antd 的 width 是后写的内联样式，会盖掉 style.width
	if !strings.Contains(modals, "width={`calc(100vw - ${REVIEW_GAP_X * 2}px)`}") {
		t.Error("弹窗宽度应当是「窗口宽度 - 两侧留白」，并走 antd 的 width 属性")
	}
	if !strings.Contains(modals, "style={{ height: `calc(100vh - ${REVIEW_GAP_Y * 2}px)` }}") {
		t.Error("弹窗高度应当是「窗口高度 - 上下留白」")
	}
	if strings.Contains(modals, "min(80vw, 1100px)") {
		t.Error("宽度不该再封顶在 1100（窗口变大时弹窗要跟着变大）")
	}
	if strings.Contains(modals, `maxHeight: "56vh"`) {
		t.Error("表格高度不该再用 56vh 封顶（高度改由弹窗的弹性链分配）")
	}
	// 内部那条竖直弹性链：表格吃掉多出来的高度并自己滚动
	for _, wanted := range []string{
		".review-modal > div {", // antd 夹在中间的容器要先撑满
		"height: 100%;",
		".review-modal .ant-modal-content {",
		".review-modal .ant-modal-body {",
		"flex: 1 1 auto;",
		"min-height: 0;",
		".review-modal .tblwrap {",
	} {
		if !strings.Contains(styles, wanted) {
			t.Errorf("回顾窗口的弹性链样式缺少：%s", wanted)
		}
	}
}

// TestTheHiddenWindowIsNotAdvertisedInTheFrontend 固定「不提示」的口径：
// 前端源码与界面文件里都不能出现说明附加窗口的文字或命名（它们会随界面打进程序）。
func TestTheHiddenWindowIsNotAdvertisedInTheFrontend(t *testing.T) {
	text := frontendSourceText(t)
	for _, hint := range []string{"彩蛋", "连点", "easter", "EASTER", "egg", "Egg", "EGG"} {
		if strings.Contains(text, hint) {
			t.Errorf("前端源码里出现了会提示附加窗口的文字：%q", hint)
		}
	}
	// 构建产物同样不许出现（前端资源是内嵌进二进制的）
	raw, err := os.ReadFile(filepath.Join("frontend", "dist", "index.html"))
	if err != nil {
		t.Fatalf("读构建产物失败：%v", err)
	}
	for _, hint := range []string{"彩蛋", "连点", "easter_egg"} {
		if strings.Contains(string(raw), hint) {
			t.Errorf("构建产物里出现了提示字样：%q", hint)
		}
	}
}

// TestTheHiddenWindowSoundShipsInsideTheApp 固定附加窗口声音的落地方式：
// 音频放在 frontend/public（构建时复制进 dist，再由 //go:embed 编进二进制），
// 不能放在会被整目录拷进发布包的 resource/；窗口打开时循环播放、关闭时立刻停。
func TestTheHiddenWindowSoundShipsInsideTheApp(t *testing.T) {
	app := readFrontendSource(t, "src/App.tsx")

	if stray, _ := filepath.Glob(filepath.Join("resource", "*.mp3")); len(stray) > 0 {
		t.Errorf("音频不要放在会被整目录拷进发布包的 resource/：%v", stray)
	}
	if !strings.Contains(app, `src="puppy-voice.mp3"`) || !strings.Contains(app, "loop") {
		t.Error("App.tsx 里缺少循环播放的音频元素")
	}
	for _, wanted := range []string{
		"voice.play()",  // 打开时播放（在点击回调里，属用户手势）
		"voice.pause()", // 关闭时立刻停
		"voice.currentTime = 0",
	} {
		if !strings.Contains(app, wanted) {
			t.Errorf("App.tsx 里缺少声音控制：%s", wanted)
		}
	}
}
