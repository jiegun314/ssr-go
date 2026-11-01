// 图标连击的时间窗口口径：5 秒内点满 8 次触发一次，触发后计数清零。

import { describe, expect, it } from "vitest";

import { ICON_CLICK_COUNT, ICON_CLICK_WINDOW_MS, recordIconClick } from "../iconClicks";

describe("图标连击判定", () => {
  it("5 秒内点满 8 次才触发，第 7 次不触发", () => {
    let times: number[] = [];
    let triggered = 0;
    for (let index = 1; index <= ICON_CLICK_COUNT; index += 1) {
      const result = recordIconClick(times, index * 100);
      times = result.times;
      if (result.triggered) triggered += 1;
      expect(result.triggered).toBe(index === ICON_CLICK_COUNT);
    }
    expect(triggered).toBe(1);
    expect(times).toEqual([]); // 触发后清零
  });

  it("点击间隔超过窗口时重新计数", () => {
    let times: number[] = [];
    let triggered = false;
    for (let index = 0; index < ICON_CLICK_COUNT; index += 1) {
      const result = recordIconClick(times, index * (ICON_CLICK_WINDOW_MS + 1));
      times = result.times;
      triggered = triggered || result.triggered;
    }
    expect(triggered).toBe(false);
    expect(times).toHaveLength(1);
  });

  it("触发后重新计数，连续两轮可以各触发一次", () => {
    let times: number[] = [];
    let triggers = 0;
    for (let round = 0; round < 2; round += 1) {
      for (let index = 1; index <= ICON_CLICK_COUNT; index += 1) {
        const result = recordIconClick(times, round * 1000 + index * 10);
        times = result.times;
        if (result.triggered) triggers += 1;
      }
    }
    expect(triggers).toBe(2);
  });
});
