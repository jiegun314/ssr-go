// 主界面：左列（数据导入 / 记录导出）+ 右列（数据整合）+ 底部（操作日志）+ 四个弹窗。
// 布局与原来一致；四个模块的容器换成 antd 的 Card（标题排版、边框、圆角都由 antd 给）。

import { Button, Card, Switch, Tag, Tooltip } from "antd";
import { ChevronLeft, ChevronRight, FileDown, Merge, RefreshCw, Trash2, Users } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import {
  REVIEW_PAGE_SIZE,
  SOURCES,
  call,
  defaultRange,
  onBusy,
  onEvent,
  toLogTime,
} from "./bridge";
import {
  ConsolidationPanel,
  ImportPanel,
  LoadingOverlay,
  LogPanel,
  MessageModal,
  RecordExportPanel,
  type MessageState,
} from "./components/Panels";
import { AlignPanel } from "./components/AlignPanel";
import { LoginScreen } from "./components/LoginScreen";
import { AboutModal, HiddenModal, ReviewModal, SettingsModal } from "./components/Modals";
import type {
  AboutInfo,
  ClearResult,
  ConsolidateResult,
  ExportResult,
  ExportTarget,
  AlignResult,
  ImportResult,
  ImportState,
  InitialState,
  LoginResult,
  LoginRole,
  SessionInfo,
  ShareOverview,
  ShareStatus,
  SourceStatus,
  ReviewResult,
  SettingsChange,
  SettingsSaveResult,
  SettingsTab,
} from "./types";

/** 关于窗口图标上的点击计数：阈值与时间窗沿用原版，属行为契约，不要改这两个数值。 */
const ICON_CLICK_COUNT = 8;
const ICON_CLICK_WINDOW_MS = 5000;

type ReviewSession = {
  kind: "source" | "cleaned-source" | "log";
  source: string;
  fileType: string;
  start: string;
  end: string;
};

