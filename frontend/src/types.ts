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
