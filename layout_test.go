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

// 前端是 React + antd + Lucide（源码在 frontend/src，构建产物在 frontend/dist）。
// 这里的契约测试读**源码**而不是打包产物：产物里的类名会被压缩，源码才是可维护的契约面。
// 少数几条（内嵌资源、提示字样）同时检查产物，因为那才是真正发给用户的东西。
//
// 两套口径：
//   - 行为契约（日志拖动、声音、弹窗尺寸、图标语义、导出门禁……）—— 与改版前一致；
//   - 设计契约 —— 现在是 **antd 默认值 + 强生红**：字号 / 圆角 / 间距 / 控件高度都不许再出现
//     PySide6 原型带过来的手工值（4px 小圆角、红底白字标题条、13px 基准字号、36px 日期框……）。

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

// TestTheLogWindowIsFilterableAndTwoColumn 固定操作日志窗口的形式（参考界面那套）：
// 顶部按日志类型分栏目（全部 / 信息 / 成功 / 警告 / 错误）+「新日志置顶」，
// 内容是一张两列表格：左列时间（行首、不换行），右列级别标签 + 正文（长内容换行、多行明细保留），
// 行之间用 antd Table 自带的浅色分割线。
func TestTheLogWindowIsFilterableAndTwoColumn(t *testing.T) {
	panels := readFrontendSource(t, "src/components/Panels.tsx")
	styles := readFrontendSource(t, "src/styles.css")
	script := readFrontendSource(t, "src/log.ts")

	for _, wanted := range []string{
		"parseLog(",  // 按行契约把日志文本拆成条目
		"<Segmented", // 级别栏目
		`{ label: "全部", value: "all" }`,
		"LOG_LEVELS.map", // 信息 / 成功 / 警告 / 错误
		"新日志置顶",          // 排序开关
		"<Switch",
		"{entries.length} 条", // 卡片头上的总条数
		"showHeader={false}", // 两列表格，不再单画一行表头
		`title: "时间",`,
		`title: "信息",`,
		"width: 156",       // 时间列固定宽度，长正文不会把它挤走
		"LOG_LEVEL_COLORS", // 级别标签着色
		"log-text",         // 正文容器（换行靠它）
		"暂无日志",
	} {
		if !strings.Contains(panels, wanted) {
			t.Errorf("LogPanel 里缺少：%s", wanted)
		}
	}

	for _, wanted := range []string{
		".log-toolbar {",
		".log-list {",
		".log-table .log-time {",
		"white-space: nowrap;",                // 时间列不换行
		"font-variant-numeric: tabular-nums;", // 数字等宽，时间才对得齐
		".log-table .log-message {",
		"overflow-wrap: anywhere;", // 长路径 / 长报错要换行
		".log-text {",
		"white-space: pre-wrap;", // 多行明细保留换行
		"max-height: 220px;",     // 没拖动时日志不会把上面的模块挤扁
	} {
		if !strings.Contains(styles, wanted) {
			t.Errorf("日志窗口样式里缺少：%s", wanted)
		}
	}
	// 浅色分割线用 antd Table 自带的行分隔线，不许在样式里抹掉
	if strings.Contains(styles, "border-bottom: none") {
		t.Error("不该把日志表格的分割线抹掉（antd Table 的行分隔线就是浅色分割线）")
	}
	// 兜底分类：行里没有级别标签时也要能分到四类之一
	for _, level := range []string{"info", "success", "warning", "error"} {
		if !strings.Contains(script, `"`+level+`"`) {
			t.Errorf("log.ts 的兜底分类缺少级别 %q", level)
		}
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

	dates := cssBlock(t, styles, ".range .dates {")
	if !strings.Contains(dates, "flex: 1 1 auto;") {
		t.Errorf("日期容器应当占满整行、又允许被压缩，实际块：\n%s", dates)
	}
	for _, wanted := range []string{
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

// TestTheUIKeepsTheProductColorsAndFont 固定主题口径：品牌红＝强生企业红（PANTONE 485 C），
// 且落到 antd 的 colorPrimary / colorInfo / colorLink 上；字体用跨平台栈（含中文回退），
// 不许再用等宽字体（Windows 上会退化成 Consolas + 微软雅黑混排）。
func TestTheUIKeepsTheProductColorsAndFont(t *testing.T) {
	theme := readFrontendSource(t, "src/theme.ts")
	styles := readFrontendSource(t, "src/styles.css")

	if !strings.Contains(theme, `export const BRAND_RED = "#DA291C"`) {
		t.Error("品牌红应当是强生企业红 #DA291C")
	}
	for _, tokenName := range []string{"colorPrimary", "colorInfo", "colorLink"} {
		if !strings.Contains(theme, tokenName+": BRAND_RED") {
			t.Errorf("theme.ts 的 %s 应当用 BRAND_RED", tokenName)
		}
	}
	if !strings.Contains(theme, `"Microsoft YaHei"`) || !strings.Contains(theme, `"PingFang SC"`) {
		t.Error("字体栈要跨平台（含微软雅黑/苹方）")
	}
	if strings.Contains(theme, "monospace") || strings.Contains(styles, "monospace") {
		t.Error("界面不该用等宽字体（Windows 上会退化成两种字体混排）")
	}
	// 结果表的四种状态在样式里都有自己的类
	for _, wanted := range []string{
		".result-row.status-Ready", ".result-row.status-Incomplete",
		".result-row.status-Duplicate", ".result-row.status-Conflict",
	} {
		if !strings.Contains(styles, wanted) {
			t.Errorf("结果表缺少状态着色：%s", wanted)
		}
	}
}

// TestNoPySideLeftoversInTheDesign 固定"不再残留 PySide6 原型的设计语言"：
// 手工 4px 圆角、红底白字标题条、13px 基准字号、36px 日期框、2px 表格内边距、
// 仿只读输入框的状态条、居中按钮的 QDialog 底栏——这些手工值一个都不许再出现。
func TestNoPySideLeftoversInTheDesign(t *testing.T) {
	text := frontendSourceText(t)

	for _, leftover := range []string{
		"border-radius: 4px",    // 统一小圆角：改回 antd 的 borderRadius
		"background: #d71600",   // 红底标题条
		"#b3261e",               // 旧主红
		"#d71600",               // 旧标题条红
		"font-size: 13px",       // 13px 基准字号（antd 默认 14px）
		"height: 36px",          // 36px 日期框 / 工具条按钮
		"padding: 2px 5px",      // 表格 2px 内边距
		`className="title-bar"`, // 手写标题条
		"status-strip .dot",     // 自绘状态圆点
		"border-bottom: 1px solid rgba(0, 0, 0, 0.24)",        // 仿只读输入框的底部描边
		"justify-content: center;\n  padding: 12px 24px 16px", // QDialog 式居中底栏
	} {
		if strings.Contains(text, leftover) {
			t.Errorf("前端源码里还残留旧设计：%q", leftover)
		}
	}
}

// TestTheModulesUseAntdCards 固定四个模块的容器：用 antd 的 Card（size="small"），
// 标题排版 / 边框 / 圆角都由 antd 给，样式里不再手写卡片外观。
func TestTheModulesUseAntdCards(t *testing.T) {
	app := readFrontendSource(t, "src/App.tsx")
	styles := readFrontendSource(t, "src/styles.css")

	for _, title := range []string{"数据导入", "记录导出", "数据整合"} {
		wanted := `className="card" size="small" title="` + title + `"`
		if title == "数据整合" {
			wanted = `className="card grow" size="small" title="` + title + `"`
		}
		if !strings.Contains(app, wanted) {
			t.Errorf("App.tsx 的「%s」模块应当用 antd Card：找不到 %s", title, wanted)
		}
	}
	if !strings.Contains(app, `<Card`) || !strings.Contains(app, `from "antd"`) {
		t.Error("App.tsx 应当从 antd 引入 Card")
	}
	// 操作日志卡片复用同一个 Card
	if panels := readFrontendSource(t, "src/components/Panels.tsx"); !strings.Contains(panels, `<Card`) ||
		!strings.Contains(panels, `className="card log-card"`) {
		t.Error("操作日志卡片应当也是 antd Card（圆角与其它模块一致）")
	}
	card := cssBlock(t, styles, ".card {")
	if strings.Contains(card, "border-radius") || strings.Contains(card, "background") {
		t.Errorf(".card 不该自绘圆角/底色（交给 antd 的 Card），实际块：\n%s", card)
	}
}

// TestTheResultTableUsesAntdDefaults 固定结果表的密度口径：
// 不加 bordered、不覆盖单元格内边距、不做圆角对齐 hack；
// .tblwrap 只是滚动容器，不再自绘边框 / 圆角 / 投影。
func TestTheResultTableUsesAntdDefaults(t *testing.T) {
	panels := readFrontendSource(t, "src/components/Panels.tsx")
	modals := readFrontendSource(t, "src/components/Modals.tsx")
	styles := readFrontendSource(t, "src/styles.css")
	theme := readFrontendSource(t, "src/theme.ts")

	for name, source := range map[string]string{"Panels.tsx": panels, "Modals.tsx": modals} {
		if strings.Contains(source, "bordered") {
			t.Errorf("%s 的结果表不该用 bordered（antd Table 默认无边框）", name)
		}
		if !strings.Contains(source, `size="small"`) {
			t.Errorf("%s 的结果表应当用 antd 的小尺寸密度", name)
		}
	}
	if strings.Contains(styles, ".ant-table-cell") || strings.Contains(styles, "border-start-start-radius") {
		t.Error("不该再覆盖 antd 表格的单元格内边距与圆角")
	}
	wrap := cssBlock(t, styles, ".tblwrap {")
	for _, selfDrawn := range []string{"border:", "border-radius", "box-shadow", "background"} {
		if strings.Contains(wrap, selfDrawn) {
			t.Errorf(".tblwrap 只是滚动容器，不该自绘 %s，实际块：\n%s", selfDrawn, wrap)
		}
	}
	if strings.Contains(theme, "Table:") {
		t.Error("theme.ts 不该再给 Table 写组件级覆盖（用 antd 默认）")
	}
}

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

// TestTheImportStatusUsesAntdBadge 固定导入区的状态表达：用 antd 的 Badge（状态语义色）
// 与 Typography 的次级文字，不再自绘圆点、也不再做"仿只读输入框"的状态条。
func TestTheImportStatusUsesAntdBadge(t *testing.T) {
	panels := readFrontendSource(t, "src/components/Panels.tsx")

	if !strings.Contains(panels, "Badge") || !strings.Contains(panels, "BadgeProps") {
		t.Error("导入状态应当用 antd Badge 表达（状态语义色由 antd 给）")
	}
	if !strings.Contains(panels, "type=\"secondary\"") || !strings.Contains(panels, "ellipsis={{ tooltip:") {
		t.Error("状态文字应当是次级色 + 过长省略（antd Typography）")
	}
	if !strings.Contains(panels, `{current.label || "尚未导入"}`) {
		t.Error("状态标签要有「尚未导入」占位")
	}
	if strings.Contains(panels, `className="dot`) {
		t.Error("不该再自绘状态圆点")
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
		`} from "antd"`,
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
// 三种有颜色的标量在白底上都要过 WCAG AA（≥ 4.5:1），
// 且 styles.css 里的颜色必须与 theme.ts 的 VALUE_COLORS 对得上（不许两处各写一套）。
func TestTheSettingsTreeValuesMeetContrastOnWhite(t *testing.T) {
	theme := readFrontendSource(t, "src/theme.ts")
	styles := readFrontendSource(t, "src/styles.css")

	colors := themeColors(t, theme, "export const VALUE_COLORS")
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

// TestTheStatusColorsStayInSyncWithTheTheme 固定结果表状态色只有一份来源：
// theme.ts 的 STATUS_COLORS 与 styles.css 里的字面值必须一致，
// 否则"工具栏标签 / 行底色 / 竖条"迟早会三套颜色。
func TestTheStatusColorsStayInSyncWithTheTheme(t *testing.T) {
	theme := readFrontendSource(t, "src/theme.ts")
	styles := readFrontendSource(t, "src/styles.css")

	colors := themeColors(t, theme, "export const STATUS_COLORS")
	for _, status := range []string{"Ready", "Incomplete", "Duplicate", "Conflict"} {
		hex, ok := colors[status]
		if !ok {
			t.Fatalf("theme.ts 的 STATUS_COLORS 里缺少 %s", status)
		}
		if !strings.Contains(strings.ToLower(styles), hex) {
			t.Errorf("styles.css 里找不到 %s 的状态色 %s（行底色/竖条应与主题一致）", status, hex)
		}
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
	if !strings.Contains(block, "overflow: visible") && !strings.Contains(cssBlock(t, styles, ".log-card {"), "overflow: visible") {
		t.Error("抓取条有 5px 在卡片外，卡片不能裁剪它")
	}
	focus := cssBlock(t, styles, ".log-resizer:focus-visible {")
	if !strings.Contains(focus, "outline: 2px solid #da291c;") {
		t.Errorf("键盘聚焦要用品牌红焦点环，实际块：\n%s", focus)
	}
}

// TestTheToolbarFitsTheNarrowestWindow 固定数据整合工具条在最小窗口（969 宽）下的行为：
// 一行装不下「按钮 + 四个状态标签 + 按钮」时，整条状态标签换到第二行，
// 而不是把四个标签挤成一列（媒体查询断点 1040 是按最小窗口倒推的）。
func TestTheToolbarFitsTheNarrowestWindow(t *testing.T) {
	styles := readFrontendSource(t, "src/styles.css")

	toolbar := cssBlock(t, styles, ".table-toolbar {")
	if !strings.Contains(toolbar, "flex-wrap: wrap;") {
		t.Errorf("工具条要允许换行，实际块：\n%s", toolbar)
	}
	if !strings.Contains(styles, "@media (max-width: 1040px)") {
		t.Error("窄窗口要给一条媒体查询，把状态标签整条移到第二行")
	}
	narrow := styles[strings.Index(styles, "@media (max-width: 1040px)"):]
	if !strings.Contains(narrow, "order: 3;") || !strings.Contains(narrow, "flex-basis: 100%;") {
		t.Error("窄窗口下状态标签应当独占一行（order + flex-basis: 100%）")
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

// TestTheStatusSummaryUsesAntdTags 固定状态汇总的表达：antd 的 Tag（success/error/warning +
// Conflict 的 magenta），不再自绘灰底胶囊。
func TestTheStatusSummaryUsesAntdTags(t *testing.T) {
	panels := readFrontendSource(t, "src/components/Panels.tsx")

	for _, wanted := range []string{`<Tag color="success">Ready`, `<Tag color="error">Incomplete`, `<Tag color="warning">Duplicate`, `<Tag color="magenta">Conflict`} {
		if !strings.Contains(panels, wanted) {
			t.Errorf("状态汇总应当用 antd Tag：找不到 %s", wanted)
		}
	}
	if !strings.Contains(panels, `id="consolidation-summary"`) {
		t.Error("汇总条的 id 要保留（验收脚本按它取数）")
	}
}

// TestTheSettingsModalUsesAntdFooterAndKeepsTheCloseIcon 固定参数设定窗口：
// 关闭按钮回到 antd 默认（右上角 X），底栏按状态给出「关闭」或「取消 / 保存」。
func TestTheSettingsModalUsesAntdFooterAndKeepsTheCloseIcon(t *testing.T) {
	modals := readFrontendSource(t, "src/components/Modals.tsx")

	start := strings.Index(modals, "export function SettingsModal")
	end := strings.Index(modals, "export function AboutModal")
	if start < 0 || end < start {
		t.Fatal("没找到 SettingsModal 与 AboutModal 的边界")
	}
	settings := modals[start:end]
	if strings.Contains(settings, "closable={false}") {
		t.Error("参数设定窗口应当保留 antd 默认的右上角关闭按钮")
	}
	for _, wanted := range []string{"关闭", "取消", "保存", "全部展开", "全部折叠", "编辑原文"} {
		if !strings.Contains(settings, wanted) {
			t.Errorf("参数设定窗口缺少底栏/工具按钮：%s", wanted)
		}
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
