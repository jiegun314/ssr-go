import { Alert, Button, Drawer, Empty, Spin, Table, Tag, Tooltip } from "antd";
import { useEffect, useMemo, useState } from "react";

import { call } from "../bridge";
import { statusLabel } from "../theme";
import type { NMPADeviceDetail } from "../types";
import type { app } from "../../wailsjs/go/models";

type ConsolidationDetailDrawerProps = {
  open: boolean;
  onClose: () => void;
  columns: string[];
  cells: string[];
  status: string;
  notes?: (app.ConflictCell | null)[];
  rowConflict?: string;
};

const PRIMARY_DI_NAMES = new Set(["primary di", "primary_di", "primarydi"]);
const STATUS_NAMES = new Set(["status", "状态"]);

function normalize(value: string): string {
  return String(value ?? "").trim();
}

function isMissingValue(value: string): boolean {
  const text = normalize(value);
  return text === "" || text === "MISSING" || text === "缺失";
}

function displayValue(value: string): string {
  return isMissingValue(value) ? "缺失" : value;
}

function primaryDIIndex(columns: string[]): number {
  return columns.findIndex((title) => PRIMARY_DI_NAMES.has(title.trim().toLowerCase()));
}

function conflictText(note: app.ConflictCell | null | undefined): string {
  if (!note?.source) return "";
  const candidates = (note.candidates ?? []).filter(Boolean).join(" / ");
  return candidates ? `${note.source}：${candidates}` : `${note.source} 的候选值不一致`;
}

function NMPADetailBody({ detail }: { detail: NMPADeviceDetail }) {
  const groups = detail.groups ?? [];
  const packages = detail.packages ?? [];
  const contacts = detail.contacts ?? [];
  const packageColumns = [
    { title: "包装 DI", dataIndex: "packageDi", key: "packageDi", ellipsis: true },
    { title: "包装类型", dataIndex: "packageType", key: "packageType", width: 100 },
    { title: "下级数量", dataIndex: "lowerLevelPackageQuantity", key: "lowerLevelPackageQuantity", width: 80 },
    { title: "下级包装标识", dataIndex: "lowerLevelPackageIdentifier", key: "lowerLevelPackageIdentifier", ellipsis: true },
  ];
  const contactColumns = [
    { title: "传真", dataIndex: "fax", key: "fax", ellipsis: true },
    { title: "邮箱", dataIndex: "email", key: "email", ellipsis: true },
    { title: "电话", dataIndex: "phone", key: "phone", width: 130 },
  ];

  return (
    <div className="nmpa-detail">
      <div className="nmpa-detail-tags">
        <Tag color="blue">{detail.versionStatus || "版本未知"}</Tag>
        <Tag>{detail.productCategory || "未分类"}</Tag>
        <span>共 {groups.reduce((sum, group) => sum + (group.fields?.length ?? 0), 0)} 个字段</span>
      </div>
      {groups.map((group) => (
        <section className="nmpa-detail-group" key={group.title}>
          <h4>{group.title}</h4>
          <div className="nmpa-detail-grid">
            {(group.fields ?? []).map((field) => (
              <div className="nmpa-detail-field" key={`${group.title}-${field.label}`}>
                <span>{field.label}</span>
                <strong title={field.value}>{field.value || "—"}</strong>
              </div>
            ))}
          </div>
        </section>
      ))}
      <section className="nmpa-detail-group">
        <h4>包装层级 <small>{packages.length} 条</small></h4>
        <Table
          className="nmpa-detail-table"
          tableLayout="fixed"
          size="small"
          rowKey="packageDi"
          columns={packageColumns}
          dataSource={packages}
          pagination={false}
          locale={{ emptyText: "无包装层级数据" }}
        />
      </section>
      <section className="nmpa-detail-group">
        <h4>联系人 <small>{contacts.length} 条</small></h4>
        <Table
          className="nmpa-detail-table"
          tableLayout="fixed"
          size="small"
          rowKey={(record) => `${record.phone}-${record.email}`}
          columns={contactColumns}
          dataSource={contacts}
          pagination={false}
          locale={{ emptyText: "无联系人数据" }}
        />
      </section>
    </div>
  );
}

/**
 * 数据整合结果行的单 Code 详情。
 *
 * 左侧展示整行全部字段；右侧只在行内存在 Primary DI 时查询本机药监库，
 * 用于人工核对。没有 Primary DI 时明确提示，不发起 NMPADetail 调用。
 */
