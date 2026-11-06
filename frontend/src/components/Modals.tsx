// 四个弹窗：数据回顾、参数设定、关于、附加窗口（图片窗口）。
//
// 弹窗的头 / 关闭按钮 / 底栏都回到 antd 默认形态：
//   标题是次级标题色 + 右上角自带 X；底栏按钮右对齐（不再做红底白字标题条、
//   也不再为了"像 QDialog"去掉 X 再自绘居中按钮）。

import {
  Alert,
  Button,
  Descriptions,
  Flex,
  Input,
  Modal,
  Pagination,
  Segmented,
  Space,
  Table,
  Tag,
  Tabs,
  Tooltip,
  Tree,
  Typography,
} from "antd";
import type { ColumnsType } from "antd/es/table";
import type { DataNode } from "antd/es/tree";
import { FileDown, Search } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";

import { DEFAULT_PAGE_SIZE, PAGE_SIZE_OPTIONS } from "../tablePagination";
import type {
  AboutInfo,
  NotificationPreview,
  ReviewResult,
  ShareRootInfo,
  SettingsChange,
  SettingsNode,
  SettingsTab,
} from "../types";
import { SettingsForm } from "./SettingsForm";

/**
 * 回顾窗口的四周留白：取自**最小窗口**（主窗口 969 宽）下的实测值——
 * 弹窗宽 = 80vw = 775，两侧各留 97；弹窗高 = 131（标题条+内边距+底栏）+ 56vh = 556，
 * 上下各留 102（Wails 在 macOS 用 initWithContentRect 建窗，969×760 就是 WebView 内容区,
 * 所以原来 56vh 的基准高度正好是 760）。
 * 窗口变大时弹窗跟着变大，这两组留白保持不变。
 */
const REVIEW_GAP_X = 97;
const REVIEW_GAP_Y = 102;

const ABOUT_SUMMARY =
  "SS Ready provides data validation, cleansing, mapping and consolidation capabilities for UDI master data preparation for SingleSource.";

const ABOUT_CREDITS = "Developed by Greater China Supply Chain & RA Team © 2026 JJMT";

/* ---------- 数据回顾 ---------- */

type ReviewModalProps = {
  open: boolean;
  result: ReviewResult | null;
  exporting: boolean;
  onPage: (page: number, pageSize: number) => void;
  /** 关键字筛选：任一列包含即命中（与数据整合结果表的查找同一个口径） */
  onSearch: (query: string) => void;
  onExport: () => void;
  onClose: () => void;
};

