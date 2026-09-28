// 主界面的四块：数据导入、记录导出、数据整合、操作日志，加上载入图层与结果弹窗。

import { Button, DatePicker, Empty, Input, Modal, Spin, Table, Tooltip } from "antd";
import type { ColumnsType } from "antd/es/table";
import dayjs from "dayjs";
import { FileDown, FileText, FolderOpen, Merge, Search, Trash2 } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { GROUP_GAP_AFTER, SOURCES } from "../bridge";
import type { ImportState } from "../types";

/* ---------- 数据导入 ---------- */

type ImportPanelProps = {
  states: Record<string, ImportState>;
  onImport: (source: string) => void;
  onReview: (source: string) => void;
  onClear: () => void;
};

export function ImportPanel({ states, onImport, onReview, onClear }: ImportPanelProps) {
  return (
    <div>
      {SOURCES.map((source, index) => {
        const current = states[source.key] ?? { state: "empty", label: "", tooltip: "" };
        return (
          <div key={source.key}>
            <div className="group">
              <div className="title-row">
                <span className={`dot ${current.state}`} title={current.tooltip ?? ""} />
                <span className="group-title">{source.title}</span>
              </div>
              <div className="actions">
                <Tooltip title="载入文件">
                  <Button
                    type="text"
                    shape="circle"
                    aria-label="载入文件"
                    icon={<FolderOpen size={18} />}
                    onClick={() => onImport(source.key)}
                  />
                </Tooltip>
                {/* 状态标签在两个按钮中间（原界面就是这个顺序） */}
                <span className="status" id={`status-${source.key}`}>
                  {current.label || "尚未导入"}
                </span>
                <Tooltip title="数据回顾">
                  <Button
                    type="text"
                    shape="circle"
                    aria-label="数据回顾"
                    icon={<FileText size={18} />}
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
      <div className="clean-row">
        <Tooltip title="清空导入数据">
          <Button
            type="text"
            shape="circle"
            aria-label="清空导入数据"
            icon={<Trash2 size={18} />}
            onClick={onClear}
          />
        </Tooltip>
      </div>
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
        <span className="date-sep" aria-hidden="true" />
        {picker(end, onEndChange, "结束日期")}
      </div>
      <span className="spacer" />
      <Tooltip title="回顾">
        <Button type="text" shape="circle" aria-label="回顾" icon={<Search size={18} />} onClick={onReview} />
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
    <div className="status-strip" id="consolidation-summary">
      {(["Ready", "Incomplete", "Duplicate"] as const).map((status) => (
        <span className="strip-item" key={status}>
          <span className={`dot status-${status.toLowerCase()}`} />
          {status} <b>{counts[status] ?? 0}</b>
        </span>
      ))}
      {(counts.Conflict ?? 0) > 0 && (
        <span className="strip-item">
          <span className="dot status-conflict" />
          Conflict <b>{counts.Conflict}</b>
        </span>
      )}
    </div>
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
          bordered
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

/**
 * 操作日志卡片：上沿可拖动调高。
 * 上限＝左侧「数据导入 / 记录导出」只剩最小间隔（12px）；下限＝启动时的默认高度。
 * 拖动时卡片改成固定高度（flex:0 0 auto），让出的高度全部给中间列。
 */
export function LogPanel({ text }: { text: string }) {
  const cardRef = useRef<HTMLDivElement>(null);
  const [pinned, setPinned] = useState<number | null>(null);
  const defaultHeight = useRef<number>(0);

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
    <div
      className="card log-card"
      ref={cardRef}
      style={pinned === null ? undefined : { flex: "0 0 auto", height: `${pinned}px` }}
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
      <h2 className="title-bar">操作日志</h2>
      <div className="card-body">
        <Input.TextArea
          className="log-area"
          id="log"
          readOnly
          value={text}
          spellCheck={false}
        />
      </div>
    </div>
  );
}

/* ---------- 载入图层 ---------- */

export function LoadingOverlay({ text }: { text: string | null }) {
  if (text === null) return null;
  return (
    <div className="loading-layer" id="loading">
      <div className="loading-card">
        <Spin size="large" />
        <div className="loading-text" id="loading-text">
          {text}
        </div>
      </div>
    </div>
  );
}

/* ---------- 结果弹窗 ---------- */

export type MessageState = { title: string; message: string } | null;

export function MessageModal({ state, onClose }: { state: MessageState; onClose: () => void }) {
  return (
    <Modal
      className="app-message"
      open={state !== null}
      title={state?.title ?? ""}
      centered
      closable={false}
      width={360} /* 内容就两行中文，窄一点更紧凑；高度由内容决定，短消息时与原来一致 */
      onCancel={onClose}
      footer={
        <Button type="primary" onClick={onClose}>
          确定
        </Button>
      }
      destroyOnClose
    >
      <p className="message-text">{state?.message ?? ""}</p>
    </Modal>
  );
}
