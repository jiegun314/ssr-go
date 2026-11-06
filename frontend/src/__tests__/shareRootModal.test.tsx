// 非 Admin 的「共享盘位置」弹窗：只编辑本机共享盘路径，其它设置不动。

import { App as AntApp, ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { ShareRootModal } from "../components/Modals";
import type { ShareRootInfo } from "../types";

const info: ShareRootInfo = {
  root: "C:\\SSRShare",
  mode: "share",
  healthy: true,
  detail: "共享盘正常",
} as ShareRootInfo;

const renderModal = (current: ShareRootInfo | null = info) => {
  const onSave = vi.fn();
  const onClose = vi.fn();
  render(
    <ConfigProvider locale={zhCN}>
      <AntApp>
        <ShareRootModal open info={current} saving={false} onSave={onSave} onClose={onClose} />
      </AntApp>
    </ConfigProvider>,
  );
  return { onSave, onClose };
};

describe("共享盘位置弹窗", () => {
  it("显示当前路径与状态，保存时回传新路径", async () => {
    const { onSave } = renderModal();
    const input = screen.getByRole("textbox", { name: "共享盘位置" });
    expect(input).toHaveValue("C:\\SSRShare");
    expect(screen.getByText(/共享盘正常/)).toBeInTheDocument();
    await userEvent.clear(input);
    await userEvent.type(input, "D:\\SSR");
    await userEvent.click(screen.getByRole("button", { name: /保\s*存/ }));
    expect(onSave).toHaveBeenCalledWith("D:\\SSR");
  });

  it("留空表示本地模式", async () => {
    const { onSave } = renderModal({ ...info, root: "" });
    await userEvent.click(screen.getByRole("button", { name: /保\s*存/ }));
    expect(onSave).toHaveBeenCalledWith("");
  });
});
