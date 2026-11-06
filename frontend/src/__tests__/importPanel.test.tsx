// 数据导入区的两个查看入口：
//   - 「数据回顾」每个来源都有，看原文表；
//   - 「查看清洗后数据」只在声明了 clean 的来源（RA 信息）出现，看整合实际读取的清洗表。
// 这里验证按钮的出现范围与回调参数（后端绑定另有 Go 测试）。

import { App as AntApp, ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { ImportPanel } from "../components/Panels";
import type { ImportState } from "../types";

const importedState = (): ImportState =>
  ({
    source: "ra_input",
    state: "imported",
    label: "导入6条记录",
    tooltip: "最近导入：2026/10/31 20:00，6 行",
    rowCount: 6,
    importTime: "2026/10/31 20:00",
  }) as ImportState;

const renderPanel = (
  states: Record<string, ImportState>,
  locked = false,
  options: {
    importable?: string[];
    viewable?: string[];
    shareInfo?: Record<string, import("../types").SourceStatus>;
    alignMode?: boolean;
    showNotify?: boolean;
  } = {},
) => {
  const onImport = vi.fn();
  const onNotify = vi.fn();
  const onReview = vi.fn();
  const onReviewCleaned = vi.fn();
  render(
    <ConfigProvider locale={zhCN}>
      <AntApp>
        <ImportPanel
          states={states}
          alignMode={options.alignMode}
          importableSources={options.importable ?? Object.keys(states)}
          viewableSources={options.viewable ?? Object.keys(states)}
          shareInfo={options.shareInfo}
          locked={locked}
          onImport={onImport}
          onNotify={onNotify}
          showNotify={options.showNotify ?? true}
          onReview={onReview}
          onReviewCleaned={onReviewCleaned}
        />
      </AntApp>
    </ConfigProvider>,
  );
  return { onImport, onNotify, onReview, onReviewCleaned };
};

/** 按来源标题定位那一行（四个来源始终整块渲染，不能只按数量断言）。 */
const sourceRow = (title: string): HTMLElement => {
  const row = screen.getByText(title).closest(".group");
  if (!row) throw new Error(`找不到来源行：${title}`);
  return row as HTMLElement;
};

describe("数据导入区的查看入口", () => {
  it("RA 信息那一行同时有「数据回顾」和「查看清洗后数据」", async () => {
    const { onReview, onReviewCleaned } = renderPanel({
      ra_input: importedState(),
      global_udi_input: { ...importedState(), source: "global_udi_input" },
    });

    const row = sourceRow("RA信息");
    expect(within(row).getByLabelText("数据回顾")).toBeInTheDocument();
    const cleaned = within(row).getByLabelText("查看清洗后数据");

    // 全界面只有 RA 这一行有这个入口
    expect(screen.getAllByLabelText("查看清洗后数据")).toHaveLength(1);

    await userEvent.click(cleaned);
    expect(onReviewCleaned).toHaveBeenCalledWith("ra_input");
    expect(onReview).not.toHaveBeenCalled();
  });

  it("没有清洗表的来源不出现「查看清洗后数据」", () => {
    renderPanel({ global_udi_input: { ...importedState(), source: "global_udi_input" } });
    const row = sourceRow("UDI团队信息");
    expect(within(row).queryByLabelText("查看清洗后数据")).toBeNull();
    expect(within(row).getByLabelText("数据回顾")).toBeInTheDocument();
  });

  it("看不到他人来源明细时只显示最后更新时间（没有按钮）", () => {
    renderPanel({ ra_input: importedState() }, false, {
      importable: ["ra_input"],
      viewable: ["ra_input"],
      shareInfo: {
        ra_input: {
          source: "ra_input", label: "RA信息", canView: true, localVersion: "",
          remoteVersion: "v1", remoteRows: 6, publishedAt: "2026-11-01 00:00:00",
          publishedBy: "zhangsan", aligned: false, pending: true,
        },
        global_udi_input: {
          source: "global_udi_input", label: "UDI团队信息", canView: false,
          localVersion: "", remoteVersion: "", remoteRows: 0,
          publishedAt: "2026-11-01 00:30:00", publishedBy: "", aligned: false, pending: false,
        },
      },
    });
    const own = sourceRow("RA信息");
    expect(within(own).getByText("待整合")).toBeInTheDocument();

    const other = sourceRow("UDI团队信息");
    expect(within(other).getByText("最后更新：2026-11-01 00:30:00")).toBeInTheDocument();
    expect(within(other).queryByLabelText("载入文件")).toBeNull();
    expect(within(other).queryByLabelText("数据回顾")).toBeNull();
  });

  it("共享盘异常（locked）时载入文件禁用", () => {
    renderPanel({ ra_input: importedState() }, true);
    const row = sourceRow("RA信息");
    expect(within(row).getByLabelText("载入文件")).toBeDisabled();
  });

  it("未导入时两个查看按钮都禁用", () => {
    renderPanel({
      ra_input: { ...importedState(), state: "empty", label: "", tooltip: "" },
    });
    const row = sourceRow("RA信息");
    expect(within(row).getByLabelText("数据回顾")).toBeDisabled();
    expect(within(row).getByLabelText("查看清洗后数据")).toBeDisabled();
  });
});

describe("Supply Chain 的数据来源（取数状态）", () => {
  const share = (overrides: Record<string, unknown>) =>
    ({
      source: "ra_input",
      label: "RA信息",
      canView: true,
      localVersion: "",
      remoteVersion: "",
      remoteRows: 0,
      publishedAt: "",
      publishedBy: "",
      aligned: false,
      pending: false,
      ...overrides,
    }) as unknown as import("../types").SourceStatus;

  it("已对齐：显示已对齐 + 最后更新时间，查看可用", () => {
    renderPanel({ ra_input: importedState() }, false, {
      alignMode: true,
      importable: [],
      viewable: ["ra_input"],
      shareInfo: {
        ra_input: share({
          remoteVersion: "20261101T141401-1ca13a38",
          publishedAt: "2026-11-01 14:14:01",
          aligned: true,
          remoteRows: 2435,
        }),
      },
    });
    expect(screen.getByText(/已对齐 · 2026-11-01 14:14/)).toBeInTheDocument();
    expect(screen.getByLabelText("数据回顾")).toBeEnabled();
    expect(screen.queryByLabelText("载入文件")).toBeNull();
  });

  it("未取数：提示未取数，查看不可用", () => {
    renderPanel({}, false, {
      alignMode: true,
      importable: [],
      viewable: ["ra_input"],
      shareInfo: {
        ra_input: share({ remoteVersion: "20261101T141401-1ca13a38", publishedAt: "2026-11-01 14:14:01" }),
      },
    });
    expect(screen.getByText(/未取数/)).toBeInTheDocument();
    expect(screen.getByLabelText("数据回顾")).toBeDisabled();
  });

  it("共享盘还没发布：显示尚未发布", () => {
    renderPanel({}, false, {
      alignMode: true,
      importable: [],
      viewable: ["ra_input"],
      shareInfo: { ra_input: share({}) },
    });
    // 四个来源都没发布时都会显示这句
    expect(screen.getAllByText(/共享盘尚未发布/).length).toBeGreaterThan(0);
  });
});

describe("数据导入区的邮件通知入口", () => {
  it("导入前禁用", () => {
    const { onNotify } = renderPanel({
      ra_input: { ...importedState(), state: "empty", label: "尚未导入", rowCount: 0 },
    });
    const button = within(sourceRow("RA信息")).getByRole("button", { name: "邮件通知" });
    expect(button).toBeDisabled();
    expect(onNotify).not.toHaveBeenCalled();
  });

  it("导入后激活并回调来源", async () => {
    const { onNotify } = renderPanel({ ra_input: importedState() });
    const button = within(sourceRow("RA信息")).getByRole("button", { name: "邮件通知" });
    expect(button).toBeEnabled();
    await userEvent.click(button);
    expect(onNotify).toHaveBeenCalledWith("ra_input");
  });
});

describe("邮件通知按钮的显示范围", () => {
  it("Supply Chain / Admin 登录时不显示按钮", () => {
    renderPanel({ ra_input: importedState() }, false, { showNotify: false });
    expect(screen.queryByRole("button", { name: "邮件通知" })).toBeNull();
  });

  it("没有该来源导入权限时按钮可见但不可用", () => {
    renderPanel({ ra_input: importedState(), product_category: importedState() }, false, {
      importable: ["ra_input"],
      showNotify: true,
    });
    const allowed = within(sourceRow("RA信息")).getByRole("button", { name: "邮件通知" });
    const denied = within(sourceRow("产品类别")).getByRole("button", { name: "邮件通知" });
    expect(allowed).toBeEnabled();
    expect(denied).toBeDisabled();
  });
});
