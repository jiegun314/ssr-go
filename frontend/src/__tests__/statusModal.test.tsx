// 运行状态弹窗（P5）：共享盘状态、四份来源、最后整合、维护按钮（清理仅 Admin）。
import { App as AntApp, ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { StatusModal } from "../components/StatusModal";
import type { ShareOverview, SourceStatus } from "../types";

const source = (overrides: Partial<SourceStatus>): SourceStatus =>
  ({
    source: "ra_input",
    label: "RA信息",
    canView: true,
    localVersion: "",
    remoteVersion: "20261101T100000-abcdef12",
    remoteRows: 6,
    publishedAt: "2026-11-01 10:00:00",
    publishedBy: "zhangsan",
    aligned: true,
    pending: false,
    ...overrides,
  }) as SourceStatus;

const overview = (): ShareOverview =>
  ({
    status: { mode: "share", healthy: true, detail: "共享盘正常：/tmp/share", checkedAt: "2026-11-01 10:00:00" },
    sources: [source({}), source({ source: "global_udi_input", label: "UDI团队信息", remoteVersion: "" })],
    lastConsolidation: {
      at: "2026-11-01 10:05:00",
      by: "lisi（Supply Chain）",
      versions: {},
      result: "consolidated/20261101T100500/result.xlsx",
      counts: { ready: 6, missing: 1, duplicate: 0, conflict: 0 },
    },
    autoAlign: true,
    pollSeconds: 30,
    enabled: true,
  }) as unknown as ShareOverview;

const renderModal = (canCleanup = true) => {
  const onRecheck = vi.fn();
  const onCleanup = vi.fn();
  const onReviewAudit = vi.fn();
  render(
    <ConfigProvider locale={zhCN}>
      <AntApp>
        <StatusModal
          open
          report={overview()}
          cleaning={false}
          canCleanup={canCleanup}
          onRecheck={onRecheck}
          onCleanup={onCleanup}
          onReviewAudit={onReviewAudit}
          onClose={vi.fn()}
        />
      </AntApp>
    </ConfigProvider>,
  );
  return { onRecheck, onCleanup, onReviewAudit };
};

describe("运行状态弹窗", () => {
  it("显示共享盘状态、四份来源与最后一次整合", () => {
    renderModal();
    expect(screen.getByText("运行状态")).toBeInTheDocument();
    expect(screen.getByText("共享盘正常：/tmp/share")).toBeInTheDocument();
    expect(screen.getByText("RA信息")).toBeInTheDocument();
    expect(screen.getByText("UDI团队信息")).toBeInTheDocument();
    expect(screen.getByText("lisi（Supply Chain）")).toBeInTheDocument();
    expect(screen.getByText(/consolidated\/20261101T100500/)).toBeInTheDocument();
    // 没有正在整合时显示空闲
    expect(screen.getByText("整合锁：空闲")).toBeInTheDocument();
  });

  it("重新检查与查看审计会回调", async () => {
    const { onRecheck, onReviewAudit } = renderModal();
    await userEvent.click(screen.getByRole("button", { name: /重新检查/ }));
    expect(onRecheck).toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: "查看审计" }));
    expect(onReviewAudit).toHaveBeenCalled();
  });

  it("清理按钮只有 Admin 可用，点击带上保留份数", async () => {
    const { onCleanup } = renderModal(true);
    const cleanup = screen.getByRole("button", { name: /清\s*理/ });
    expect(cleanup).toBeEnabled();
    await userEvent.click(cleanup);
    expect(onCleanup).toHaveBeenCalledWith(5);
  });

  it("非 Admin 的清理按钮禁用", () => {
    renderModal(false);
    expect(screen.getByRole("button", { name: /清\s*理/ })).toBeDisabled();
  });
});
