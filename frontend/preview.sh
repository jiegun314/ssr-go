#!/usr/bin/env bash
# SSR 前端本地预览：不需要 Go 后端与 Wails 运行时，直接把 frontend/dist 起成一个本地页面，
# 并注入一份模拟的 Wails 桥接（window.go / window.runtime），用来核对界面与「操作日志」窗口。
#
#   ./frontend/preview.sh              # 用现有 dist（改过源码就先加 --build）
#   ./frontend/preview.sh --build      # 先 npm run build 再预览
#   ./frontend/preview.sh --no-open    # 不自动开浏览器
#   PORT=9000 ./frontend/preview.sh    # 换起始端口（被占用会自动往后找）
#
# 页面里的按钮都是活的：导入 / 整合 / 导出 / 回顾 / 保存配置 都会往日志里写一条
# 带级别、带中文的新日志，方便核对日志窗口的分栏目、换行与分割线。
#
# 用 ?screen=... 可以直接打开某个画面：
#   （不传）主界面   review 数据回顾   settings 参数设定   about 关于
#   message 提示弹窗   log-error / log-warning 切到某个日志栏目
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DIST="$DIR/dist"
PORT="${PORT:-8788}"
BUILD=0
OPEN_BROWSER=1

for arg in "$@"; do
  case "$arg" in
    --build) BUILD=1 ;;
    --no-open) OPEN_BROWSER=0 ;;
    -h|--help) sed -n '2,15p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "未知参数：$arg（可用：--build / --no-open / --help）" >&2; exit 2 ;;
  esac
done

if [ "$BUILD" = 1 ]; then
  echo "▸ 构建前端：npm run build"
  (cd "$DIR" && npm run build)
fi
if [ ! -f "$DIST/index.html" ]; then
  echo "找不到 $DIST/index.html —— 先执行 (cd frontend && npm run build)，或加 --build" >&2
  exit 1
fi

PYTHON="$(command -v python3 || command -v python || true)"
if [ -z "$PYTHON" ]; then
  echo "预览需要一个静态文件服务器：请先安装 python3（或者用任意静态服务器指向 frontend/dist）" >&2
  exit 1
fi

WORK="$(mktemp -d "${TMPDIR:-/tmp}/ssr-preview.XXXXXX")"
SERVER_PID=""
cleanup() {
  if [ -n "$SERVER_PID" ]; then kill "$SERVER_PID" 2>/dev/null || true; fi
  rm -rf "$WORK"
}
trap cleanup EXIT INT TERM

cp -R "$DIST/." "$WORK/"

JS="$(cd "$DIST/assets" && ls ./*.js 2>/dev/null | head -1)"
CSS="$(cd "$DIST/assets" && ls ./*.css 2>/dev/null | head -1)"
if [ -z "$JS" ] || [ -z "$CSS" ]; then
  echo "$DIST/assets 里缺 js/css —— 先执行 (cd frontend && npm run build)" >&2
  exit 1
fi

# 预览自己的 index.html：模拟桥接必须在 bundle 之前执行
cat > "$WORK/index.html" <<HTML
<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>SSR 本地预览</title>
    <script src="./mock.js"></script>
    <link rel="stylesheet" href="./assets/${CSS#./}" />
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="./assets/${JS#./}"></script>
  </body>
</html>
HTML

