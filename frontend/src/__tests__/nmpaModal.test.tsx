// 药监 UDI 参考库窗口：只读状态、搜索与检查更新；同步全量走确认弹窗，不在本测试触发。
import { App as AntApp, ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { NMPAModal } from "../components/NMPAModal";
import { call } from "../bridge";
import type { NMPADeviceDetail, NMPAQueryResult, NMPAStatus } from "../types";

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
  columns: ["Primary DI", "产品名称", "通用名称", "规格型号", "注册人/备案人", "注册证/备案凭证编号", "产品类别", "版本状态"],
  rows: [["DI-1", "椎间融合器", "融合器", "规格-A", "强生", "国械注准1", "骨科", "新增"]],
});

const detail = (): NMPADeviceDetail => ({
  primaryDi: "DI-1",
  displayName: "椎间融合器",
  versionStatus: "新增",
  productCategory: "骨科",
  groups: [
    { title: "标识与包装", fields: [{ label: "发码机构", value: "GS1" }, { label: "使用单元 DI", value: "" }] },
    { title: "版本与更正", fields: [{ label: "记录键", value: "KEY-1" }] },
  ],
  packages: [{ packageDi: "PKG-1", packageType: "盒", lowerLevelPackageQuantity: "2", lowerLevelPackageIdentifier: "PKG-0" }],
  contacts: [{ fax: "", email: "a@example.com", phone: "123" }],
});

function renderModal(overrides: Partial<NMPAStatus> = {}, canManage = true) {
  const onClose = vi.fn();
  render(
    <ConfigProvider locale={zhCN}>
      <AntApp>
        <NMPAModal open onClose={onClose} canManage={canManage} />
      </AntApp>
    </ConfigProvider>,
  );
  return { onClose, current: status(overrides) };
}

beforeEach(() => {
  mockedCall.mockReset();
  mockedCall.mockImplementation(async (method: string) => {
    if (method === "NMPAAutoCheck") return status();
    if (method === "NMPAStatus") return status();
    if (method === "NMPASearch") return queryResult();
    if (method === "NMPACheck") return status({ lastCheckAt: "2026-11-24 10:00:00" });
    if (method === "NMPADetail") return detail();
    if (method === "SelectNMPAExportTarget") {
      return { defaultName: "药监UDI参考库.xlsx", target: "/tmp/nmpa.xlsx" };
    }
    if (method === "NMPAExport") {
      return { failed: false, path: "/tmp/nmpa.xlsx", message: "已导出 1 条药监 UDI 记录" };
    }
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
    await waitFor(() => expect(mockedCall).toHaveBeenCalledWith("NMPAAutoCheck"));
    await userEvent.click(screen.getByRole("button", { name: /检查更新/ }));
    await waitFor(() => expect(mockedCall).toHaveBeenCalledWith("NMPACheck"));
    expect(await screen.findByText(/2026-11-24 10:00:00/)).toBeInTheDocument();
  });

  it("未配置统一社会信用代码时给出明确提示", async () => {
    mockedCall.mockImplementation(async (method: string) => {
      if (method === "NMPAAutoCheck" || method === "NMPAStatus") {
        return status({ socialCreditCode: "", codeConfigured: false, deviceCount: 0, packageCount: 0, contactCount: 0 });
      }
      if (method === "NMPASearch") return { total: 0, columns: [], rows: [] };
      return undefined;
    });
    renderModal();
    expect(await screen.findByText("还没有配置统一社会信用代码")).toBeInTheDocument();
    expect(screen.getAllByText("未配置").length).toBeGreaterThan(0);
  });

  it("打开窗口只自动检查，不自动同步全量或周更", async () => {
    renderModal();
    await waitFor(() => expect(mockedCall).toHaveBeenCalledWith("NMPAAutoCheck"));
    expect(mockedCall).not.toHaveBeenCalledWith("NMPASyncFull");
    expect(mockedCall).not.toHaveBeenCalledWith("NMPASyncWeekly");
  });

  it("Supply Chain 只能查询和导出，看不到更新按钮", async () => {
    renderModal({}, false);
    await waitFor(() => expect(screen.getByText("42")).toBeInTheDocument());
    expect(screen.queryByRole("button", { name: /检查更新/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /同步全量/ })).not.toBeInTheDocument();
    expect(screen.getByText(/更新仅 Admin 可操作/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /导\s*出/ })).toBeInTheDocument();
  });

  it("摘要表只保留 8 个字段，不含包装数和联系人", async () => {
    renderModal();
    expect(await screen.findByText("椎间融合器")).toBeInTheDocument();
    expect(screen.queryByRole("columnheader", { name: "包装数" })).not.toBeInTheDocument();
    expect(screen.queryByRole("columnheader", { name: "联系人" })).not.toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "版本状态" })).toBeInTheDocument();
  });

  it("点击行打开全部字段详情和包装、联系人", async () => {
    renderModal();
    await waitFor(() => expect(mockedCall).toHaveBeenCalledWith("NMPASearch", "", 1, 20));
    await userEvent.click(screen.getByText("椎间融合器"));
    await waitFor(() => expect(mockedCall).toHaveBeenCalledWith("NMPADetail", "DI-1"));
    expect(await screen.findByText("发码机构")).toBeInTheDocument();
    expect(screen.getByText("GS1")).toBeInTheDocument();
    expect(screen.getByText("记录键")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /包装层级/ })).toBeInTheDocument();
    expect(screen.getByText("PKG-1")).toBeInTheDocument();
    expect(screen.getByText("a@example.com")).toBeInTheDocument();
  });

  it("导出当前查询结果到 xlsx", async () => {
    renderModal();
    await waitFor(() => expect(screen.getByText("椎间融合器")).toBeInTheDocument());
    await userEvent.click(screen.getByRole("button", { name: /导\s*出/ }));
    await waitFor(() => expect(mockedCall).toHaveBeenCalledWith("SelectNMPAExportTarget"));
    await waitFor(() =>
      expect(mockedCall).toHaveBeenCalledWith("NMPAExport", "", "/tmp/nmpa.xlsx"),
    );
  });
});
