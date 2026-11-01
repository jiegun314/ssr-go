// 关于窗口（参考 CLEAR 的格式）：图标 / 产品名 / 副标题 / 中文名 / 版本 /
// 路径信息表（数据目录、参数文件、数据库、运行环境）/ 技术方案 / 说明文字。

import { App as AntApp, ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { AboutModal } from "../components/Modals";
import type { AboutInfo } from "../types";

const info = {
  name: "SingleSourceReady",
  tagline: "Single Source Ready · UDI Data Consolidation Platform",
  chineseName: "UDI 数据整合与单源数据准备平台",
  version: "版本 2.3.13+23.g61a087b",
  detail: "commit 61a087b, build 2026-11-01T15:14:36+08:00",
  dataDir: "/Users/me/ssr",
  configDir: "/Users/me/ssr/config",
  database: "/Users/me/ssr/data/udi_data.sqlite3",
  runtime: "go1.25.4 · darwin/arm64",
  techStack: ["Go + Wails v2 + SQLite（modernc，纯 Go、无 CGO）", "前端 React 18 + Ant Design 5 + Vite"],
} as unknown as AboutInfo;

const renderAbout = () => {
  const onIconClick = vi.fn();
  render(
    <ConfigProvider locale={zhCN}>
      <AntApp>
        <AboutModal open info={info} onIconClick={onIconClick} onClose={vi.fn()} />
      </AntApp>
    </ConfigProvider>,
  );
  return { onIconClick };
};

describe("关于窗口", () => {
  it("显示产品名、副标题、中文名与版本", () => {
    renderAbout();
    expect(screen.getByText("SingleSourceReady")).toBeInTheDocument();
    expect(screen.getByText(/Single Source Ready · UDI Data Consolidation Platform/)).toBeInTheDocument();
    expect(
      screen.getByText(/SS Ready provides data validation, cleansing/),
    ).toBeInTheDocument();
    expect(screen.getByText("版本 2.3.13+23.g61a087b")).toBeInTheDocument();
  });

  it("显示路径信息表与运行环境", () => {
    renderAbout();
    for (const label of ["数据目录", "参数文件", "数据库", "运行环境"]) {
      expect(screen.getByText(label)).toBeInTheDocument();
    }
    expect(screen.getByText("/Users/me/ssr/config")).toBeInTheDocument();
    expect(screen.getByText("go1.25.4 · darwin/arm64")).toBeInTheDocument();
  });

  it("显示技术方案清单", () => {
    renderAbout();
    expect(screen.getByText("技术方案")).toBeInTheDocument();
    expect(screen.getByText(/Go \+ Wails v2 \+ SQLite/)).toBeInTheDocument();
    expect(screen.getByText(/React 18 \+ Ant Design 5/)).toBeInTheDocument();
  });
});

describe("关于窗口的图标", () => {
  it("图标可点击（每次点击都回调，由外层决定何时切换窗口）", async () => {
    const { onIconClick } = renderAbout();
    const icon = screen.getByAltText("SingleSourceReady");
    for (let index = 0; index < 8; index += 1) {
      await userEvent.click(icon);
    }
    expect(onIconClick).toHaveBeenCalledTimes(8);
  });
});