cat > "$WORK/mock.js" <<'MOCK'
// 模拟 Wails 注入的 window.go / window.runtime。仅由 frontend/preview.sh 使用，不进构建产物。
(function () {
  const LEVEL_LABEL = { info: "信息", success: "成功", warning: "警告", error: "错误" };
  const handlers = {};
  const pad = (value) => String(value).padStart(2, "0");
  let clock = new Date(2026, 0, 5, 9, 12, 3);
  const lines = [];

  const stamp = () => {
    clock = new Date(clock.getTime() + 1000);
    return (
      `${clock.getFullYear()}-${pad(clock.getMonth() + 1)}-${pad(clock.getDate())} ` +
      `${pad(clock.getHours())}:${pad(clock.getMinutes())}:${pad(clock.getSeconds())}`
    );
  };
  const push = (level, message) => lines.push(`${stamp()} [${LEVEL_LABEL[level]}] ${message}`);
  const logText = () => lines.join("\n");

  push("info", "应用已启动");
  push("info", "配置快照已保存：/Users/demo/ssr/config/.backup/20260105-091203/setting.yaml");
  push("info", "启动未清理暂存表（cleanup_on_startup = false）");
  push("info", "检测到 2 个来源有上一次运行留下的数据：医保代码信息 1,248 行、UDI团队信息 3,072 行");
  push("success", "医保代码信息导入成功，共 1,248 行");
  push("warning", "产品类别导入完成：412 行，其中 3 行缺值被跳过");
  push("error", "UDI团队信息导入失败：\n第 14 行：使用单元产品标识（device_identifier_use_unit）为空，因为 最小销售单元中使用单元的数量（quantity_per_min_sales_unit） = 2.5 > 1\n技术详情：row 14 violates required_when condition");
  push("success", "数据整合完成：整合 9 行，缺失 2 行，重复 2 行，变更 1 行，冲突 1 行");
  push("warning", "缺失数据（2 行）：00841000111、00841000222");
  push("success", "整合结果已导出到 /Users/demo/ssr/output/export/SSR_20260105_UDI_master_data_consolidated.xlsx");
  push("info", "记录回顾完成：时间范围 2025/01/05 00:00:00 至 2026/01/05 23:59:59，共 137 行");

  const imports = {
    medical_insurance_code: {
      source: "medical_insurance_code", state: "existing",
      label: "1,248 行 · 2026-01-05 09:12", tooltip: "医保代码信息：1,248 行（上一次运行导入的）",
      rowCount: 1248, importTime: "2026-01-05 09:12",
    },
    product_category: {
      source: "product_category", state: "imported",
      label: "412 行 · 2026-01-05 09:13", tooltip: "产品类别：412 行，3 行缺值被跳过",
      rowCount: 412, importTime: "2026-01-05 09:13",
    },
    global_udi_input: {
      source: "global_udi_input", state: "failed",
      label: "导入失败", tooltip: "导入失败，详情见日志窗口",
      rowCount: 0, importTime: "",
    },
    ra_input: {
      source: "ra_input", state: "empty", label: "", tooltip: "尚未导入", rowCount: 0, importTime: "",
    },
  };

  const columns = ["UDI-DI", "品牌名称", "产品类别", "规格型号", "医保代码", "生产企业", "状态", "备注"];
  const statuses = ["Ready", "Ready", "Incomplete", "Duplicate", "Conflict", "Ready", "Incomplete", "Ready", "Duplicate"];
  const rows = statuses.map((status, index) => [
    `0084${String(100000 + index * 37)}`,
    index % 3 === 0 ? "ETHICON" : index % 3 === 1 ? "DePuy Synthes" : "CERENOVUS",
    index % 2 === 0 ? "骨科植入物" : "心血管介入",
    index % 4 === 0 ? "MISSING" : `规格-${index + 1}`,
    index % 3 === 0 ? "MISSING" : `C${3000 + index}`,
    "强生（上海）医疗器材有限公司",
    status,
    status === "Conflict" ? "同一 UDI-DI 的医保代码冲突" : "",
  ]);

  const settingsTree = [
    {
      key: "sources", kind: "map", children: [
        { key: "medical_insurance_code", kind: "string", value: "medical_insurance_code.xlsx" },
        { key: "global_udi_input", kind: "string", value: "global_udi_input.xlsx" },
        { key: "column_mapping", kind: "map", children: [
          { key: "udi_di", kind: "string", value: "UDI-DI" },
          { key: "brand", kind: "string", value: "品牌名称" },
        ] },
      ],
    },
    {
      key: "consolidation", kind: "map", children: [
        { key: "strict", kind: "bool", value: "true" },
        { key: "max_duplicates", kind: "number", value: "3" },
        { key: "ignored_fields", kind: "seq", children: [{ key: "0", kind: "string", value: "备注" }] },
        { key: "fallback", kind: "null", value: "null" },
        { key: "empty_section", kind: "map", children: [] },
      ],
    },
  ];
  const settingsTabs = [
    { key: "app", title: "应用", note: "界面与运行参数", path: "config/app.yaml", tree: settingsTree, raw: "sources:\n  medical_insurance_code: medical_insurance_code.xlsx\n" },
    { key: "rules", title: "规则", note: "字段校验与整合规则", path: "config/rules.yaml", tree: settingsTree, raw: "rules:\n  - name: required_udi_di\n" },
  ];

  const reviewRows = (count) => Array.from({ length: count }, (_, index) => [
    `2026/01/0${(index % 5) + 1} 09:1${index % 10}:03`,
    index % 2 === 0 ? "整合" : "导入",
    index % 3 === 0 ? "MISSING" : "成功",
    index % 4 === 0 ? "医保代码信息" : "UDI团队信息",
  ]);
  const reviewResult = (fileType, page) => ({
    log: logText(), title: fileType === "operation_log" ? "记录回顾 · 操作日志" : "数据回顾 · 医保代码信息",
    message: "", failed: false,
    columns: ["日志时间", "操作", "结果", "对象"],
    rows: reviewRows(14), total: 137, page: page || 1, pageSize: 100, pageCount: 2,
  });

  const app = {
    InitialState: async () => ({ version: "1.4.0", operationLog: logText(), imports, busy: false }),
    About: async () => ({ name: "SingleSource Ready", version: "1.4.0 (build 20260105)", detail: "本地预览 · 模拟数据" }),

    SelectImportFile: async () => "demo/来源文件.xlsx",
    ImportSource: async (source, filePath) => {
      const name = (imports[source] && imports[source].label) || source;
      push("success", `${source} 导入成功，共 1,248 行（${filePath}）`);
      return {
        source, fileName: filePath, rowCount: 1248, state: imports[source], log: logText(),
        title: "导入成功", message: `导入成功\n导入行数：1,248\n（预览数据，来源：${name}）`, failed: false,
      };
    },
    ClearImportedData: async () => {
      push("success", "导入数据已清空：医保代码信息、产品类别、UDI团队信息");
      return { log: logText(), title: "成功", message: "数据已清空", cleared: [], failed: false };
    },

    Consolidate: async () => {
      push("success", "数据整合完成：整合 9 行，缺失 2 行，重复 2 行，变更 1 行，冲突 1 行");
      push("warning", "缺失数据（2 行）：00841000111、00841000222");
      return {
        log: logText(), title: "整合完成", message: "一共 9 行：Ready 4、Incomplete 2、Duplicate 2、Conflict 1。",
        failed: false, columns, rows, statuses,
      };
    },

    SelectExportTarget: async () => ({ defaultName: "SSR_20260105.xlsx", target: "/Users/demo/ssr/output/export/SSR_20260105.xlsx" }),
    Export: async (defaultName, target) => {
      push("success", `整合结果已导出到 ${target}`);
      return { log: logText(), title: "成功", message: "整合结果已导出。", failed: false, path: target };
    },

    SelectReviewExportTarget: async () => ({ defaultName: "review.xlsx", target: "/Users/demo/ssr/output/export/review.xlsx" }),
    ExportReviewData: async (fileType, target) => {
      push("success", `回顾数据已导出到 ${target}（14 行）`);
      return { log: logText(), title: "成功", message: `已导出 14 行到：\n${target}`, failed: false, path: target };
    },

    ReviewLog: async (start, end, page) => reviewResult("operation_log", page),
    ReviewSource: async (source, page) => reviewResult("source", page),

    ConfigurationDocument: async () => settingsTabs,
    SaveConfigurationFile: async (key, value) => {
      push("success", `配置已保存：${key}`);
      return { title: "成功", message: `配置已保存：${key}\n原文件已备份为 config/.backup/20260105-091203/${key}\n注意：改动在重启程序后生效。`, failed: false, backup: "" };
    },
  };

  window.go = { main: { App: app } };
  window.runtime = {
    EventsOn(name, callback) { (handlers[name] = handlers[name] || []).push(callback); },
    EventsOff(name) { delete handlers[name]; },
  };

  const fire = (name, payload) => (handlers[name] || []).forEach((fn) => fn(payload));
  const after = (ms, fn) => window.setTimeout(fn, ms);
  const screen = new URLSearchParams(window.location.search).get("screen") || "";

  // 主界面先摆出"整合完成"的样子
  after(500, () => {
    const button = Array.from(document.querySelectorAll("button"))
      .find((node) => node.textContent.trim() === "数据整合");
    if (button) button.click();
  });

  const clickLogTab = (label) => after(1800, () => {
    const node = Array.from(document.querySelectorAll(".ant-segmented-item-label"))
      .find((item) => item.textContent.trim() === label);
    if (node) node.click();
  });

  if (screen === "review") after(1200, () => {
    const button = document.querySelector('[aria-label="回顾"]');
    if (button) button.click();
  });
  if (screen === "settings") after(1200, () => fire("show-settings"));
  if (screen === "about") after(1200, () => fire("show-about"));
  if (screen === "message") after(1200, () => fire("show-message", {
    title: "导入完成",
    message: "已载入 医保代码信息：1,248 行\n文件：medical_insurance_code.xlsx",
  }));
  if (screen === "log-error") clickLogTab("错误");
  if (screen === "log-warning") clickLogTab("警告");
})();
MOCK

