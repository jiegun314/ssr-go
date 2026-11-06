// 前端用到的数据类型。
//
// 口径（见 AGENTS.md §11）：**Go 返回的结构以生成的模型为准** ——
// frontend/wailsjs/go/models.ts 由 Wails 从 Go 结构体生成，字段名与 json tag 一一对应。
// 这里不再手写一遍字段，而是从生成模型派生：
//
//   - 纯数据袋（结果 / 状态）：直接等价于生成模型，Go 侧加字段会自动跟着出现；
//   - 需要"收窄"的地方：用 Omit + 交叉类型补上前端自己的联合类型
//     （例如 state 只有四种、SettingsNode.kind 只有六种），既保留穷尽判断的安全性，
//     又不必手工同步字段清单。
//
// 只有两类留在本文件手写：Go 侧没有对应结构体的（AboutInfo）与纯前端的（SourceDefinition）。

import type { app, share } from "../wailsjs/go/models";

/**
 * 只取数据字段：Wails 生成的类上还带一个 convertValues 方法（原型方法，类型里也有），
 * 它不该出现在"数据结构"的类型里 —— 否则纯对象字面量无法满足该类型。
 */
type Data<T> = Omit<T, "convertValues">;

/** 单个来源的导入状态（Go: Data<app.ImportState>）。state 在 Go 里是字符串常量，这里收窄。 */
export type ImportState = Omit<Data<app.ImportState>, "state"> & {
  state: "empty" | "imported" | "existing" | "failed";
};

/** 界面初始状态（Go 的 InitialState 结构体在生成模型里叫 State）。 */
export type InitialState = Omit<Data<app.State>, "imports"> & {
  imports: Record<string, ImportState>;
};

export type ImportResult = Omit<Data<app.ImportResult>, "state"> & {
  state: ImportState;
};

export type ClearResult = Data<app.ClearResult>;

export type ConsolidateResult = Data<app.ConsolidateResult>;

export type ExportResult = Data<app.ExportResult>;

export type ExportTarget = Data<app.ExportTarget>;

export type ReviewResult = Data<app.ReviewResult>;

/** 参数设定的一个节点（Go: Data<app.SettingsNode>）。kind 收窄成六种，children 递归到本类型。 */
export type SettingsNode = Omit<Data<app.SettingsNode>, "kind" | "children"> & {
  kind: "map" | "seq" | "string" | "number" | "bool" | "null";
  children?: SettingsNode[];
};

/** 一次结构化修改（Go: Data<app.SettingsChange>）：改标量，或对标量列表做增删。 */
export type SettingsChange = Omit<Data<app.SettingsChange>, "action"> & {
  action?: "set" | "append" | "remove";
};

export type SettingsTab = Omit<Data<app.SettingsTab>, "tree"> & {
  tree: SettingsNode[];
};

export type SettingsSaveResult = Data<app.SettingsSaveResult>;

/* ---------- 多人协作版（P1）：登录与会话 ---------- */

export type LoginRole = Data<app.LoginRole>;
export type SessionInfo = Data<app.SessionInfo>;
/** 更新通知的预览（收件人 / 主题 / 正文） */
export type NotificationPreview = Data<app.NotificationPreview>;
/** 更新通知的发送结果 */
export type NotificationResult = Data<app.NotificationResult>;

export type LoginResult = Omit<Data<app.LoginResult>, "session"> & {
  session?: SessionInfo;
};
export type ShareStatus = Data<share.Status>;
export type SourceStatus = Data<app.SourceStatus>;
export type LastConsolidation = Data<app.LastConsolidation>;
export type ShareOverview = Omit<Data<app.ShareOverview>, "sources" | "lastConsolidation" | "status"> & {
  sources: SourceStatus[];
  lastConsolidation?: LastConsolidation;
  status: ShareStatus;
};
export type AlignResult = Data<app.AlignResult>;
export type CleanupResult = Data<app.CleanupResult>;
export type DiagnosticsResult = Data<app.DiagnosticsResult>;

/* ---------- 以下为前端自有类型（Go 侧没有对应结构体） ---------- */

export type AboutInfo = Data<app.AboutInfo>;

/** 四个来源的界面定义（顺序与标题是界面口径，不是后端数据）。 */
export type SourceDefinition = {
  key: string;
  title: string;
};
