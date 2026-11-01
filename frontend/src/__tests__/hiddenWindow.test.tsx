// 附加窗口（隐藏窗口）：只认右上角的 X —— 点遮罩、按 Esc 都不能关。

import { App as AntApp, ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { HiddenModal } from "../components/Modals";

const renderHidden = () => {
  const onClose = vi.fn();
  render(
    <ConfigProvider locale={zhCN}>
      <AntApp>
        <HiddenModal open onClose={onClose} />
      </AntApp>
    </ConfigProvider>,
  );
  return { onClose };
};

describe("附加窗口的关闭方式", () => {
  it("点遮罩不关闭", () => {
    const { onClose } = renderHidden();
    const wrap = document.querySelector(".ant-modal-wrap") as HTMLElement;
    fireEvent.mouseDown(wrap);
    fireEvent.click(wrap);
    expect(onClose).not.toHaveBeenCalled();
  });

  it("按 Esc 不关闭", () => {
    const { onClose } = renderHidden();
    fireEvent.keyDown(document, { key: "Escape", code: "Escape" });
    expect(onClose).not.toHaveBeenCalled();
  });

  it("点右上角 X 才关闭", () => {
    const { onClose } = renderHidden();
    const close = document.querySelector(".ant-modal-close") as HTMLElement;
    expect(close).not.toBeNull();
    fireEvent.click(close);
    expect(onClose).toHaveBeenCalled();
  });
});
