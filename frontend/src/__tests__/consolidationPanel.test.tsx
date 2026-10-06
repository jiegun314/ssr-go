// 数据整合结果表的行为测试：状态片筛选、查找框按整行匹配、两者叠加、空态文案。
//
// 真实渲染 + 真实点击，替代以前"在 Go 里 grep TSX 源码"的间接断言。

import { App as AntApp } from "antd";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { ConsolidationPanel } from "../components/Panels";

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
