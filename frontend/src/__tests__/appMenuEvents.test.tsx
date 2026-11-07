// 菜单事件回归：Admin 登录后「参数设定」必须打开完整设置，而不是「共享盘位置」。
// 之前的 bug 来自 onEvent 不返回取消函数，登录前的旧监听一直留着。

import { App as AntApp, ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import App from "../App";
import type { LoginRole, SessionInfo } from "../types";

const mocks = vi.hoisted(() => ({
  call: vi.fn(),
  handlers: new Map<string, Set<(...args: unknown[]) => void>>(),
}));

vi.mock("../bridge", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../bridge")>();
  return { ...actual, call: mocks.call };
});

const roles: LoginRole[] = [
  { key: "ra", label: "RA", hint: "RA 信息上传", needsPassword: false },
  { key: "eama", label: "EAMA", hint: "产品类别 / 医保代码", needsPassword: false },
  { key: "udi", label: "Global UDI Team", hint: "UDI 团队信息", needsPassword: false },
  { key: "supply_chain", label: "Supply Chain", hint: "数据整合", needsPassword: false },
  { key: "admin", label: "Admin", hint: "全部权限", needsPassword: true },
];

let activeRole: "admin" | "supply_chain" | null = null;

const adminSession: SessionInfo = {
  name: "Admin",
  role: "admin",
  roleLabel: "管理员",
  username: "tester",
  machine: "test-machine",
  at: "2026-11-07 22:00:00",
  singleUser: false,
  shareReady: true,
  shareDetail: "共享盘正常",
  actions: ["settings:read", "settings:write", "share:root", "share:write"],
};

const supplyChainSession: SessionInfo = {
  ...adminSession,
  name: "Supply Chain",
  role: "supply_chain",
  roleLabel: "供应链",
  actions: ["review:any", "share:root"],
};

const initialState = (session: SessionInfo | null) => ({
  version: "test",
  operationLog: "",
  imports: {},
  busy: null,
  session,
  roles,
  username: "tester",
  singleUser: false,
  share: null,
});

beforeEach(() => {
  mocks.call.mockReset();
  mocks.handlers.clear();
  activeRole = null;

  mocks.call.mockImplementation(async (method: string) => {
    switch (method) {
      case "InitialState":
        return initialState(
          activeRole === "admin" ? adminSession : activeRole === "supply_chain" ? supplyChainSession : null,
        );
      case "Login":
        activeRole = "supply_chain";
        return {
          session: supplyChainSession,
          failed: false,
          title: "",
          message: "",
          log: "",
        };
      case "LoginAdmin":
        activeRole = "admin";
        return { session: adminSession, failed: false, title: "", message: "", log: "" };
      case "ConfigurationDocument":
        return [{ key: "setting", title: "基础参数", note: "", path: "setting.yaml", tree: [], raw: "" }];
      case "GetShareRoot":
        return { path: "/tmp/share", status: "共享盘正常" };
      case "ShareOverview":
        return null;
      default:
        return null;
    }
  });

  window.runtime = {
    EventsOn: (name, handler) => {
      let set = mocks.handlers.get(name);
      if (!set) {
        set = new Set();
        mocks.handlers.set(name, set);
      }
      set.add(handler);
      return () => {
        set?.delete(handler);
      };
    },
    EventsOff: vi.fn(),
  };
});

describe("菜单事件", () => {
  it("Admin 登录后旧监听被清掉，参数设定打开完整设置", async () => {
    render(
      <ConfigProvider locale={zhCN}>
        <AntApp>
          <App />
        </AntApp>
      </ConfigProvider>,
    );

    await screen.findByText("Welcome, tester");
    // 登录页上已经注册过一份 show-settings（此时 session 为空）。
    expect(mocks.handlers.get("show-settings")?.size).toBe(1);

    await userEvent.click(screen.getByRole("button", { name: "Admin" }));
    await userEvent.type(screen.getByLabelText("Admin 密码"), "SsrAdmin@2026");
    await userEvent.click(screen.getByRole("button", { name: /登\s*录/ }));
    await userEvent.click(screen.getByRole("button", { name: /继\s*续/ }));

    await screen.findByText(/数据(导入|来源)/);

    // 登录后应只剩新的监听；旧闭包会走「共享盘位置」。
    await waitFor(() => expect(mocks.handlers.get("show-settings")?.size).toBe(1));

    await act(async () => {
      for (const handler of mocks.handlers.get("show-settings") ?? []) handler();
    });

    await waitFor(() => expect(mocks.call).toHaveBeenCalledWith("ConfigurationDocument"));
    expect(await screen.findByText("参数设定")).toBeInTheDocument();
    expect(mocks.call).not.toHaveBeenCalledWith("GetShareRoot");
  });

  it("普通用户登录后参数设定仍只打开共享盘位置", async () => {
    render(
      <ConfigProvider locale={zhCN}>
        <AntApp>
          <App />
        </AntApp>
      </ConfigProvider>,
    );

    await screen.findByText("Welcome, tester");
    await userEvent.click(screen.getByRole("button", { name: "Supply Chain" }));
    await screen.findByText(/数据(导入|来源)/);

    await waitFor(() => expect(mocks.handlers.get("show-settings")?.size).toBe(1));
    await act(async () => {
      for (const handler of mocks.handlers.get("show-settings") ?? []) handler();
    });

    await waitFor(() => expect(mocks.call).toHaveBeenCalledWith("GetShareRoot"));
    expect(mocks.call).not.toHaveBeenCalledWith("ConfigurationDocument");
  });
});
