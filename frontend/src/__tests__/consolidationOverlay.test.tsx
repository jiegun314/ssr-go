// 整合是唯一会阻塞整个工作区的长任务：点击后主界面要压暗、按钮禁用，转圈图标固定在视口正中。

import { App as AntApp, ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import App from "../App";
import type { SessionInfo } from "../types";

const mocks = vi.hoisted(() => ({ call: vi.fn() }));

vi.mock("../bridge", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../bridge")>();
  return { ...actual, call: mocks.call };
});

const session: SessionInfo = {
  name: "Admin",
  role: "admin",
  roleLabel: "管理员",
  username: "tester",
  machine: "test-machine",
  at: "2026-11-24 23:00:00",
  singleUser: true,
  shareReady: true,
  shareDetail: "单机模式",
  actions: ["consolidate", "export"],
};

beforeEach(() => {
  mocks.call.mockReset();
  window.runtime = {
    EventsOn: () => () => undefined,
    EventsOff: vi.fn(),
  };
});

describe("整合处理中", () => {
  it("锁定主界面并在正中显示动态图标，完成后恢复", async () => {
    let finish!: (value: unknown) => void;
    const pending = new Promise((resolve) => {
      finish = resolve;
    });

    mocks.call.mockImplementation(async (method: string) => {
      if (method === "InitialState") {
        return {
          version: "test",
          operationLog: "",
          imports: {},
          busy: null,
          session,
          roles: [],
          username: "tester",
          singleUser: true,
          share: null,
        };
      }
      if (method === "Consolidate") return pending;
      return null;
    });

    render(
      <ConfigProvider locale={zhCN}>
        <AntApp>
          <App />
        </AntApp>
      </ConfigProvider>,
    );

    await screen.findByText("数据整合");
    const button = screen.getByRole("button", { name: /整\s*合/ });
    await userEvent.click(button);

    expect(await screen.findByRole("status", { name: "正在整合数据" })).toBeInTheDocument();
    expect(button).toBeDisabled();

    await act(async () => {
      finish({
        log: "",
        failed: false,
        columns: [],
        rows: [],
        statuses: [],
        conflicts: [],
        rowConflicts: [],
      });
    });

    await waitFor(() => expect(screen.queryByRole("status", { name: "正在整合数据" })).not.toBeInTheDocument());
    expect(button).not.toBeDisabled();
  });
});
