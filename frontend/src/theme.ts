// antd 主题：把现有的配色与字体映射到 token 上，组件本身用 antd 默认规则。

import type { ThemeConfig } from "antd";

/** 品牌红：filled 主按钮与强调色（白字对比度约 5.9:1）。 */
export const BRAND_RED = "#B3261E";
/** 标题条红：数据导入 / 记录导出 / 数据整合 / 操作日志的标题栏底色。 */
export const TITLE_RED = "#D71600";

/**
 * 界面字体栈：跨平台（Windows 走 Segoe UI + 微软雅黑，macOS 走系统字体 + 苹方）。
 * 刻意不用等宽字体——Windows 上等宽栈会退化成 Consolas + 微软雅黑两种字体混排。
 */
export const UI_FONT =
  '"Segoe UI", -apple-system, "PingFang SC", "Microsoft YaHei", "Helvetica Neue", Arial, sans-serif';

export const theme: ThemeConfig = {
  token: {
    colorPrimary: BRAND_RED,
    colorInfo: BRAND_RED,
    colorLink: BRAND_RED,
    colorError: "#A4262C",
    colorWarning: "#B26A00",
    colorSuccess: "#1B7F3B",
    borderRadius: 4,
    borderRadiusLG: 8,
    fontFamily: UI_FONT,
    fontSize: 13,
    colorText: "rgba(0, 0, 0, 0.87)",
    colorTextSecondary: "rgba(0, 0, 0, 0.60)",
    colorBorder: "rgba(0, 0, 0, 0.24)",
    controlHeight: 32,
  },
  components: {
    Button: { paddingInline: 16, fontWeight: 500 },
    Modal: { borderRadiusLG: 16, paddingContentHorizontal: 24 },
    Table: { headerBg: "#F2F2F2", headerSplitColor: "#DCDCDC", borderColor: "#D6D6D6" },
    Tree: { titleHeight: 22, nodeHoverBg: "rgba(0, 0, 0, 0.04)" },
    Tabs: { horizontalItemPadding: "8px 16px" },
  },
};

/** 值文本的类型配色：与界面其它地方的语义色一致。 */
export const VALUE_COLORS: Record<string, string> = {
  string: "#1B7F3B",
  number: "#A4262C",
  bool: "#B26A00",
  null: "rgba(0, 0, 0, 0.60)",
};
