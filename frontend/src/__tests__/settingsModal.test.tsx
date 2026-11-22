// 参数设定工作台的行为测试：左侧配置文件导航、右侧文件头、底部保存状态。
// 不重复 SettingsForm 的控件细节（那部分由 settingsForm.test.tsx 覆盖）。

import { App as AntApp } from "antd";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { SettingsModal } from "../components/Modals";
import type { SettingsTab } from "../types";

const TREE: SettingsTab["tree"] = [{
  key: "app",
  kind: "map",
  path: ["app"],
  label: "应用信息",
  children: [
    {
      key: "name",
      kind: "string",
      value: "SingleSourceReady",
      control: "input",
      editable: true,
      label: "应用名称",
      path: ["app", "name"],
    },
  ],
}];

const TABS: SettingsTab[] = [
  {
    key: "setting",
    title: "基础设置",
    note: "路径、表名与启动策略",
    path: "config/setting.yaml",
    tree: TREE,
    raw: "",
  },
  {
    key: "consolidation",
    title: "整合映射",
    note: "匹配与导出规则",
    path: "config/consolidation_mapping.yaml",
    tree: TREE,
    raw: "",
  },
];

const onSaveValues = vi.fn(async () => true);

beforeEach(() => onSaveValues.mockClear());

function Harness() {
  const [activeKey, setActiveKey] = useState(TABS[0].key);
  return (
    <AntApp>
      <SettingsModal
        open
        tabs={TABS}
        activeKey={activeKey}
        editing={false}
        editorValue=""
        path={[]}
        expandedKeys={[]}
        onSelectTab={setActiveKey}
        onSelectNode={() => undefined}
        onExpandedChange={() => undefined}
        onToggleAll={() => undefined}
        onStartEdit={() => undefined}
        onCancelEdit={() => undefined}
        onEditorChange={() => undefined}
        onSave={() => undefined}
        onSaveValues={onSaveValues}
        onClose={() => undefined}
      />
    </AntApp>
  );
}

describe("参数设定工作台", () => {
  it("左侧导航切换配置文件，右侧文件头随之更新", async () => {
    const user = userEvent.setup();
    render(<Harness />);

    expect(screen.getByText("config/setting.yaml")).toBeInTheDocument();
    const consolidation = screen.getByRole("button", { name: /整合映射/ });
    await user.click(consolidation);

    expect(screen.getByText("config/consolidation_mapping.yaml")).toBeInTheDocument();
    expect(consolidation.className).toContain("is-active");
  });

  it("改动后底栏计数与撤销全部生效，保存交给上层", async () => {
    const user = userEvent.setup();
    render(<Harness />);

    const input = screen.getByDisplayValue("SingleSourceReady");
    await user.clear(input);
    await user.type(input, "SSR");

    expect(screen.getByText("1 项未保存")).toBeInTheDocument();
    const save = screen.getByRole("button", { name: "保存改动" });
    expect(save).toBeEnabled();

    await user.click(screen.getByRole("button", { name: "撤销全部" }));
    expect(screen.getByText("当前文件无未保存改动")).toBeInTheDocument();
    expect(screen.getByDisplayValue("SingleSourceReady")).toBeInTheDocument();

    await user.clear(screen.getByDisplayValue("SingleSourceReady"));
    await user.type(screen.getByDisplayValue(""), "SSR");
    await user.click(screen.getByRole("button", { name: "保存改动" }));
    await vi.waitFor(() => expect(onSaveValues).toHaveBeenCalledWith([{ path: ["app", "name"], value: "SSR" }]));
  });
});
