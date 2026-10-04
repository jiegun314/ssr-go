// 主界面：左列（数据导入 / 记录导出）+ 右列（数据整合）+ 底部（操作日志）+ 四个弹窗。
// 布局与原来一致；四个模块的容器换成 antd 的 Card（标题排版、边框、圆角都由 antd 给）。

import { Button, Card, Tooltip } from "antd";
import { ChevronLeft, ChevronRight, FileDown, Merge, Trash2 } from "lucide-react";
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
import { AboutModal, HiddenModal, ReviewModal, SettingsModal } from "./components/Modals";
import type {
  AboutInfo,
  ClearResult,
  ConsolidateResult,
  ExportResult,
  ExportTarget,
  ImportResult,
  ImportState,
  InitialState,
  ReviewResult,
  SettingsSaveResult,
  SettingsTab,
} from "./types";

/** 关于窗口图标上的点击计数：阈值与时间窗沿用原版，属行为契约，不要改这两个数值。 */
const ICON_CLICK_COUNT = 8;
const ICON_CLICK_WINDOW_MS = 5000;

type ReviewSession = {
  kind: "source" | "log";
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
  }, []);

  useEffect(() => {
    void refreshInitial();
  }, [refreshInitial]);

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

  const loadReviewPage = useCallback(async (page: number) => {
    const session = reviewSession.current;
    if (!session) return;
    const result =
      session.kind === "log"
        ? await call<ReviewResult>("ReviewLog", session.start, session.end, page, REVIEW_PAGE_SIZE)
        : await call<ReviewResult>("ReviewSource", session.source, page, REVIEW_PAGE_SIZE);
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
    const summary: Record<string, number> = {};
    for (const status of result.statuses ?? []) {
      summary[status] = (summary[status] ?? 0) + 1;
    }
    setCounts(summary);
  }, []);

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

  return (
    <>
      <div className="app-shell">
        <div className={`columns${leftCollapsed ? " left-collapsed" : ""}`}>
          <div className="left">
            <Card
              className="card"
              size="small"
              title="数据导入"
              extra={
                // 清空：只有白色图标（无文字、无边框），标题栏里靠右，省下模块内那一行的高度
                <Tooltip title="清空导入数据">
                  <Button
                    className="head-icon-action"
                    type="text"
                    aria-label="清空导入数据"
                    icon={<Trash2 size={16} />}
                    onClick={handleClear}
                  />
                </Tooltip>
              }
            >
              <ImportPanel
                states={imports}
                onImport={handleImport}
                onReview={openSourceReview}
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
            <Card
              className="card grow consolidation-card"
              size="small"
              title={
                // 标题栏：模块名在左，「整合」主按钮在整条标题栏正中（绝对定位居中，见 styles.css）
                <>
                  <span className="consolidation-name">数据整合</span>
                  <Button
                    className="consolidation-run"
                    type="primary"
                    icon={<Merge size={16} />}
                    onClick={handleConsolidate}
                  >
                    整合
                  </Button>
                </>
              }
              extra={
                // 生成文件：只有图标（靠右），没有合格行时禁用并给出原因
                <Tooltip title={readyCount === 0 ? "还没有可导出的行（只有合格的行会导出）" : "生成文件"}>
                  <Button
                    className="head-icon-action"
                    type="text"
                    aria-label="生成文件"
                    icon={<FileDown size={18} />}
                    disabled={readyCount === 0}
                    onClick={handleExport}
                  />
                </Tooltip>
              }
            >
              <ConsolidationPanel
                columns={columns}
                rows={rows}
                statuses={statuses}
                counts={counts}
              />
            </Card>
          </div>
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
