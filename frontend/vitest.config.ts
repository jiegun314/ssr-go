import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

// 前端单元测试（jsdom）：组件在真实 DOM 里渲染，测"行为"——
// 过滤、点开详情、复制、控件改值与保存载荷、抽屉内容等。
//
// 与 Go 侧契约测试的分工：
//   - 这里：渲染出来的东西（文本、属性、交互结果、回调载荷）；
//   - Go 侧（layout_*_test.go）：跨语言与设计口径（颜色 token、中文文案、产物内嵌、CSS 规则）。
export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
    // antd 组件量大，jsdom 首次渲染偏慢；给足超时
    testTimeout: 20000,
    restoreMocks: true,
  },
});
