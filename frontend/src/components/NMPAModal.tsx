import { Alert, Button, Drawer, Input, Modal, Pagination, Spin, Table, Tag, message } from "antd";
import {
  CloudDownload,
  Database,
  Download,
  FileDown,
  RefreshCw,
  Search,
  ShieldCheck,
} from "lucide-react";
import { useEffect, useState } from "react";

import { call } from "../bridge";
import type {
  ExportResult,
  ExportTarget,
  NMPADeviceDetail,
  NMPAQueryResult,
  NMPAStatus,
  NMPASyncResult,
} from "../types";

type Props = {
  open: boolean;
  onClose: () => void;
  canManage: boolean;
};

const PAGE_SIZE = 20;

function formatBytes(value: number): string {
  if (!value || value < 0) {
    return "—";
  }
  if (value < 1024 * 1024) {
    return `${Math.max(1, Math.round(value / 1024))} KB`;
  }
  return `${(value / (1024 * 1024)).toFixed(1)} MB`;
}

function formatTime(value?: string): string {
  return value || "—";
}

/**
 * 药监 UDI 参考库：独立于导入/整合流程，Admin / Supply Chain 可查询本企业数据。
 *
 * 数据来自药监 RSS 发布包，按 setting.yaml 的统一社会信用代码过滤后写入
 * 独立的 nmpa_udi.sqlite3；这里不出现"导入到 UDI 来源"之类的动作。
 * 自动检查只读 RSS；全量/周更仍必须由 Admin 手动执行。
 */
