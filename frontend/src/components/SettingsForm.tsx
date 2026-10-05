// 参数设定的「结构化表单」：把 YAML 的每一层渲染成分块表单。
//
// 口径（与用户确认的方案 B）：
//   - 所有层级都分块显示（map / seq 各是一块，嵌套缩进），键名 + 注释一并显示；
//   - 标量按后端给的 control 渲染成 antd 控件（输入框 / 数字框 / 开关 / 下拉）；
//   - 只读项（DB 字段名、样本工厂专用块、条件表达式、生成物）显示值 + 原因，不给控件；
//   - 列表：标量列表可增删条目；`sources.*.columns` 这种"对象列表"用表格 + 抽屉编辑，
//     **不允许新增 / 删除 / 排序**（顺序即暂存表列顺序）；
//   - 改动先攒在 pending 里，由窗口底部的「保存」一次性提交（后端只改被编辑的那几行）。

import {
  Button,
  Drawer,
  Flex,
  Input,
  InputNumber,
  Select,
  Switch,
  Table,
  Tag,
  Tooltip,
  Typography,
} from "antd";
import type { ColumnsType } from "antd/es/table";
import { RotateCcw, Trash2 } from "lucide-react";
import { useState } from "react";

import type { SettingsChange, SettingsNode } from "../types";

type PendingList = SettingsChange[];

/**
 * 注释显示口径：去掉每行行首的 "#"（以及紧随的一个空格），其余缩进与空行保留 ——
 * YAML 里存的是带 "#" 的原文，界面上只看正文；"#  1. …" 这类列表仍能看出层级，
 * 单独一个 "#" 变成空行，保留注释里的分段。
 */
