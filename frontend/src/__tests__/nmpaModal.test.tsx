// 药监 UDI 参考库窗口：只读状态、搜索与检查更新；同步全量走确认弹窗，不在本测试触发。
import { App as AntApp, ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { NMPAModal } from "../components/NMPAModal";
import { call } from "../bridge";
import type { NMPAQueryResult, NMPAStatus } from "../types";

vi.mock("../bridge", () => ({ call: vi.fn() }));

const mockedCall = vi.mocked(call);

const status = (overrides: Partial<NMPAStatus> = {}): NMPAStatus => ({
  socialCreditCode: "91310115703465170L",
  codeConfigured: true,
  lastFullFile: "UDID_FULL_RELEASE_20261101.zip",
  lastFullDate: "2026-11-01",
  lastFullAt: "2026-11-02 10:00:00",
  lastWeekEnd: "2026-11-22",
  lastWeeklyFile: "UDID_WEEKLY_UPDATE_20261116_20261122.zip",
  lastSyncAt: "2026-11-24 09:00:00",
  lastCheckAt: "2026-11-24 09:00:00",
  lastError: "",
  deviceCount: 42,
  packageCount: 7,
  contactCount: 3,
  databasePath: "/tmp/nmpa_udi.sqlite3",
  databaseBytes: 2048,
  needsFull: false,
  codeChanged: false,
  weeklyAvailable: true,
  fullDue: false,
  error: "",
  ...overrides,
});

const queryResult = (): NMPAQueryResult => ({
  total: 1,
  columns: ["Primary DI", "产品名称"],
  rows: [["DI-1", "椎间融合器"]],
});

function renderModal(overrides: Partial<NMPAStatus> = {}) {
  const onClose = vi.fn();
  render(
    <ConfigProvider locale={zhCN}>
      <AntApp>
        <NMPAModal open onClose={onClose} />
      </AntApp>
    </ConfigProvider>,
  );
  return { onClose, current: status(overrides) };
}

beforeEach(() => {
  mockedCall.mockReset();
  mockedCall.mockImplementation(async (method: string) => {
    if (method === "NMPAStatus") return status();
    if (method === "NMPASearch") return queryResult();
    if (method === "NMPACheck") return status({ lastCheckAt: "2026-11-24 10:00:00" });
    return undefined;
  });
});

describe("药监 UDI 参考库", () => {
  it("显示企业代码、本地统计与全量文件", async () => {
    renderModal();
    await waitFor(() => expect(screen.getByText("91310115703465170L")).toBeInTheDocument());
    expect(screen.getByText("42")).toBeInTheDocument();
    expect(screen.getByText("7")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
    expect(screen.getByText(/UDID_FULL_RELEASE_20261101/)).toBeInTheDocument();
    expect(screen.getByText("有可用周更")).toBeInTheDocument();
  });

  it("按关键词搜索并把结果显示为表格", async () => {
    renderModal();
    await waitFor(() => expect(mockedCall).toHaveBeenCalledWith("NMPASearch", "", 1, 20));
    await userEvent.type(screen.getByPlaceholderText(/Primary DI/), "融合器");
    await userEvent.click(screen.getByRole("button", { name: /查\s*找/ }));
    await waitFor(() =>
      expect(mockedCall).toHaveBeenCalledWith("NMPASearch", "融合器", 1, 20),
    );
    expect(await screen.findByText("椎间融合器")).toBeInTheDocument();
    expect(screen.getByText("DI-1")).toBeInTheDocument();
  });

  it("检查更新会刷新状态", async () => {
    renderModal();
    await waitFor(() => expect(mockedCall).toHaveBeenCalledWith("NMPAStatus"));
    await userEvent.click(screen.getByRole("button", { name: /检查更新/ }));
    await waitFor(() => expect(mockedCall).toHaveBeenCalledWith("NMPACheck"));
    expect(await screen.findByText(/2026-11-24 10:00:00/)).toBeInTheDocument();
  });

  it("未配置统一社会信用代码时给出明确提示", async () => {
    mockedCall.mockImplementation(async (method: string) => {
      if (method === "NMPAStatus") {
        return status({ socialCreditCode: "", codeConfigured: false, deviceCount: 0, packageCount: 0, contactCount: 0 });
      }
      if (method === "NMPASearch") return { total: 0, columns: [], rows: [] };
      return undefined;
    });
    renderModal();
    expect(await screen.findByText("还没有配置统一社会信用代码")).toBeInTheDocument();
    expect(screen.getAllByText("未配置").length).toBeGreaterThan(0);
  });
});
