// 数据对齐面板（P3，只有 Supply Chain / Admin 看到）：
// 四行"来源 / 本机版本 → 共享盘版本 / 发布时间 / 状态 / 对齐按钮"，
// 顶部有「自动对齐」开关与「全部对齐」按钮。
//
// 口径（docs/design/multiuser-workflow.md §8.3）：
//   - 本机版本 ≠ 共享盘版本 = 未对齐（整合按钮保持置灰）；
//   - 对齐后若共享盘又发布新版本，会重新回到未对齐；
//   - 「已对齐 · 待整合」表示已对齐但这次版本还没被整合过。

import { Button, Switch, Tag, Typography } from "antd";
import { RefreshCw } from "lucide-react";

import type { SourceStatus } from "../types";

type AlignPanelProps = {
  sources: SourceStatus[];
  autoAlign: boolean;
  aligning: boolean;
  onAutoAlignChange: (value: boolean) => void;
  onAlignAll: () => void;
  onAlign: (source: string) => void;
};

export function AlignPanel({
  sources,
  autoAlign,
  aligning,
  onAutoAlignChange,
  onAlignAll,
  onAlign,
}: AlignPanelProps) {
  const unaligned = sources.filter((source) => !source.aligned && source.remoteVersion);
  const unpublished = sources.filter((source) => !source.remoteVersion);

  return (
    <div className="align-panel">
      <div className="align-head">
        <span className="align-title">数据对齐</span>
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
        <Button
          size="small"
          icon={<RefreshCw size={14} />}
          aria-label="全部对齐"
          loading={aligning}
          disabled={aligning || unaligned.length === 0}
          onClick={onAlignAll}
        >
          全部对齐
        </Button>
      </div>
      <div className="align-rows">
        {sources.map((source) => (
          <div className="align-row" key={source.source}>
            <span className="align-label">{source.label}</span>
            <span className="align-version" title="本机版本 → 共享盘版本">
              {source.localVersion || "—"} → {source.remoteVersion || "（未发布）"}
            </span>
            <span className="align-time">{source.publishedAt || ""}</span>
            <Tag
              className="align-tag"
              color={source.aligned ? (source.pending ? "gold" : "green") : "orange"}
            >
              {source.aligned ? (source.pending ? "已对齐 · 待整合" : "已对齐") : "未对齐"}
            </Tag>
            <Button
              size="small"
              disabled={aligning || source.aligned || !source.remoteVersion}
              onClick={() => onAlign(source.source)}
            >
              对齐
            </Button>
          </div>
        ))}
      </div>
    </div>
  );
}
