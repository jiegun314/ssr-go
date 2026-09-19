// 前端：调 Go 侧绑定（window.go.main.App.*），不引入任何框架。
//
// 界面结构与文案照搬现有 PySide6 版（AGENTS.md §6）：四个导入组（圆点 + 标题 + 导入按钮 +
// 状态标签 + 「…」回顾按钮）、右下角「清空导入数据」、整合结果表（按状态着色、MISSING 加粗）、
// 记录导出、操作日志。

// 四个来源的顺序与标题照搬原界面（ui/forms/main_window.ui 的布局顺序）：
// 医保代码信息在最上，与下面三个之间有一个空行（原设计里是 horizontalSpacer），
// 然后是产品类别、UDI团队信息、RA信息。界面写「医保代码信息」，配置里是「医保编码信息」。
const SOURCES = [
  { key: "medical_insurance_code", title: "医保代码信息" },
  { key: "product_category", title: "产品类别" },
  { key: "global_udi_input", title: "UDI团队信息" },
  { key: "ra_input", title: "RA信息" },
];

// 第一个分组之后插一个空行（对应原界面的 horizontalSpacer）。
const GROUP_GAP_AFTER = 1;

const STATUS_COLOURS = {
  Ready: "#DFF6DD",
  Incomplete: "#FDE7E9",
  Duplicate: "#FFF4CE",
  Conflict: "#F4CCCC",
};

let state = { imports: {}, busy: false };

// 数据回顾窗口的状态：按页取（每页 100 行），大表不会一次性塞给前端。
const REVIEW_PAGE_SIZE = 100;
let review = { kind: "source", source: "", fileType: "", start: "", end: "", page: 1, pageCount: 1 };

async function openReviewPage(page) {
  const result = review.kind === "log"
    ? await call("ReviewLog", review.start, review.end, page, REVIEW_PAGE_SIZE)
    : await call("ReviewSource", review.source, page, REVIEW_PAGE_SIZE);
  setLog(result.log);
  if (result.failed) {
    showModal(result.title, result.message);
    return;
  }
  renderTable("review-table", result.columns, result.rows);
  document.getElementById("review-title").textContent = result.title;
  review.page = result.page || 1;
  review.pageCount = result.pageCount || 1;
  document.getElementById("review-page-info").textContent =
    `第 ${review.page} / ${review.pageCount} 页（共 ${result.total} 行）`;
  document.getElementById("review-prev").disabled = review.page <= 1;
  document.getElementById("review-next").disabled = review.page >= review.pageCount;
}

// Google Material 图标的官方路径（Apache-2.0），内嵌在页面里 —— 这是离线工具，
// 不能引外链字体。尺寸与配色由 .icon-btn 控制。
const ICONS = {
  // file_download：导入（箭头朝内落到托盘上）。注意 file_upload 的箭头朝外，
  // 那是「导出/上传」的语义，用在这里是错的。
  import:
    '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M19 9h-4V3H9v6H5l7 7 7-7zM5 18v2h14v-2H5z"/></svg>',
  // visibility：数据回顾
  review:
    '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 4.5C7 4.5 2.73 7.61 1 12c1.73 4.39 6 7.5 11 7.5s9.27-3.11 11-7.5c-1.73-4.39-6-7.5-11-7.5zM12 17c-2.76 0-5-2.24-5-5s2.24-5 5-5 5 2.24 5 5-2.24 5-5 5zm0-8c-1.66 0-3 1.34-3 3s1.34 3 3 3 3-1.34 3-3-1.34-3-3-3z"/></svg>',
  // delete：清空导入数据
  clear:
    '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 19c0 1.1.9 2 2 2h8c1.1 0 2-.9 2-2V7H6v12zM19 4h-3.5l-1-1h-5l-1 1H5v2h14V4z"/></svg>',
  // history：记录导出区的「回顾」（按时间区间回看操作日志）
  history:
    '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M13 3c-4.97 0-9 4.03-9 9H1l3.89 3.89.07.14L9 12H6c0-3.87 3.13-7 7-7s7 3.13 7 7-3.13 7-7 7c-1.93 0-3.68-.79-4.94-2.06l-1.42 1.42C8.27 19.99 10.51 21 13 21c4.97 0 9-4.03 9-9s-4.03-9-9-9zm-1 5v5l4.28 2.54.72-1.21-3.5-2.08V8H12z"/></svg>',
};

const bridge = () => (window.go && window.go.main ? window.go.main.App : null);

