// 四个弹窗：数据回顾、参数设定、关于、附加窗口（图片窗口）。

import { Button, Input, Modal, Pagination, Table, Tabs, Tree, Tooltip } from "antd";
import type { ColumnsType } from "antd/es/table";
import type { DataNode } from "antd/es/tree";
import { Eye, FileDown, X } from "lucide-react";
import type { ReactNode } from "react";
import { useMemo, useRef } from "react";

import type { AboutInfo, ReviewResult, SettingsNode, SettingsTab } from "../types";

/**
 * 回顾窗口的四周留白：取自**最小窗口**（主窗口 969 宽）下的实测值——
 * 弹窗宽 = 80vw = 775，两侧各留 97；弹窗高 = 131（标题条+内边距+底栏）+ 56vh = 556，
 * 上下各留 102（Wails 在 macOS 用 initWithContentRect 建窗，969×760 就是 WebView 内容区，
 * 所以原来 56vh 的基准高度正好是 760）。
 * 窗口变大时弹窗跟着变大，这两组留白保持不变（不再像以前那样把宽度卡在 1100）。
 */
const REVIEW_GAP_X = 97;
const REVIEW_GAP_Y = 102;

const ABOUT_INTRO = `SS Ready provides data validation, cleansing, 
mapping and consolidation capabilities for UDI 
master data preparation for SingleSource.

Developed by
Greater China Supply Chain & RA Team
© 2026 JJMT`;

/* ---------- 数据回顾 ---------- */

type ReviewModalProps = {
  open: boolean;
  result: ReviewResult | null;
  exporting: boolean;
  onPage: (page: number) => void;
  onExport: () => void;
  onClose: () => void;
};

export function ReviewModal({ open, result, onPage, onExport, onClose }: ReviewModalProps) {
  const columns: ColumnsType<string[]> = (result?.columns ?? []).map((title, index) => ({
    title,
    dataIndex: index,
    key: String(index),
    render: (value: string) =>
      value === "MISSING" ? <span className="cell-MISSING">{value}</span> : value,
  }));

  return (
    <Modal
      className="app-modal review-modal"
      open={open}
      title={result?.title ?? "数据回顾"}
      // 四周留白固定成最小窗口下的那一份：窗口变大，弹窗跟着一起变大。
      // 宽度必须走 width 属性（antd 的 width 是后写的内联样式，会盖掉 style.width）。
      width={`calc(100vw - ${REVIEW_GAP_X * 2}px)`}
      style={{ height: `calc(100vh - ${REVIEW_GAP_Y * 2}px)` }}
      centered
      onCancel={onClose}
      footer={
        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
          <Button icon={<FileDown size={16} />} onClick={onExport}>
            导出
          </Button>
          <div style={{ flex: "1 1 auto", display: "flex", justifyContent: "center" }}>
            <Pagination
              current={result?.page ?? 1}
              pageSize={result?.pageSize ?? 100}
              total={result?.total ?? 0}
              showSizeChanger={false}
              onChange={onPage}
            />
          </div>
          <Button type="primary" onClick={onClose}>
            关闭
          </Button>
        </div>
      }
      destroyOnClose
    >
      <div className="tblwrap">
        <Table<string[]>
          size="small"
          bordered
          sticky
          pagination={false}
          dataSource={result?.rows ?? []}
          columns={columns}
          rowKey={(_, index) => String(index)}
          scroll={{ x: "max-content" }}
        />
      </div>
    </Modal>
  );
}

/* ---------- 参数设定 ---------- */

type SettingsModalProps = {
  open: boolean;
  tabs: SettingsTab[];
  activeKey: string;
  editing: boolean;
  editorValue: string;
  path: string[];
  expandedKeys: string[];
  onSelectTab: (key: string) => void;
  onSelectNode: (path: string[]) => void;
  onExpandedChange: (keys: string[]) => void;
  onToggleAll: (expand: boolean) => void;
  onStartEdit: () => void;
  onCancelEdit: () => void;
  onEditorChange: (value: string) => void;
  onSave: () => void;
  onClose: () => void;
};

