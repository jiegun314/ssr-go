package main

import (
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// 前端已经换成 React + antd + Lucide（源码在 frontend/src，构建产物在 frontend/dist）。
// 这里的契约测试读**源码**而不是打包产物：产物里的类名会被压缩，源码才是可维护的契约面。
// 少数几条（内嵌资源、提示字样）同时检查产物，因为那才是真正发给用户的东西。

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

// TestTheFrontendShipsInsideTheBinary 固定「前端随二进制走」：dist 里有页面、打包后的
// 资源，以及三份静态文件（图标、附加窗口图片、音效）——它们由 public/ 复制而来。
func TestTheFrontendShipsInsideTheBinary(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("frontend", "dist"))
	if err != nil {
		t.Fatalf("读 frontend/dist 失败（先在 frontend/ 里跑 npm run build）：%v", err)
	}
	found := map[string]bool{}
	for _, entry := range entries {
		found[entry.Name()] = true
	}
	for _, wanted := range []string{"index.html", "logo.png", "puppy.png", "puppy-voice.mp3"} {
		if !found[wanted] {
			t.Errorf("frontend/dist 里缺少 %s（构建产物要提交进仓库）", wanted)
		}
	}
	assets, err := os.ReadDir(filepath.Join("frontend", "dist", "assets"))
	if err != nil {
		t.Fatalf("读 frontend/dist/assets 失败：%v", err)
	}
	hasJS, hasCSS := false, false
	for _, entry := range assets {
		if strings.HasSuffix(entry.Name(), ".js") {
			hasJS = true
		}
		if strings.HasSuffix(entry.Name(), ".css") {
			hasCSS = true
		}
	}
	if !hasJS || !hasCSS {
		t.Errorf("dist/assets 里应该有打包后的 js 与 css（js=%v css=%v）", hasJS, hasCSS)
	}
	// 源码里也要有那三份静态文件（public/ 是它们的家）
	for _, wanted := range []string{"logo.png", "puppy.png", "puppy-voice.mp3"} {
		if _, err := os.Stat(filepath.Join("frontend", "public", wanted)); err != nil {
			t.Errorf("frontend/public 里缺少 %s：%v", wanted, err)
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
	dist := frontendSourceText(t)
	_ = dist
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

// TestTheOperationLogCardHasADragHandle 固定操作日志上沿可拖动：
// 分隔条用 row-resize、键盘可用，拖动逻辑按「上限＝左侧间隔最小、下限＝默认高度」夹紧。
func TestTheOperationLogCardHasADragHandle(t *testing.T) {
	panels := readFrontendSource(t, "src/components/Panels.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	for _, wanted := range []string{
		`className="log-resizer"`,
		`role="separator"`,
		`aria-orientation="horizontal"`,
		"onPointerDown={startDrag}",
		"onKeyDown={onKeyDown}",
		"leftMinimumHeight()",
		"document.body.classList.add(\"log-resizing\")",
	} {
		if !strings.Contains(panels, wanted) {
			t.Errorf("Panels.tsx 里缺少拖动逻辑：%s", wanted)
		}
	}
	if !strings.Contains(styles, ".log-resizer {") || !strings.Contains(styles, "cursor: row-resize") {
		t.Error("样式里缺少 log-resizer 的 row-resize 光标")
	}
}

// TestTheOperationLogResizerKeepsTheLeftColumnGap 固定上限口径的来源：
// 上限＝左侧「数据导入 / 记录导出」只剩 .spacer 的最小间隔（12px），下限＝默认高度。
func TestTheOperationLogResizerKeepsTheLeftColumnGap(t *testing.T) {
	panels := readFrontendSource(t, "src/components/Panels.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	if !strings.Contains(styles, ".left > .spacer {") || !strings.Contains(styles, "min-height: 12px") {
		t.Error("左列两个模块之间的最小间隔应为 12px")
	}
	if !strings.Contains(panels, `document.querySelector<HTMLElement>(".columns")`) {
		t.Error("拖动上限应当根据中间列还能让出多少高度来算")
	}
	if !strings.Contains(panels, "flex: \"0 0 auto\"") {
		t.Error("拖动时卡片应改为固定高度（让出的高度全部给中间列）")
	}
}

// TestTheLogExportRowSurvivesWiderPlatformFonts 固定「记录导出」整行可压缩的口径：
// Windows（WebView2）下日期控件固有宽度更大，整行不可压缩就会冒出横向滚动条。
func TestTheLogExportRowSurvivesWiderPlatformFonts(t *testing.T) {
	styles := readFrontendSource(t, "src/styles.css")

	for _, wanted := range []string{
		".range .dates {",      // 日期容器可收缩
		"flex: 0 1 auto;",      // 不参与扩张，但可以被压缩
		".range .ant-picker {", // antd 的日期控件同样要能退让
		"min-width: 100px;",
		"max-width: 100%;",
		".range .ant-btn {", // 回顾按钮不参与收缩
	} {
		if !strings.Contains(styles, wanted) {
			t.Errorf("样式里缺少记录导出整行可压缩的规则：%s", wanted)
		}
	}
}

// TestTheUIKeepsTheProductColorsAndFont 固定配色与字体：
// 主红/标题条红不变，字体用跨平台栈（Windows 上不能退化成 Consolas + 微软雅黑混排）。
func TestTheUIKeepsTheProductColorsAndFont(t *testing.T) {
	theme := readFrontendSource(t, "src/theme.ts")
	styles := readFrontendSource(t, "src/styles.css")

	if !strings.Contains(theme, `export const BRAND_RED = "#B3261E"`) {
		t.Error("主色应保持 #B3261E")
	}
	if !strings.Contains(theme, `export const TITLE_RED = "#D71600"`) {
		t.Error("标题条红应保持 #D71600")
	}
	if !strings.Contains(styles, "background: #d71600") {
		t.Error("标题条要用产品指定的红色（#D71600 底 + 白字）")
	}
	if !strings.Contains(theme, `"Microsoft YaHei"`) || !strings.Contains(theme, `"PingFang SC"`) {
		t.Error("字体栈要跨平台（含微软雅黑/苹方）")
	}
	if strings.Contains(theme, "monospace") || strings.Contains(styles, "monospace") {
		t.Error("界面不该用等宽字体（Windows 上会退化成两种字体混排）")
	}
	// 状态圆点配色与原界面一致
	for _, wanted := range []string{".status-ready", ".status-incomplete", ".status-duplicate", ".status-conflict"} {
		if !strings.Contains(styles, wanted) {
			t.Errorf("结果状态圆点缺少 %s", wanted)
		}
	}
}

// TestTheDisplayDialogsCloseFromTheTopRightIcon 固定两个展示型窗口（关于 / 附加窗口）：
// 都用 antd Modal（closable=false，自己画右上角 X）、遮罩可关、尺寸固定 323×323。
func TestTheDisplayDialogsCloseFromTheTopRightIcon(t *testing.T) {
	modals := readFrontendSource(t, "src/components/Modals.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	for _, wanted := range []string{
		`className="about-window"`,  // 关于窗口
		`className="hidden-window"`, // 附加窗口
		"closable={false}",          // 不用 antd 自带的关闭按钮
		"maskClosable",              // 点遮罩也能关
		`className="dialog-close"`,  // 右上角那个 X
		`aria-label="关闭"`,
		"width={323}",
	} {
		if !strings.Contains(modals, wanted) {
			t.Errorf("Modals.tsx 里缺少展示型窗口的契约：%s", wanted)
		}
	}
	for _, wanted := range []string{
		".about-window {",
		".hidden-window {",
		"width: 323px !important;",
		"height: 323px;",
		"object-fit: cover", // 附加窗口的图片要铺满
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

// TestTheImportPanelKeepsTheSourceOrderAndIcons 固定导入区：四个来源的顺序（医保代码信息
// 在最上、与下面三组之间留空行）、以及三个图标按钮的可读名称。
func TestTheImportPanelKeepsTheSourceOrderAndIcons(t *testing.T) {
	bridge := readFrontendSource(t, "src/bridge.ts")
	panels := readFrontendSource(t, "src/components/Panels.tsx")

	order := []string{"medical_insurance_code", "product_category", "global_udi_input", "ra_input"}
	position := -1
	for _, key := range order {
		index := strings.Index(bridge, `key: "`+key+`"`)
		if index < 0 {
			t.Fatalf("SOURCES 里缺少来源 %s", key)
		}
		if index < position {
			t.Errorf("来源顺序不对：%s 应该排在前面", key)
		}
		position = index
	}
	if !strings.Contains(bridge, "GROUP_GAP_AFTER = 1") {
		t.Error("第一个分组之后要有空行（原界面的 horizontalSpacer）")
	}
	for _, wanted := range []string{`aria-label="载入文件"`, `aria-label="数据回顾"`, `aria-label="清空导入数据"`} {
		if !strings.Contains(panels, wanted) {
			t.Errorf("导入区的图标按钮缺少可读名称：%s", wanted)
		}
	}
	// 图标全部用 Lucide，并按原版 PySide6 的 ThemeIcon 语义一一对应：
	//   FolderOpen（载入文件）/ DocumentOpen→FileText（数据回顾）/
	//   EditDelete→Trash2（清空）/ SystemSearch→Search（记录导出的回顾）
	for _, icon := range []string{"FolderOpen", "FileText", "Trash2", "Search"} {
		if !strings.Contains(panels, icon) {
			t.Errorf("图标应当使用 Lucide 的 %s（对应原版图标语义）", icon)
		}
	}
	if strings.Contains(panels, `"anticon"`) || strings.Contains(panels, "<path d=") {
		t.Error("图标不应再手写 SVG 路径")
	}
}

// TestTheModuleCornersShareOneSmallRadius 固定圆角口径：所有模块级容器
// （卡片 / 导入分组 / 结果表容器 / 状态条 / 操作日志）用同一个 4px 小圆角，
// 避免同一页出现"大方框直角、小方框圆角"的观感。
func TestTheModuleCornersShareOneSmallRadius(t *testing.T) {
	styles := readFrontendSource(t, "src/styles.css")
	for _, selector := range []string{".card {", ".group {", ".tblwrap {", ".status-strip {"} {
		start := strings.Index(styles, selector)
		if start < 0 {
			t.Fatalf("样式里找不到 %s", selector)
		}
		body := styles[start:]
		if end := strings.Index(body, "}"); end >= 0 {
			body = body[:end]
		}
		if !strings.Contains(body, "border-radius: 4px") {
			t.Errorf("%s 的圆角应为统一的 4px：%s", selector, body)
		}
	}
	// 操作日志卡片是通过共用的 .card 类拿到同一个圆角的
	if panels := readFrontendSource(t, "src/components/Panels.tsx"); !strings.Contains(panels, `className="card log-card"`) {
		t.Error("操作日志卡片应当复用 .card 类（圆角与其它模块一致）")
	}
}

// TestTheModuleCardsClipTheirTitleBar 固定"标题条上沿要有圆角"的做法：
// 标题条是直角矩形，只有卡片自己 overflow:hidden 把上沿两角裁掉，
// 才能和外框的圆角对齐 —— 否则红色标题条的方角露在圆角外面，
// 看起来比下方面板"方"、也像没有边框（用户报的问题）。
func TestTheModuleCardsClipTheirTitleBar(t *testing.T) {
	styles := readFrontendSource(t, "src/styles.css")
	start := strings.Index(styles, ".card {")
	if start < 0 {
		t.Fatal("样式里找不到 .card")
	}
	body := styles[start:]
	if end := strings.Index(body, "}"); end >= 0 {
		body = body[:end]
	}
	if !strings.Contains(body, "overflow: hidden") {
		t.Errorf("卡片必须裁剪标题条的方角（overflow: hidden）：%s", body)
	}
	if !strings.Contains(body, "border-radius: 4px") {
		t.Errorf("卡片的圆角应当是统一的小圆角：%s", body)
	}
}

// TestTheEmptyResultTableHasNoFrame 固定空结果表的口径：没有数据时不画那个带边框的
// 长方形（它会和模块外框叠成两层），而是在整个模块区域正中间显示"暂无数据"。
func TestTheEmptyResultTableHasNoFrame(t *testing.T) {
	panels := readFrontendSource(t, "src/components/Panels.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	if !strings.Contains(panels, "rows.length === 0 ?") {
		t.Error("结果表要按「有没有数据」分支渲染")
	}
	if !strings.Contains(panels, "<Empty description=\"暂无数据\" />") {
		t.Error("空状态应当用 antd 的 Empty + 暂无数据")
	}
	if !strings.Contains(styles, ".table-empty {") ||
		!strings.Contains(styles, "justify-content: center") {
		t.Error("空状态应当在模块区域里居中显示")
	}
}

// TestTheSettingsTreeUsesAntdTree 固定参数设定：Go 返回结构化树 → antd Tree 渲染，
// 容器标 {n}/[n]、标量按类型着色、空集合有占位，另有路径条与全部展开/折叠。
func TestTheSettingsTreeUsesAntdTree(t *testing.T) {
	modals := readFrontendSource(t, "src/components/Modals.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	for _, wanted := range []string{
		`import { Button, Input, Modal, Pagination, Table, Tabs, Tree, Tooltip } from "antd"`,
		"treeData={treeData}",
		"expandedKeys={expandedKeys}",
		"onSelect={(keys) => onSelectNode(",
		"tree-empty", // 空集合占位
		"全部展开",
		"全部折叠",
		"settings-path", // 路径条
	} {
		if !strings.Contains(modals, wanted) {
			t.Errorf("SettingsModal 里缺少：%s", wanted)
		}
	}
	// 类型着色：四类都有自己的颜色，且不是等宽字体
	for _, wanted := range []string{
		".tree-value.type-string", ".tree-value.type-number",
		".tree-value.type-bool", ".tree-value.type-null",
	} {
		if !strings.Contains(styles, wanted) {
			t.Errorf("样式里缺少值文本的类型配色：%s", wanted)
		}
	}
}

// TestTheReviewModalKeepsTheFooterLayout 固定回顾窗口的底栏：
// 导出在最左、翻页居中、关闭在最右（同一行），每页 100 行。
func TestTheReviewModalKeepsTheFooterLayout(t *testing.T) {
	modals := readFrontendSource(t, "src/components/Modals.tsx")
	if !strings.Contains(modals, "justifyContent: \"center\"") {
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
		".review-modal > div {", // antd 夹在中间的 motion/panel 容器要先撑满
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

// TestAllDialogsAreCenteredAndTheMessageDialogIsStyled 固定弹窗的两条口径：
//  1. 所有弹窗都必须居中（antd 的 centered 属性），包括关于窗口与附加窗口；
//  2. 导入/导出完成后的统一提示弹窗：不要右上角 X、标题条红底白字、确认按钮居中；
//     并且关掉 antd 在两个汉字之间插空格的默认行为（"确 定" → "确定"）。
func TestAllDialogsAreCenteredAndTheMessageDialogIsStyled(t *testing.T) {
	panels := readFrontendSource(t, "src/components/Panels.tsx")
	modals := readFrontendSource(t, "src/components/Modals.tsx")
	styles := readFrontendSource(t, "src/styles.css")
	main := readFrontendSource(t, "src/main.tsx")

	// 1) 五个弹窗（提示 / 回顾 / 参数设定 / 关于 / 附加窗口）全部居中
	if got := strings.Count(modals, "centered"); got < 4 {
		t.Errorf("回顾/参数设定/关于/附加窗口都要居中（centered），实际 %d 处", got)
	}
	if !strings.Contains(panels, "centered") {
		t.Error("提示弹窗也要居中（centered）")
	}
	// 2) 提示弹窗的样式
	if !strings.Contains(panels, "closable={false}") {
		t.Error("提示弹窗不该有右上角关闭按钮")
	}
	if !strings.Contains(panels, `className="app-message"`) {
		t.Error("提示弹窗要带 app-message 类（红底白字标题条靠它）")
	}
	for _, wanted := range []string{
		".app-message .ant-modal-header {",
		"background: #d71600",
		".app-message .ant-modal-title {",
		"color: #fff",
		".app-message .ant-modal-footer {",
		"justify-content: center",
	} {
		if !strings.Contains(styles, wanted) {
			t.Errorf("提示弹窗样式缺少：%s", wanted)
		}
	}
	// 3) 关于窗口：内容纵向居中 + 图标与文字留出间距
	for _, wanted := range []string{"justify-content: center", "height: 323px"} {
		if !strings.Contains(styles, wanted) {
			t.Errorf("关于窗口内容区样式缺少：%s", wanted)
		}
	}
	if !strings.Contains(styles, "margin-top: 16px") {
		t.Error("图标与下方文字之间要留出间距")
	}
	// 4) 中文按钮不要自动插空格
	if !strings.Contains(main, "autoInsertSpace: false") {
		t.Error("要关掉 antd 在两个汉字之间插空格的默认行为")
	}
}

// TestTheWailsBuildRunsTheFrontendBuild 固定构建链：wails build 之前先构建前端，
// 保证打进二进制的 dist 一定和源码一致。
func TestTheWailsBuildRunsTheFrontendBuild(t *testing.T) {
	raw, err := os.ReadFile("wails.json")
	if err != nil {
		t.Fatalf("读 wails.json 失败：%v", err)
	}
	if !strings.Contains(string(raw), "npm run build") {
		t.Error("wails.json 的 frontend:build 应当构建前端（避免 dist 与源码不一致）")
	}
}

// ---------- 发布前的设计体检（A 组：可读性 / 可点性 / 一致性） ----------

// TestTheSettingsTreeValuesMeetContrastOnWhite 固定参数设定里值文本的可读性：
// 三种有颜色的标量在白底上都要过 WCAG AA（12px 小字 ≥ 4.5:1），
// 且 styles.css 里的颜色必须与 theme.ts 的 VALUE_COLORS 对得上（不许两处各写一套）。
func TestTheSettingsTreeValuesMeetContrastOnWhite(t *testing.T) {
	theme := readFrontendSource(t, "src/theme.ts")
	styles := readFrontendSource(t, "src/styles.css")

	colors := treeValueColors(t, theme)
	for _, kind := range []string{"string", "number", "bool"} {
		hex, ok := colors[kind]
		if !ok {
			t.Fatalf("theme.ts 的 VALUE_COLORS 里缺少 %s", kind)
		}
		if ratio := contrastOnWhite(t, hex); ratio < 4.5 {
			t.Errorf("tree-value.type-%s 的颜色 %s 在白底上只有 %.2f:1，低于 AA 的 4.5:1", kind, hex, ratio)
		}
	}
	// 样式里那份必须跟主题一致（历史上这里漏改成 4.24:1 的 #B26A00）
	want := "color: " + colors["bool"] + ";"
	if !strings.Contains(styles, want) {
		t.Errorf("styles.css 的 .tree-value.type-bool 应当用 theme.ts 里的 %s（找不到 %q）", colors["bool"], want)
	}
}

// TestTheLogResizerHasAComfortableHitAreaAndAFocusRing 固定操作日志拖动条：
// 可见抓取条还是 3px，但热区放到 16px（鼠标更容易抓），键盘聚焦时用品牌红焦点环。
func TestTheLogResizerHasAComfortableHitAreaAndAFocusRing(t *testing.T) {
	styles := readFrontendSource(t, "src/styles.css")

	block := cssBlock(t, styles, ".log-resizer {")
	if !strings.Contains(block, "height: 16px;") {
		t.Errorf("拖动条热区高度应为 16px，实际块：\n%s", block)
	}
	if !strings.Contains(block, "align-items: flex-start;") {
		t.Error("热区变高后要靠 flex-start 让可见抓取条仍然贴着卡片上沿")
	}
	if !strings.Contains(styles, ".log-resizer::before {") || !strings.Contains(styles, "height: 3px;") {
		t.Error("可见的抓取条仍然只有 3px 高")
	}
	focus := cssBlock(t, styles, ".log-resizer:focus-visible {")
	if !strings.Contains(focus, "outline: 2px solid #b3261e;") {
		t.Errorf("键盘聚焦要用品牌红焦点环，实际块：\n%s", focus)
	}
}

// TestTheTableCornersMatchTheContainerRadius 固定整合结果表格的圆角：
// antd 默认给表头首尾格 8px，外层 .tblwrap 是 4px —— 两处必须统一成 4px。
func TestTheTableCornersMatchTheContainerRadius(t *testing.T) {
	theme := readFrontendSource(t, "src/theme.ts")
	styles := readFrontendSource(t, "src/styles.css")

	if !strings.Contains(theme, "borderRadius: 4,") {
		t.Error("theme.ts 的 Table 组件应当把圆角设成 4px（与容器一致）")
	}
	for _, wanted := range []string{
		".tblwrap .ant-table-container,",
		".tblwrap .ant-table-container table > thead > tr:first-child > th:first-child,",
		".tblwrap .ant-table-container table > thead > tr:first-child > th:last-child {",
		"border-start-start-radius: 4px;",
		"border-start-end-radius: 4px;",
	} {
		if !strings.Contains(styles, wanted) {
			t.Errorf("样式里缺少表格圆角对齐规则：%s", wanted)
		}
	}
}

// ---------- 发布前的设计体检（B 组：层级 / 状态 / 一致性） ----------

// TestTheExportActionIsSecondaryAndWaitsForReadyRows 固定数据整合工具条的层级与门禁：
// 一个区域只留一个主按钮（数据整合 = primary，生成文件 = 次按钮），
// 且没有 Ready 行时「生成文件」禁用（只有 Ready 会导出，点了也是白点）。
func TestTheExportActionIsSecondaryAndWaitsForReadyRows(t *testing.T) {
	panels := readFrontendSource(t, "src/components/Panels.tsx")

	if !strings.Contains(panels, `const readyCount = statuses.filter((status) => status === "Ready").length;`) {
		t.Error("工具条应当按 Ready 行数决定「生成文件」能不能点")
	}
	exportButton := panels[strings.Index(panels, `icon={<FileDown size={16} />}`):]
	exportButton = exportButton[:strings.Index(exportButton, "</Button>")]
	if strings.Contains(exportButton, `type="primary"`) {
		t.Error("「生成文件」应当是次按钮：一个区域只留「数据整合」一个主按钮")
	}
	if !strings.Contains(exportButton, "disabled={readyCount === 0}") {
		t.Error("没有 Ready 行时「生成文件」要禁用")
	}
	if !strings.Contains(panels, "还没有可导出的行（只有 Ready 状态会导出）") {
		t.Error("禁用时要给出原因（Tooltip）")
	}
	if !strings.Contains(panels, `type="primary"`) || !strings.Contains(panels, "数据整合") {
		t.Error("「数据整合」仍然是这个区域的主按钮")
	}
}

// TestTheStatusLabelFallsBackToAPlaceholder 固定：状态标签还没有内容时显示「尚未导入」，
// 不能留一片空白（原来导入区那一行会突然塌成空的）。
func TestTheStatusLabelFallsBackToAPlaceholder(t *testing.T) {
	panels := readFrontendSource(t, "src/components/Panels.tsx")

	if !strings.Contains(panels, `{current.label || "尚未导入"}`) {
		t.Error("状态标签要有「尚未导入」占位")
	}
}

// TestTheActionModalsReuseTheRedTitleBar 固定数据回顾 / 参数设定这两个带动作的弹窗：
// 标题条也用产品红底白字（跟提示弹窗一套），右上角的 X 改成白色。
func TestTheActionModalsReuseTheRedTitleBar(t *testing.T) {
	modals := readFrontendSource(t, "src/components/Modals.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	// 回顾窗口还会带 review-modal（尺寸口径），所以只认前缀
	if got := strings.Count(modals, `className="app-modal`); got != 2 {
		t.Errorf("数据回顾与参数设定两个弹窗都要带 app-modal 类，实际 %d 处", got)
	}
	for _, wanted := range []string{
		".app-modal .ant-modal-header {",
		".app-modal .ant-modal-title {",
		".app-modal .ant-modal-close {",
	} {
		if !strings.Contains(styles, wanted) {
			t.Errorf("弹窗标题条样式缺少：%s", wanted)
		}
	}
	header := cssBlock(t, styles, ".app-modal .ant-modal-header {")
	if !strings.Contains(header, "background: #d71600;") {
		t.Errorf("标题条要用产品红，实际块：\n%s", header)
	}
	title := cssBlock(t, styles, ".app-modal .ant-modal-title {")
	if !strings.Contains(title, "color: #fff;") {
		t.Errorf("标题文字要用白色，实际块：\n%s", title)
	}
	closeButton := cssBlock(t, styles, ".app-modal .ant-modal-close {")
	if !strings.Contains(closeButton, "color: #fff;") {
		t.Errorf("右上角的 X 要用白色，实际块：\n%s", closeButton)
	}
}

// TestTheSettingsModalMatchesTheModuleCornersAndDropsTheCloseIcon 固定参数设定窗口：
//   - 标题条上沿的圆角跟主窗口模块一样是 4px（antd 的 borderRadiusLG 是 16px，显得过大，
//     而且弹窗本体的圆角也得跟着收，否则标题条拐角处会露出底下的一小块白）；
//   - 右上角不要关闭按钮，只留底栏那个「关闭」。
func TestTheSettingsModalMatchesTheModuleCornersAndDropsTheCloseIcon(t *testing.T) {
	modals := readFrontendSource(t, "src/components/Modals.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	start := strings.Index(modals, "export function SettingsModal")
	end := strings.Index(modals, "export function AboutModal")
	if start < 0 || end < start {
		t.Fatal("没找到 SettingsModal 与 AboutModal 的边界")
	}
	settings := modals[start:end]
	if !strings.Contains(settings, "closable={false}") {
		t.Error("参数设定窗口不该有右上角的关闭按钮")
	}
	if !strings.Contains(settings, "关闭") {
		t.Error("参数设定窗口要保留底栏的关闭按钮")
	}
	if strings.Contains(styles, "border-radius: 16px 16px 0 0") {
		t.Error("标题条圆角不该再用 16px（跟主窗口模块的 4px 不一致）")
	}
	header := cssBlock(t, styles, ".app-modal .ant-modal-header {")
	if !strings.Contains(header, "border-radius: 4px 4px 0 0;") {
		t.Errorf("标题条上沿应当是 4px 圆角，实际块：\n%s", header)
	}
	content := cssBlock(t, styles, ".app-modal .ant-modal-content {")
	if !strings.Contains(content, "border-radius: 4px;") {
		t.Errorf("弹窗本体的圆角要跟标题条一起收成 4px，实际块：\n%s", content)
	}
	// 提示弹窗（导入/导出完成）用的是同一条红底白字标题，圆角也统一
	messageHeader := cssBlock(t, styles, ".app-message .ant-modal-header {")
	if !strings.Contains(messageHeader, "border-radius: 4px 4px 0 0;") {
		t.Errorf("提示弹窗的标题条圆角也要统一成 4px，实际块：\n%s", messageHeader)
	}
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

// treeValueColors 从 theme.ts 的 VALUE_COLORS 里读出 {类型: #RRGGBB}。
func treeValueColors(t *testing.T, theme string) map[string]string {
	t.Helper()
	block := cssBlock(t, theme, "export const VALUE_COLORS")
	pattern := regexp.MustCompile(`(\w+):\s*"(#[0-9A-Fa-f]{6})"`)
	colors := map[string]string{}
	for _, match := range pattern.FindAllStringSubmatch(block, -1) {
		colors[match[1]] = strings.ToLower(match[2])
	}
	if len(colors) == 0 {
		t.Fatal("没有从 VALUE_COLORS 里解析出任何颜色")
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
