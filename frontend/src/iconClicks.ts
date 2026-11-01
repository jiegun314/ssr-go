// 图标连击判定：只统计最近 5 秒内的点击，达到 8 次即触发一次（触发后计数清零）。
//
// 单独放在这里是为了能直接单测时间窗口的口径（App 里只负责"触发之后做什么"）。

export const ICON_CLICK_COUNT = 8;
export const ICON_CLICK_WINDOW_MS = 5000;

export type IconClickResult = {
  /** 保留的点击时间戳（触发后为空） */
  times: number[];
  triggered: boolean;
};

export function recordIconClick(times: number[], now: number): IconClickResult {
  const recent = times.filter((time) => now - time < ICON_CLICK_WINDOW_MS);
  recent.push(now);
  if (recent.length >= ICON_CLICK_COUNT) {
    return { times: [], triggered: true };
  }
  return { times: recent, triggered: false };
}