for offset in 0 1 2 3 4 5 6 7 8 9; do
  candidate=$((PORT + offset))
  (cd "$WORK" && exec "$PYTHON" -m http.server "$candidate" --bind 127.0.0.1) >/dev/null 2>&1 &
  SERVER_PID=$!
  sleep 0.6
  if curl -fsS "http://127.0.0.1:$candidate/index.html" >/dev/null 2>&1; then
    PORT="$candidate"
    break
  fi
  kill "$SERVER_PID" 2>/dev/null || true
  SERVER_PID=""
done
if [ -z "$SERVER_PID" ]; then
  echo "端口 ${PORT}..$((PORT + 9)) 都被占用，换一个：PORT=9000 ./frontend/preview.sh" >&2
  exit 1
fi

BASE="http://127.0.0.1:$PORT/index.html"
cat <<INFO

▸ 本地预览已启动（模拟数据，不需要 Go 后端）
  主界面        $BASE
  数据回顾      $BASE?screen=review
  参数设定      $BASE?screen=settings
  关于          $BASE?screen=about
  提示弹窗      $BASE?screen=message
  日志栏目      $BASE?screen=log-error    $BASE?screen=log-warning

  核对要点：日志窗口按 全部/信息/成功/警告/错误 分栏目；左列时间、右列级别标签+正文；
  长路径自动换行；行间浅色分割线；右上角「N 条」；右侧「新日志置顶」可切换顺序；
  页面上的导入 / 整合 / 导出 / 回顾 / 保存 都会实时往日志里追加一条中文日志。

  按 Ctrl+C 结束预览。
INFO

if [ "$OPEN_BROWSER" = 1 ]; then
  case "$(uname -s)" in
    Darwin) open "$BASE" >/dev/null 2>&1 || true ;;
    Linux) xdg-open "$BASE" >/dev/null 2>&1 || true ;;
    MINGW*|MSYS*|CYGWIN*) cmd.exe /c start "" "$BASE" >/dev/null 2>&1 || true ;;
  esac
fi

wait "$SERVER_PID"
