// 数据整合结果表的行为测试：状态片筛选、查找框按整行匹配、两者叠加、空态文案。
//
// 真实渲染 + 真实点击，替代以前"在 Go 里 grep TSX 源码"的间接断言。

import { App as AntApp, ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { ConsolidationPanel } from "../components/Panels";
import { DEFAULT_PAGE_SIZE, PAGE_SIZE_OPTIONS } from "../tablePagination";

const COLUMNS = ["Primary DI", "Product Name", "Medical Insurance Code"];
const ROWS = [
  ["DI-001", "超声探头", "12345"],
  ["DI-002", "监护仪", "67890"],
  ["DI-003", "注射泵", ""],
];
const STATUSES = ["Ready", "Ready", "Incomplete"];
const COUNTS = { Ready: 2, Incomplete: 1, Duplicate: 0 };

const renderPanel = () =>
  render(
    <AntApp>
      <ConsolidationPanel columns={COLUMNS} rows={ROWS} statuses={STATUSES} counts={COUNTS} />
    </AntApp>,
  );

const visibleCells = () =>
  Array.from(document.querySelectorAll(".ant-table-tbody .ant-table-row")).map(
    (row) => row.textContent ?? "",
  );

describe("数据整合结果表", () => {
  it("状态片用中文文案并显示各状态数量", () => {
    renderPanel();
    const chips = Array.from(document.querySelectorAll(".status-chip")).map((n) => n.textContent ?? "");
    expect(chips.some((text) => text.includes("合格") && text.includes("2"))).toBe(true);
    expect(chips.some((text) => text.includes("缺失") && text.includes("1"))).toBe(true);
    expect(chips.some((text) => text.includes("重复"))).toBe(true);
    // 没有冲突时不显示第四个片
    expect(chips.some((text) => text.includes("冲突"))).toBe(false);
  });

  it("点状态片只留下该状态的行，再点一次恢复全部", async () => {
    const user = userEvent.setup();
    renderPanel();
    expect(visibleCells()).toHaveLength(3);

    await user.click(screen.getByRole("button", { name: /缺失/ }));
    const incomplete = visibleCells();
    expect(incomplete).toHaveLength(1);
    expect(incomplete[0]).toContain("DI-003");
    // 片上的 aria-pressed 标出当前筛选
    expect(screen.getByRole("button", { name: /缺失/ })).toHaveAttribute("aria-pressed", "true");

    await user.click(screen.getByRole("button", { name: /缺失/ }));
    expect(visibleCells()).toHaveLength(3);
  });

  it("查找框按整行任意单元格匹配（大小写不敏感）", async () => {
    const user = userEvent.setup();
    renderPanel();
    const search = screen.getByLabelText("查找表格内容");

    await user.type(search, "监护");
    expect(visibleCells()).toHaveLength(1);
    expect(visibleCells()[0]).toContain("DI-002");

    await user.clear(search);
    await user.type(search, "di-003");
    expect(visibleCells()).toHaveLength(1);
    expect(visibleCells()[0]).toContain("DI-003");
  });

  it("查找与状态片叠加生效，查不到时给专门空态", async () => {
    const user = userEvent.setup();
    renderPanel();
    await user.click(screen.getByRole("button", { name: /合格/ }));
    await user.type(screen.getByLabelText("查找表格内容"), "注射泵");
    // 注射泵是缺失行，与"合格"筛选叠加后应为空
    expect(visibleCells()).toHaveLength(0);
    expect(screen.getByText("没有找到匹配的数据")).toBeInTheDocument();
  });

  it("没有数据时给空态而不是空表格", () => {
    render(
      <AntApp>
        <ConsolidationPanel columns={COLUMNS} rows={[]} statuses={[]} counts={{}} />
      </AntApp>,
    );
    expect(screen.getByText("暂无数据")).toBeInTheDocument();
    expect(document.querySelector(".ant-table-tbody")).toBeNull();
  });

  it("出现冲突时补一个冲突片", () => {
    render(
      <AntApp>
        <ConsolidationPanel
          columns={COLUMNS}
          rows={ROWS}
          statuses={STATUSES}
          counts={{ ...COUNTS, Conflict: 3 }}
        />
      </AntApp>,
    );
    expect(screen.getByRole("button", { name: /冲突/ }).textContent).toContain("3");
  });
});

// 冲突单元格的标记（AGENTS.md §11 第 5 步）：只有带提示的格套 .cell-conflict，
// 提示文案原样进 title；没有提示的格保持原样（MISSING 仍然只套 .cell-MISSING）。
const CONFLICT_NOTE = "UDI团队信息：000MAT-001+DI-001 有多个候选值：10, 999";

describe("数据整合结果表的冲突单元格", () => {
  const renderWithConflicts = () =>
    render(
      <AntApp>
        <ConsolidationPanel
          columns={["status", ...COLUMNS]}
          rows={[
            ["Conflict", "DI-001", "超声探头", "12345"],
            ["Incomplete", "DI-002", "MISSING", ""],
          ]}
          statuses={["Conflict", "Incomplete"]}
          counts={{ Ready: 0, Incomplete: 1, Duplicate: 0, Conflict: 1 }}
          conflicts={[
            ["", "", CONFLICT_NOTE, ""],
            ["", "", "", ""],
          ]}
        />
      </AntApp>,
    );

  it("只标记有提示的格", () => {
    renderWithConflicts();

    const marked = document.querySelectorAll(".cell-conflict");
    expect(marked).toHaveLength(1);
    expect(marked[0]).toHaveTextContent("超声探头");
  });

  it("鼠标停上去立刻出提示（不用停在文字上等系统延迟）", async () => {
    const user = userEvent.setup();
    renderWithConflicts();

    await user.hover(document.querySelector(".cell-conflict") as HTMLElement);
    expect(await screen.findByRole("tooltip")).toHaveTextContent(CONFLICT_NOTE);
  });

  it("没有提示的格保持原样：MISSING 只套 cell-MISSING", () => {
    renderWithConflicts();

    const missing = document.querySelectorAll(".cell-MISSING");
    expect(missing).toHaveLength(1);
    expect(missing[0]).toHaveTextContent("MISSING");
    expect(missing[0]).not.toHaveClass("cell-conflict");
  });

  it("不传 conflicts（老调用方）时照常渲染，一个冲突标记都没有", () => {
    renderPanel();
    expect(document.querySelectorAll(".cell-conflict")).toHaveLength(0);
  });
});

// 状态筛选片：出现冲突时是 4 个（最小窗口也要放得下，见 Go 契约里的宽度核算），
// 每个片悬停要说清「这个状态意味着什么」。
describe("数据整合结果表的状态筛选片", () => {
  const renderWithFourChips = () =>
    render(
      <AntApp>
        <ConsolidationPanel
          columns={["status", ...COLUMNS]}
          rows={[["Conflict", "DI-001", "超声探头", "12345"]]}
          statuses={["Conflict"]}
          counts={{ Ready: 0, Incomplete: 1, Duplicate: 0, Conflict: 12 }}
        />
      </AntApp>,
    );

  it("四个状态片都在（冲突片不因宽度被丢掉）", () => {
    renderWithFourChips();
    const chips = Array.from(document.querySelectorAll(".status-chip"));
    expect(chips).toHaveLength(4);
    expect(chips.map((chip) => chip.textContent)).toEqual([
      "合格 0",
      "缺失 1",
      "重复 0",
      "冲突 12",
    ]);
    // Tooltip 直接挂在片上（不加包裹元素），否则 grid 轨道会被多套一层、等宽口径就守不住了
    for (const chip of chips) {
      expect(chip.parentElement?.classList.contains("status-chips")).toBe(true);
    }
  });

  it("悬停每个片给出该状态的含义（不是只说「只看…」）", async () => {
    const user = userEvent.setup();
    renderWithFourChips();

    await user.hover(screen.getByRole("button", { name: /冲突/ }));
    expect(await screen.findByText(/同一键的多条来源记录内容不一致/)).toBeInTheDocument();

    await user.hover(screen.getByRole("button", { name: /缺失/ }));
    expect(await screen.findByText(/有必填（或按条件必填）的单元格为空/)).toBeInTheDocument();

    await user.hover(screen.getByRole("button", { name: /合格/ }));
    expect(await screen.findByText(/只有合格行会写进导出文件/)).toBeInTheDocument();
  });
});

// 大批量行的分页行为：数据量大时表格卡顿的根因是"一次渲染全部行"，
// 所以这里固定：默认 100 行/页、可切 50/100/200/500、翻页只换当前页的行。
const BIG_ROWS = Array.from({ length: 250 }, (_, index) => [
  `DI-${String(index + 1).padStart(3, "0")}`,
  `产品 ${index + 1}`,
  String(10000 + index),
]);
const BIG_STATUSES = BIG_ROWS.map(() => "Ready");

// 与 main.tsx 一致：真实应用用中文 locale，分页按钮标题才是「上一页 / 下一页」
const renderBigPanel = () =>
  render(
    <ConfigProvider locale={zhCN}>
      <AntApp>
        <ConsolidationPanel
          columns={COLUMNS}
          rows={BIG_ROWS}
          statuses={BIG_STATUSES}
          counts={{ Ready: BIG_ROWS.length, Incomplete: 0, Duplicate: 0 }}
        />
      </AntApp>
    </ConfigProvider>,
  );

const renderedRowCount = () =>
  document.querySelectorAll(".ant-table-tbody .ant-table-row").length;

describe("数据整合结果表的分页", () => {
  it("默认只渲染 100 行（不是全部 250 行）", () => {
    renderBigPanel();
    expect(DEFAULT_PAGE_SIZE).toBe(100);
    expect(renderedRowCount()).toBe(100);
  });

  it("分页器给出总行数，并提供 50/100/200/500 四个每页行数选项", () => {
    renderBigPanel();
    expect(screen.getByText("共 250 行")).toBeTruthy();
    const options = PAGE_SIZE_OPTIONS;
    expect(options).toEqual([50, 100, 200, 500]);
    // antd 的分页器默认可切换每页行数（下拉里就是这四个选项）
    const sizeChanger = document.querySelector(".ant-pagination-options");
    expect(sizeChanger).toBeTruthy();
  });

  it("点「下一页」翻到第二页，只渲染这一页的行", async () => {
    const user = userEvent.setup();
    renderBigPanel();
    expect(visibleCells()[0]).toContain("DI-001");

    await user.click(screen.getByTitle("下一页"));
    const cells = visibleCells();
    expect(cells[0]).toContain("DI-101");
    expect(renderedRowCount()).toBe(100);
    // 最后一页只有 50 行
    await user.click(screen.getByTitle("下一页"));
    expect(visibleCells()[0]).toContain("DI-201");
    expect(renderedRowCount()).toBe(50);
  });

  it("点「上一页」能翻回去", async () => {
    const user = userEvent.setup();
    renderBigPanel();
    await user.click(screen.getByTitle("下一页"));
    expect(visibleCells()[0]).toContain("DI-101");
    await user.click(screen.getByTitle("上一页"));
    expect(visibleCells()[0]).toContain("DI-001");
  });

  it("查找会回到第一页（不会停在越界的空页上）", async () => {
    const user = userEvent.setup();
    renderBigPanel();
    await user.click(screen.getByTitle("下一页"));
    expect(visibleCells()[0]).toContain("DI-101");

    await user.type(screen.getByLabelText("查找表格内容"), "DI-001");
    const cells = visibleCells();
    expect(cells.length).toBeGreaterThan(0);
    expect(cells[0]).toContain("DI-001");
    expect(screen.getByText("共 1 行")).toBeTruthy();
  });

  it("查找框与三个筛选片在同一行（查找在左、筛选在右）", () => {
    renderBigPanel();
    const toolbar = document.querySelector(".table-toolbar");
    expect(toolbar).toBeTruthy();
    // 同一行里既有查找框、也有状态筛选片，且查找框排在筛选片前面
    const search = toolbar?.querySelector(".table-search");
    const strip = toolbar?.querySelector(".status-strip");
    expect(search).toBeTruthy();
    expect(strip).toBeTruthy();
    expect(toolbar?.querySelectorAll(".status-chip").length).toBe(3);
    expect(
      search!.compareDocumentPosition(strip!) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });
});
