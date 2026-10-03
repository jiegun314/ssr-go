// antd 主题：只覆盖品牌色与字体栈，字号 / 圆角 / 间距 / 控件高度全部用 antd 默认值。
//
// 这次重做的口径就是这一句：界面语言用 antd 的，不再用 PySide6 原型带过来的那套
// 手工 Material 样式（4px 小圆角、红底标题条、13px 基准字号、36px 输入框、2px 表格内边距……）。
// 所以这里的 token 越少越好，只有一个例外——品牌红必须落到强生企业红上。

import type { ThemeConfig } from "antd";

/** 品牌红：强生企业红（PANTONE 485 C）。主按钮 / 链接 / 强调色，白字对比度 4.87:1。 */
export const BRAND_RED = "#DA291C";

/**
 * 危险红：antd 默认的 colorError 是 #ff4d4f，白底只有 3.27:1，
 * 小字与图标都不到 AA；换成同色系的 red-7，同时也是「Incomplete」状态色。
 */
export const DANGER_RED = "#CF1322";

/**
 * 界面字体栈：antd 默认栈（system-ui 系列）后面补上中文字体。
 * 刻意不用等宽字体——Windows 上等宽栈会退化成 Consolas + 微软雅黑两种字体混排。
 */
export const UI_FONT =
  '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, "Noto Sans", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif, "Apple Color Emoji", "Segoe UI Emoji"';

export const theme: ThemeConfig = {
  token: {
    colorPrimary: BRAND_RED,
    colorInfo: BRAND_RED,
    colorLink: BRAND_RED,
    colorError: DANGER_RED,
    fontFamily: UI_FONT,
  },
  components: {
    // 日志栏目的过滤条（antd Segmented）：选中项用品牌红，和参考界面的"当前栏目"一致。
    Segmented: {
      itemSelectedBg: BRAND_RED,
      itemSelectedColor: "#ffffff",
    },
  },
};

/**
 * 参数设定里值文本的类型配色：取 antd 色板里在白底上过 AA（≥ 4.5:1）的那几档。
 * styles.css 里的字面值必须与这里一致（layout_test.go 会校对，避免两处各写一套）。
 */
export const VALUE_COLORS: Record<string, string> = {
  string: "#237804", // antd green-8，白底 5.59:1
  number: "#CF1322", // antd red-7，白底 5.57:1
  bool: "#874D00",   // antd gold-9，白底 6.79:1
  null: "rgba(0, 0, 0, 0.45)",
};

/**
 * 数据整合结果的四种状态：状态点 / 工具栏标签 / 结果表行底色共用这一组色。
 * 前三种对应 antd 的 success / error / warning，Conflict 另用 magenta 区分
 * （它和 Incomplete 都是"有问题的行"，不能同色）。
 */
export const STATUS_COLORS = {
  Ready: "#389E0D",      // antd green-7
  Incomplete: "#CF1322", // antd red-7
  Duplicate: "#D48806",  // antd gold-7
  Conflict: "#C41D7F",   // antd magenta-6
} as const;
