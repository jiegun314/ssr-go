// 数据对齐面板（P3，只有 Supply Chain / Admin 看到）：
// 四个来源各一张小卡片（共享盘版本 / 本机版本 / 发布时间与发布人 / 状态 / 对齐按钮），
// 宽度够时四个一行，窗口变窄自动换成 2×2（容器查询，见 styles.css 的 .align-grid-wrap）。
//
// 口径（docs/design/multiuser-workflow.md §8.3）：
//   - 本机版本 ≠ 共享盘版本 = 未对齐（整合按钮保持置灰）；
//   - 「已对齐 · 待整合」表示已对齐但这次版本还没被整合过；
//   - 顶部工具栏只有"待对齐数量 + 自动对齐开关"，「全部对齐」在模块标题栏里。

import { Button, Switch, Tag, Typography } from "antd";

import type { SourceStatus } from "../types";

type AlignPanelProps = {
  sources: SourceStatus[];
  autoAlign: boolean;
  aligning: boolean;
  onAutoAlignChange: (value: boolean) => void;
  onAlign: (source: string) => void;
};

export function AlignPanel({
  sources,
  autoAlign,
  aligning,
  onAutoAlignChange,
  onAlign,
}: AlignPanelProps) {
  const unaligned = sources.filter((source) => !source.aligned && source.remoteVersion);
  const unpublished = sources.filter((source) => !source.remoteVersion);

  return (
    <div className="align-panel">
      <div className="align-toolbar">
        {unaligned.length > 0 ? (
          <Tag className="align-count" color="orange">
            {unaligned.length} 份待对齐
          </Tag>
        ) : unpublished.length > 0 ? (
          <Tag className="align-count">{unpublished.length} 份未发布</Tag>
        ) : (
          <Tag className="align-count" color="green">
            全部已对齐
          </Tag>
        )}
        <span className="align-auto">
          <Typography.Text type="secondary">自动对齐</Typography.Text>
          <Switch size="small" checked={autoAlign} onChange={onAutoAlignChange} />
        </span>
      </div>

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
              <Button
                size="small"
                className="align-source-run"
                disabled={aligning || source.aligned || !source.remoteVersion}
                onClick={() => onAlign(source.source)}
              >
                对齐
              </Button>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