async function call(method, ...args) {
  const app = bridge();
  if (!app || typeof app[method] !== "function") {
    return { failed: true, title: "Error", message: "后端绑定不可用（请通过 wails build 的产物运行）" };
  }
  setBusy(true, method);
  // 等两帧：确保载入图层先绘制出来，再开始调用后端（不然窗口会先"假死"一下）
  await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
  try {
    return await app[method](...args);
  } catch (error) {
    return { failed: true, title: "Error", message: String(error) };
  } finally {
    setBusy(false, method);
  }
}

// 长任务的载入文案：导入 / 整合 / 回顾三条沿用原界面状态栏的原文（§6.2）。
const BUSY_TEXT = {
  SelectImportFile: "正在打开文件夹",
  ImportSource: "正在导入 Excel 数据...",
  Consolidate: "正在整合数据...",
  ReviewSource: "正在加载已导入数据...",
  ReviewLog: "正在加载已导入数据...",
  SelectExportTarget: "正在选择保存位置",
  Export: "正在导出文件...",
  SelectReviewExportTarget: "正在选择保存位置",
  ExportReviewData: "正在导出文件...",
  ClearImportedData: "正在清空导入数据...",
};

function setBusy(busy, method) {
  state.busy = busy;
  document.body.classList.toggle("busy", busy);
  const layer = document.getElementById("loading");
  const text = document.getElementById("loading-text");
  if (!layer) return;
  layer.hidden = !busy;
  if (busy && text) text.textContent = BUSY_TEXT[method] || "正在处理...";
}

function showModal(title, message) {
  if (!title) return;
  document.getElementById("modal-title").textContent = title;
  document.getElementById("modal-text").textContent = message || "";
  document.getElementById("modal").showModal();
}

function renderImports() {
  const area = document.getElementById("import-area");
  area.innerHTML = "";
  SOURCES.forEach((source, index) => {
    const current = state.imports[source.key] || { state: "empty", tooltip: "尚未导入", label: "" };
    const group = document.createElement("div");
    group.className = "group";
    group.innerHTML = `
      <div class="title-row">
        <span class="dot ${current.state}" title="${current.tooltip}"></span>
        <span style="margin-left:8px" title="${current.tooltip}">${source.title}</span>
      </div>
      <div class="actions">
        <button class="icon-btn" data-action="import" data-source="${source.key}"
                title="导入" aria-label="导入">${ICONS.import}</button>
        <span class="status" id="status-${source.key}">${current.label || ""}</span>
        <button class="icon-btn" data-action="review" data-source="${source.key}"
                title="数据回顾" aria-label="数据回顾">${ICONS.review}</button>
      </div>`;
    area.appendChild(group);
    if (index + 1 === GROUP_GAP_AFTER) {
      const gap = document.createElement("div");
      gap.className = "group-gap";
      area.appendChild(gap);
    }
  });
  const clean = document.createElement("div");
  clean.className = "clean-row";
  clean.innerHTML =
    `<button class="icon-btn" id="btn-clean" title="清空导入数据" aria-label="清空导入数据">${ICONS.clear}</button>`;
  area.appendChild(clean);
}

function applyImportStates(imports) {
  if (!imports) return;
  state.imports = imports;
  for (const source of SOURCES) {
    const current = imports[source.key];
    if (!current) continue;
    const group = document.querySelector(`[data-source="${source.key}"]`)?.closest(".group");
    const dot = group?.querySelector(".dot");
    if (dot) {
      dot.className = "dot " + current.state;
      dot.title = current.tooltip || "";
    }
    const status = document.getElementById("status-" + source.key);
    if (status) status.textContent = current.label || "";
  }
}

function renderTable(tableId, columns, rows, statuses) {
  const table = document.getElementById(tableId);
  table.innerHTML = "";
  if (!columns || columns.length === 0) return;
  const head = document.createElement("tr");
  for (const column of columns) {
    const cell = document.createElement("th");
    cell.textContent = column;
    head.appendChild(cell);
  }
  table.appendChild(head);
  rows.forEach((row, index) => {
    const line = document.createElement("tr");
    row.forEach((value, columnIndex) => {
      const cell = document.createElement("td");
      cell.textContent = value;
      if (value === "MISSING") cell.classList.add("MISSING");
      if (columnIndex === 0 && statuses && STATUS_COLOURS[statuses[index]]) {
        line.style.background = STATUS_COLOURS[statuses[index]];
      }
      line.appendChild(cell);
    });
    table.appendChild(line);
  });
}

function setLog(text) {
  const log = document.getElementById("log");
  log.value = text || "";
  log.scrollTop = log.scrollHeight;
}

function defaultRange() {
  const today = new Date();
  const start = new Date(today);
  start.setFullYear(start.getFullYear() - 1);
  const end = new Date(today);
  end.setDate(end.getDate() + 1);
  const iso = (date) => date.toISOString().slice(0, 10).replace(/-/g, "/");
  document.getElementById("start-date").value = iso(start);
  document.getElementById("end-date").value = iso(end);
}

