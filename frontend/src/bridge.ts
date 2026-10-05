// 与 Go 侧的桥接：调用绑定方法（带载入图层）、订阅事件、以及界面上的固定口径。

import type { SourceDefinition } from "./types";

declare global {
  interface Window {
    go?: { main?: { App?: Record<string, (...args: unknown[]) => Promise<unknown>> } };
    runtime?: {
      EventsOn(name: string, callback: (...args: unknown[]) => void): void;
      EventsOff(name: string): void;
    };
  }
}

// 四个来源的顺序与标题照搬原界面：医保代码信息在最上，与下面三组之间留一个空行，
// 然后是产品类别、UDI团队信息、RA信息。
export const SOURCES: SourceDefinition[] = [
  { key: "medical_insurance_code", title: "医保代码信息" },
  { key: "product_category", title: "产品类别" },
  { key: "global_udi_input", title: "UDI团队信息" },
  { key: "ra_input", title: "RA信息" },
];

// 第一个分组之后插一个空行（对应原界面的 horizontalSpacer）。

export const REVIEW_PAGE_SIZE = 100;

// 长任务的载入文案：与原来的状态栏原文一致。
export const BUSY_TEXT: Record<string, string> = {
  SelectImportFile: "正在打开文件夹",
  ImportSource: "正在导入 Excel 数据...",
  Consolidate: "正在整合数据...",
  ReviewSource: "正在加载已导入数据...",
  ReviewLog: "正在加载已导入数据...",
  Export: "正在导出文件...",
  ExportReviewData: "正在导出文件...",
  SelectExportTarget: "正在选择保存位置",
  SelectReviewExportTarget: "正在选择保存位置",
  ClearImportedData: "正在清空导入数据...",
  ConfigurationDocument: "正在加载配置...",
  SaveConfigurationFile: "正在保存配置...",
  BackupDatabase: "正在备份数据库...",
};

type BusyListener = (text: string | null) => void;
const busyListeners = new Set<BusyListener>();

/** 订阅"当前是否有长任务在跑"，有任务时给出要显示的文案。 */
export function onBusy(listener: BusyListener): () => void {
  busyListeners.add(listener);
  return () => busyListeners.delete(listener);
}

function publishBusy(text: string | null): void {
  for (const listener of busyListeners) listener(text);
}

function bridge(): Record<string, (...args: unknown[]) => Promise<unknown>> | null {
  return window.go?.main?.App ?? null;
}

/**
 * 把文本复制到剪贴板：先让后端（Wails 运行时）写系统剪贴板，失败再退回浏览器方案。
 * 返回是否成功，调用方据此给出「已复制 / 复制失败」的提示。
 */
export async function copyToClipboard(text: string): Promise<boolean> {
  // 直接调桥接，不走 call()：复制是一瞬间的事，没必要闪一下「正在处理…」的载入图层
  const app = bridge();
  if (app && typeof app.CopyToClipboard === "function") {
    try {
      if (await app.CopyToClipboard(text)) return true;
    } catch {
      // 后端不可用（例如预览环境）→ 继续走浏览器方案
    }
  }
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch {
    // webview 里 Clipboard API 可能被安全上下文拦下，继续走兜底
  }
  try {
    const area = document.createElement("textarea");
    area.value = text;
    area.setAttribute("readonly", "");
    area.style.position = "fixed";
    area.style.top = "-1000px";
    area.style.opacity = "0";
    document.body.appendChild(area);
    area.select();
    const copied = document.execCommand("copy");
    document.body.removeChild(area);
    return copied;
  } catch {
    return false;
  }
}

const nextFrame = (): Promise<void> =>
  new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve())));

/**
 * 调用一个绑定方法。绑定不可用时返回 undefined（界面按"没反应"处理），
 * 调用期间显示载入图层——先等两帧确保图层画出来，再真正开始，避免窗口先"假死"。
 */
export async function call<T>(method: string, ...args: unknown[]): Promise<T | undefined> {
  const app = bridge();
  if (!app || typeof app[method] !== "function") return undefined;
  publishBusy(BUSY_TEXT[method] ?? "正在处理...");
  await nextFrame();
  try {
    return (await app[method](...args)) as T;
  } catch (error) {
    // 绑定层抛错（例如后端未就绪）也要让界面回到可用状态
    console.error(`${method} failed`, error);
    return undefined;
  } finally {
    publishBusy(null);
  }
}

/** 订阅 Go 侧菜单发来的事件；没有 runtime 时（纯浏览器预览）静默跳过。 */
export function onEvent(name: string, handler: (...args: unknown[]) => void): void {
  window.runtime?.EventsOn(name, handler);
}

/**
 * 日期框只取日期，查询时补足当天的起止时刻（起始 00:00:00、结束 23:59:59）。
 *
 * 分隔符必须是**斜杠**：`operation_log.log_time` 按 `YYYY/MM/DD HH:MM:SS` 存（R22），
 * 而按时间区间取记录是**字符串比较**（与原版 pandas 的 between 一致）。日期框给的是
 * `YYYY-MM-DD`，横线 `-`(0x2D) 和斜杠 `/`(0x2F) 在同一个年份里就能分出大小，
 * 于是 `记录 <= 结束时间` 恒为假 —— 整个记录导出窗口一条都查不出来（都是空）。
 */
export function toLogTime(value: string, endOfDay: boolean): string {
  if (!value) return "";
  const day = value.replace(/-/g, "/");
  return `${day} ${endOfDay ? "23:59:59" : "00:00:00"}`;
}

/** 记录导出的默认时间窗：起始＝一年前的今天，结束＝今天。 */
export function defaultRange(today: Date): { start: string; end: string } {
  const format = (date: Date): string =>
    `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
  const start = new Date(today.getTime());
  start.setFullYear(start.getFullYear() - 1);
  return { start: format(start), end: format(today) };
}
