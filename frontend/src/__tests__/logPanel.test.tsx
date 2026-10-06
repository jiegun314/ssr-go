// 操作日志面板的行为测试：分栏筛选、新日志置顶、点开警告/错误看全文并复制。
//
// 这些以前只能靠 Go 侧对源码做字符串断言（改个类名就"失败"、真出问题却"通过"）；
// 现在在 jsdom 里渲染真实组件断言结果。

import { App as AntApp } from "antd";
import { render, screen, within } from "@testing-library/react";
import type { ReactElement } from "react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { LogPanel } from "../components/Panels";

// 契约：与 Go 侧 logLinePattern 一致的「时间 [级别] 消息」；续行不带前缀。
// antd 的 message 需要 <App> 上下文（main.tsx 里就是这样包的）
const renderWithApp = (ui: ReactElement) => render(<AntApp>{ui}</AntApp>);

const LOG_TEXT = [
  "2026-01-05 09:12:03 [信息] 应用已启动",
  "2026-01-05 09:12:05 [成功] 配置快照已保存：20260105-091205",
  "2026-01-05 09:12:10 [警告] 配置 X 缺失，已用默认配置 Y 生成",
  "2026-01-05 09:12:12 [错误] 导入失败：RA信息",
  "  第 13 行：统一社会信用代码 为空",
  "  第 14 行：注册证编号 为空",
].join("\n");

describe("操作日志面板", () => {
  it("解析每条日志的级别，并在标题里显示总条数", () => {
    render(<LogPanel text={LOG_TEXT} />);
    // 4 条（多行明细算同一条的续行）
    expect(screen.getByText("4 条")).toBeInTheDocument();
    // 级别标签都在表格里出现
    for (const label of ["信息", "成功", "警告", "错误"]) {
      expect(screen.getAllByTitle(label).length).toBeGreaterThan(0);
    }
  });

  it("多行明细接回同一条日志（续行不新增条目）", () => {
    render(<LogPanel text={LOG_TEXT} />);
    const row = screen.getByTitle(/^导入失败：RA信息/);
    expect(row.textContent).toContain("第 13 行：统一社会信用代码 为空");
    expect(row.textContent).toContain("第 14 行：注册证编号 为空");
  });

  it("默认新日志置顶，关掉开关后回到日志原顺序", async () => {
    const user = userEvent.setup();
    render(<LogPanel text={LOG_TEXT} />);
    const rows = () => screen.getAllByRole("row").map((row) => row.textContent ?? "");
    const firstMessageRow = () =>
      Array.from(document.querySelectorAll(".log-text")).map((node) => node.textContent ?? "");

    expect(firstMessageRow()[0]).toContain("导入失败");
    await user.click(screen.getByRole("switch"));
    expect(firstMessageRow()[0]).toContain("应用已启动");
    expect(rows().length).toBeGreaterThan(0);
  });

  it("按级别筛选：只看警告时其它级别不显示", async () => {
    const user = userEvent.setup();
    const { container } = render(<LogPanel text={LOG_TEXT} />);
    // "警告" 既是筛选片也是级别标签：用筛选片的类名精确点
    await user.click(container.querySelector(".log-filter-warning") as HTMLElement);

    const messages = Array.from(container.querySelectorAll(".log-text")).map((n) => n.textContent ?? "");
    expect(messages).toHaveLength(1);
    expect(messages[0]).toContain("配置 X 缺失");
  });

  it("点开错误日志能看全文并复制（带反馈）", async () => {
    const user = userEvent.setup();
    const writeText = vi.spyOn(navigator.clipboard, "writeText").mockResolvedValue(undefined);
    const { container } = renderWithApp(<LogPanel text={LOG_TEXT} />);

    await user.click(container.querySelector(".log-filter-error") as HTMLElement);
    await user.click(screen.getByTitle(/^导入失败：RA信息/));

    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("错误信息")).toBeInTheDocument();
    // 全文（含续行）在弹窗里完整可见
    expect(dialog.textContent).toContain("第 14 行：注册证编号 为空");

    await user.click(within(dialog).getByRole("button", { name: "复制全部信息" }));
    // 复制内容应当包含时间、级别与完整正文
    expect(writeText).toHaveBeenCalledTimes(1);
    const copied = writeText.mock.calls[0][0] as string;
    expect(copied).toContain("2026-01-05 09:12:12");
    expect(copied).toContain("导入失败：RA信息");
    expect(copied).toContain("第 14 行：注册证编号 为空");
    // antd 的 message 挂在 body 上（不在弹窗内）
    expect(await screen.findByText(/已复制全部信息/)).toBeInTheDocument();
  });

  it("信息类日志不可点开（只有警告/错误给详情）", async () => {
    const user = userEvent.setup();
    const { container } = render(<LogPanel text={LOG_TEXT} />);
    const infoRow = Array.from(container.querySelectorAll("tr")).find((row) =>
      row.textContent?.includes("应用已启动"),
    );
    expect(infoRow?.className).not.toContain("log-row-actionable");
    if (infoRow) {
      await user.click(infoRow);
    }
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
});
