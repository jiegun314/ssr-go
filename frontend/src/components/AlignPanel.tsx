// 数据对齐面板（P3，只有 Supply Chain / Admin 看到）：
// 四个来源各一张紧凑卡片：标题行（来源 + 状态 + 右上角「对齐」按钮）+ 共享盘版本 + 本机版本 + 发布时间。
// 宽度够时四个一行，窗口变窄自动换成 2×2（容器查询，见 styles.css 的 .align-grid-wrap）。
//
// 「发布状态（几份待对齐）」与「自动对齐」开关在模块标题栏里（App.tsx 的 Card extra），
// 这里只负责卡片本身，尽量压低高度。

import { Button, Tag } from "antd";

import type { SourceStatus } from "../types";

type AlignPanelProps = {
  sources: SourceStatus[];
  aligning: boolean;
  onAlign: (source: string) => void;
};

export function AlignPanel({ sources, aligning, onAlign }: AlignPanelProps) {
  return (
    <div className="align-grid-wrap">
      <div className="align-grid">
        {sources.map((source) => (
          <div className="align-source-card" key={source.source}>
            <div className="align-source-head">
              <span className="align-source-label">{source.label}</span>
              <Tag
                className="align-tag"
                color={source.aligned ? (source.pending ? "gold" : "green") : "orange"}
              >
                {source.aligned ? (source.pending ? "已对齐 · 待整合" : "已对齐") : "未对齐"}
              </Tag>
              <Button
                size="small"
                className="align-source-run"
                disabled={aligning || source.aligned || !source.remoteVersion}
                onClick={() => onAlign(source.source)}
              >
                对齐
              </Button>
            </div>
            <div className="align-source-line">
              <span className="align-key">共享盘</span>
              <span className="align-value">{source.remoteVersion || "未发布"}</span>
            </div>
            <div className="align-source-line">
              <span className="align-key">本机</span>
              <span className="align-value">{source.localVersion || "—"}</span>
            </div>
            <div className="align-source-meta">
              {source.publishedAt || "—"}
              {source.remoteRows ? ` · ${source.remoteRows.toLocaleString("zh-CN")} 行` : ""}
              {source.publishedBy ? ` · ${source.publishedBy}` : ""}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
