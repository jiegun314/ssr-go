// 登录页（P1）：按**组别**选择，不显示具体人名。
//
// 布局（用户口径）：
//   - 顶部 Welcome, <本机用户名>；
//   - 上面一行四个方块：图标在上、组别在下（RA / EAMA / Global UDI Team / Supply Chain）；
//   - 下面一行居中一个：管理员（图标 + 组别），点它才出现密码框；
//   - 除 Admin 外点一下即进入（不读域信息，只记录本机用户名）。

import { Button, Input, Typography } from "antd";
import {
  Barcode,
  ClipboardList,
  Merge,
  ShieldCheck,
  Tags,
  type LucideIcon,
} from "lucide-react";
import { useState } from "react";

import type { LoginRole } from "../types";

/** 组别 → 图标（上图标、下文字）。 */
const ROLE_ICONS: Record<string, LucideIcon> = {
  ra: ClipboardList,
  eama: Tags,
  udi: Barcode,
  supply_chain: Merge,
  admin: ShieldCheck,
};

type LoginScreenProps = {
  roles: LoginRole[];
  /** 本机用户名（Welcome, XXX） */
  username: string;
  /** 选中组别后的回调：非 Admin 的 password 传空串 */
  onLogin: (role: string, password: string) => void;
  busy?: string | null;
};

export function LoginScreen({ roles, username, onLogin, busy }: LoginScreenProps) {
  const [passwordOpen, setPasswordOpen] = useState(false);
  const [password, setPassword] = useState("");
  const disabled = Boolean(busy);
  const mainRoles = roles.filter((role) => !role.needsPassword);
  const adminRole = roles.find((role) => role.needsPassword);

  const choose = (role: LoginRole) => {
    if (role.needsPassword) {
      setPasswordOpen(true);
      setPassword("");
      return;
    }
    setPasswordOpen(false);
    onLogin(role.key, "");
  };

  const submitAdmin = () => {
    if (!adminRole || disabled || password.trim() === "") return;
    onLogin(adminRole.key, password);
  };

  const tile = (role: LoginRole) => {
    const Icon = ROLE_ICONS[role.key] ?? ClipboardList;
    return (
      <Button
        key={role.key}
        className="login-tile"
        aria-label={role.label}
        disabled={disabled}
        onClick={() => choose(role)}
      >
        <Icon size={30} strokeWidth={1.6} />
        <span className="login-tile-label">{role.label}</span>
        {role.hint ? <span className="login-tile-hint">{role.hint}</span> : null}
      </Button>
    );
  };

  return (
    <div className="login-screen">
      <div className="login-panel">
        <div className="login-welcome">Welcome, {username}</div>
        <Typography.Text type="secondary" className="login-subtitle">
          请选择登录身份
        </Typography.Text>

        <div className="login-row login-row-main">{mainRoles.map(tile)}</div>
        <div className="login-row login-row-admin">{adminRole ? tile(adminRole) : null}</div>

        {passwordOpen && adminRole ? (
          <div className="login-password">
            <Typography.Text>
              以 <b>{adminRole.label}</b> 登录
            </Typography.Text>
            <div className="login-password-row">
              <Input.Password
                aria-label="Admin 密码"
                placeholder="请输入 Admin 密码"
                value={password}
                autoFocus
                onChange={(event) => setPassword(event.target.value)}
                onPressEnter={submitAdmin}
              />
              <Button
                type="primary"
                disabled={disabled || password.trim() === ""}
                onClick={submitAdmin}
              >
                登录
              </Button>
            </div>
          </div>
        ) : null}
      </div>
    </div>
  );
}