export function ReviewModal({ open, result, exporting, onPage, onSearch, onExport, onClose }: ReviewModalProps) {
  // 查找框：输入不触发，点「查找」或回车才查（点 × 清空时立即恢复全量）
  const [query, setQuery] = useState("");
  const applySearch = () => onSearch(query.trim());
  const columns: ColumnsType<string[]> = (result?.columns ?? []).map((title, index) => ({
    title,
    dataIndex: index,
    key: String(index),
    render: (value: string) =>
      value === "MISSING" ? (
        <Tooltip title="此格缺失，导出时为空">
          <span className="cell-missing">缺失</span>
        </Tooltip>
      ) : (
        value
      ),
  }));

  return (
    <Modal
      className="review-modal"
      open={open}
      title={result?.title ?? "数据回顾"}
      // 四周留白固定成最小窗口下的那一份：窗口变大，弹窗跟着一起变大。
      // 宽度必须走 width 属性（antd 的 width 是后写的内联样式，会盖掉 style.width）。
      width={`calc(100vw - ${REVIEW_GAP_X * 2}px)`}
      style={{ height: `calc(100vh - ${REVIEW_GAP_Y * 2}px)` }}
      centered
      onCancel={onClose}
      // 底栏仍是「导出在最左、翻页居中、关闭在最右」这一行。
      footer={
        <Flex align="center" gap={8}>
          <Button icon={<FileDown size={16} />} loading={exporting} onClick={onExport}>
            导出
          </Button>
          <Flex flex="1 1 auto" justify="center">
            <Pagination
              current={result?.page ?? 1}
              pageSize={result?.pageSize ?? DEFAULT_PAGE_SIZE}
              total={result?.total ?? 0}
              pageSizeOptions={PAGE_SIZE_OPTIONS}
              showSizeChanger
              showTotal={(total) => `共 ${total} 行`}
              // 换了每页行数就回第一页（与结果表口径一致）
              onChange={(page, pageSize) =>
                onPage(pageSize !== (result?.pageSize ?? DEFAULT_PAGE_SIZE) ? 1 : page, pageSize)
              }
            />
          </Flex>
          <Button type="primary" onClick={onClose}>
            关闭
          </Button>
        </Flex>
      }
      destroyOnHidden
    >
      <div className="review-toolbar">
        <div className="table-search-group">
          <Input
            className="table-search"
            allowClear
            value={query}
            aria-label="查找表格内容"
            placeholder="查找表格内容"
            prefix={<Search size={14} aria-hidden="true" />}
            onChange={(event) => {
              const value = event.target.value;
              setQuery(value);
              if (value === "") onSearch("");
            }}
            onPressEnter={applySearch}
          />
          <Button size="small" icon={<Search size={14} />} aria-label="查找" onClick={applySearch} />
        </div>
      </div>
      <div className="tblwrap">
        <Table<string[]>
          size="small"
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
  onSaveValues: (changes: SettingsChange[]) => Promise<boolean>;
  onClose: () => void;
};

/** 参数设定的三种视图：结构化表单（默认）/ 结构树 / 原文。 */
type SettingsView = "form" | "tree" | "raw";

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
  onSaveValues,
  onClose,
}: SettingsModalProps) {
  const active = tabs.find((tab) => tab.key === activeKey) ?? tabs[0];
  const paths = useRef<Record<string, string[]>>({});
  // 默认给「结构化表单」；结构树与原文作为查看/兜底手段保留
  const [view, setView] = useState<SettingsView>("form");
  const [pending, setPending] = useState<SettingsChange[]>([]);
  const [saving, setSaving] = useState(false);

  // 攒改动：同一路径后写覆盖先写；列表增删按序追加
  const applyChange = (change: SettingsChange) =>
    setPending((current) => {
      if (change.action === undefined || change.action === "set") {
        return [...current.filter((item) => !(item.action !== "remove" && item.path.join("\u0000") === change.path.join("\u0000"))), change];
      }
      return [...current, change];
    });
  const resetPath = (path: string[]) =>
    setPending((current) =>
      current.filter((item) => !(item.action !== "remove" && item.path.join("\u0000") === path.join("\u0000"))),
    );
  const resetAll = () => setPending([]);

  const submit = async () => {
    if (pending.length === 0) return;
    setSaving(true);
    const ok = await onSaveValues(pending);
    setSaving(false);
    if (ok) setPending([]);
  };

  // 切换页签 / 重新载入配置后，待保存改动作废（避免把 A 文件的路径写到 B 文件）
  const activeTabKey = active?.key ?? "";
  const [pendingTab, setPendingTab] = useState(activeTabKey);
  if (pendingTab !== activeTabKey) {
    setPendingTab(activeTabKey);
    setPending([]);
  }

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
              {empty && (
                <span className="tree-empty">
                  （空 {node.kind === "map" ? "object" : "array"}）
                </span>
              )}
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
      open={open}
      title="参数设定"
      width="min(80vw, 900px)"
      centered
      onCancel={onClose}
      // 底栏：表单模式是「撤销改动 / 保存改动」；原文模式沿用「取消 / 保存」；结构树只有「关闭」。
      footer={
        view === "raw" ? (
          editing ? (
            <Space>
              <Button onClick={onCancelEdit}>取消</Button>
              <Button type="primary" onClick={onSave}>
                保存
              </Button>
            </Space>
          ) : (
            <Button type="primary" onClick={onClose}>
              关闭
            </Button>
          )
        ) : view === "form" ? (
          <Space>
            <Button disabled={pending.length === 0 || saving} onClick={resetAll}>
              撤销改动
            </Button>
            <Button type="primary" loading={saving} disabled={pending.length === 0} onClick={() => void submit()}>
              保存改动
            </Button>
          </Space>
        ) : (
          <Button type="primary" onClick={onClose}>
            关闭
          </Button>
        )
      }
      destroyOnHidden
    >
      <Tabs
        items={tabs.map((tab) => ({ key: tab.key, label: tab.title }))}
        activeKey={active?.key}
        onChange={onSelectTab}
      />
      <Flex className="settings-head" align="center" justify="space-between" gap={12}>
        <Typography.Text className="settings-note" type="secondary">
          {active?.title}（{active?.path}）
          {active?.note ? " · " + active.note : ""}
        </Typography.Text>
        <Space>
          <Segmented
            size="small"
            value={view}
            onChange={(value) => {
              setView(value as SettingsView);
              if (value === "raw") onStartEdit();
            }}
            options={[
              { label: "表单", value: "form" },
              { label: "结构树", value: "tree" },
              { label: "原文", value: "raw" },
            ]}
          />
          {view === "tree" && (
            <>
              <Button onClick={() => onToggleAll(true)}>全部展开</Button>
              <Button onClick={() => onToggleAll(false)}>全部折叠</Button>
            </>
          )}
          {view === "tree" && <Button onClick={onStartEdit}>编辑原文</Button>}
        </Space>
      </Flex>
      {view === "form" ? (
        <div className="settings-form-wrap">
          <SettingsForm
            nodes={active?.tree ?? []}
            pending={pending}
            onChange={applyChange}
            onResetPath={resetPath}
            onReset={resetAll}
          />
        </div>
      ) : view === "raw" || editing ? (
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
      footer={null}
      width={560}
      maskClosable
      onCancel={onClose}
      destroyOnHidden
    >
      <div className="about-body">
        <img
          className="about-icon"
          src="/logo.png"
          alt="SingleSourceReady"
          title="关于"
          onClick={onIconClick}
        />
        <div className="about-name">{info?.name ?? "SingleSourceReady"}</div>
        <div className="about-tagline">{info?.tagline ?? ""}</div>
        <div className="about-summary">{ABOUT_SUMMARY}</div>
        <div className="about-version" title={info?.detail ?? ""}>
          {info?.version ?? ""}
        </div>
        <Descriptions className="about-table" size="small" bordered column={1}>
          <Descriptions.Item label="数据目录">{info?.dataDir || "—"}</Descriptions.Item>
          <Descriptions.Item label="参数文件">{info?.configDir || "—"}</Descriptions.Item>
          <Descriptions.Item label="数据库">{info?.database || "—"}</Descriptions.Item>
          <Descriptions.Item label="运行环境">{info?.runtime || "—"}</Descriptions.Item>
        </Descriptions>
        <div className="about-tech">
          <div className="about-tech-title">技术方案</div>
          <ul>
            {(info?.techStack ?? []).map((item) => (
              <li key={item}>{item}</li>
            ))}
          </ul>
        </div>
        <div className="about-credits">{ABOUT_CREDITS}</div>
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
      footer={null}
      width={323}
      // 附加窗口只认右上角的 X：点遮罩 / 按 Esc 都不关（避免它刚出现就被同一次点击的余波关掉）
      maskClosable={false}
      keyboard={false}
      onCancel={onClose}
      destroyOnHidden
    >
      <div className="hidden-body">
        <img src="puppy.png" alt="🐶Puppy Approved!🐾" />
        <h3 className="hidden-title">🐶Puppy Approved!🐾</h3>
      </div>
    </Modal>
  );
}

type NotificationModalProps = {
  open: boolean;
  preview: NotificationPreview | null;
  sending: boolean;
  onSend: () => void;
  onClose: () => void;
};

/**
 * 数据更新通知的确认窗：先把收件人、主题与正文摊开给用户看，再决定发不发。
 * 发送方式由 workflow.yaml 的 email.send_mode 决定（send = 直接发；draft = 打开草稿）。
 */
export function NotificationModal({ open, preview, sending, onSend, onClose }: NotificationModalProps) {
  const draft = preview?.mode === "draft";
  return (
    <Modal
      open={open}
      title="发送更新通知"
      width={560}
      onCancel={sending ? undefined : onClose}
      footer={[
        <Button key="cancel" disabled={sending} onClick={onClose}>
          取消
        </Button>,
        <Button
          key="send"
          type="primary"
          loading={sending}
          disabled={!preview?.ready}
          onClick={onSend}
        >
          {draft ? "打开草稿" : "发送邮件"}
        </Button>,
      ]}
    >
      {preview ? (
        <div className="notify-preview">
          <Descriptions column={1} size="small" bordered>
            <Descriptions.Item label="收件人">
              <Space size={[4, 4]} wrap>
                {preview.to.length === 0 ? (
                  <Typography.Text type="secondary">未配置收件人</Typography.Text>
                ) : (
                  preview.to.map((address) => <Tag key={address}>{address}</Tag>)
                )}
              </Space>
            </Descriptions.Item>
            <Descriptions.Item label="主题">{preview.subject}</Descriptions.Item>
            <Descriptions.Item label="方式">
              {draft ? "打开 Microsoft Outlook 草稿（手动点发送）" : "通过本机 Microsoft Outlook 直接发送"}
            </Descriptions.Item>
          </Descriptions>
          <pre className="notify-body">{preview.body}</pre>
          {!preview.ready ? (
            <Alert type="warning" showIcon message={preview.reason || "暂时不能发送"} />
          ) : null}
        </div>
      ) : null}
    </Modal>
  );
}

type ShareRootModalProps = {
  open: boolean;
  info: ShareRootInfo | null;
  saving: boolean;
  onSave: (root: string) => void;
  onClose: () => void;
};

/**
 * 「共享盘位置」：非 Admin 唯一能改的设置。
 * 只写 workflow.yaml 的 share.root；留空 = 本地模式。其它配置仍由 Admin 管理。
 */
export function ShareRootModal({ open, info, saving, onSave, onClose }: ShareRootModalProps) {
  const [root, setRoot] = useState("");
  // 每次打开时用当前值填充（关闭时未保存的输入不保留）
  useEffect(() => {
    if (open) setRoot(info?.root ?? "");
  }, [open, info?.root]);
  return (
    <Modal
      open={open}
      title="共享盘位置"
      width={520}
      onCancel={saving ? undefined : onClose}
      footer={[
        <Button key="cancel" disabled={saving} onClick={onClose}>
          取消
        </Button>,
        <Button key="save" type="primary" loading={saving} onClick={() => onSave(root)}>
          保存
        </Button>,
      ]}
    >
      <div className="share-root-form">
        <Typography.Paragraph type="secondary" className="share-root-hint">
          本机使用的共享盘路径（每台电脑可以不同）。留空表示本地模式。
          只修改这一项，其它设置仍由 Admin 维护。
        </Typography.Paragraph>
        <Input
          aria-label="共享盘位置"
          placeholder="例如 C:\SSRShare 或 \\server\share"
          value={root}
          disabled={saving}
          onChange={(event) => setRoot(event.target.value)}
          onPressEnter={() => onSave(root)}
        />
        {info?.detail ? (
          <Typography.Paragraph type="secondary" className="share-root-status">
            当前状态：{info.detail}
          </Typography.Paragraph>
        ) : null}
      </div>
    </Modal>
  );
}
