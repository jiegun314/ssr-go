// vitest 全局垫片：jsdom 里缺的浏览器 API（antd 会用到）。
//
// 只补齐"没有就会报错"的部分，不做任何业务相关的 mock ——
// 需要 Wails 桥接的测试自己按 preview.sh 的方式挂 window.go。

import "@testing-library/jest-dom/vitest";

import { afterEach, vi } from "vitest";
import { cleanup } from "@testing-library/react";

// matchMedia：antd 的响应式断点（Grid / useBreakpoint）依赖它
if (!window.matchMedia) {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }),
  });
}

// ResizeObserver：antd 的 Table/Tooltip/Select 都会用
if (!window.ResizeObserver) {
  window.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
}

// jsdom 没实现第二参数（伪元素）：rc-table 量滚动条宽度时会传它，
// 不补就会刷一堆 "Not implemented: window.getComputedStyle(elt, pseudoElt)" 噪音。
const originalGetComputedStyle = window.getComputedStyle.bind(window);
window.getComputedStyle = ((element: Element) =>
  originalGetComputedStyle(element)) as typeof window.getComputedStyle;

// 复制到剪贴板：jsdom 没有实现，测试里桩掉（组件会走 navigator.clipboard 分支）
// configurable 必须为 true：@testing-library/user-event 会重新定义它
if (!navigator.clipboard) {
  Object.defineProperty(navigator, "clipboard", {
    writable: true,
    configurable: true,
    value: { writeText: vi.fn().mockResolvedValue(undefined) },
  });
}

afterEach(() => {
  cleanup();
  delete (window as unknown as { go?: unknown }).go;
});