document.addEventListener("click", async (event) => {
  const button = event.target.closest("button");
  if (!button) return;
  const source = button.dataset.source;
  switch (button.dataset.action || button.id) {
    case "import": {
      // 第一步：打开文件对话框（载入图层显示「正在打开文件夹」）
      const filePath = await call("SelectImportFile", source);
      if (typeof filePath !== "string" || filePath === "") break; // 取消或失败：什么都不做
      // 第二步：真正导入（载入图层显示「正在导入 Excel 数据...」）
      const result = await call("ImportSource", source, filePath);
      applyImportStates(result.state ? { ...state.imports, [source]: result.state } : state.imports);
      setLog(result.log);
      showModal(result.title, result.message);
      break;
    }
    case "review": {
      review = { kind: "source", source, fileType: source, page: 1, pageCount: 1 };
      await openReviewPage(1);
      document.getElementById("review").showModal();
      break;
    }
    case "review-prev": {
      if (review.page > 1) await openReviewPage(review.page - 1);
      break;
    }
    case "review-next": {
      if (review.page < review.pageCount) await openReviewPage(review.page + 1);
      break;
    }
    case "review-export": {
      const choice = await call("SelectReviewExportTarget", review.fileType);
      if (!choice || !choice.target) break; // 取消：什么都不做
      const result = await call("ExportReviewData", review.fileType, choice.target);
      setLog(result.log);
      showModal(result.title, result.message);
      break;
    }
    case "btn-clean": {
      const result = await call("ClearImportedData");
      setLog(result.log);
      if (!result.failed) {
        const cleared = result.cleared || [];
        const next = { ...state.imports };
        for (const key of cleared) {
          next[key] = { state: "empty", label: "已清空", tooltip: "尚未导入" };
        }
        applyImportStates(next);
      }
      showModal(result.title, result.message);
      break;
    }
    case "btn-consolidate": {
      const result = await call("Consolidate");
      setLog(result.log);
      if (result.failed) return showModal(result.title, result.message);
      renderTable("result-table", result.columns, result.rows, result.statuses);
      break;
    }
    case "btn-export": {
      // 第一步：选择保存位置（载入图层显示「正在选择保存位置」）
      const choice = await call("SelectExportTarget");
      if (!choice || !choice.target) break; // 取消：什么都不做
      // 第二步：写文件（载入图层显示「正在导出文件...」）
      const result = await call("Export", choice.defaultName, choice.target);
      setLog(result.log);
      showModal(result.title, result.message);
      break;
    }
    case "btn-review-log": {
      const start = document.getElementById("start-date").value;
      const end = document.getElementById("end-date").value;
      review = { kind: "log", source: "", fileType: "operation_log", start, end, page: 1, pageCount: 1 };
      await openReviewPage(1);
      document.getElementById("review").showModal();
      break;
    }
  }
});

// 关于窗口（§6.4）：图标 + 名称 + 一行「Version: x.y.z」，commit 与构建时间放在 tooltip 里。
async function openAbout() {
  const about = await call("About");
  if (!about || !about.version) return;
  // 名称与说明段是固定文案（与 ui/forms/about_dialog.ui 一致），这里只写版本与 tooltip
  const version = document.getElementById("about-version");
  version.textContent = about.version;
  version.title = about.detail || "";
  document.getElementById("about").showModal();
}

// 彩蛋：在关于窗口的图标上 5 秒内连点 8 次（原版行为，必须保留）。
let aboutClicks = [];
const EASTER_EGG_CLICKS = 8;
const EASTER_EGG_WINDOW_MS = 5000;

document.addEventListener("DOMContentLoaded", () => {
  defaultRange();
  document.getElementById("about-icon").addEventListener("click", () => {
    const now = Date.now();
    aboutClicks = aboutClicks.filter((time) => now - time < EASTER_EGG_WINDOW_MS);
    aboutClicks.push(now);
    if (aboutClicks.length >= EASTER_EGG_CLICKS) {
      aboutClicks = [];
      document.getElementById("about").close();
      document.getElementById("egg-dialog").showModal();
    }
  });
  // 菜单「关于」由 Go 侧发事件（runtime.EventsEmit("show-about")）
  if (window.runtime && window.runtime.EventsOn) {
    window.runtime.EventsOn("show-about", () => openAbout());
  }
});

(async () => {
  const initial = await call("InitialState");
  if (initial && initial.imports) {
    applyImportStates(initial.imports);
    renderImports();
    applyImportStates(initial.imports);
  } else {
    renderImports();
  }
  setLog(initial && initial.operationLog);
})();
