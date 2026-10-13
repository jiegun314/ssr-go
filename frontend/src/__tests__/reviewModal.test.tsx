// 数据回顾窗口的表格分页：与结果表同一套口径（默认 100 行/页，可切 50/100/200/500）。
//
// 回顾窗口的分页在 Go 侧做（result.rows 已经只是当前页），所以这里验证的是分页器本身：
// 默认页大小、四个可选项、翻页回调带上的 (page, pageSize)。

import { App as AntApp, ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { ReviewModal } from "../components/Modals";
import { DEFAULT_PAGE_SIZE, PAGE_SIZE_OPTIONS } from "../tablePagination";
import type { ReviewResult } from "../types";

const buildResult = (overrides: Partial<ReviewResult> = {}): ReviewResult =>
  ({
    title: "数据回顾",
    columns: ["产品代码", "产品名称"],
    rows: Array.from({ length: DEFAULT_PAGE_SIZE }, (_, index) => [
      `000MAT-${index + 1}`,
      `产品 ${index + 1}`,
    ]),
    total: 250,
    page: 1,
    pageSize: DEFAULT_PAGE_SIZE,
    pageCount: 3,
    ...overrides,
  }) as ReviewResult;

const renderModal = (result: ReviewResult, onPage = vi.fn()) =>
  render(
    <ConfigProvider locale={zhCN}>
      <AntApp>
        <ReviewModal
          open
          result={result}
          exporting={false}
          onPage={onPage}
          onExport={vi.fn()}
          onClose={vi.fn()}
        />
      </AntApp>
    </ConfigProvider>,
  );

describe("数据回顾窗口的分页", () => {
  it("默认 100 行/页，并显示总行数", () => {
    renderModal(buildResult());
    expect(DEFAULT_PAGE_SIZE).toBe(100);
    expect(screen.getByText("共 250 行")).toBeTruthy();
    // 已经由 Go 侧分好页：只渲染当前页这 100 行
    expect(document.querySelectorAll(".ant-table-tbody .ant-table-row").length).toBe(100);
  });

  it("提供 50 / 100 / 200 / 500 四个每页行数选项", async () => {
    const user = userEvent.setup();
    renderModal(buildResult());
    expect(PAGE_SIZE_OPTIONS).toEqual([50, 100, 200, 500]);

    const selector = document.querySelector(".ant-pagination-options .ant-select-selector");
    expect(selector).toBeTruthy();
    await user.click(selector as Element);
    const options = Array.from(document.querySelectorAll(".ant-select-item-option")).map(
      (node) => node.textContent ?? "",
    );
    // 四个候选值都在（文案由 antd 的 zhCN 决定，这里只认数字，避免绑死措辞）
    expect(options.length).toBe(PAGE_SIZE_OPTIONS.length);
    for (const size of PAGE_SIZE_OPTIONS) {
      expect(options.some((text) => text.trim().startsWith(String(size)))).toBe(true);
    }
  });

  it("点「下一页」把页码交给 Go 侧（页码回调带上当前页大小）", async () => {
    const user = userEvent.setup();
    const onPage = vi.fn();
    renderModal(buildResult(), onPage);
    await user.click(screen.getByTitle("下一页"));
    expect(onPage).toHaveBeenCalledWith(2, DEFAULT_PAGE_SIZE);
  });

  it("换每页行数会回到第一页", async () => {
    const user = userEvent.setup();
    const onPage = vi.fn();
    renderModal(buildResult({ page: 3 }), onPage);

    const selector = document.querySelector(".ant-pagination-options .ant-select-selector");
    await user.click(selector as Element);
    const options = Array.from(
      document.querySelectorAll(".ant-select-item-option"),
    ) as HTMLElement[];
    const twoHundred = options.find((node) => (node.textContent ?? "").trim().startsWith("200"));
    expect(twoHundred).toBeTruthy();
    await user.click(twoHundred as HTMLElement);
    expect(onPage).toHaveBeenCalledWith(1, 200);
  });
});