export function NMPAModal({ open, onClose, canManage }: Props) {
  const [status, setStatus] = useState<NMPAStatus | null>(null);
  const [query, setQuery] = useState("");
  const [appliedQuery, setAppliedQuery] = useState("");
  const [page, setPage] = useState(1);
  const [result, setResult] = useState<NMPAQueryResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [checking, setChecking] = useState(false);
  const [exporting, setExporting] = useState(false);
  const [detail, setDetail] = useState<NMPADeviceDetail | null>(null);
  const [detailOpen, setDetailOpen] = useState(false);
  const [detailLoading, setDetailLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) {
      return;
    }
    setPage(1);
    setQuery("");
    setAppliedQuery("");
    setResult(null);
    setError(null);
    setDetail(null);
    setDetailOpen(false);
    void loadStatus(true);
    void loadSearch(1, "");
    const timer = window.setInterval(() => void loadStatus(true), 30 * 60 * 1000);
    return () => window.clearInterval(timer);
    // open 变化时才重新拉取；查询动作由按钮触发。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  async function loadStatus(auto = false) {
    const next = auto
      ? await call<NMPAStatus>("NMPAAutoCheck")
      : await call<NMPAStatus>("NMPAStatus");
    if (!next && auto) {
      // 旧预览/旧后端没有自动检查方法时退回只读状态，不阻塞打开窗口。
      return loadStatus(false);
    }
    if (next) {
      setStatus(next);
      if (next.error) {
        setError(next.error);
      }
    }
  }

  async function loadSearch(targetPage: number, text: string) {
    setLoading(true);
    setError(null);
    try {
      const next = await call<NMPAQueryResult>("NMPASearch", text, targetPage, PAGE_SIZE);
      if (next) {
        setResult(next);
        setPage(targetPage);
        setAppliedQuery(text);
      } else {
        setError("查询药监参考库失败，请确认数据库状态后重试。");
      }
    } finally {
      setLoading(false);
    }
  }

  function search() {
    void loadSearch(1, query.trim());
  }

  async function checkUpdates() {
    setChecking(true);
    setError(null);
    try {
      const next = await call<NMPAStatus>("NMPACheck");
      if (next) {
        setStatus(next);
        if (next.error) {
          setError(next.error);
        } else {
          message.success("药监发布索引已更新");
        }
      } else {
        setError("检查药监更新失败，请确认网络和共享盘状态。");
      }
    } finally {
      setChecking(false);
    }
  }

  async function exportResults() {
    setExporting(true);
    setError(null);
    try {
      const target = await call<ExportTarget>("SelectNMPAExportTarget");
      if (!target?.target) {
        return;
      }
      const result = await call<ExportResult>("NMPAExport", appliedQuery, target.target);
      if (!result) {
        setError("导出药监查询结果失败，请查看操作日志。");
        return;
      }
      if (result.failed) {
        setError(result.message || "导出药监查询结果失败。");
        message.error(result.message || "导出失败");
        return;
      }
      message.success(result.message || "已导出");
    } finally {
      setExporting(false);
    }
  }

  async function openDetail(primaryDI: string) {
    if (!primaryDI) {
      return;
    }
    setDetailOpen(true);
    setDetail(null);
    setDetailLoading(true);
    try {
      const next = await call<NMPADeviceDetail>("NMPADetail", primaryDI);
      if (next) {
        setDetail(next);
      } else {
        setError("读取设备详情失败，请稍后重试。");
      }
    } finally {
      setDetailLoading(false);
    }
  }

  function syncFull() {
    Modal.confirm({
      title: "同步药监全量数据？",
      content:
        "会下载约 338MB 的药监全量包，只保留当前企业统一社会信用代码下的设备，并替换本机参考库。全量同步可能需要几分钟。",
      okText: "开始同步",
      cancelText: "取消",
      onOk: async () => {
        const next = await call<NMPASyncResult>("NMPASyncFull");
        if (!next) {
          setError("全量同步失败，请查看操作日志。");
          return;
        }
        if (next.status) {
          setStatus(next.status);
        }
        if (next.failed) {
          setError(next.message || "全量同步失败。");
          message.error(next.message || "全量同步失败");
        } else {
          message.success(next.message);
        }
        void loadSearch(1, appliedQuery);
      },
    });
  }

  async function syncWeekly() {
    setError(null);
    const next = await call<NMPASyncResult>("NMPASyncWeekly");
    if (!next) {
      setError("周更同步失败，请查看操作日志。");
      return;
    }
    if (next.status) {
      setStatus(next.status);
    }
    if (next.failed) {
      setError(next.message || "周更同步失败。");
      message.error(next.message || "周更同步失败");
    } else {
      message.success(next.message);
    }
    void loadSearch(1, appliedQuery);
  }

  const columns = (result?.columns ?? []).map((title, index) => ({
    title,
    dataIndex: String(index),
    key: String(index),
    ellipsis: { showTitle: true },
    width: [110, 150, 110, 80, 150, 140, 85, 80][index] ?? 100,
  }));
  const rows = (result?.rows ?? []).map((row, index) => {
    const item: Record<string, string> = { key: `${index}:${row[0] ?? ""}` };
    row.forEach((cell, cellIndex) => {
      item[String(cellIndex)] = cell;
    });
    return item;
  });

  const hasData = (status?.deviceCount ?? 0) > 0;
  const weeklyLabel = status?.weeklyAvailable ? "有可用周更" : "暂无新周更";

  return (
    <>
    <Modal
      className="nmpa-modal"
      open={open}
      width="min(94vw, 1080px)"
      title={
        <div className="nmpa-title">
          <span className="nmpa-title-icon">
            <ShieldCheck size={18} />
          </span>
          <span>
            <strong>药监 UDI 参考库</strong>
            <small>独立于导入与整合，仅用于本机查询</small>
          </span>
        </div>
      }
      onCancel={onClose}
      footer={
        <Button onClick={onClose}>关闭</Button>
      }
    >
      <div className="nmpa-body">
        <div className="nmpa-hero">
          <div>
            <div className="nmpa-code-line">
              <span className="nmpa-code-label">企业代码</span>
              <code>{status?.socialCreditCode || "未配置"}</code>
              {status?.codeConfigured ? <Tag color="green">已配置</Tag> : <Tag>未配置</Tag>}
            </div>
            <p>
              药监 RSS 每月全量、每周增量。全量包先按统一社会信用代码筛选，再写入本地
              <code>nmpa_udi.sqlite3</code>；不会写入 UDI 来源或参与整合。
            </p>
          </div>
          <div className="nmpa-actions">
            {canManage ? (
              <>
                <Button
                  icon={<RefreshCw size={14} />}
                  loading={checking}
                  onClick={() => void checkUpdates()}
                >
                  检查更新
                </Button>
                <Button icon={<CloudDownload size={14} />} onClick={syncFull}>
                  同步全量
                </Button>
                <Button type="primary" icon={<Download size={14} />} onClick={() => void syncWeekly()}>
                  更新周更
                </Button>
              </>
            ) : (
              <span className="nmpa-manage-hint">发布检查自动进行；全量与周更更新仅 Admin 可操作。</span>
            )}
          </div>
        </div>

        {error ? <Alert className="nmpa-alert" type="error" showIcon message={error} /> : null}
        {!status?.codeConfigured ? (
          <Alert
            className="nmpa-alert"
            type="warning"
            showIcon
            message="还没有配置统一社会信用代码"
            description="请先在「参数设定 → 基础设置」填写 social_credit_code，再同步药监数据。"
          />
        ) : null}
        {status?.codeChanged ? (
          <Alert
            className="nmpa-alert"
            type="warning"
            showIcon
            message="统一社会信用代码已变化"
            description="本地库仍是旧企业的数据，请重新执行全量同步。"
          />
        ) : null}
        {status?.needsFull ? (
          <Alert
            className="nmpa-alert"
            type="info"
            showIcon
            message="本地参考库还没有全量基线"
            description="首次使用请先同步全量；之后的周更会在此基础上叠加。"
          />
        ) : null}
        {status?.fullDue ? (
          <Alert
            className="nmpa-alert"
            type="info"
            showIcon
            message="有新的药监全量包"
            description="请先同步全量，再应用周更；周更会在最新全量基线上叠加。"
          />
        ) : null}

        <div className="nmpa-metrics">
          <div>
            <span>本机设备</span>
            <strong>{status?.deviceCount ?? 0}</strong>
            <small>按 Primary DI 去重</small>
          </div>
          <div>
            <span>包装层级</span>
            <strong>{status?.packageCount ?? 0}</strong>
            <small>独立子表</small>
          </div>
          <div>
            <span>联系人</span>
            <strong>{status?.contactCount ?? 0}</strong>
            <small>独立子表</small>
          </div>
          <div>
            <span>本地库</span>
            <strong>{formatBytes(status?.databaseBytes ?? 0)}</strong>
            <small>{status?.databasePath || "—"}</small>
          </div>
        </div>

        <div className="nmpa-sync-line">
          <span>全量：{status?.lastFullFile || "尚未同步"}</span>
          <span>最近同步：{formatTime(status?.lastSyncAt)}</span>
          <span>最近检查：{formatTime(status?.lastCheckAt)}</span>
          <span>{weeklyLabel}</span>
        </div>
        {status?.lastError ? (
          <Alert className="nmpa-alert" type="warning" showIcon message={`上次同步提示：${status.lastError}`} />
        ) : null}

        <div className="nmpa-search-head">
          <div>
            <strong>企业数据查询</strong>
            <small>{hasData ? `共 ${status?.deviceCount ?? 0} 台设备，按 Primary DI 展示` : "同步全量后可查询"}</small>
          </div>
          <div className="nmpa-search">
            <Input
              allowClear
              value={query}
              placeholder="Primary DI、产品名称、注册证编号…"
              onChange={(event) => setQuery(event.target.value)}
              onPressEnter={search}
            />
            <Button type="primary" icon={<Search size={14} />} onClick={search} loading={loading}>
              查找
            </Button>
            <Button
              icon={<FileDown size={14} />}
              onClick={() => void exportResults()}
              loading={exporting}
              disabled={!result || result.total === 0}
            >
              导出
            </Button>
          </div>
        </div>

        <Table
          className="nmpa-table"
          tableLayout="fixed"
          size="small"
          rowKey="key"
          columns={columns}
          dataSource={rows}
          loading={loading}
          pagination={false}
          scroll={{ x: 905 }}
          onRow={(record) => ({
            onClick: () => void openDetail(record["0"] ?? ""),
          })}
          locale={{
            emptyText: (
              <div className="nmpa-empty">
                <Database size={24} />
                <span>{hasData ? "没有匹配的设备" : "还没有同步药监数据"}</span>
              </div>
            ),
          }}
        />
        {result && result.total > 0 ? (
          <div className="nmpa-table-hint">点击行查看全部 47 个字段；单元格内容过长时，可悬停查看完整值。</div>
        ) : null}
        {result && result.total > 0 ? (
          <div className="nmpa-pager">
            <span>共 {result.total} 条</span>
            <Pagination
              size="small"
              current={page}
              pageSize={PAGE_SIZE}
              total={result.total}
              showSizeChanger={false}
              onChange={(nextPage) => void loadSearch(nextPage, appliedQuery)}
            />
          </div>
        ) : null}
      </div>
    </Modal>
    <NMPADetailDrawer
      open={detailOpen}
      detail={detail}
      loading={detailLoading}
      onClose={() => setDetailOpen(false)}
    />
    </>
  );
}

