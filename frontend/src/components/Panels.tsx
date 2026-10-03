// 主界面的四块：数据导入、记录导出、数据整合、操作日志，加上载入图层与结果弹窗。
//
// 组件选型按 antd 默认语言来：
//   状态 → Badge / Tag（不再自绘圆点、不再做"仿只读输入框"的状态条）；
//   空状态 → Empty；表格 → Table 默认密度（不加外框线、不压 2px 内边距）；
//   四个模块 → Card（size="small"），标题排版交给 antd。

import {
  Badge,
  Button,
  Card,
  DatePicker,
  Empty,
  Flex,
  Modal,
  Segmented,
  Space,
  Spin,
  Switch,
  Table,
  Tag,
  Tooltip,
  Typography,
} from "antd";
import type { BadgeProps } from "antd";
import type { ColumnsType } from "antd/es/table";
import dayjs from "dayjs";
import { ArrowRight, FileDown, FileText, FolderOpen, Merge, Search, Trash2 } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";

import { GROUP_GAP_AFTER, SOURCES } from "../bridge";
import {
  LOG_LEVELS,
  LOG_LEVEL_COLORS,
  LOG_LEVEL_LABELS,
  parseLog,
  type LogEntry,
  type LogLevel,
} from "../log";
import type { ImportState } from "../types";

/* ---------- 数据导入 ---------- */

type ImportPanelProps = {
  states: Record<string, ImportState>;
  onImport: (source: string) => void;
  onReview: (source: string) => void;
  onClear: () => void;
};

/** 导入状态 → antd Badge 的语义色（imported / existing 都是"已有数据"，同色）。 */
function importBadge(state: string): BadgeProps["status"] {
  switch (state) {
    case "imported":
    case "existing":
      return "success";
    case "failed":
      return "error";
    default:
      return "default";
  }
}

export function ImportPanel({ states, onImport, onReview, onClear }: ImportPanelProps) {
  return (
    <div>
      {SOURCES.map((source, index) => {
        const current = states[source.key] ?? { state: "empty", label: "", tooltip: "" };
        return (
          <div key={source.key}>
            <div className="group">
              <div className="title-row">
                <Badge
                  status={importBadge(current.state)}
                  text={<span className="group-title">{source.title}</span>}
                />
              </div>
              <div className="actions">
                <Tooltip title="载入文件">
                  <Button
                    type="text"
                    aria-label="载入文件"
                    icon={<FolderOpen size={16} />}
                    onClick={() => onImport(source.key)}
                  />
                </Tooltip>
                {/* 状态标签在两个按钮中间（原界面就是这个顺序） */}
                <Typography.Text
                  className="status"
                  id={`status-${source.key}`}
                  type="secondary"
                  ellipsis={{ tooltip: current.tooltip || undefined }}
                >
                  {current.label || "尚未导入"}
                </Typography.Text>
                <Tooltip title="数据回顾">
                  <Button
                    type="text"
                    aria-label="数据回顾"
                    icon={<FileText size={16} />}
                    disabled={current.state !== "imported" && current.state !== "existing"}
                    onClick={() => onReview(source.key)}
                  />
                </Tooltip>
              </div>
            </div>
            {index === GROUP_GAP_AFTER - 1 && <div className="group-gap" />}
          </div>
        );
      })}
      <Flex justify="flex-end">
        <Tooltip title="清空导入数据">
          <Button
            type="text"
            danger
            aria-label="清空导入数据"
            icon={<Trash2 size={16} />}
            onClick={onClear}
          />
        </Tooltip>
      </Flex>
    </div>
  );
}

/* ---------- 记录导出 ---------- */

type RecordExportPanelProps = {
  start: string;
  end: string;
  onStartChange: (value: string) => void;
  onEndChange: (value: string) => void;
  onReview: () => void;
};

export function RecordExportPanel({
  start,
  end,
  onStartChange,
  onEndChange,
  onReview,
}: RecordExportPanelProps) {
  // 两个框只选日期，查询时各自补足当天的起止时刻（00:00:00 / 23:59:59）。
  const picker = (value: string, onChange: (next: string) => void, label: string) => (
    <DatePicker
      value={value ? dayjs(value) : null}
      format="YYYY-MM-DD"
      allowClear={false}
      aria-label={label}
      onChange={(date) => onChange(date ? date.format("YYYY-MM-DD") : "")}
    />
  );

  return (
    <div className="range">
      <div className="dates">
        {picker(start, onStartChange, "起始日期")}
        <ArrowRight className="date-sep" size={14} aria-hidden="true" />
        {picker(end, onEndChange, "结束日期")}
      </div>
      <Tooltip title="回顾">
        <Button type="text" aria-label="回顾" icon={<Search size={16} />} onClick={onReview} />
      </Tooltip>
    </div>
  );
}

/* ---------- 数据整合 ---------- */

