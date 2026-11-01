// 运行状态（P5，Admin；SC 也可看审计）：共享盘、四份来源、最后整合、整合锁、维护操作。
//
// 数据来自 ShareOverview（与对齐面板同一个来源），审计走 ReviewAudit（分页窗口）。

import { Button, Descriptions, InputNumber, Modal, Table, Tag, Tooltip } from "antd";
import type { ColumnsType } from "antd/es/table";
import { RefreshCw, ShieldAlert, Trash2 } from "lucide-react";
import { useState } from "react";

import type { ShareOverview, SourceStatus } from "../types";

type StatusModalProps = {
  open: boolean;
  report: ShareOverview | null;
  cleaning: boolean;
  /** 只有 Admin 能清理旧版本 */
  canCleanup: boolean;
  onRecheck: () => void;
  onCleanup: (keep: number) => void;
  onReviewAudit: () => void;
  onClose: () => void;
};

export function StatusModal({
  open,
  report,
  cleaning,
  canCleanup,
  onRecheck,
  onCleanup,
  onReviewAudit,
  onClose,
}: StatusModalProps) {
  const [keep, setKeep] = useState(5);
  const sources = report?.sources ?? [];
  const last = report?.lastConsolidation;
  const running = report?.running;

  const columns: ColumnsType<SourceStatus> = [
    { title: "来源", dataIndex: "label", key: "label", width: 120 },
    {
      title: "共享盘版本",
      dataIndex: "remoteVersion",
      key: "remoteVersion",
      render: (value: string) => value || "未发布",
    },
    { title: "发布时间", dataIndex: "publishedAt", key: "publishedAt", width: 150 },
    { title: "发布人", dataIndex: "publishedBy", key: "publishedBy", width: 90 },
    {
      title: "行数",
      dataIndex: "remoteRows",
      key: "remoteRows",
      width: 80,
      render: (value: number) => (value ? value.toLocaleString("zh-CN") : "—"),
    },
    {
      title: "本机",
      dataIndex: "aligned",
      key: "aligned",
      width: 90,
      render: (aligned: boolean, source) =>
        aligned ? <Tag color="green">已对齐</Tag> : source.remoteVersion ? <Tag color="orange">未对齐</Tag> : "—",
    },
  ];

  return (
    <Modal
      open={open}
      title="运行状态"
      width={760}
      onCancel={onClose}
      footer={<Button onClick={onClose}>关闭</Button>}
      destroyOnHidden
    >
      <div className="status-modal">
        <section className="status-section">
          <div className="status-section-head">
            <span className="status-section-title">共享盘</span>
            {report?.status ? (
              <Tag color={report.status.healthy ? "green" : "red"}>
                {report.status.mode === "local" ? "本地模式" : report.status.healthy ? "正常" : "异常"}
              </Tag>
            ) : null}
            <Button size="small" icon={<RefreshCw size={14} />} onClick={onRecheck}>
              重新检查
            </Button>
          </div>
          <div className="status-section-body">{report?.status?.detail ?? "—"}</div>
        </section>

        <section className="status-section">
          <div className="status-section-head">
            <span className="status-section-title">四份来源</span>
            {running ? (
              <Tag color="gold" icon={<ShieldAlert size={12} />}>
                {running.owner}（{running.role}）正在整合，自 {running.since}
              </Tag>
            ) : (
              <Tag>整合锁：空闲</Tag>
            )}
          </div>
          <Table<SourceStatus>
            size="small"
            rowKey="source"
            pagination={false}
            dataSource={sources}
            columns={columns}
          />
        </section>

        <section className="status-section">
          <div className="status-section-head">
            <span className="status-section-title">最后一次整合</span>
          </div>
          {last ? (
            <Descriptions size="small" column={2} bordered>
              <Descriptions.Item label="时间">{last.at}</Descriptions.Item>
              <Descriptions.Item label="操作人">{last.by}</Descriptions.Item>
              <Descriptions.Item label="合格 / 缺失">
                {last.counts.ready ?? last.counts.ready} / {last.counts.missing}
              </Descriptions.Item>
              <Descriptions.Item label="重复 / 冲突">
                {last.counts.duplicate} / {last.counts.conflict}
              </Descriptions.Item>
              <Descriptions.Item label="结果文件" span={2}>
                {last.result ? (
                  <Tooltip title={last.result}>
                    <span className="status-result">{last.result}</span>
                  </Tooltip>
                ) : (
                  "还没有导出（点「生成文件」后会归档）"
                )}
              </Descriptions.Item>
            </Descriptions>
          ) : (
            <div className="status-section-body">还没有整合过。</div>
          )}
        </section>

        <section className="status-section">
          <div className="status-section-head">
            <span className="status-section-title">维护</span>
          </div>
          <div className="status-maintenance">
            <span>清理共享盘旧版本，保留每个来源最近</span>
            <InputNumber
              size="small"
              min={1}
              max={50}
              value={keep}
              onChange={(value) => setKeep(value ?? 5)}
              aria-label="保留份数"
            />
            <span>份（当前版本与最后整合用到的版本始终保留）</span>
            <Button
              size="small"
              danger
              icon={<Trash2 size={14} />}
              loading={cleaning}
              disabled={!canCleanup}
              onClick={() => onCleanup(keep)}
            >
              清理
            </Button>
            <Button size="small" onClick={onReviewAudit}>
              查看审计
            </Button>
          </div>
        </section>
      </div>
    </Modal>
  );
}
