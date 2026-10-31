// 登录页（P1）：按组别选择（不显示人名）；Welcome, <本机用户名>；
// 上面一行四个方块（图标 + 组别），下面一行居中管理员；只有管理员需要密码。

import { App as AntApp, ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { LoginScreen } from "../components/LoginScreen";
import type { LoginRole } from "../types";

const roles: LoginRole[] = [
  { key: "ra", label: "RA", hint: "RA 信息上传", needsPassword: false },
  { key: "eama", label: "EAMA", hint: "产品类别 / 医保代码", needsPassword: false },
  { key: "udi", label: "Global UDI Team", hint: "UDI 团队信息", needsPassword: false },
  { key: "supply_chain", label: "Supply Chain", hint: "数据整合", needsPassword: false },
  { key: "admin", label: "Admin", hint: "全部权限", needsPassword: true },
];

const renderLogin = () => {
  const onLogin = vi.fn();
  render(
    <ConfigProvider locale={zhCN}>
      <AntApp>
        <LoginScreen roles={roles} username="zhangsan" onLogin={onLogin} />
      </AntApp>
    </ConfigProvider>,
  );
  return { onLogin };
};

describe("登录页", () => {
  it("显示 Welcome + 本机用户名，上面一行四个组别、下面一行管理员", () => {
    renderLogin();
    expect(screen.getByText("Welcome, zhangsan")).toBeInTheDocument();

    const mainRow = document.querySelector(".login-row-main") as HTMLElement;
    expect(mainRow.querySelectorAll(".login-tile")).toHaveLength(4);
    for (const label of ["RA", "EAMA", "Global UDI Team", "Supply Chain"]) {
      expect(screen.getByRole("button", { name: label })).toBeInTheDocument();
    }

    const adminRow = document.querySelector(".login-row-admin") as HTMLElement;
    expect(adminRow.querySelectorAll(".login-tile")).toHaveLength(1);
    expect(screen.getByRole("button", { name: "Admin" })).toBeInTheDocument();

    // Admin 方块是红底白字（login-tile-admin），其余组别不带这个类
    expect(screen.getByRole("button", { name: "Admin" })).toHaveClass("login-tile-admin");
    expect(screen.getByRole("button", { name: "RA" })).not.toHaveClass("login-tile-admin");
  });

  it("点非 Admin 的组别直接登录（不需要密码）", async () => {
    const { onLogin } = renderLogin();
    await userEvent.click(screen.getByRole("button", { name: "RA" }));
    expect(onLogin).toHaveBeenCalledWith("ra", "");
  });

  it("点 Admin 才出现密码框，密码为空时不能提交", async () => {
    const { onLogin } = renderLogin();
    expect(screen.queryByLabelText("Admin 密码")).toBeNull();

    await userEvent.click(screen.getByRole("button", { name: "Admin" }));
    expect(screen.getByLabelText("Admin 密码")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /登\s*录/ })).toBeDisabled();
    expect(onLogin).not.toHaveBeenCalled();
  });

  it("输入密码后提交带密码", async () => {
    const { onLogin } = renderLogin();
    await userEvent.click(screen.getByRole("button", { name: "Admin" }));
    await userEvent.type(screen.getByLabelText("Admin 密码"), "SsrAdmin@2026");
    await userEvent.click(screen.getByRole("button", { name: /登\s*录/ }));
    expect(onLogin).toHaveBeenCalledWith("admin", "SsrAdmin@2026");
  });
});