type ConsolidationPanelProps = {
  columns: string[];
  rows: string[][];
  statuses: string[];
  counts: Record<string, number>;
  onConsolidate: () => void;
  onExport: () => void;
};

export function ConsolidationPanel({
  columns,
  rows,
  statuses,
  counts,
  onConsolidate,
  onExport,
}: ConsolidationPanelProps) {
  const tableColumns: ColumnsType<string[]> = columns.map((title, index) => ({
    title,
    dataIndex: index,
    key: String(index),
    ellipsis: false,
    render: (value: string) =>
      value === "MISSING" ? <span className="cell-MISSING">{value}</span> : value,
  }));

  // 只有 Ready 行会被导出：没有它的时候「生成文件」没有意义
  const readyCount = statuses.filter((status) => status === "Ready").length;

  const strip = (
    <Space className="status-strip" id="consolidation-summary" size={[8, 8]} wrap>
      <Tag color="success">Ready {counts.Ready ?? 0}</Tag>
      <Tag color="error">Incomplete {counts.Incomplete ?? 0}</Tag>
      <Tag color="warning">Duplicate {counts.Duplicate ?? 0}</Tag>
      {(counts.Conflict ?? 0) > 0 && <Tag color="magenta">Conflict {counts.Conflict}</Tag>}
    </Space>
  );

  return (
    <>
      <div className="table-toolbar">
        <Button type="primary" icon={<Merge size={16} />} onClick={onConsolidate}>
          数据整合
        </Button>
        {strip}
        {/* 一个区域只留一个主按钮：生成文件是「整合之后才轮到」的动作，用次按钮 */}
        <Tooltip title={readyCount === 0 ? "还没有可导出的行（只有 Ready 状态会导出）" : ""}>
          <span>
            <Button
              type="default"
              icon={<FileDown size={16} />}
              disabled={readyCount === 0}
              onClick={onExport}
            >
              生成文件
            </Button>
          </span>
        </Tooltip>
      </div>
      {rows.length === 0 ? (
        <div className="table-empty">
          <Empty description="暂无数据" />
        </div>
      ) : (
        <div className="tblwrap">
          <Table<string[]>
            size="small"
            sticky
            pagination={false}
            dataSource={rows}
            columns={tableColumns}
            rowKey={(_, index) => String(index)}
            rowClassName={(_, index) => `result-row status-${statuses[index] ?? "Ready"}`}
            scroll={{ x: "max-content" }}
          />
        </div>
      )}
    </>
  );
}

/* ---------- 操作日志 ---------- */

/** 日志栏目的取值：全部 + 四个级别。 */
type LogFilter = LogLevel | "all";

/**
 * 操作日志卡片：上沿可拖动调高。
 * 上限＝左侧「数据导入 / 记录导出」只剩最小间隔（12px）；下限＝启动时的默认高度。
 * 拖动时卡片改成固定高度（flex:0 0 auto），让出的高度全部给中间列。
 *
 * 内容按参考界面的形式排成一张两列表格：左列时间（行首、不换行），
 * 右列「级别标签 + 正文」（长内容换行、多行明细照原样保留），行之间用 antd 的浅色分割线；
 * 顶部是级别栏目（全部 / 信息 / 成功 / 警告 / 错误）与「新日志置顶」开关。
 */
