// 设计 token 的一致性测试：styles.css 的 :root 变量必须与 design-tokens.ts 一一相等。
//
// 这就是"单一来源"的保证：值只在 design-tokens.ts 里定义一次，CSS 用 var() 引用；
// 如果谁只改了一边，这条测试立刻失败并指出差异。

import { describe, expect, it } from "vitest";

import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { CSS_TOKENS } from "../design-tokens";

// 直接读源文件：vitest 默认不处理 CSS，用 ?raw 会拿到空模块；
// 测试的 CWD 是 frontend/（vitest 的 root 就是这里）。
const styles = readFileSync(resolve(process.cwd(), "src/styles.css"), "utf8");

/** 从 CSS 文本里取 :root 块中声明的自定义属性。 */
function rootVariables(css: string): Record<string, string> {
  const block = /:root\s*\{([\s\S]*?)\}/.exec(css);
  if (!block) throw new Error("styles.css 里找不到 :root 块");
  const found: Record<string, string> = {};
  for (const line of block[1].split("\n")) {
    const match = /--([a-z0-9-]+)\s*:\s*([^;]+);/.exec(line);
    if (match) found[match[1]] = match[2].trim();
  }
  return found;
}

describe("设计 token 单一来源", () => {
  const declared = rootVariables(styles);

  it("每个 token 都在 :root 里声明，且值与 design-tokens.ts 相同", () => {
    for (const [name, value] of Object.entries(CSS_TOKENS)) {
      expect(declared[name], `:root 里缺少 --${name}`).toBeDefined();
      expect(declared[name], `--${name} 与 design-tokens.ts 不一致`).toBe(value);
    }
  });

  it("CSS 里不再重复写这些字面量（一律走 var()）", () => {
    // 颜色与关键尺寸必须通过变量引用，避免两处维护
    // 只看规则体：注释里提到色值（说明性文字）不算重复维护
    const body = styles
      .replace(/:root\s*\{[\s\S]*?\}/, "")
      .replace(/\/\*[\s\S]*?\*\//g, "");
    // 这些"同一语义"的规则必须走变量（同色值在别处可能是别的意思，例如状态条、值着色，
    // 所以不能对整份 CSS 做"不许出现该字面量"的粗暴检查）
    const sameMeaningRules = [
      ".log-level-info", ".log-level-success", ".log-level-warning", ".log-level-error",
      ".log-filter-info", ".log-filter-success", ".log-filter-warning", ".log-filter-error",
    ];
    for (const selector of sameMeaningRules) {
      const block = new RegExp(`${selector.replace(/\./g, "\\.")}\\s*\\{([^}]*)\\}`).exec(body);
      expect(block, `找不到规则 ${selector}`).not.toBeNull();
      expect(block?.[1], `${selector} 应当用 var(--log-*)`).toMatch(/var\(--log-/);
      expect(block?.[1], `${selector} 里不该再写颜色字面量`).not.toMatch(/#[0-9a-fA-F]{3,6}/);
    }
  });

  it("关键规则确实引用了这些变量", () => {
    expect(styles).toContain("grid-auto-columns: var(--status-chip-width);");
    expect(styles).toContain("width: var(--log-time-width);");
    expect(styles).toContain("width: var(--log-level-slot);");
    expect(styles).toContain("min-height: var(--log-min-height);");
    expect(styles).toContain("background: var(--log-head-bg);");
    expect(styles).toContain("border: 1px solid var(--module-border);");
  });
});
