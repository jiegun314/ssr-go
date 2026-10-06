// 参数设定「结构化表单」的行为测试：
// 控件按 control 渲染、只读项给值与原因、注释去掉行首 #、标量列表增删、
// 列清单用表格 + 抽屉且不给增删排序、改动以"路径 + 值"的载荷交给上层保存。

import { App as AntApp } from "antd";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { SettingsForm, stripCommentMarks } from "../components/SettingsForm";
import type { SettingsChange, SettingsNode } from "../types";

const scalar = (over: Partial<SettingsNode> & { key: string; path: string[] }): SettingsNode => ({
  kind: "string",
  control: "input",
  editable: true,
  ...over,
});

const COLUMN = (index: number, name: string, status: string): SettingsNode => ({
  key: String(index),
  kind: "map",
  path: ["sources", "ra_input", "columns", String(index)],
  children: [
    scalar({ key: "chinese_name", value: name, label: "列名", path: ["sources", "ra_input", "columns", String(index), "chinese_name"] }),
    scalar({
      key: "db_field",
      value: `db_${index}`,
      control: "none",
      editable: false,
      reason: "决定暂存表的列名：改动会让已有数据表对不上",
      path: ["sources", "ra_input", "columns", String(index), "db_field"],
    }),
    scalar({
      key: "mandatory_status",
      value: status,
      control: "select",
      options: ["required", "optional", "required_if_applicable"],
      path: ["sources", "ra_input", "columns", String(index), "mandatory_status"],
    }),
  ],
});

const NODES: SettingsNode[] = [
  scalar({ key: "version", value: "1.0", control: "none", editable: false, reason: "版本号由程序维护", path: ["version"] }),
  {
    key: "startup",
    kind: "map",
    path: ["startup"],
    comment: "# 启动行为：默认 false —— 重启后四张来源表原样保留。\n# 需要每次启动都清空时改成 true。",
    children: [
      scalar({ key: "cleanup_on_startup", value: "false", control: "switch", label: "启动时清空导入数据", path: ["startup", "cleanup_on_startup"] }),
    ],
  },
  {
    key: "export_template",
    kind: "map",
    path: ["export_template"],
    children: [
      scalar({ key: "header_row", value: "4", kind: "number", control: "number", min: 1, max: 100000, path: ["export_template", "header_row"] }),
      scalar({ key: "english_header_row", value: "null", kind: "null", control: "number", note: "只给样本工厂用；留空表示 null", path: ["export_template", "english_header_row"] }),
    ],
  },
  {
    key: "global_settings",
    kind: "map",
    path: ["global_settings"],
    children: [
      {
        key: "mandatory_options",
        kind: "seq",
        path: ["global_settings", "mandatory_options"],
        listEdit: true,
        label: "必填状态可选值",
        children: [
          scalar({ key: "0", value: "required", path: ["global_settings", "mandatory_options", "0"] }),
          scalar({ key: "1", value: "optional", path: ["global_settings", "mandatory_options", "1"] }),
        ],
      },
    ],
  },
  {
    key: "sources",
    kind: "map",
    path: ["sources"],
    children: [
      {
        key: "ra_input",
        kind: "map",
        path: ["sources", "ra_input"],
        children: [
          scalar({
            key: "chinese_name",
            value: "RA信息",
            label: "来源中文名",
            path: ["sources", "ra_input", "chinese_name"],
          }),
          {
            key: "columns",
            kind: "seq",
            path: ["sources", "ra_input", "columns"],
            listEdit: false,
            comment: "# 列定义：表头匹配只认 chinese_name 的精确值。",
            children: [COLUMN(0, "注册证编号", "required"), COLUMN(1, "原注册证编号", "optional")],
          },
        ],
      },
    ],
  },
];

type Handlers = {
  onChange: ReturnType<typeof vi.fn>;
  onResetPath: ReturnType<typeof vi.fn>;
  onReset: ReturnType<typeof vi.fn>;
};

const renderForm = (pending: SettingsChange[] = []): Handlers => {
  const handlers: Handlers = { onChange: vi.fn(), onResetPath: vi.fn(), onReset: vi.fn() };
  render(
    <AntApp>
      <SettingsForm
        nodes={NODES}
        pending={pending}
        onChange={handlers.onChange}
        onResetPath={handlers.onResetPath}
        onReset={handlers.onReset}
      />
    </AntApp>,
  );
  return handlers;
};

describe("stripCommentMarks（注释显示口径）", () => {
  it("去掉行首 # 与紧随的一个空格，保留缩进与空行", () => {
    expect(stripCommentMarks("# 说明")).toBe("说明");
    expect(stripCommentMarks("#  缩进保留")).toBe(" 缩进保留");
    expect(stripCommentMarks("# 第一行\n#\n#   条目")).toBe("第一行\n\n  条目");
    expect(stripCommentMarks("没有井号的行")).toBe("没有井号的行");
  });
});