export function SettingsModal({
  open,
  tabs,
  activeKey,
  editing,
  editorValue,
  path,
  expandedKeys,
  onSelectTab,
  onSelectNode,
  onExpandedChange,
  onToggleAll,
  onStartEdit,
  onCancelEdit,
  onEditorChange,
  onSave,
  onClose,
}: SettingsModalProps) {
  const active = tabs.find((tab) => tab.key === activeKey) ?? tabs[0];
  const paths = useRef<Record<string, string[]>>({});

  // 结构化树 → antd Tree 的 treeData：容器显示子项数量，标量按类型着色。
  // key 里不能用 "/" 拼路径（配置的键名本身可能含 "/"），所以路径另存一张表。
  const buildNodes = (nodes: SettingsNode[], parentPath: string[]): DataNode[] =>
    nodes.map((node) => {
      const nodePath = [...parentPath, node.key];
      const nodeKey = nodePath.map((part) => encodeURIComponent(part)).join("/");
      paths.current[nodeKey] = nodePath;
      const children = node.children ?? [];
      if (node.kind === "map" || node.kind === "seq") {
        const empty = children.length === 0;
        const meta = node.kind === "map" ? `{${children.length}}` : `[${children.length}]`;
        return {
          key: nodeKey,
          isLeaf: empty,
          title: (
            <span>
              <span className="tree-key">{node.key}</span>
              <span className="tree-meta">{meta}</span>
              {empty && <span className="tree-empty">（空 {node.kind === "map" ? "object" : "array"}）</span>}
            </span>
          ),
          children: empty ? undefined : buildNodes(children, nodePath),
        };
      }
      return {
        key: nodeKey,
        isLeaf: true,
        title: (
          <span>
            <span className="tree-key">{node.key}</span>
            <span className="tree-sep">:</span>
            <span className={`tree-value type-${node.kind}`}>{node.value}</span>
          </span>
        ),
      };
    });

  const treeData = useMemo(
    () => buildNodes(active?.tree ?? [], []),
    // 切换页签或重新加载配置时重建
    [active?.key, active?.tree],
  );

  return (
    <Modal
      className="app-modal"
      open={open}
      title="参数设定"
      width="min(80vw, 900px)"
      centered
      // 右上角不要关闭按钮：只留底栏那个「关闭」
      closable={false}
      onCancel={onClose}
      footer={
        <Button type="primary" onClick={onClose}>
          关闭
        </Button>
      }
      destroyOnClose
    >
      <Tabs
        items={tabs.map((tab) => ({ key: tab.key, label: tab.title }))}
        activeKey={active?.key}
        onChange={onSelectTab}
      />
      <div className="settings-head">
        <div className="settings-note">
          {active?.title}（{active?.path}）{active?.note ? " · " + active.note : ""}
        </div>
        {!editing && (
          <>
            <Button onClick={() => onToggleAll(true)}>全部展开</Button>
            <Button onClick={() => onToggleAll(false)}>全部折叠</Button>
            <Button onClick={onStartEdit}>编辑原文</Button>
          </>
        )}
        {editing && (
          <>
            <Button onClick={onCancelEdit}>取消</Button>
            <Button type="primary" onClick={onSave}>
              保存
            </Button>
          </>
        )}
      </div>
      {editing ? (
        <Input.TextArea
          className="settings-editor"
          spellCheck={false}
          value={editorValue}
          onChange={(event) => onEditorChange(event.target.value)}
        />
      ) : (
        <>
          <div className="settings-path" aria-live="polite">
            {path.map((part, index) => (
              <span key={`${part}-${index}`}>
                {index > 0 && <span className="crumb-sep">▸</span>}
                {part}
              </span>
            ))}
          </div>
          <div className="settings-tree">
            <Tree
              treeData={treeData}
              expandedKeys={expandedKeys}
              onExpand={(keys) => onExpandedChange(keys as string[])}
              onSelect={(keys) => onSelectNode(paths.current[String(keys[0])] ?? [])}
            />
          </div>
        </>
      )}
    </Modal>
  );
}

/* ---------- 关于 ---------- */

type AboutModalProps = {
  open: boolean;
  info: AboutInfo | null;
  onIconClick: () => void;
  onClose: () => void;
};

export function AboutModal({ open, info, onIconClick, onClose }: AboutModalProps) {
  return (
    <Modal
      className="about-window"
      open={open}
      centered
      closable={false}
      footer={null}
      width={323}
      maskClosable
      onCancel={onClose}
      destroyOnClose
    >
      <Button
        className="dialog-close"
        type="text"
        shape="circle"
        aria-label="关闭"
        title="关闭"
        icon={<X size={18} />}
        onClick={onClose}
      />
      <div className="about-body">
        <img className="about-icon" src="logo.png" alt="SingleSourceReady" title="关于" onClick={onIconClick} />
        <div className="name">SingleSource Ready (SSR)</div>
        <div className="version" title={info?.detail ?? ""}>
          {info?.version ?? ""}
        </div>
        <div className="intro">{ABOUT_INTRO}</div>
      </div>
    </Modal>
  );
}

/* ---------- 附加窗口（图片窗口） ---------- */

type HiddenModalProps = { open: boolean; onClose: () => void };

export function HiddenModal({ open, onClose }: HiddenModalProps) {
  return (
    <Modal
      className="hidden-window"
      open={open}
      centered
      closable={false}
      footer={null}
      width={323}
      maskClosable
      onCancel={onClose}
      destroyOnClose
    >
      <div className="hidden-body">
        <img src="puppy.png" alt="🐶Puppy Approved!🐾" />
        <h3 className="hidden-title">🐶Puppy Approved!🐾</h3>
        <Tooltip title="关闭">
          <Button
            className="dialog-close"
            type="text"
            shape="circle"
            aria-label="关闭"
            icon={<X size={18} />}
            onClick={onClose}
          />
        </Tooltip>
      </div>
    </Modal>
  );
}

export function ReviewIcon() {
  return <Eye size={18} />;
}

export type { ReactNode };
