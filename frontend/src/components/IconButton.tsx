// 统一图标按钮：无文字，名称只放在 hover 提示与 aria-label 里。
//
// 口径（与 AGENTS.md §8 的按钮规范配套）：
//   - 尺寸只有两档：small = 24px / 图标 14px，middle = 32px / 图标 16px；
//     状态栏的 20px 图标是历史特例，不走这里。
//   - 颜色表达语义：默认中性灰（查看 / 刷新 / 复制 / 关闭），danger = 危险红（清空 / 删除 / 清理）；
//     模块标题条已改为浅色底，不再需要反白变体；品牌红填充只留给「整合 / 登录 / 保存 / 发送 / 确定」这类主决策。
//   - 每个按钮都必须有 Tooltip + aria-label，二者用同一个 label，避免鼠标提示和读屏名称各说一套。
import { Button, Tooltip } from "antd";
import type { ReactNode } from "react";

export type IconButtonProps = {
  /** 唯一名称：同时用作 hover 提示与 aria-label */
  label: string;
  /** 可选的更长提示；读屏名称仍用 label */
  tooltip?: string;
  icon: ReactNode;
  onClick?: () => void;
  disabled?: boolean;
  loading?: boolean;
  danger?: boolean;
  size?: "small" | "middle";
  className?: string;
  ariaExpanded?: boolean;
};

export function IconButton({
  label,
  tooltip,
  icon,
  onClick,
  disabled,
  loading,
  danger,
  size = "small",
  className,
  ariaExpanded,
}: IconButtonProps) {
  const classes = [
    "ui-icon-button",
    `ui-icon-button-${size}`,
    danger ? "ui-icon-button-danger" : "",
    className ?? "",
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <Tooltip title={tooltip ?? label}>
      <Button
        type="text"
        size={size}
        className={classes}
        aria-label={label}
        aria-expanded={ariaExpanded}
        icon={icon}
        loading={loading}
        disabled={disabled}
        danger={danger}
        onClick={onClick}
      />
    </Tooltip>
  );
}