describe("参数设定结构化表单", () => {
  it("每一层是一个分块，标量给控件，只读项给值与原因", () => {
    renderForm();
    // 顶层分块
    expect(document.querySelectorAll('.settings-block[data-depth="0"]').length).toBe(4);
    // 只读项：显示值 + 原因，且不给输入框
    expect(screen.getByText("1.0")).toBeInTheDocument();
    expect(screen.getByText("版本号由程序维护")).toBeInTheDocument();
    // 开关 / 数字 / 下拉 / 输入框都按 control 渲染
    expect(document.querySelector(".ant-switch")).not.toBeNull();
    expect(document.querySelector(".ant-input-number")).not.toBeNull();
    expect(document.querySelectorAll(".settings-field .ant-input").length).toBeGreaterThan(0);
  });

  it("注释显示时不带行首 #，多行与空行保留", () => {
    renderForm();
    const comment = document.querySelector(".settings-comment");
    expect(comment?.textContent).toContain("启动行为：默认 false");
    expect(comment?.textContent).not.toContain("# 启动行为");
    expect(comment?.textContent?.split("\n")).toHaveLength(2);
  });

  it("改开关：以「路径 + 新值」的形式交给上层", async () => {
    const user = userEvent.setup();
    const { onChange } = renderForm();
    await user.click(document.querySelector(".ant-switch") as HTMLElement);
    expect(onChange).toHaveBeenCalledWith({ path: ["startup", "cleanup_on_startup"], value: "true" });
  });

  it("改数字：写入字符串形态的数字；清空表示 null", async () => {
    const user = userEvent.setup();
    const { onChange } = renderForm();
    const inputs = document.querySelectorAll(".ant-input-number input");
    const numberInput = inputs[0] as HTMLInputElement;
    await user.clear(numberInput);
    await user.type(numberInput, "7");
    const calls = onChange.mock.calls;
    const last = calls[calls.length - 1]?.[0] as SettingsChange;
    expect(last.path).toEqual(["export_template", "header_row"]);
    expect(last.value).toBe("7");
  });

  it("下拉的候选值来自后端给的 options（列清单抽屉里）", async () => {
    const user = userEvent.setup();
    renderForm();
    // 打开第一列的抽屉：必填状态是 select，候选值必须来自节点上的 options
    const table = document.querySelector(".settings-columns") as HTMLElement;
    await user.click(table.querySelector(".ant-table-tbody tr.ant-table-row") as HTMLElement);
    const drawer = await screen.findByRole("dialog");
    const select = drawer.querySelector(".ant-select") as HTMLElement;
    expect(select.textContent).toContain("required");

    await user.click(select.querySelector(".ant-select-selector") as HTMLElement);
    const options = Array.from(document.querySelectorAll(".ant-select-item-option")).map(
      (node) => node.textContent ?? "",
    );
    expect(options).toEqual(["required", "optional", "required_if_applicable"]);
  });

  it("标量列表：删除给 remove + 下标，新增给 append + 值", async () => {
    const user = userEvent.setup();
    const { onChange } = renderForm();
    await user.click(screen.getByRole("button", { name: "删除第 1 条" }));
    expect(onChange).toHaveBeenCalledWith({
      path: ["global_settings", "mandatory_options"],
      action: "remove",
      index: 0,
      value: "",
    });

    const addInput = screen.getByLabelText("向 必填状态可选值 添加一条");
    await user.type(addInput, "required_if_applicable");
    await user.click(screen.getByRole("button", { name: "添加一条" }));
    expect(onChange).toHaveBeenCalledWith({
      path: ["global_settings", "mandatory_options"],
      action: "append",
      value: "required_if_applicable",
    });
  });

  it("列清单：表格一行一列，明确不支持增删排序，且不给增删入口", async () => {
    const user = userEvent.setup();
    renderForm();
    const table = document.querySelector(".settings-columns") as HTMLElement;
    expect(table).not.toBeNull();
    expect(table.querySelectorAll(".ant-table-tbody tr.ant-table-row")).toHaveLength(2);
    expect(screen.getByText(/不支持新增 \/ 删除 \/ 排序/)).toBeInTheDocument();

    // 点第一行打开抽屉：能看到该列的字段与只读原因
    await user.click(table.querySelector(".ant-table-tbody tr.ant-table-row") as HTMLElement);
    const drawer = await screen.findByRole("dialog");
    expect(within(drawer).getByDisplayValue("注册证编号")).toBeInTheDocument();
    expect(drawer.textContent).toContain("决定暂存表的列名");
    // 抽屉里只有这一列的字段，没有"添加一条"这类列表操作
    expect(within(drawer).queryByRole("button", { name: "添加一条" })).toBeNull();
  });

  it("列清单里的只读字段不给输入框", () => {
    renderForm();
    const readonly = Array.from(document.querySelectorAll(".settings-readonly")).map(
      (node) => node.textContent ?? "",
    );
    expect(readonly).toContain("db_0");
  });

  it("有未保存改动时给出条数，并能整批撤销", async () => {
    const user = userEvent.setup();
    const { onReset } = renderForm([
      { path: ["startup", "cleanup_on_startup"], value: "true" },
      { path: ["export_template", "header_row"], value: "7" },
    ]);
    const banner = document.querySelector(".settings-pending") as HTMLElement;
    expect(banner.textContent).toContain("2");
    await user.click(screen.getByRole("button", { name: /全部撤销/ }));
    expect(onReset).toHaveBeenCalledTimes(1);
  });

  it("待保存的值直接体现在控件上（不依赖重新拉配置）", () => {
    renderForm([{ path: ["startup", "cleanup_on_startup"], value: "true" }]);
    expect(document.querySelector(".ant-switch")).toHaveClass("ant-switch-checked");
    // 改动行有高亮标记
    expect(document.querySelector(".settings-field[data-dirty]")).not.toBeNull();
  });
});