export function LogPanel({ text }: { text: string }) {
  const cardRef = useRef<HTMLDivElement>(null);
  const [pinned, setPinned] = useState<number | null>(null);
  const [filter, setFilter] = useState<LogFilter>("all");
  const [newestFirst, setNewestFirst] = useState(true);
  const defaultHeight = useRef<number>(0);

  const entries = useMemo(() => parseLog(text), [text]);
  // 过滤 + 排序都只影响显示，解析结果本身保持日志原有顺序
  const visible = useMemo(() => {
    const filtered = filter === "all" ? entries : entries.filter((entry) => entry.level === filter);
    return newestFirst ? [...filtered].reverse() : filtered;
  }, [entries, filter, newestFirst]);

  const columns: ColumnsType<LogEntry> = [
    { title: "时间", dataIndex: "time", width: 156, className: "log-time", render: (value: string) => value || "—" },
    {
      title: "信息",
      dataIndex: "message",
      className: "log-message",
      render: (_: string, entry: LogEntry) => (
        <Flex gap={8} align="flex-start">
          <Tag color={LOG_LEVEL_COLORS[entry.level]}>{LOG_LEVEL_LABELS[entry.level]}</Tag>
          <span className="log-text">{entry.message}</span>
        </Flex>
      ),
    },
  ];

  useEffect(() => {
    if (cardRef.current && defaultHeight.current === 0) {
      defaultHeight.current = Math.round(cardRef.current.getBoundingClientRect().height);
    }
  }, []);

  const leftMinimumHeight = (): number => {
    const left = document.querySelector<HTMLElement>(".left");
    if (!left) return 0;
    let total = 0;
    Array.from(left.children).forEach((child) => {
      const element = child as HTMLElement;
      if (element.classList.contains("spacer")) {
        total += parseFloat(getComputedStyle(element).minHeight) || 0;
      } else {
        total += element.getBoundingClientRect().height;
      }
    });
    return total;
  };

  const limits = (): { min: number; max: number } => {
    const card = cardRef.current;
    const columns = document.querySelector<HTMLElement>(".columns");
    if (!card || !columns) return { min: 0, max: 0 };
    const current = card.getBoundingClientRect().height;
    const shrinkable = columns.getBoundingClientRect().height - leftMinimumHeight();
    const min = Math.min(defaultHeight.current || current, current);
    return { min, max: Math.max(min, Math.round(current + shrinkable)) };
  };

  const startDrag = (event: React.PointerEvent<HTMLDivElement>) => {
    event.preventDefault();
    const card = cardRef.current;
    if (!card) return;
    const startY = event.clientY;
    const startHeight = card.getBoundingClientRect().height;
    document.body.classList.add("log-resizing");
    const move = (moveEvent: PointerEvent) => {
      const { min, max } = limits();
      const next = Math.min(Math.max(startHeight + (startY - moveEvent.clientY), min), max);
      setPinned(Math.round(next));
    };
    const finish = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", finish);
      window.removeEventListener("pointercancel", finish);
      document.body.classList.remove("log-resizing");
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", finish);
    window.addEventListener("pointercancel", finish);
  };

  const onKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    const step = 16;
    const card = cardRef.current;
    if (!card) return;
    const { min, max } = limits();
    const current = pinned ?? card.getBoundingClientRect().height;
    let next: number | null = null;
    if (event.key === "ArrowUp") next = current + step;
    if (event.key === "ArrowDown") next = current - step;
    if (next === null) return;
    event.preventDefault();
    setPinned(Math.round(Math.min(Math.max(next, min), max)));
  };

  return (
    <Card
      className="card log-card"
      ref={cardRef}
      size="small"
      title="操作日志"
      extra={<Typography.Text type="secondary">{entries.length} 条</Typography.Text>}
      style={
        pinned === null
          ? undefined
          : { flex: "0 0 auto", height: `${pinned}px`, maxHeight: `${pinned}px` }
      }
    >
      <div
        className="log-resizer"
        id="log-resizer"
        role="separator"
        aria-orientation="horizontal"
        aria-label="拖动调整操作日志高度"
        title="拖动调整操作日志高度"
        tabIndex={0}
        onPointerDown={startDrag}
        onKeyDown={onKeyDown}
      />
      <Flex className="log-toolbar" align="center" justify="space-between" gap={8}>
        <Segmented
          size="small"
          value={filter}
          onChange={(value) => setFilter(value as LogFilter)}
          options={[
            { label: "全部", value: "all" },
            ...LOG_LEVELS.map((level) => ({ label: LOG_LEVEL_LABELS[level], value: level })),
          ]}
        />
        <Flex align="center" gap={8}>
          <Typography.Text type="secondary">新日志置顶</Typography.Text>
          <Switch size="small" checked={newestFirst} onChange={setNewestFirst} />
        </Flex>
      </Flex>
      <div className="log-list">
        <Table<LogEntry>
          className="log-table"
          size="small"
          tableLayout="fixed"
          showHeader={false}
          pagination={false}
          dataSource={visible}
          columns={columns}
          rowKey={(entry) => entry.key}
          locale={{ emptyText: "暂无日志" }}
        />
      </div>
    </Card>
  );
}

/* ---------- 载入图层 ---------- */

export function LoadingOverlay({ text }: { text: string | null }) {
  if (text === null) return null;
  return (
    <div className="loading-layer" id="loading">
      <div className="loading-card">
        <Spin size="large" />
        <Typography.Text className="loading-text" id="loading-text">
          {text}
        </Typography.Text>
      </div>
    </div>
  );
}

/* ---------- 结果弹窗 ---------- */

export type MessageState = { title: string; message: string } | null;

/**
 * 导入 / 导出 / 错误统一提示：就是 antd 默认的 Modal（标题 + 右上角 X + 右对齐底栏），
 * 不再做 PySide 那套"红底标题条 + 居中按钮 + 去掉 X"的 QDialog 布局。
 * className 留作稳定的测试/定制挂点，本身不写任何样式。
 */
export function MessageModal({ state, onClose }: { state: MessageState; onClose: () => void }) {
  return (
    <Modal
      className="app-message"
      open={state !== null}
      title={state?.title ?? ""}
      centered
      width={416}
      onCancel={onClose}
      footer={
        <Button type="primary" onClick={onClose}>
          确定
        </Button>
      }
      destroyOnHidden
    >
      <Typography.Paragraph className="message-text">{state?.message ?? ""}</Typography.Paragraph>
    </Modal>
  );
}
