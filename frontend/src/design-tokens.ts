// 设计 token 的**唯一来源**（颜色与关键尺寸）。
//
// 为什么要有这个文件：这些值以前散落在 styles.css 的字面量里，由 Go 契约测试 grep 字符串来
// 守着 —— 改个颜色要同时改 CSS 与测试里的字面量，容易漂移。现在的口径是：
//
//   1. 这里定义值；
//   2. styles.css 在 :root 里用同名 CSS 自定义属性（--brand-red 等）声明，规则里用 var(...)；
//   3. frontend/src/__tests__/designTokens.test.ts 校验"CSS 里的变量值 == 这里的值"。
//
// 于是颜色/尺寸只有一处真源：改这里 + 改 CSS 变量，测试会立刻指出两边不一致。

/** 品牌红：强生企业红（PANTONE 485 C）。主按钮 / 链接 / 强调色，白字对比度 4.87:1。 */
export const BRAND_RED = "#DA291C";

/** 危险红：antd 默认 colorError（#ff4d4f）白底只有 3.27:1，换成同色系 red-7。 */
export const DANGER_RED = "#CF1322";

/** 功能模块描边：比 antd 默认的 #f0f0f0 深，模块间隔很窄时也能一眼分开。 */
export const MODULE_BORDER = "#d9d9d9";

/** 操作日志标题条的灰底：比模块描边浅、比 antd colorFillAlter 深一档。 */
export const LOG_HEAD_BG = "#e8e8e8";

/** 日志时间列宽：装得下 HH:MM:SS（12px 等宽数字约 54px）。 */
export const LOG_TIME_WIDTH = "64px";

/** 日志级别槽位：两个字 + 级别色，横向垂直都居中。 */
export const LOG_LEVEL_SLOT = "32px";

/** 操作日志卡的下限高度（左列很矮时也要能看几行）。 */
export const LOG_MIN_HEIGHT = "72px";

/** 整合状态片固定宽度：不随数字位数变化，三个片永远等宽对齐。 */
export const STATUS_CHIP_WIDTH = "86px";

/**
 * 日志级别色（前端 src/log.ts 的四个级别）：与 Go 侧 app.go 的 logLevelLabels 一一对应。
 * 值同时用在 CSS（.log-level-* ）里，所以走 CSS 变量。
 */
export const LOG_LEVEL_COLORS: Record<"info" | "success" | "warning" | "error", string> = {
  info: "#1677ff",
  success: "#389e0d",
  warning: "#d48806",
  error: "#CF1322",
};

/**
 * 需要与 CSS 自定义属性对齐的 token 清单：
 * 键是 styles.css :root 里的变量名（不含 --），值是它必须等于的 token 值。
 */
export const CSS_TOKENS: Record<string, string> = {
  "brand-red": BRAND_RED,
  "danger-red": DANGER_RED,
  "module-border": MODULE_BORDER,
  "log-head-bg": LOG_HEAD_BG,
  "log-time-width": LOG_TIME_WIDTH,
  "log-level-slot": LOG_LEVEL_SLOT,
  "log-min-height": LOG_MIN_HEIGHT,
  "status-chip-width": STATUS_CHIP_WIDTH,
  "log-info": LOG_LEVEL_COLORS.info,
  "log-success": LOG_LEVEL_COLORS.success,
  "log-warning": LOG_LEVEL_COLORS.warning,
  "log-error": LOG_LEVEL_COLORS.error,
};
