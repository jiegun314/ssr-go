package main

import (
	"os"
	"path/filepath"
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
