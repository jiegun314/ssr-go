// 运行日志的解析与分类。
//
// Go 侧每一行写成「时间 [级别] 消息」（契约见 app.go 的 logLevelLabels 与 log_test.go 的
// logLinePattern）：
//
//	2026-01-05 09:12:03 [信息] 应用已启动
//
// 多行消息的续行不带前缀，这里接回上一条 —— 界面因此能把它渲染成一张两列表格：
// 左列是时间（行首、不换行），右列是级别标签 + 正文（可换行、可多行）。

export type LogLevel = "info" | "success" | "warning" | "error";

export type LogEntry = {
  /** 列表 key：按解析顺序生成，日志文本变化时整体重建。 */
  key: string;
  /** 行首的时间戳；没有时间戳的兜底行是空串。 */
  time: string;
  level: LogLevel;
  message: string;
};

/** 与 Go 侧 logLinePattern 逐字一致的契约（改一处必须改两处，log_test.go 会校对）。 */
const LINE_PATTERN = /^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})\s+\[(信息|成功|警告|错误)\]\s?([\s\S]*)$/;

/** 级别标签 → 级别（写日志时 Go 已经定好级别，这里只是翻译回枚举）。 */
const LEVEL_BY_LABEL: Record<string, LogLevel> = {
  信息: "info",
  成功: "success",
  警告: "warning",
  错误: "error",
};

/** 栏目的顺序：全部 → 信息 → 成功 → 警告 → 错误。 */
export const LOG_LEVELS: LogLevel[] = ["info", "success", "warning", "error"];

export const LOG_LEVEL_LABELS: Record<LogLevel, string> = {
  info: "信息",
  success: "成功",
  warning: "警告",
  error: "错误",
};

/**
 * 兜底分类：行里没有级别标签时（旧格式日志、或别的程序写进来的行）按关键词猜一个。
 * 正常路径不会走到这里 —— Go 侧每条运行日志都带级别。
 */
export function classifyLogLine(line: string): LogLevel {
  const text = line.toLowerCase();
  if (/失败|错误|异常|无法|不支持|failed|error|fatal|exception/.test(text)) return "error";
  if (/警告|注意|缺失|重复|冲突|warning|warn/.test(text)) return "warning";
  if (/成功|完成|已清理|已保存|已导出|已备份|success|completed/.test(text)) return "success";
  return "info";
}

/**
 * 只取时段：日志是一次会话内的，日期全天不变 —— 窄栏里省下 8 个字符的宽度
 * （完整时间戳仍然保留在 time 字段里，界面用 title 显示）。
 */
export function shortTime(time: string): string {
  return time.length >= 19 ? time.slice(11) : time;
}

/** 把运行日志文本解析成一条条日志（时间 / 级别 / 正文）。 */
export function parseLog(text: string): LogEntry[] {
  const entries: LogEntry[] = [];
  for (const raw of text.split("\n")) {
    const line = raw.endsWith("\r") ? raw.slice(0, -1) : raw;
    const match = LINE_PATTERN.exec(line);
    if (match) {
      entries.push({
        key: `log-${entries.length}`,
        time: match[1],
        level: LEVEL_BY_LABEL[match[2]] ?? "info",
        message: match[3],
      });
      continue;
    }
    const last = entries[entries.length - 1];
    if (last) {
      // 续行：多行消息（导入失败明细等）挂在同一条日志上
      last.message = last.message === "" ? line : `${last.message}\n${line}`;
      continue;
    }
    if (line.trim() === "") continue;
    entries.push({
      key: `log-${entries.length}`,
      time: "",
      level: classifyLogLine(line),
      message: line,
    });
  }
  return entries;
}