export function ConsolidationDetailDrawer({
  open,
  onClose,
  columns,
  cells,
  status,
  notes = [],
  rowConflict = "",
}: ConsolidationDetailDrawerProps) {
  const [nmpaDetail, setNmpaDetail] = useState<NMPADeviceDetail | null>(null);
  const [nmpaLoading, setNmpaLoading] = useState(false);
  const [nmpaError, setNmpaError] = useState("");

  const primaryIndex = useMemo(() => primaryDIIndex(columns), [columns]);
  const primaryDI = primaryIndex >= 0 ? normalize(cells[primaryIndex]) : "";
  const hasPrimaryDI = primaryIndex >= 0 && !isMissingValue(primaryDI);
  const statusIndex = useMemo(
    () => columns.findIndex((title) => STATUS_NAMES.has(title.trim().toLowerCase())),
    [columns],
  );
  const fields = useMemo(
    () =>
      columns
        .map((title, index) => ({ title, value: cells[index] ?? "", index }))
        .filter(({ index }) => index !== statusIndex),
    [columns, cells, statusIndex],
  );

  useEffect(() => {
    if (!open) return;
    setNmpaDetail(null);
    setNmpaError("");
    if (!hasPrimaryDI) {
      setNmpaLoading(false);
      return;
    }
    let active = true;
    setNmpaLoading(true);
    void call<NMPADeviceDetail>("NMPADetail", primaryDI).then((detail) => {
      if (!active) return;
      setNmpaLoading(false);
      if (detail) {
        setNmpaDetail(detail);
      } else {
        setNmpaError("未找到匹配的药监数据，或本机没有查询药监数据的权限。");
      }
    });
    return () => {
      active = false;
    };
  }, [open, hasPrimaryDI, primaryDI]);

  return (
    <Drawer
      className="consolidation-drawer"
      open={open}
      onClose={onClose}
      width="min(96vw, 1160px)"
      zIndex={1100}
      title={
        <div className="consolidation-detail-title">
          <strong>单 Code 详情</strong>
          <small>{hasPrimaryDI ? `Primary DI ${primaryDI}` : "该行没有 Primary DI"}</small>
        </div>
      }
      footer={<Button onClick={onClose}>关闭</Button>}
    >
      <div className="consolidation-detail-body">
        <div className="consolidation-detail-summary">
          <Tag className={`result-status result-status-${status}`}>{statusLabel(status)}</Tag>
          <span>整合字段 {fields.length} 个</span>
          {!hasPrimaryDI ? (
            <Tag>未查询药监数据</Tag>
          ) : nmpaLoading ? (
            <Tag>正在查询药监数据…</Tag>
          ) : nmpaDetail ? (
            <Tag color="green">药监已匹配</Tag>
          ) : (
            <Tag>药监未匹配</Tag>
          )}
        </div>

        {rowConflict ? (
          <Alert className="consolidation-detail-alert" type="warning" showIcon message={rowConflict} />
        ) : null}

        <div className="consolidation-detail-compare">
          <section className="consolidation-detail-pane">
            <header>
              <h4>整合行字段</h4>
              <span>全部 {fields.length} 个字段</span>
            </header>
            <div className="consolidation-detail-fields">
              <div className="consolidation-detail-field consolidation-detail-status">
                <span>整合状态</span>
                <strong>{statusLabel(status)}</strong>
              </div>
              {fields.map(({ title, value, index }) => {
                const note = notes[index];
                const conflict = conflictText(note);
                return (
                  <div
                    className={`consolidation-detail-field${conflict ? " is-conflict" : ""}`}
                    key={`${title}-${index}`}
                  >
                    <span>{title}</span>
                    {conflict ? (
                      <Tooltip title={conflict} mouseEnterDelay={0.1}>
                        <strong>{displayValue(value)}</strong>
                      </Tooltip>
                    ) : (
                      <strong>{displayValue(value)}</strong>
                    )}
                  </div>
                );
              })}
            </div>
          </section>

          <section className="consolidation-detail-pane consolidation-detail-nmpa">
            <header>
              <h4>药监 UDI 数据</h4>
              <span>{hasPrimaryDI ? "按 Primary DI 查询本机药监库" : "无 Primary DI，跳过查询"}</span>
            </header>
            {!hasPrimaryDI ? (
              <Alert
                className="consolidation-detail-alert"
                type="info"
                showIcon
                message="该行没有 Primary DI，未查询药监数据。"
              />
            ) : nmpaLoading ? (
              <div className="nmpa-detail-loading"><Spin /></div>
            ) : nmpaError ? (
              <Alert className="consolidation-detail-alert" type="warning" showIcon message={nmpaError} />
            ) : nmpaDetail ? (
              <NMPADetailBody detail={nmpaDetail} />
            ) : (
              <Empty description="暂无药监数据" />
            )}
          </section>
        </div>
      </div>
    </Drawer>
  );
}
