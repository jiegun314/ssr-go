import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Wails 的 asset server 与 file:// 预览都要能用：资源用相对路径。
// 构建产物直接落在 frontend/dist（随 //go:embed 编进二进制）。
export default defineConfig({
  base: "./",
  plugins: [react()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
    target: "es2020",
    sourcemap: false,
  },
});
