// 数据对齐面板（P3）：四个来源各一张卡片（共享盘/本机版本、发布时间、状态、对齐按钮），
// 「全部对齐」在模块标题栏（这里只测卡片与单行对齐）。

import { App as AntApp, ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { AlignPanel } from "../components/AlignPanel";
import type { SourceStatus } from "../types";

const status = (overrides: Partial<SourceStatus>): SourceStatus =>
  ({
    source: "ra_input",
    label: "RA信息",
    canView: true,
    localVersion: "",
    remoteVersion: "",
    remoteRows: 0,
    publishedAt: "",
    publishedBy: "",
    aligned: false,
    pending: false,
    ...overrides,
  }) as SourceStatus;

const renderPanel = (sources: SourceStatus[]) => {
  const onAlign = vi.fn();
  render(
    <ConfigProvider locale={zhCN}>
      <AntApp>
        <AlignPanel sources={sources} aligning={false} onAlign={onAlign} />
      </AntApp>
    </ConfigProvider>,
  );
  return { onAlign };
};

describe("数据对齐面板", () => {
  it("四个来源各一张卡片，显示状态与发布时间；共享盘/本机版本hover后显示", async () => {
    const user = userEvent.setup();
    renderPanel([
      status({ source: "ra_input", label: "RA信息", localVersion: "v0", remoteVersion: "v1" }),
      status({
        source: "global_udi_input",
        label: "UDI团队信息",
        remoteVersion: "v9",
        aligned: true,
        publishedAt: "2026-11-01 00:00:00",
        publishedBy: "zhangsan",
        remoteRows: 6,
      }),
      status({ source: "product_category", label: "产品类别" }),
      status({ source: "medical_insurance_code", label: "医保代码信息" }),
    ]);

    const cards = document.querySelectorAll(".align-source-card");
    expect(cards).toHaveLength(4);
    const first = cards[0] as HTMLElement;
    expect(within(first).getByText("RA信息")).toBeInTheDocument();
    expect(within(first).getByText("未对齐")).toBeInTheDocument();
    // 共享盘/本机版本不再直接显示
    expect(within(first).queryByText("v1")).not.toBeInTheDocument();
    expect(within(first).queryByText("v0")).not.toBeInTheDocument();

    await user.hover(within(first).getByText("未对齐"));
    const tooltip = await screen.findByRole("tooltip");
    expect(tooltip).toHaveTextContent("共享盘：v1");
    expect(tooltip).toHaveTextContent("本机：v0");

    const second = cards[1] as HTMLElement;
    expect(within(second).getByText("已对齐")).toBeInTheDocument();
    expect(within(second).getByText(/2026-11-01 00:00:00 · 6 行 · zhangsan/)).toBeInTheDocument();
  });

  it("未发布的来源 hover 状态标签后 tooltip 显示共享盘：未发布", async () => {
    const user = userEvent.setup();
    renderPanel([status({ source: "product_category", label: "产品类别" })]);

    const card = document.querySelector(".align-source-card") as HTMLElement;
    await user.hover(within(card).getByText("未对齐"));
    const tooltip = await screen.findByRole("tooltip");
    expect(tooltip).toHaveTextContent("共享盘：未发布");
  });

  it("未对齐的可单行对齐，已对齐/未发布的按钮禁用", async () => {
    const { onAlign } = renderPanel([
      status({ source: "ra_input", remoteVersion: "v1" }),
      status({ source: "global_udi_input", label: "UDI团队信息", remoteVersion: "v9", aligned: true }),
      status({ source: "product_category", label: "产品类别" }),
    ]);
    const cards = document.querySelectorAll(".align-source-card");
    await userEvent.click(within(cards[0] as HTMLElement).getByRole("button", { name: /对\s*齐/ }));
    expect(onAlign).toHaveBeenCalledWith("ra_input");
    expect(within(cards[1] as HTMLElement).getByRole("button", { name: /对\s*齐/ })).toBeDisabled();
    expect(within(cards[2] as HTMLElement).getByRole("button", { name: /对\s*齐/ })).toBeDisabled();
  });

  it("「已对齐 · 待整合」显示在卡片状态里", () => {
    renderPanel([
      status({ source: "ra_input", remoteVersion: "v1", aligned: true, pending: true }),
      status({ source: "global_udi_input", label: "UDI团队信息", remoteVersion: "v9" }),
    ]);
    expect(screen.getByText("已对齐 · 待整合")).toBeInTheDocument();
    expect(screen.getByText("未对齐")).toBeInTheDocument();
  });
});
