// 数据对齐面板（P3）：四行状态 + 全部对齐按钮 + 单行对齐。
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

const renderPanel = (sources: SourceStatus[], autoAlign = true) => {
  const onAlignAll = vi.fn();
  const onAlign = vi.fn();
  render(
    <ConfigProvider locale={zhCN}>
      <AntApp>
        <AlignPanel
          sources={sources}
          autoAlign={autoAlign}
          aligning={false}
          onAutoAlignChange={vi.fn()}
          onAlignAll={onAlignAll}
          onAlign={onAlign}
        />
      </AntApp>
    </ConfigProvider>,
  );
  return { onAlignAll, onAlign };
};

describe("数据对齐面板", () => {
  it("逐行显示本机版本 → 共享盘版本与状态，未对齐可单行对齐", async () => {
    const { onAlign } = renderPanel([
      status({ source: "ra_input", label: "RA信息", localVersion: "v0", remoteVersion: "v1" }),
      status({ source: "global_udi_input", label: "UDI团队信息", remoteVersion: "v9", aligned: true }),
    ]);
    expect(screen.getByText(/v0 → v1/)).toBeInTheDocument();
    expect(screen.getByText("未对齐")).toBeInTheDocument();
    expect(screen.getByText("已对齐")).toBeInTheDocument();

    const rows = document.querySelectorAll(".align-row");
    await userEvent.click(within(rows[0] as HTMLElement).getByRole("button", { name: /对\s*齐/ }));
    expect(onAlign).toHaveBeenCalledWith("ra_input");
    // 已对齐的那行按钮禁用
    expect(within(rows[1] as HTMLElement).getByRole("button", { name: /对\s*齐/ })).toBeDisabled();
  });

  it("全部对齐：有未对齐时可用，全部对齐后禁用", async () => {
    const { onAlignAll } = renderPanel([
      status({ remoteVersion: "v1" }),
      status({ source: "global_udi_input", label: "UDI团队信息", remoteVersion: "v9" }),
    ]);
    const all = screen.getByRole("button", { name: "全部对齐" });
    expect(all).toBeEnabled();
    await userEvent.click(all);
    expect(onAlignAll).toHaveBeenCalled();
  });

  it("全部已对齐时「全部对齐」禁用，且显示待整合标记", () => {
    renderPanel([
      status({ remoteVersion: "v1", aligned: true, pending: true }),
      status({ source: "global_udi_input", label: "UDI团队信息", remoteVersion: "v9", aligned: true }),
    ]);
    expect(screen.getByRole("button", { name: "全部对齐" })).toBeDisabled();
    expect(screen.getByText("已对齐 · 待整合")).toBeInTheDocument();
  });
});
