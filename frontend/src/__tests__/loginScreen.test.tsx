// 登录页（P1）：按角色分组列出人员；非 Admin 点一下就登录；Admin 才出现密码框。

import { App as AntApp, ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { LoginScreen } from "../components/LoginScreen";
import type { LoginUser } from "../types";

const users: LoginUser[] = [
  { name: "张三", role: "ra", roleLabel: "RA", needsPassword: false, note: "RA 组" },
  { name: "李四", role: "eama", roleLabel: "EAMA", needsPassword: false, note: "" },
  { name: "赵六", role: "supply_chain", roleLabel: "Supply Chain", needsPassword: false, note: "" },
  { name: "管理员", role: "admin", roleLabel: "Admin", needsPassword: true, note: "" },
];

const renderLogin = () => {
  const onLogin = vi.fn();
  render(
    <ConfigProvider locale={zhCN}>
      <AntApp>
        <LoginScreen users={users} onLogin={onLogin} />
      </AntApp>
    </ConfigProvider>,
  );
  return { onLogin };
};

describe("登录页", () => {
  it("按角色分组列出人员，非 Admin 点一下直接登录", async () => {
    const { onLogin } = renderLogin();
    const ra = screen.getByText("RA 组").closest(".login-group") as HTMLElement;
    expect(within(ra).getByText("张三")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /张三/ }));
    expect(onLogin).toHaveBeenCalledWith("张三", "");
  });

  it("点 Admin 才出现密码框，密码为空时不能提交", async () => {
    const { onLogin } = renderLogin();
    expect(screen.queryByLabelText("Admin 密码")).toBeNull();

    await userEvent.click(screen.getByRole("button", { name: /管理员/ }));
    const input = screen.getByLabelText("Admin 密码");
    expect(input).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /登录/ })).toBeDisabled();
    expect(onLogin).not.toHaveBeenCalled();
  });

  it("输入密码后提交带密码", async () => {
    const { onLogin } = renderLogin();
    await userEvent.click(screen.getByRole("button", { name: /管理员/ }));
    await userEvent.type(screen.getByLabelText("Admin 密码"), "SsrAdmin@2026");
    await userEvent.click(screen.getByRole("button", { name: /登录/ }));
    expect(onLogin).toHaveBeenCalledWith("管理员", "SsrAdmin@2026");
  });
});
