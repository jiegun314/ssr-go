// 数据导入区的两个查看入口：
//   - 「数据回顾」每个来源都有，看原文表；
//   - 「查看清洗后数据」只在声明了 clean 的来源（RA 信息）出现，看整合实际读取的清洗表。
// 这里验证按钮的出现范围与回调参数（后端绑定另有 Go 测试）。

import { App as AntApp, ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { ImportPanel } from "../components/Panels";
import type { ImportState } from "../types";

const importedState = (): ImportState =>
  ({
    source: "ra_input",
    state: "imported",
    label: "导入6条记录",
    tooltip: "最近导入：2026/10/31 20:00，6 行",
    rowCount: 6,
    importTime: "2026/10/31 20:00",
  }) as ImportState;

const renderPanel = (states: Record<string, ImportState>) => {
  const onImport = vi.fn();
  const onReview = vi.fn();
  const onReviewCleaned = vi.fn();
  render(
    <ConfigProvider locale={zhCN}>
      <AntApp>
        <ImportPanel
          states={states}
          onImport={onImport}
          onReview={onReview}
          onReviewCleaned={onReviewCleaned}
        />
      </AntApp>
    </ConfigProvider>,
  );
  return { onImport, onReview, onReviewCleaned };
};

/** 按来源标题定位那一行（四个来源始终整块渲染，不能只按数量断言）。 */
const sourceRow = (title: string): HTMLElement => {
  const row = screen.getByText(title).closest(".group");
  if (!row) throw new Error(`找不到来源行：${title}`);
  return row as HTMLElement;
};

describe("数据导入区的查看入口", () => {
  it("RA 信息那一行同时有「数据回顾」和「查看清洗后数据」", async () => {
    const { onReview, onReviewCleaned } = renderPanel({
      ra_input: importedState(),
      global_udi_input: { ...importedState(), source: "global_udi_input" },
    });

    const row = sourceRow("RA信息");
    expect(within(row).getByLabelText("数据回顾")).toBeInTheDocument();
    const cleaned = within(row).getByLabelText("查看清洗后数据");

    // 全界面只有 RA 这一行有这个入口
    expect(screen.getAllByLabelText("查看清洗后数据")).toHaveLength(1);

    await userEvent.click(cleaned);
    expect(onReviewCleaned).toHaveBeenCalledWith("ra_input");
    expect(onReview).not.toHaveBeenCalled();
  });

  it("没有清洗表的来源不出现「查看清洗后数据」", () => {
    renderPanel({ global_udi_input: { ...importedState(), source: "global_udi_input" } });
    const row = sourceRow("UDI团队信息");
    expect(within(row).queryByLabelText("查看清洗后数据")).toBeNull();
    expect(within(row).getByLabelText("数据回顾")).toBeInTheDocument();
  });

  it("未导入时两个查看按钮都禁用", () => {
    renderPanel({
      ra_input: { ...importedState(), state: "empty", label: "", tooltip: "" },
    });
    const row = sourceRow("RA信息");
    expect(within(row).getByLabelText("数据回顾")).toBeDisabled();
    expect(within(row).getByLabelText("查看清洗后数据")).toBeDisabled();
  });
});
