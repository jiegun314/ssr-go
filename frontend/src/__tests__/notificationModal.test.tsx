// 数据更新通知确认窗：先展示收件人/主题/正文，再发送；draft 模式按钮改成「打开草稿」。

import { App as AntApp, ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { NotificationModal } from "../components/Modals";
import type { NotificationPreview } from "../types";

const preview = (overrides: Partial<NotificationPreview> = {}): NotificationPreview =>
  ({
    source: "ra_input",
    to: ["supply@example.com", "admin@example.com"],
    subject: "【SSR】RA 信息数据已更新（2026/11/05 20:00）",
    body: "SSR 数据更新通知\n\n数据来源：RA 信息\n数据量：6 行",
    transport: "outlook",
    mode: "send",
    ready: true,
    reason: "",
    ...overrides,
  }) as NotificationPreview;

const renderModal = (value: NotificationPreview | null, sending = false) => {
  const onSend = vi.fn();
  const onClose = vi.fn();
  render(
    <ConfigProvider locale={zhCN}>
      <AntApp>
        <NotificationModal
          open={Boolean(value)}
          preview={value}
          sending={sending}
          onSend={onSend}
          onClose={onClose}
        />
      </AntApp>
    </ConfigProvider>,
  );
  return { onSend, onClose };
};

describe("更新通知确认窗", () => {
  it("展示收件人、主题、正文，并可直接发送", async () => {
    const { onSend } = renderModal(preview());
    expect(screen.getByText("supply@example.com")).toBeInTheDocument();
    expect(screen.getByText("admin@example.com")).toBeInTheDocument();
    expect(screen.getByText(/RA 信息数据已更新/)).toBeInTheDocument();
    expect(screen.getByText(/数据量：6 行/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "发送邮件" }));
    expect(onSend).toHaveBeenCalled();
  });

  it("draft 模式按钮是「打开草稿」", () => {
    renderModal(preview({ mode: "draft" }));
    expect(screen.getByRole("button", { name: "打开草稿" })).toBeInTheDocument();
  });

  it("条件不满足时禁用发送并给出原因", () => {
    renderModal(preview({ ready: false, reason: "没有收件人" }));
    expect(screen.getByText("没有收件人")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "发送邮件" })).toBeDisabled();
  });
});