export function stripCommentMarks(text: string): string {
  return text
    .split("\n")
    .map((line) => {
      const trimmed = line.replace(/^[ \t]+/, "");
      if (!trimmed.startsWith("#")) return line;
      const indent = line.slice(0, line.length - trimmed.length);
      return indent + trimmed.replace(/^#[ ]?/, "");
    })
    .join("\n");
}

const samePath = (left: string[], right: string[]) =>
  left.length === right.length && left.every((part, index) => part === right[index]);

/** 路径 → 字符串 key（列表下标用 "#" 分隔，避免和键名里的点混淆）。 */
export const pathKey = (path: string[]) => path.join("\u0000");

/** 找出待保存改动里某路径的值（没有则用文件里的原值）。 */
function pendingValue(pending: PendingList, path: string[], fallback: string): string {
  const hit = pending.find((item) => item.action !== "remove" && samePath(item.path, path));
  return hit ? hit.value : fallback;
}

function isPending(pending: PendingList, path: string[]): boolean {
  return pending.some((item) => item.action !== "remove" && samePath(item.path, path));
}

type BlockProps = {
  node: SettingsNode;
  depth: number;
  pending: PendingList;
  onChange: (change: SettingsChange) => void;
  onResetPath: (path: string[]) => void;
};

/** 一个块：map / seq 是「分块 + 子项」，标量是「标签 + 控件」。 */
export function SettingsBlock({ node, depth, pending, onChange, onResetPath }: BlockProps) {
  if (node.kind === "map" || node.kind === "seq") {
    return (
      <section className="settings-block" data-depth={depth} data-kind={node.kind}>
        <header className="settings-block-head">
          <span className="settings-block-key">{node.label || node.key}</span>
          <span className="settings-block-meta">
            {node.kind === "map" ? `{${node.children?.length ?? 0}}` : `[${node.children?.length ?? 0}]`}
          </span>
          {/* 只读容器的说明（例如生成物、样本工厂专用块） */}
          {node.reason && <span className="settings-block-reason">{node.reason}</span>}
        </header>
        {node.comment && <pre className="settings-comment">{stripCommentMarks(node.comment)}</pre>}
        <div className="settings-block-body">
          {isColumnList(node) ? (
            <ColumnList node={node} depth={depth} pending={pending} onChange={onChange} onResetPath={onResetPath} />
          ) : isScalarList(node) ? (
            <ScalarList node={node} pending={pending} onChange={onChange} />
          ) : (
            (node.children ?? []).map((child) => (
              <SettingsBlock
                key={pathKey(child.path ?? [child.key])}
                node={child}
                depth={depth + 1}
                pending={pending}
                onChange={onChange}
                onResetPath={onResetPath}
              />
            ))
          )}
        </div>
      </section>
    );
  }

  const path = node.path ?? [];
  const dirty = isPending(pending, path);
  return (
    <div className="settings-field" data-dirty={dirty || undefined} data-depth={depth}>
      <span className="settings-field-label" title={path.join(".")}>
        {node.label || node.key}
      </span>
      <div className="settings-field-control">
        <ScalarControl node={node} pending={pending} onChange={onChange} />
      </div>
      {dirty && (
        <Tooltip title="丢弃这一项改动">
          <Button
            className="settings-field-reset"
            type="text"
            size="small"
            aria-label="丢弃这一项改动"
            icon={<RotateCcw size={14} />}
            onClick={() => onResetPath(path)}
          />
        </Tooltip>
      )}
      <span className="settings-field-key">{node.key}</span>
      {node.note && <span className="settings-field-note">{node.note}</span>}
      {!node.editable && node.reason && <span className="settings-field-reason">{node.reason}</span>}
    </div>
  );
}

/** 标量控件：只读项只显示值；可编辑项按 control 给 antd 控件。 */
function ScalarControl({
  node,
  pending,
  onChange,
}: {
  node: SettingsNode;
  pending: PendingList;
  onChange: (change: SettingsChange) => void;
}) {
  const path = node.path ?? [];
  const current = pendingValue(pending, path, node.value ?? "");
  if (!node.editable) {
    const shown = node.alias ? `*${node.alias}` : current === "" ? "（空）" : current;
    return (
      <Tooltip title={node.reason || "只读"}>
        <span className={`settings-readonly type-${node.kind}`}>{shown}</span>
      </Tooltip>
    );
  }
  switch (node.control) {
    case "switch":
      return (
        <Switch
          size="small"
          checked={current === "true"}
          onChange={(checked) => onChange({ path, value: checked ? "true" : "false" })}
        />
      );
    case "number":
      return (
        <InputNumber
          size="small"
          className="settings-number"
          min={node.min}
          max={node.max}
          value={current === "" || current === "null" ? null : Number(current)}
          placeholder={node.kind === "null" ? "（null）" : undefined}
          onChange={(value) => onChange({ path, value: value === null || value === undefined ? "" : String(value) })}
        />
      );
    case "select":
      return (
        <Select
          size="small"
          className="settings-select"
          value={current === "" ? undefined : current}
          placeholder="（未设置）"
          options={(node.options ?? []).map((option) => ({ label: option, value: option }))}
          onChange={(value) => onChange({ path, value })}
        />
      );
    default:
      return (
        <Input
          size="small"
          className="settings-input"
          value={current}
          placeholder={node.kind === "null" ? "（null）" : undefined}
          onChange={(event) => onChange({ path, value: event.target.value })}
        />
      );
  }
}

/** 标量列表（可以增删条目）：例如必填状态可选值、条件判断的空值。 */
function isScalarList(node: SettingsNode): boolean {
  return (
    node.kind === "seq" &&
    node.listEdit === true &&
    (node.children ?? []).every((child) => child.kind !== "map" && child.kind !== "seq")
  );
}

function ScalarList({
  node,
  pending,
  onChange,
}: {
  node: SettingsNode;
  pending: PendingList;
  onChange: (change: SettingsChange) => void;
}) {
  const [draft, setDraft] = useState("");
  const items = node.children ?? [];
  const path = node.path ?? [];
  // 待删除的条目（按原文件下标），保存时一起提交
  const removed = new Set(
    pending.filter((item) => item.action === "remove" && samePath(item.path, path)).map((item) => item.index ?? -1),
  );
  const appended = pending.filter((item) => item.action === "append" && samePath(item.path, path));

  return (
    <div className="settings-list">
      {items.map((item, index) => {
        if (removed.has(index)) return null;
        return (
          <div className="settings-list-row" key={pathKey(item.path ?? [item.key])}>
            <span className="settings-list-index">{index}</span>
            <Input
              size="small"
              value={pendingValue(pending, item.path ?? [], item.value ?? "")}
              placeholder={item.value === "" ? "（空字符串）" : undefined}
              onChange={(event) => onChange({ path: item.path ?? [], value: event.target.value })}
            />
            <Tooltip title="删除这一条（保存后生效）">
              <Button
                type="text"
                size="small"
                danger
                aria-label={`删除第 ${index + 1} 条`}
                icon={<Trash2 size={14} />}
                onClick={() => onChange({ path, action: "remove", index, value: "" })}
              />
            </Tooltip>
          </div>
        );
      })}
      {appended.map((item, index) => (
        <div className="settings-list-row" key={`appended-${index}`}>
          <span className="settings-list-index">新增</span>
          <Input size="small" value={item.value} readOnly />
          <Tooltip title="撤销这条新增">
            <Button
              type="text"
              size="small"
              aria-label="撤销新增"
              icon={<RotateCcw size={14} />}
              onClick={() =>
                onChange({ path, action: "remove", index: items.length + index, value: "" })
              }
            />
          </Tooltip>
        </div>
      ))}
      <Flex className="settings-list-add" gap={8} align="center">
        <Input
          size="small"
          value={draft}
          aria-label={`向 ${node.label || node.key} 添加一条`}
          placeholder="新条目内容"
          onChange={(event) => setDraft(event.target.value)}
        />
        <Button
          size="small"
          disabled={draft === ""}
          onClick={() => {
            onChange({ path, action: "append", value: draft });
            setDraft("");
          }}
        >
          添加一条
        </Button>
      </Flex>
    </div>
  );
}

/** 对象列表：`sources.*.columns` 走这条 —— 表格 + 抽屉，禁止增删排序。 */
function isColumnList(node: SettingsNode): boolean {
  const path = node.path ?? [];
  const isColumns = path.length >= 3 && path[0] === "sources" && path[path.length - 1] === "columns";
  return node.kind === "seq" && isColumns && (node.children ?? []).length > 0;
}

function ColumnList({ node, pending, onChange, onResetPath }: BlockProps) {
  const [openIndex, setOpenIndex] = useState<number | null>(null);
  const rows = node.children ?? [];
  const scalarOf = (item: SettingsNode, key: string): SettingsNode | undefined =>
    (item.children ?? []).find((child) => child.key === key);
  const shown = (item: SettingsNode, key: string) => {
    const child = scalarOf(item, key);
    if (!child) return "";
    return pendingValue(pending, child.path ?? [], child.value ?? "");
  };

  const columns: ColumnsType<SettingsNode> = [
    {
      title: "列名（Excel 表头）",
      key: "chinese_name",
      render: (_: unknown, item) => (
        <span className={isPending(pending, scalarOf(item, "chinese_name")?.path ?? []) ? "settings-dirty" : ""}>
          {shown(item, "chinese_name")}
        </span>
      ),
    },
    {
      title: "DB 字段",
      key: "db_field",
      render: (_: unknown, item) => <span className="settings-readonly">{shown(item, "db_field")}</span>,
    },
    {
      title: "必填状态",
      key: "mandatory_status",
      width: 150,
      render: (_: unknown, item) => (
        <span className={isPending(pending, scalarOf(item, "mandatory_status")?.path ?? []) ? "settings-dirty" : ""}>
          {shown(item, "mandatory_status")}
        </span>
      ),
    },
    {
      title: "其它",
      key: "extra",
      width: 130,
      render: (_: unknown, item) => (
        <Flex gap={4}>
          {scalarOf(item, "value_mapping") && <Tag>值映射</Tag>}
          {scalarOf(item, "required_when") && <Tag color="gold">条件</Tag>}
        </Flex>
      ),
    },
  ];

  const openItem = openIndex === null ? null : rows[openIndex];
  return (
    <>
      <Typography.Text className="settings-list-hint" type="secondary">
        共 {rows.length} 列 · 顺序即暂存表列顺序，不支持新增 / 删除 / 排序（要改结构请用『编辑原文』）
      </Typography.Text>
      <Table<SettingsNode>
        className="settings-columns"
        size="small"
        pagination={false}
        rowKey={(item) => pathKey(item.path ?? [item.key])}
        dataSource={rows}
        columns={columns}
        scroll={{ y: 320 }}
        onRow={(_item, index) => ({
          onClick: () => setOpenIndex(index ?? null),
          style: { cursor: "pointer" },
        })}
      />
      <Drawer
        className="settings-drawer"
        open={openItem !== null}
        width={520}
        title={openItem ? `第 ${(openIndex ?? 0) + 1} 列：${shown(openItem, "chinese_name")}` : ""}
        onClose={() => setOpenIndex(null)}
      >
        {openItem && (
          <div className="settings-form">
            <SettingsBlock
              node={openItem}
              depth={0}
              pending={pending}
              onChange={onChange}
              onResetPath={onResetPath}
            />
          </div>
        )}
      </Drawer>
    </>
  );
}

/** 表单根：把一份配置的顶层键逐个渲染成分块。 */
export function SettingsForm({
  nodes,
  pending,
  onChange,
  onResetPath,
  onReset,
}: {
  nodes: SettingsNode[];
  pending: PendingList;
  onChange: (change: SettingsChange) => void;
  onResetPath: (path: string[]) => void;
  onReset: () => void;
}) {
  return (
    <div className="settings-form">
      {pending.length > 0 && (
        <Flex className="settings-pending" align="center" justify="space-between" gap={8}>
          <Typography.Text>
            有 <strong>{pending.length}</strong> 项改动待保存（保存后立即热重载）
          </Typography.Text>
          <Button size="small" icon={<Trash2 size={14} />} onClick={onReset}>
            全部撤销
          </Button>
        </Flex>
      )}
      {nodes.map((node) => (
        <SettingsBlock
          key={pathKey(node.path ?? [node.key])}
          node={node}
          depth={0}
          pending={pending}
          onChange={onChange}
          onResetPath={onResetPath}
        />
      ))}
    </div>
  );
}