function NMPADetailDrawer({
  open,
  detail,
  loading,
  onClose,
}: {
  open: boolean;
  detail: NMPADeviceDetail | null;
  loading: boolean;
  onClose: () => void;
}) {
  const packageColumns = [
    { title: "包装 DI", dataIndex: "packageDi", key: "packageDi", ellipsis: true },
    { title: "包装类型", dataIndex: "packageType", key: "packageType", width: 110 },
    { title: "下级数量", dataIndex: "lowerLevelPackageQuantity", key: "lowerLevelPackageQuantity", width: 90 },
    { title: "下级包装标识", dataIndex: "lowerLevelPackageIdentifier", key: "lowerLevelPackageIdentifier", ellipsis: true },
  ];
  const contactColumns = [
    { title: "传真", dataIndex: "fax", key: "fax", ellipsis: true },
    { title: "邮箱", dataIndex: "email", key: "email", ellipsis: true },
    { title: "电话", dataIndex: "phone", key: "phone", width: 150 },
  ];
  return (
    <Drawer
      className="nmpa-drawer"
      open={open}
      onClose={onClose}
      width={760}
      zIndex={1100}
      title={
        detail ? (
          <div className="nmpa-detail-title">
            <strong>{detail.displayName || detail.primaryDi}</strong>
            <small>{detail.primaryDi}</small>
          </div>
        ) : "设备详情"
      }
      footer={<Button onClick={onClose}>关闭</Button>}
    >
      {loading ? (
        <div className="nmpa-detail-loading"><Spin /></div>
      ) : detail ? (
        <div className="nmpa-detail">
          <div className="nmpa-detail-tags">
            <Tag color="blue">{detail.versionStatus || "版本未知"}</Tag>
            <Tag>{detail.productCategory || "未分类"}</Tag>
            <span>共 {detail.groups.reduce((sum, group) => sum + group.fields.length, 0)} 个字段</span>
          </div>
          {detail.groups.map((group) => (
            <section className="nmpa-detail-group" key={group.title}>
              <h4>{group.title}</h4>
              <div className="nmpa-detail-grid">
                {group.fields.map((field) => (
                  <div className="nmpa-detail-field" key={`${group.title}-${field.label}`}>
                    <span>{field.label}</span>
                    <strong title={field.value}>{field.value || "—"}</strong>
                  </div>
                ))}
              </div>
            </section>
          ))}
          <section className="nmpa-detail-group">
            <h4>包装层级 <small>{detail.packages.length} 条</small></h4>
            <Table
              className="nmpa-detail-table"
              tableLayout="fixed"
              size="small"
              rowKey="packageDi"
              columns={packageColumns}
              dataSource={detail.packages}
              pagination={false}
              locale={{ emptyText: "无包装层级数据" }}
            />
          </section>
          <section className="nmpa-detail-group">
            <h4>联系人 <small>{detail.contacts.length} 条</small></h4>
            <Table
              className="nmpa-detail-table"
              tableLayout="fixed"
              size="small"
              rowKey="phone"
              columns={contactColumns}
              dataSource={detail.contacts}
              pagination={false}
              locale={{ emptyText: "无联系人数据" }}
            />
          </section>
        </div>
      ) : (
        <div className="nmpa-detail-empty">未读取到设备详情</div>
      )}
    </Drawer>
  );
}