export default function App() {
  const [imports, setImports] = useState<Record<string, ImportState>>({});
  const [log, setLog] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
  const [message, setMessage] = useState<MessageState>(null);

  const [columns, setColumns] = useState<string[]>([]);
  const [rows, setRows] = useState<string[][]>([]);
  const [statuses, setStatuses] = useState<string[]>([]);
  // 单元格级冲突提示：与 rows 同形同序（空串 = 该格不冲突）
  const [conflicts, setConflicts] = useState<string[][]>([]);
  const [counts, setCounts] = useState<Record<string, number>>({});
  // 生成文件的门禁：只有合格（Ready）行会被导出，所以按全部行里的 Ready 数量算
  const readyCount = statuses.filter((status) => status === "Ready").length;

  const range = defaultRange(new Date());
  const [startDate, setStartDate] = useState(range.start);
  const [endDate, setEndDate] = useState(range.end);

  const [reviewOpen, setReviewOpen] = useState(false);
  const [review, setReview] = useState<ReviewResult | null>(null);
  const reviewSession = useRef<ReviewSession | null>(null);

  const [settingsOpen, setSettingsOpen] = useState(false);
  const [tabs, setTabs] = useState<SettingsTab[]>([]);
  const [activeTab, setActiveTab] = useState("");
  const [editing, setEditing] = useState(false);
  const [editorValue, setEditorValue] = useState("");
  const [treePath, setTreePath] = useState<string[]>([]);
  const [expandedKeys, setExpandedKeys] = useState<string[]>([]);

  // 多人协作版（P1）：登录会话、登录页候选人、共享盘状态
  const [session, setSession] = useState<SessionInfo | null>(null);
  const [roles, setRoles] = useState<LoginRole[]>([]);
  const [username, setUsername] = useState("");
  const [singleUser, setSingleUser] = useState(false);
  const [share, setShare] = useState<ShareStatus | null>(null);
  // 共享盘总览（对齐/待整合/最后整合时间）：SC 看全部，其他角色只看到自己那份的明细
  const [overview, setOverview] = useState<ShareOverview | null>(null);
  const [aligning, setAligning] = useState(false);
  const [autoAlign, setAutoAlign] = useState(true);

  const [aboutOpen, setAboutOpen] = useState(false);
  const [about, setAbout] = useState<AboutInfo | null>(null);
  const [hiddenOpen, setHiddenOpen] = useState(false);
  // 左侧两个模块（数据导入 + 操作日志）可以整体收起，把整个窗口宽度让给右边的结果表
  const [leftCollapsed, setLeftCollapsed] = useState(false);

  const iconClicks = useRef<number[]>([]);
  const voiceRef = useRef<HTMLAudioElement>(null);

  useEffect(() => onBusy(setBusy), []);

  /* ---------- 载入与状态 ---------- */

  const refreshInitial = useCallback(async () => {
    const initial = await call<InitialState>("InitialState");
    if (!initial) return;
    setImports(initial.imports ?? {});
    setLog(initial.operationLog ?? "");
    setSession(initial.session ?? null);
    setRoles(initial.roles ?? []);
    setUsername(initial.username ?? "");
    setSingleUser(initial.singleUser ?? false);
    setShare(initial.share ?? null);
  }, []);

  useEffect(() => {
    void refreshInitial();
  }, [refreshInitial]);

  /* ---------- 登录与权限 ---------- */

  const handleLogin = useCallback(
    async (role: string, password: string) => {
      const result = await call<LoginResult>("Login", role, password);
      if (!result) return;
      setLog(result.log ?? "");
      if (result.failed) {
        setMessage({ title: result.title, message: result.message, failed: true });
        return;
      }
      setSession(result.session ?? null);
      await refreshInitial();
    },
    [refreshInitial],
  );

  const handleLogout = useCallback(async () => {
    const result = await call<LoginResult>("Logout");
    if (!result) return;
    setLog(result.log ?? "");
    setSession(result.session ?? null);
    await refreshInitial();
  }, [refreshInitial]);

  /** 共享盘异常（配置了共享盘但不健康）：锁住导入/发布与整合。 */
  const shareBlocked = Boolean(share && share.mode === "share" && !share.healthy);

  const handleRecheckShare = useCallback(async () => {
    const status = await call<ShareStatus>("RecheckShare");
    if (!status) return;
    setShare(status);
    if (!status.healthy) {
      setMessage({ title: "共享盘异常", message: status.detail, failed: true });
    }
  }, []);

  /** can 按后端下发的 actions 决定界面显隐（后端仍会再裁决一次）。 */
  const can = useCallback(
    (action: string) => Boolean(session?.actions?.includes(action)),
    [session],
  );

  /** 能导入/发布的来源（决定"载入文件"按钮）。 */
  const importableSources = SOURCES.filter((source) => {
    const action =
      source.key === "ra_input"
        ? "import:ra_input"
        : source.key === "global_udi_input"
          ? "import:global_udi_input"
          : "import:eama";
    return can(action);
  }).map((source) => source.key);

  /** 能查看明细的来源：SC/Admin 看全部，其余只看自己那份。 */
  const viewableSources = can("review:any")
    ? SOURCES.map((source) => source.key)
    : importableSources;

  const shareInfo: Record<string, SourceStatus> = {};
  for (const item of overview?.sources ?? []) shareInfo[item.source] = item;

  /** 是否可以在整合前对齐（只有 SC/Admin 有 align 动作）。 */
  const canAlign = can("align");

  /** 标题栏里的"发布状态"：还有几份待对齐 / 几份未发布 / 全部已对齐。 */
  const alignSummary = (() => {
    const sources = overview?.sources ?? [];
    const pending = sources.filter((item) => Boolean(item.remoteVersion) && !item.aligned).length;
    const unpublished = sources.filter((item) => !item.remoteVersion).length;
    if (pending > 0) return { pending, text: `${pending} 份待对齐`, color: "orange" };
    if (unpublished > 0) return { pending, text: `${unpublished} 份未发布`, color: "default" };
    return { pending, text: "全部已对齐", color: "green" };
  })();
  /** 四份都已对齐（本地模式视为已对齐）。 */
  const allAligned =
    !overview?.enabled ||
    overview.sources.every((item) => Boolean(item.remoteVersion) && item.aligned);

  const refreshOverview = useCallback(async () => {
    const next = await call<ShareOverview>("ShareOverview");
    if (!next) return;
    setOverview(next);
    setAutoAlign(next.autoAlign ?? true);
    if (next.status) setShare(next.status);
  }, []);

  const applyAlignResult = useCallback(
    (result: AlignResult) => {
      setLog(result.log ?? "");
      if (result.message) {
        setMessage({ title: result.title, message: result.message, failed: result.failed });
      }
      void refreshOverview();
    },
    [refreshOverview],
  );

  const handleAlignAll = useCallback(async () => {
    setAligning(true);
    const result = await call<AlignResult>("AlignAll");
    setAligning(false);
    if (result) applyAlignResult(result);
  }, [applyAlignResult]);

  const handleAlign = useCallback(
    async (source: string) => {
      setAligning(true);
      const result = await call<AlignResult>("AlignSource", source);
      setAligning(false);
      if (result) applyAlignResult(result);
    },
    [applyAlignResult],
  );

  // 打开界面/切换身份后拉一次总览，之后按 pollSeconds 轮询（网络盘上文件监听不可靠）
  useEffect(() => {
    if (!session) return;
    void refreshOverview();
    const seconds = overview?.pollSeconds && overview.pollSeconds > 0 ? overview.pollSeconds : 30;
    const timer = window.setInterval(() => {
      void refreshOverview();
    }, Math.max(5, seconds) * 1000);
    const onFocus = () => {
      void refreshOverview();
    };
    window.addEventListener("focus", onFocus);
    return () => {
      window.clearInterval(timer);
      window.removeEventListener("focus", onFocus);
    };
    // 只在登录/退出时重建定时器；轮询间隔用首次拿到的值
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [session]);

  // 自动对齐：开了开关、有未对齐的来源、且当前身份能对齐时，自动跑一次（15 秒内不重复）
  const lastAutoAlign = useRef(0);
  useEffect(() => {
    if (!session || !autoAlign || !overview?.enabled || !canAlign || aligning) return;
    const pendingAlign = overview.sources.some((item) => Boolean(item.remoteVersion) && !item.aligned);
    if (!pendingAlign) return;
    if (Date.now() - lastAutoAlign.current < 15000) return;
    lastAutoAlign.current = Date.now();
    void handleAlignAll();
  }, [session, autoAlign, overview, canAlign, aligning, handleAlignAll]);

  const applyImportStates = useCallback((next: Record<string, ImportState>) => {
    setImports((current) => ({ ...current, ...next }));
  }, []);

  const fail = useCallback((title: string, detail: string) => {
    setMessage({ title, message: detail, failed: true });
  }, []);

  /* ---------- 数据导入 ---------- */

  const handleImport = useCallback(
    async (source: string) => {
      const filePath = await call<string>("SelectImportFile", source);
      if (!filePath) return; // 取消
      const result = await call<ImportResult>("ImportSource", source, filePath);
      if (!result) return;
      setLog(result.log);
      void refreshOverview();
      if (result.state) applyImportStates({ [source]: result.state });
      // 成功时把结果摊成「信息类别 / 导入数量」两行，失败时只给原因（failed 让标题配红色图标）
      const category = SOURCES.find((item) => item.key === source)?.title ?? source;
      setMessage({
        title: result.title,
        message: result.message,
        failed: result.failed,
        details:
          !result.failed && typeof result.rowCount === "number"
            ? [
                { label: "信息类别", value: category },
                { label: "导入数量", value: `${result.rowCount.toLocaleString("zh-CN")} 行` },
                ...(result.publishedVersion
                  ? [{ label: "发布版本", value: result.publishedVersion }]
                  : []),
              ]
            : undefined,
      });
    },
    [applyImportStates],
  );

  const handleClear = useCallback(async () => {
    const result = await call<ClearResult>("ClearImportedData");
    if (!result) return;
    setLog(result.log);
    setMessage({ title: result.title, message: result.message, failed: result.failed });
    if (!result.failed) await refreshInitial();
  }, [refreshInitial]);

  /* ---------- 数据回顾 ---------- */

  const loadReviewPage = useCallback(async (page: number, pageSize: number = REVIEW_PAGE_SIZE) => {
    const session = reviewSession.current;
    if (!session) return;
    const result =
      session.kind === "log"
        ? await call<ReviewResult>("ReviewLog", session.start, session.end, page, pageSize)
        : session.kind === "cleaned-source"
          ? await call<ReviewResult>("ReviewCleanedSource", session.source, page, pageSize)
          : await call<ReviewResult>("ReviewSource", session.source, page, pageSize);
    if (!result) return;
    setLog(result.log);
    if (result.failed) {
      setMessage({ title: result.title, message: result.message, failed: result.failed });
      return;
    }
    setReview(result);
  }, []);

  const openSourceReview = useCallback(
    async (source: string) => {
      reviewSession.current = { kind: "source", source, fileType: source, start: "", end: "" };
      await loadReviewPage(1);
      setReviewOpen(true);
    },
    [loadReviewPage],
  );

  const openCleanedReview = useCallback(
    async (source: string) => {
      reviewSession.current = { kind: "cleaned-source", source, fileType: source, start: "", end: "" };
      await loadReviewPage(1);
      setReviewOpen(true);
    },
    [loadReviewPage],
  );

  const openLogReview = useCallback(async () => {
    // 选中的起止两天都完整包含：起始补 00:00:00、结束补 23:59:59
    reviewSession.current = {
      kind: "log",
      source: "",
      fileType: "operation_log",
      start: toLogTime(startDate, false),
      end: toLogTime(endDate, true),
    };
    await loadReviewPage(1);
    setReviewOpen(true);
  }, [endDate, loadReviewPage, startDate]);

  const handleReviewExport = useCallback(async () => {
    const session = reviewSession.current;
    if (!session) return;
    // 拆两步：先选保存位置（载入图层显示"正在选择保存位置"），再真正写文件
    const choice = await call<ExportTarget>("SelectReviewExportTarget", session.fileType);
    if (!choice?.target) return;
    const result = await call<ExportResult>("ExportReviewData", session.fileType, choice.target);
    if (!result) return;
    setLog(result.log);
    setMessage({ title: result.title, message: result.message, failed: result.failed });
  }, []);

  /* ---------- 整合与导出 ---------- */

  const handleConsolidate = useCallback(async () => {
    const result = await call<ConsolidateResult>("Consolidate");
    if (!result) return;
    setLog(result.log);
    if (result.failed) {
      setMessage({ title: result.title, message: result.message, failed: result.failed });
      return;
    }
    setColumns(result.columns ?? []);
    setRows(result.rows ?? []);
    setStatuses(result.statuses ?? []);
    setConflicts(result.conflicts ?? []);
    const summary: Record<string, number> = {};
    for (const status of result.statuses ?? []) {
      summary[status] = (summary[status] ?? 0) + 1;
    }
    setCounts(summary);
    if (result.notice) {
      setMessage({ title: "提示", message: result.notice, failed: false });
    }
    // 整合成功后「待整合」标志要消失、最后整合时间要更新
    void refreshOverview();
  }, [refreshOverview]);

  const handleExport = useCallback(async () => {
    const choice = await call<ExportTarget>("SelectExportTarget");
    if (!choice?.target) return; // 取消
    const result = await call<ExportResult>("Export", choice.defaultName, choice.target);
    if (!result) return;
    setLog(result.log);
    setMessage({ title: result.title, message: result.message, failed: result.failed });
  }, []);

  /* ---------- 参数设定 ---------- */

  const openSettings = useCallback(async () => {
    const loaded = await call<SettingsTab[]>("ConfigurationDocument");
    if (!Array.isArray(loaded) || loaded.length === 0) {
      fail("Error", "无法读取配置文件");
      return;
    }
    setTabs(loaded);
    setActiveTab(loaded[0].key);
    setEditing(false);
    setTreePath([]);
    setExpandedKeys(defaultExpandedKeys(loaded[0].tree));
    setSettingsOpen(true);
  }, [fail]);

  const selectTab = useCallback(
    (key: string) => {
      const tab = tabs.find((item) => item.key === key);
      setActiveTab(key);
      setEditing(false);
      setTreePath([]);
      setExpandedKeys(defaultExpandedKeys(tab?.tree ?? []));
    },
    [tabs],
  );

  const toggleAll = useCallback(
    (expand: boolean) => {
      const tab = tabs.find((item) => item.key === activeTab);
      setExpandedKeys(expand ? allKeys(tab?.tree ?? []) : []);
    },
    [activeTab, tabs],
  );

  const saveSettings = useCallback(async () => {
    const result = await call<SettingsSaveResult>("SaveConfigurationFile", activeTab, editorValue);
    if (!result) return;
    setLog((current) => current);
    setMessage({ title: result.title, message: result.message, failed: result.failed });
    if (result.failed) return;
    const refreshed = await call<SettingsTab[]>("ConfigurationDocument");
    if (Array.isArray(refreshed) && refreshed.length > 0) {
      setTabs(refreshed);
      setExpandedKeys(defaultExpandedKeys(refreshed.find((tab) => tab.key === activeTab)?.tree ?? []));
    }
    setEditing(false);
  }, [activeTab, editorValue]);

  // 结构化保存：把表单里攒下的改动一次性提交，成功后热重载并刷新文档
  const saveSettingsValues = useCallback(
    async (changes: SettingsChange[]): Promise<boolean> => {
      const result = await call<SettingsSaveResult>("SaveSettingsValues", activeTab, changes);
      if (!result) return false;
      setMessage({ title: result.title, message: result.message, failed: result.failed });
      if (result.failed) return false;
      const refreshed = await call<SettingsTab[]>("ConfigurationDocument");
      if (Array.isArray(refreshed) && refreshed.length > 0) {
        setTabs(refreshed);
      }
      // 配置已热重载：来源规则 / 日志列都可能变了，顺手刷新界面状态
      await refreshInitial();
      return true;
    },
    [activeTab, refreshInitial],
  );

  /* ---------- 关于 / 附加窗口 ---------- */

  const openAbout = useCallback(async () => {
    const info = await call<AboutInfo>("About");
    if (!info?.version) return;
    setAbout(info);
    setAboutOpen(true);
  }, []);

  const stopVoice = useCallback(() => {
    const voice = voiceRef.current;
    if (!voice) return;
    voice.pause();
    voice.currentTime = 0;
  }, []);

  const handleIconClick = useCallback(() => {
    const now = Date.now();
    iconClicks.current = iconClicks.current.filter((time) => now - time < ICON_CLICK_WINDOW_MS);
    iconClicks.current.push(now);
    if (iconClicks.current.length < ICON_CLICK_COUNT) return;
    iconClicks.current = [];
    setAboutOpen(false);
    setHiddenOpen(true);
    // 声音在同一个点击回调里播放：属于用户手势，不会被自动播放策略拦下
    const voice = voiceRef.current;
    if (voice) {
      voice.currentTime = 0;
      void voice.play().catch(() => undefined);
    }
  }, []);

  const closeHidden = useCallback(() => {
    stopVoice(); // 不能只靠关闭事件：个别引擎下它会延迟派发
    setHiddenOpen(false);
  }, [stopVoice]);

  /* ---------- Go 侧事件 ---------- */

  useEffect(() => {
    onEvent("show-about", () => void openAbout());
    onEvent("show-settings", () => void openSettings());
    onEvent("log-updated", (text) => setLog(String(text ?? "")));
    onEvent("show-message", (payload) => {
      const data = payload as { title?: string; message?: string; failed?: boolean } | undefined;
      if (!data) return;
      setMessage({
        title: data.title ?? "Info",
        message: data.message ?? "",
        failed: data.failed ?? false,
      });
    });
  }, [openAbout, openSettings]);

  // 多人模式（有 users.yaml）但没有会话 → 先登录；单机模式后端已自动建立会话，直接进主界面。
  if (roles.length > 0 && !session && !singleUser) {
    return (
      <>
        <LoginScreen roles={roles} username={username} onLogin={handleLogin} busy={busy} />
        <LoadingOverlay text={busy} />
        <MessageModal state={message} onClose={() => setMessage(null)} />
      </>
    );
  }

  return (
    <>
      <div className="app-frame">
        <div className="app-shell">
          <div className={`columns${leftCollapsed ? " left-collapsed" : ""}`}>
          <div className="left">
            <Card
              className="card"
              size="small"
              title="数据导入"
              extra={
                // 清空：只有白色图标（无文字、无边框），标题栏里靠右，省下模块内那一行的高度；
                // 只有 Admin 有这个动作（权限矩阵 clear）
                can("clear") ? (
                  <Tooltip title="清空导入数据">
                    <Button
                      className="head-icon-action"
                      type="text"
                      aria-label="清空导入数据"
                      icon={<Trash2 size={16} />}
                      onClick={handleClear}
                    />
                  </Tooltip>
                ) : null
              }
            >
              <ImportPanel
                states={imports}
                importableSources={importableSources}
                viewableSources={viewableSources}
                shareInfo={shareInfo}
                locked={shareBlocked}
                onImport={handleImport}
                onReview={openSourceReview}
                onReviewCleaned={openCleanedReview}
              />
            </Card>
            {/* 记录导出：与其余模块同构（红标题栏 + 正文一行），位置在数据导入与操作日志之间 */}
            <Card className="card record-card" size="small" title="记录导出">
              <RecordExportPanel
                start={startDate}
                end={endDate}
                onStartChange={setStartDate}
                onEndChange={setEndDate}
                onReview={openLogReview}
              />
            </Card>
            {/* 操作日志住在左列最下面：按左列剩余高度撑满，窗口越高日志越长，不留空白 */}
            <LogPanel text={log} />
          </div>
          {/* 竖向分隔上的收起/展开按钮：收起后右侧结果表吃满整个窗口宽度 */}
          <Tooltip title={leftCollapsed ? "展开左侧面板" : "收起左侧面板"}>
            <Button
              className="left-toggle"
              aria-label={leftCollapsed ? "展开左侧面板" : "收起左侧面板"}
              aria-expanded={!leftCollapsed}
              icon={leftCollapsed ? <ChevronRight size={12} /> : <ChevronLeft size={12} />}
              onClick={() => setLeftCollapsed((current) => !current)}
            />
          </Tooltip>
          <div className="right">
            {canAlign && overview?.enabled ? (
              <Card
                className="card align-card"
                size="small"
                title="数据对齐"
                extra={
                  <div className="align-head-extra">
                    <Tag className="align-status" color={alignSummary.color}>
                      {alignSummary.text}
                    </Tag>
                    <span className="align-auto">
                      <span className="align-auto-label">自动对齐</span>
                      <Switch size="small" checked={autoAlign} onChange={setAutoAlign} />
                    </span>
                    <Tooltip
                      title={
                        alignSummary.pending === 0
                          ? "没有需要对齐的来源"
                          : "把落后的来源都对齐到共享盘最新版本"
                      }
                    >
                      <Button
                        className="align-run"
                        aria-label="全部对齐"
                        icon={<RefreshCw size={16} />}
                        loading={aligning}
                        disabled={aligning || alignSummary.pending === 0}
                        onClick={handleAlignAll}
                      >
                        全部对齐
                      </Button>
                    </Tooltip>
                  </div>
                }
              >
                <AlignPanel sources={overview.sources} aligning={aligning} onAlign={handleAlign} />
              </Card>
            ) : null}
            <Card
              className="card grow consolidation-card"
              size="small"
              title={
                // 标题栏：模块名在左，「整合」主按钮在整条标题栏正中（绝对定位居中，见 styles.css）
                <>
                  <span className="consolidation-name">数据整合</span>
                  <Tooltip
                    title={
                      shareBlocked
                        ? "共享盘异常：处理后再整合"
                        : !can("consolidate")
                          ? "当前身份没有整合权限"
                          : !allAligned
                            ? "还有数据未对齐：先在「数据对齐」里对齐"
                            : "整合数据"
                    }
                  >
                    <Button
                      className="consolidation-run"
                      type="primary"
                      icon={<Merge size={16} />}
                      disabled={!can("consolidate") || shareBlocked || !allAligned}
                      onClick={handleConsolidate}
                    >
                      整合
                    </Button>
                  </Tooltip>
                </>
              }
              extra={
                // 生成文件：与「整合」同一形式（白底、红字、图标 + 文字、28px 高），留在标题栏右侧；
                // 没有合格行时禁用并给出原因
                <Tooltip
                  title={
                    !can("export")
                      ? "当前身份没有导出权限"
                      : readyCount === 0
                        ? "还没有可导出的行（只有合格的行会导出）"
                        : "生成文件"
                  }
                >
                  <Button
                    className="consolidation-export"
                    aria-label="生成文件"
                    icon={<FileDown size={16} />}
                    disabled={!can("export") || readyCount === 0}
                    onClick={handleExport}
                  >
                    生成文件
                  </Button>
                </Tooltip>
              }
            >
              <ConsolidationPanel
                columns={columns}
                rows={rows}
                statuses={statuses}
                counts={counts}
                conflicts={conflicts}
              />
            </Card>
          </div>
        </div>
        </div>

        {/* 底部状态栏：登录人 + 共享盘状态 + 切换身份（图标），贴着窗口最底部 */}
        <div className="status-bar">
          <span className="status-user">
            已登录：{session?.name ?? "—"}（{session?.roleLabel ?? "—"}）
          </span>
          {share ? (
            <span className={share.healthy ? "status-share" : "status-share status-share-bad"}>
              共享盘：{share.mode === "local" ? "本地模式" : share.healthy ? "正常" : "异常"}
            </span>
          ) : null}
          {overview?.lastConsolidation ? (
            <span className="status-consolidated">
              最后整合：{overview.lastConsolidation.at}（{overview.lastConsolidation.by}）
            </span>
          ) : null}
          {share?.mode === "share" ? (
            <Tooltip title="重新检查共享盘">
              <Button
                className="status-recheck"
                type="text"
                size="small"
                aria-label="重新检查共享盘"
                icon={<RefreshCw size={13} />}
                onClick={handleRecheckShare}
              />
            </Tooltip>
          ) : null}
          {session?.singleUser ? (
            <span className="status-share">单机模式（全权限）</span>
          ) : (
            <Tooltip title="切换身份">
              <Button
                className="status-switch"
                type="text"
                size="small"
                aria-label="切换身份"
                icon={<Users size={14} />}
                onClick={handleLogout}
              />
            </Tooltip>
          )}
        </div>
      </div>

      {/* 附加窗口的声音：loop 表示窗口开着就一直响；播放/停止见上面的点击与关闭逻辑 */}
      <audio id="puppy-voice" ref={voiceRef} src="puppy-voice.mp3" loop preload="auto" />

      <LoadingOverlay text={busy} />
      <MessageModal state={message} onClose={() => setMessage(null)} />
      <ReviewModal
        open={reviewOpen}
        result={review}
        exporting={false}
        onPage={loadReviewPage}
        onExport={handleReviewExport}
        onClose={() => setReviewOpen(false)}
      />
      <SettingsModal
        open={settingsOpen}
        tabs={tabs}
        activeKey={activeTab}
        editing={editing}
        editorValue={editorValue}
        path={treePath}
        expandedKeys={expandedKeys}
        onSelectTab={selectTab}
        onSelectNode={setTreePath}
        onExpandedChange={setExpandedKeys}
        onToggleAll={toggleAll}
        onStartEdit={() => {
          const tab = tabs.find((item) => item.key === activeTab);
          setEditorValue(tab?.raw ?? "");
          setEditing(true);
        }}
        onCancelEdit={() => setEditing(false)}
        onEditorChange={setEditorValue}
        onSave={saveSettings}
        onSaveValues={saveSettingsValues}
        onClose={() => setSettingsOpen(false)}
      />
      <AboutModal
        open={aboutOpen}
        info={about}
        onIconClick={handleIconClick}
        onClose={() => setAboutOpen(false)}
      />
      <HiddenModal open={hiddenOpen} onClose={closeHidden} />
    </>
  );
}

/** 前两层默认展开：打开就能看到顶层键与来源/字段轮廓，一千多行的配置不至于铺满整屏。 */
function defaultExpandedKeys(nodes: SettingsNodeList, depth = 0): string[] {
  if (depth > 1) return [];
  const keys: string[] = [];
  const walk = (list: SettingsNodeList, path: string[], level: number): void => {
    if (level > 1) return;
    for (const node of list) {
      const next = [...path, node.key];
      keys.push(next.map((part) => encodeURIComponent(part)).join("/"));
      if (node.children && node.children.length > 0) walk(node.children, next, level + 1);
    }
  };
  walk(nodes, [], depth);
  return keys;
}

function allKeys(nodes: SettingsNodeList): string[] {
  const keys: string[] = [];
  const walk = (list: SettingsNodeList, path: string[]): void => {
    for (const node of list) {
      const next = [...path, node.key];
      if (node.children && node.children.length > 0) {
        keys.push(next.map((part) => encodeURIComponent(part)).join("/"));
        walk(node.children, next);
      }
    }
  };
  walk(nodes, []);
  return keys;
}

type SettingsNodeList = import("./types").SettingsNode[];
