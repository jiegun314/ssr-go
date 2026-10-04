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
  Input,
  DatePicker,
  Descriptions,
  Empty,
  Flex,
  Modal,
  Segmented,
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
import {
  ArrowRight,
  CheckCircle2,
  CircleAlert,
  FileSpreadsheet,
  Search,
  Table as TableIcon,
} from "lucide-react";
import { useMemo, useState } from "react";

import { SOURCES } from "../bridge";
import {
  LOG_LEVELS,
  LOG_LEVEL_LABELS,
  parseLog,
  shortTime,
  type LogEntry,
  type LogLevel,
} from "../log";
import { statusLabel } from "../theme";
import type { ImportState } from "../types";

/* ---------- 数据导入 ---------- */

type ImportPanelProps = {
  states: Record<string, ImportState>;
  onImport: (source: string) => void;
  onReview: (source: string) => void;
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

export function ImportPanel({ states, onImport, onReview }: ImportPanelProps) {
  return (
    <div>
      {SOURCES.map((source) => {
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
                    icon={<FileSpreadsheet size={16} />}
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
                    icon={<TableIcon size={16} />}
                    disabled={current.state !== "imported" && current.state !== "existing"}
                    onClick={() => onReview(source.key)}
                  />
                </Tooltip>
              </div>
            </div>
          </div>
        );
      })}
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

  // 记录导出在左列（340 宽）：两个日期 + 箭头 + 纯图标「回顾」，一行正好放下并整组居中。
  // 回顾改成纯图标是为了让这一行塞进窄栏（带文字会多 52px，一行放不下），语义靠 Tooltip + aria-label。
  return (
    <div className="range">
      {picker(start, onStartChange, "起始日期")}
      <ArrowRight className="date-sep" size={14} aria-hidden="true" />
      {picker(end, onEndChange, "结束日期")}
      <Tooltip title="回顾">
        <Button icon={<Search size={16} />} aria-label="回顾" onClick={onReview} />
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
};

/** 结果表的一行：原始单元格 + 状态（状态筛选与行底色都要用，所以连下标一起包起来）。 */
type ConsolidationRow = { key: string; status: string; cells: string[] };

export function ConsolidationPanel({
  columns,
  rows,
  statuses,
  counts,
}: ConsolidationPanelProps) {
  // 状态筛选：点某个状态片只看该状态的行，再点一次回到全部（导出始终只看合格行，与筛选无关）
  const [statusFilter, setStatusFilter] = useState<string | null>(null);
  // 关键字：在整行任意单元格里做包含匹配（不区分大小写），与状态筛选叠加生效
  const [query, setQuery] = useState("");

  const tableColumns: ColumnsType<ConsolidationRow> = columns.map((title, index) => ({
    title,
    key: String(index),
    ellipsis: false,
    render: (_: unknown, entry: ConsolidationRow) => {
      const value = entry.cells[index];
      return value === "MISSING" ? <span className="cell-MISSING">{value}</span> : value;
    },
  }));

  const dataSource: ConsolidationRow[] = rows.map((cells, index) => ({
    key: String(index),
    status: statuses[index] ?? "Ready",
    cells,
  }));
  const needle = query.trim().toLowerCase();
  const visible = dataSource
    .filter((entry) => statusFilter === null || entry.status === statusFilter)
    .filter(
      (entry) =>
        needle === "" ||
        entry.cells.some((cell) => String(cell ?? "").toLowerCase().includes(needle)),
    );

  const toggleStatus = (status: string) =>
    setStatusFilter((current) => (current === status ? null : status));

  // 三个状态片（出现冲突时多一个）：等宽、与左右按钮同高、点击筛选。
  // 文案用中文（合格 / 缺失 / 重复），与日志窗口口径一致；后端状态常量仍是英文。
  const chips: { status: string; color: string }[] = [
    { status: "Ready", color: "success" },
    { status: "Incomplete", color: "error" },
    { status: "Duplicate", color: "warning" },
  ];
  if ((counts.Conflict ?? 0) > 0) chips.push({ status: "Conflict", color: "magenta" });

  return (
    <>
      <div className="table-toolbar">
        {/* 查找：输入即筛表格内容（与状态片叠加），靠左；状态片靠右 */}
        <Input
          className="table-search"
          allowClear
          value={query}
          aria-label="查找表格内容"
          placeholder="查找表格内容"
          prefix={<Search size={14} aria-hidden="true" />}
          onChange={(event) => setQuery(event.target.value)}
        />
        <div
          className="status-strip"
          id="consolidation-summary"
          data-filtered={statusFilter !== null}
        >
          <div className="status-chips">
            {chips.map(({ status, color }) => (
              <Tag
                key={status}
                className="status-chip"
                color={color}
                role="button"
                tabIndex={0}
                aria-pressed={statusFilter === status}
                title={`只看「${statusLabel(status)}」的行（再点一次显示全部）`}
                onClick={() => toggleStatus(status)}
                onKeyDown={(event) => {
                  if (event.key !== "Enter" && event.key !== " ") return;
                  event.preventDefault();
                  toggleStatus(status);
                }}
              >
                {statusLabel(status)} <span className="status-count">{counts[status] ?? 0}</span>
              </Tag>
            ))}
          </div>
        </div>
      </div>
      {rows.length === 0 ? (
        <div className="table-empty">
          <Empty description="暂无数据" />
        </div>
      ) : (
        <div className="tblwrap">
          <Table<ConsolidationRow>
            size="small"
            sticky
            pagination={false}
            dataSource={visible}
            columns={tableColumns}
            rowKey={(entry) => entry.key}
            rowClassName={(entry) => `result-row status-${entry.status}`}
            scroll={{ x: "max-content" }}
            locale={{
              emptyText: (
                <Empty
                  image={Empty.PRESENTED_IMAGE_SIMPLE}
                  description={
                    needle
                      ? "没有找到匹配的数据"
                      : statusFilter
                        ? `${statusLabel(statusFilter)} 状态没有数据`
                        : "暂无数据"
                  }
                />
              ),
            }}
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
  const [filter, setFilter] = useState<LogFilter>("all");
  const [newestFirst, setNewestFirst] = useState(true);

  const entries = useMemo(() => parseLog(text), [text]);
  // 过滤 + 排序都只影响显示，解析结果本身保持日志原有顺序
  const visible = useMemo(() => {
    const filtered = filter === "all" ? entries : entries.filter((entry) => entry.level === filter);
    return newestFirst ? [...filtered].reverse() : filtered;
  }, [entries, filter, newestFirst]);

  // 窄栏里日期不重复显示：只留 HH:MM:SS，完整时间戳放在 title 里
  const columns: ColumnsType<LogEntry> = [
    {
      title: "时间",
      dataIndex: "time",
      width: 64,
      className: "log-time",
      render: (value: string) => <span title={value}>{value ? shortTime(value) : "—"}</span>,
    },
    {
      title: "信息",
      dataIndex: "message",
      className: "log-message",
      render: (_: string, entry: LogEntry) => (
        <Flex gap={4} align="center">
          {/* 级别：两个字 + 级别色，固定槽位内水平垂直都居中（不再靠上） */}
          <span
            className={`log-level log-level-${entry.level}`}
            title={LOG_LEVEL_LABELS[entry.level]}
          >
            {LOG_LEVEL_LABELS[entry.level]}
          </span>
          {/* 正文最多两行，超出省略；完整内容悬浮可见 */}
          <span className="log-text" title={entry.message}>
            {entry.message}
          </span>
        </Flex>
      ),
    },
  ];

  return (
    <Card
      className="card log-card"
      size="small"
      title={
        <span>
          操作日志
          <span className="log-count">{entries.length} 条</span>
        </span>
      }
      extra={
        // 新日志置顶：标题栏里靠右，文字比标题小一号
        <Flex className="log-sort" align="center" gap={6}>
          <Typography.Text type="secondary">新日志置顶</Typography.Text>
          <Switch size="small" checked={newestFirst} onChange={setNewestFirst} />
        </Flex>
      }
    >
      {/* 筛选单独一行：每个栏目的文字颜色跟日志里对应级别一致（信息蓝 / 成功绿 / 警告橙 / 错误红） */}
      <div className="log-toolbar">
        <Segmented
          size="small"
          value={filter}
          onChange={(value) => setFilter(value as LogFilter)}
          options={[
            { label: <span className="log-filter log-filter-all">全部</span>, value: "all" },
            ...LOG_LEVELS.map((level) => ({
              label: (
                <span className={`log-filter log-filter-${level}`}>
                  {LOG_LEVEL_LABELS[level]}
                </span>
              ),
              value: level,
            })),
          ]}
        />
      </div>
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

/** 提示弹窗里的一行键值（如「信息类别 / UDI团队信息」「导入数量 / 34,712 行」）。 */
export type MessageDetail = { label: string; value: string };

export type MessageState = {
  title: string;
  message: string;
  /** 失败时标题前显示红色警示图标，成功时显示绿色对勾。 */
  failed?: boolean;
  /** 结构化明细：有它就把结果摊成两行键值，而不是丢一段纯文本。 */
  details?: MessageDetail[];
} | null;

/**
 * 导入 / 导出 / 错误统一提示：就是 antd 默认的 Modal（标题 + 右上角 X + 右对齐底栏），
 * 不再做 PySide 那套"红底标题条 + 居中按钮 + 去掉 X"的 QDialog 布局。
 * className 留作稳定的测试/定制挂点，本身不写任何样式。
 */
export function MessageModal({ state, onClose }: { state: MessageState; onClose: () => void }) {
  const failed = state?.failed ?? false;
  const details = state?.details ?? [];
  return (
    <Modal
      className="app-message"
      open={state !== null}
      title={
        // 标题前一个状态图标：成功绿对勾 / 失败红警示（颜色与日志里的级别色同一套）
        <Flex align="center" gap={8}>
          {failed ? (
            <CircleAlert className="message-icon message-icon-failed" size={18} aria-hidden="true" />
          ) : (
            <CheckCircle2 className="message-icon message-icon-ok" size={18} aria-hidden="true" />
          )}
          <span>{state?.title ?? ""}</span>
        </Flex>
      }
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
      {details.length > 0 ? (
        // 结果摊成 antd 默认的键值表：标签灰、值深色加粗
        <Descriptions
          className="message-details"
          column={1}
          size="small"
          colon={false}
          items={details.map((detail) => ({
            key: detail.label,
            label: detail.label,
            children: <Typography.Text strong>{detail.value}</Typography.Text>,
          }))}
        />
      ) : (
        <Typography.Paragraph className="message-text">{state?.message ?? ""}</Typography.Paragraph>
      )}
    </Modal>
  );
}
