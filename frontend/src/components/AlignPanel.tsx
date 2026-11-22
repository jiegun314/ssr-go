// 数据对齐面板（P3，只有 Supply Chain / Admin 看到）：
// 四个来源各一张紧凑卡片：标题行（来源 + 状态 + 右上角「对齐」按钮）+ 发布时间摘要。
// 共享盘/本机版本不再直接展示，鼠标悬停在状态标签上时通过 Tooltip 浮出。
// 宽度够时四个一行，窗口变窄自动换成 2×2（容器查询，见 styles.css 的 .align-grid-wrap）。
//
// 「发布状态（几份待对齐）」与「自动对齐」开关在模块标题栏里（App.tsx 的 Card extra），
// 这里只负责卡片本身，尽量压低高度。

import { Tag, Tooltip } from "antd";
import { RefreshCw } from "lucide-react";

import { IconButton } from "./IconButton";

import type { SourceStatus } from "../types";

type AlignPanelProps = {
  sources: SourceStatus[];
  aligning: boolean;
  onAlign: (source: string) => void;
};

function alignTooltip(source: SourceStatus): React.ReactNode {
  return (
    <div style={{ lineHeight: "1.7", fontSize: 12 }}>
      <div>
        <span style={{ color: "rgba(0,0,0,0.45)" }}>共享盘：</span>
        <span>{source.remoteVersion || "未发布"}</span>
      </div>
      <div>
        <span style={{ color: "rgba(0,0,0,0.45)" }}>本机：</span>
        <span>{source.localVersion || "—"}</span>
      </div>
    </div>
  );
}

export function AlignPanel({ sources, aligning, onAlign }: AlignPanelProps) {
  return (
    <div className="align-grid-wrap">
      <div className="align-grid">
        {sources.map((source) => {
          const tagText = source.aligned
            ? source.pending
              ? "已对齐 · 待整合"
              : "已对齐"
            : "未对齐";
          const tagColor = source.aligned ? (source.pending ? "gold" : "green") : "orange";
          return (
            <div className="align-source-card" key={source.source}>
              <div className="align-source-head">
                <span className="align-source-label">{source.label}</span>
                <Tooltip title={alignTooltip(source)} mouseEnterDelay={0.1}>
                  <Tag className="align-tag" color={tagColor}>
                    {tagText}
                  </Tag>
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
              <div className="align-source-meta">
                {source.publishedAt || "—"}
                {source.remoteRows ? ` · ${source.remoteRows.toLocaleString("zh-CN")} 行` : ""}
                {source.publishedBy ? ` · ${source.publishedBy}` : ""}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
