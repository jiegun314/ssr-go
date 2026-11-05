// 桥接层的"载入图层"规则：只有登记在 BUSY_TEXT 里的长任务才闪遮罩。
// 状态轮询（ShareOverview）必须静默，否则每次轮询都闪一下，看起来像界面在刷新。

import { afterEach, describe, expect, it, vi } from "vitest";

import { BUSY_TEXT, call, onBusy } from "../bridge";

type BusyLog = (string | null)[];

const installBridge = (method: string) => {
  const handler = vi.fn(async () => ({ ok: true }));
  (window as unknown as { go?: unknown }).go = {
    app: { App: { [method]: handler } },
  };
  return handler;
};

describe("载入图层的触发规则", () => {
  afterEach(() => {
    delete (window as unknown as { go?: unknown }).go;
  });

  it("轮询 ShareOverview 不触发载入图层", async () => {
    installBridge("ShareOverview");
    const log: BusyLog = [];
    const off = onBusy((text) => log.push(text));
    await call("ShareOverview");
    off();
    expect(log).toEqual([]);
  });

  it("导入这类长任务会先给出文案、结束后清空", async () => {
    installBridge("ImportSource");
    const log: BusyLog = [];
    const off = onBusy((text) => log.push(text));
    await call("ImportSource", "ra_input", "/tmp/a.xlsx");
    off();
    expect(log[0]).toBe(BUSY_TEXT.ImportSource);
    expect(log[log.length - 1]).toBeNull();
  });
});
