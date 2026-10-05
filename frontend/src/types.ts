// Go 侧返回结构的类型（字段名与 internal/... 的 json tag 一一对应）。

export type ImportState = {
  source: string;
  state: "empty" | "imported" | "existing" | "failed";
  label: string;
  tooltip: string;
  rowCount: number;
  importTime: string;
};

export type InitialState = {
  version: string;
  operationLog: string;
  imports: Record<string, ImportState>;
  busy: boolean;
};

export type ImportResult = {
  source: string;
  fileName: string;
  rowCount: number;
  state: ImportState;
  log: string;
  title: string;
  message: string;
  failed: boolean;
};

export type ClearResult = {
  log: string;
  title: string;
  message: string;
  cleared: string[];
  failed: boolean;
};

export type ConsolidateResult = {
  log: string;
  title: string;
  message: string;
  failed: boolean;
  columns: string[];
  rows: string[][];
  statuses: string[];
};

export type ExportResult = {
  log: string;
  title: string;
  message: string;
  failed: boolean;
  path: string;
};

export type ExportTarget = {
  defaultName: string;
  target: string;
};

export type ReviewResult = {
  log: string;
  title: string;
  message: string;
  failed: boolean;
  columns: string[];
  rows: string[][];
  total: number;
  page: number;
  pageSize: number;
  pageCount: number;
};

export type SettingsNode = {
  key: string;
  kind: "map" | "seq" | "string" | "number" | "bool" | "null";
  value?: string;
  children?: SettingsNode[];
  /** 从文件根到这里的键路径（写回时按它定位） */
  path?: string[];
  /** 该节点上挂着的全部注释（原样展示） */
  comment?: string;
  /** input | number | switch | select | none */
  control?: string;
  options?: string[];
  label?: string;
  note?: string;
  editable?: boolean;
  reason?: string;
  /** 列表是否允许增删条目 */
  listEdit?: boolean;
  flow?: boolean;
  alias?: string;
  style?: string;
  min?: number;
  max?: number;
};

/** 一次结构化修改：改标量，或对标量列表做增删。 */
export type SettingsChange = {
  path: string[];
  action?: "set" | "append" | "remove";
  index?: number;
  value: string;
};

export type SettingsTab = {
  key: string;
  title: string;
  note: string;
  path: string;
  tree: SettingsNode[];
  raw: string;
};

export type SettingsSaveResult = {
  title: string;
  message: string;
  failed: boolean;
  backup: string;
};

export type AboutInfo = {
  name: string;
  version: string;
  detail: string;
};

export type SourceDefinition = {
  key: string;
  title: string;
};
