// 数据对齐面板（P3，只有 Supply Chain / Admin 看到）：
// 四个来源各一张紧凑卡片：标题行（来源 + 状态图标 + 右上角「对齐」按钮）+ 发布时间摘要。
// 状态采用小图标指示（已对齐 / 待整合 / 未对齐），鼠标悬停时通过 Tooltip 浮出具体状态与版本。
// 发布时间采用 YY/MM/DD 紧凑格式，鼠标悬停显示完整发布时间。
//
// 「发布状态（几份待对齐）」与「自动对齐」开关在模块标题栏里（App.tsx 的 Card extra），
// 这里只负责卡片本身，尽量压低高度并杜绝内容溢出。

import { Tooltip } from "antd";
import { AlertCircle, CheckCircle2, Clock, MinusCircle, RefreshCw } from "lucide-react";

import { IconButton } from "./IconButton";

import type { SourceStatus } from "../types";

type AlignPanelProps = {
  sources: SourceStatus[];
  aligning: boolean;
  onAlign: (source: string) => void;
};

function alignTooltip(source: SourceStatus, statusText: string): React.ReactNode {
  return (
    <div style={{ lineHeight: "1.7", fontSize: 12 }}>
      <div>
        <span style={{ color: "rgba(255,255,255,0.7)" }}>当前状态：</span>
        <strong style={{ color: "#fff" }}>{statusText}</strong>
      </div>
      <div>
        <span style={{ color: "rgba(255,255,255,0.7)" }}>共享盘：</span>
        <span>{source.remoteVersion || "未发布"}</span>
      </div>
      <div>
        <span style={{ color: "rgba(255,255,255,0.7)" }}>本机：</span>
        <span>{source.localVersion || "—"}</span>
      </div>
    </div>
  );
}

/** 将 ISO 日期字符串格式化为 YY/MM/DD 格式 */
function formatShortDate(publishedAt?: string): string {
  if (!publishedAt) return "—";
  const match = publishedAt.match(/(\d{2,4})[-/](\d{1,2})[-/](\d{1,2})/);
  if (match) {
    const year = match[1].slice(-2);
    const month = match[2].padStart(2, "0");
    const day = match[3].padStart(2, "0");
    return `${year}/${month}/${day}`;
  }
  return publishedAt;
}

export function AlignPanel({ sources, aligning, onAlign }: AlignPanelProps) {
  return (
    <div className="align-grid-wrap">
      <div className="align-grid">
        {sources.map((source) => {
          const tagText = !source.remoteVersion
            ? "未发布"
            : source.aligned
              ? source.pending
                ? "已对齐 · 待整合"
                : "已对齐"
              : "未对齐";

          const tagColor = !source.remoteVersion
            ? "default"
            : source.aligned
              ? source.pending
                ? "gold"
                : "green"
              : "orange";

          const StatusIcon = !source.remoteVersion
            ? MinusCircle
            : source.aligned
              ? source.pending
                ? Clock
                : CheckCircle2
              : AlertCircle;

          return (
            <div className="align-source-card" key={source.source}>
              <div className="align-source-head">
                <span className="align-source-label" title={source.label}>
                  {source.label}
                </span>
                <Tooltip title={alignTooltip(source, tagText)} mouseEnterDelay={0.1}>
                  <span
                    className={`align-status-badge align-status-${tagColor}`}
                    role="img"
                    aria-label={tagText}
                  >
                    <StatusIcon size={12} strokeWidth={2.4} />
                    <span className="sr-only">{tagText}</span>
                  </span>
                </Tooltip>
                <IconButton
                  className="align-source-run"
                  label="对齐"
                  size="middle"
                  tooltip={
                    source.aligned
                      ? "本机已经是共享盘最新版本"
                      : !source.remoteVersion
                        ? "共享盘还没有这个来源的版本"
                        : "对齐到共享盘最新版本"
                  }
                  icon={<RefreshCw />}
                  disabled={aligning || source.aligned || !source.remoteVersion}
                  onClick={() => onAlign(source.source)}
                />
              </div>
              <div
                className="align-source-meta"
                title={`发布时间：${source.publishedAt || "—"}${source.remoteRows ? `\n数据量：${source.remoteRows.toLocaleString("zh-CN")} 行` : ""}${source.publishedBy ? `\n操作人：${source.publishedBy}` : ""}`}
              >
                <span className="align-meta-visible">
                  {formatShortDate(source.publishedAt)}
                  {source.remoteRows ? ` · ${source.remoteRows.toLocaleString("zh-CN")}行` : ""}
                </span>
                <span className="sr-only">
                  {source.publishedAt || "—"}
                  {source.remoteRows ? ` · ${source.remoteRows.toLocaleString("zh-CN")} 行` : ""}
                  {source.publishedBy ? ` · ${source.publishedBy}` : ""}
                </span>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
