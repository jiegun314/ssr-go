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

const bridge = () => (window.go && window.go.main ? window.go.main.App : null);

async function call(method, ...args) {
  const app = bridge();
  if (!app || typeof app[method] !== "function") {
    return { failed: true, title: "Error", message: "后端绑定不可用（请通过 wails build 的产物运行）" };
  }
  setBusy(true);
  try {
    return await app[method](...args);
  } catch (error) {
    return { failed: true, title: "Error", message: String(error) };
  } finally {
    setBusy(false);
  }
}

function setBusy(busy) {
  state.busy = busy;
  document.body.classList.toggle("busy", busy);
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
        <button data-action="import" data-source="${source.key}">导入</button>
        <span class="status" id="status-${source.key}">${current.label || ""}</span>
        <button data-action="review" data-source="${source.key}" title="数据回顾">...</button>
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
  clean.innerHTML = `<button id="btn-clean" title="清空导入数据">...</button>`;
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
      const result = await call("ImportSource", source);
      applyImportStates(result.state ? { ...state.imports, [source]: result.state } : state.imports);
      setLog(result.log);
      showModal(result.title, result.message);
      break;
    }
    case "review": {
      const result = await call("ReviewSource", source);
      setLog(result.log);
      if (result.failed) return showModal(result.title, result.message);
      renderTable("review-table", result.columns, result.rows);
      document.getElementById("review-title").textContent = result.title;
      document.getElementById("review").showModal();
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
      const result = await call("Export");
      setLog(result.log);
      showModal(result.title, result.message);
      break;
    }
    case "btn-review-log": {
      const start = document.getElementById("start-date").value;
      const end = document.getElementById("end-date").value;
      const result = await call("ReviewLog", start, end);
      setLog(result.log);
      if (result.failed) return showModal(result.title, result.message);
      renderTable("review-table", result.columns, result.rows);
      document.getElementById("review-title").textContent = result.title;
      document.getElementById("review").showModal();
      break;
    }
  }
});

// 关于窗口的 8 连击彩蛋（§6.4，必须保留）：5 秒内点 8 次图标就放彩蛋图。
let clickTimes = [];
document.addEventListener("DOMContentLoaded", () => {
  const icon = document.createElement("img");
  icon.id = "about-icon";
  icon.src = "logo.png";
  icon.alt = "SingleSourceReady";
  icon.title = "关于";
  icon.addEventListener("click", async () => {
    const now = Date.now();
    clickTimes = clickTimes.filter((time) => now - time < 5000);
    clickTimes.push(now);
    if (clickTimes.length >= 8) {
      clickTimes = [];
      document.getElementById("egg").style.display = "flex";
      return;
    }
    const about = await call("About");
    if (typeof about === "object" && about.version) {
      showModal("About", `${about.name}\n${about.version}\n${about.detail}`);
    }
  });
  document.querySelector(".wrap").prepend(icon);
  defaultRange();
});

document.addEventListener("keydown", (event) => {
  if (event.key === "Escape") document.getElementById("egg").style.display = "none";
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
