// 登录页（P1）：按**组别**选择，不显示具体人名。
//
// 布局（用户口径）：
//   - 顶部 Welcome, <本机用户名>；
//   - 上面一行四个方块：图标在上、组别在下（RA / EAMA / Global UDI Team / Supply Chain）；
//   - 下面一行居中一个：管理员（图标 + 组别），点它才出现密码框；
//   - 除 Admin 外点一下即进入（不读域信息，只记录本机用户名）。

import { Alert, Button, Input, Radio, Typography } from "antd";
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
  /** Admin 登录：密码 + 运行模式（默认联网；本地模式需要先确认警告） */
  onAdminLogin?: (password: string, mode: "online" | "local") => void;
  busy?: string | null;
};

type AdminStep = "password" | "mode" | "warning";

export function LoginScreen({ roles, username, onLogin, onAdminLogin, busy }: LoginScreenProps) {
  const [passwordOpen, setPasswordOpen] = useState(false);
  const [password, setPassword] = useState("");
  // 管理员登录三步：密码 → 选择运行模式（默认联网） → （本地模式）警告确认
  const [adminStep, setAdminStep] = useState<AdminStep>("password");
  const [mode, setMode] = useState<"online" | "local">("online");
  const disabled = Boolean(busy);
  const mainRoles = roles.filter((role) => !role.needsPassword);
  const adminRole = roles.find((role) => role.needsPassword);

  const choose = (role: LoginRole) => {
    if (role.needsPassword) {
      setPasswordOpen(true);
      setPassword("");
      setAdminStep("password");
      setMode("online");
      return;
    }
    setPasswordOpen(false);
    onLogin(role.key, "");
  };

  const finishAdminLogin = (selected: "online" | "local") => {
    if (!adminRole) return;
    if (onAdminLogin) {
      onAdminLogin(password, selected);
      return;
    }
    // 兼容没有提供 onAdminLogin 的调用方（如纯界面预览）
    onLogin(adminRole.key, password);
  };

  const submitAdmin = () => {
    if (!adminRole || disabled || password.trim() === "") return;
    // 先选运行模式；本地模式还要再确认一次警告
    setAdminStep("mode");
  };

  const continueWithMode = () => {
    if (mode === "local") {
      setAdminStep("warning");
      return;
    }
    finishAdminLogin("online");
  };

  // 警告里的"否"：退回到登录用户选择界面（清空密码与模式）
  const cancelLocalMode = () => {
    setPasswordOpen(false);
    setAdminStep("password");
    setPassword("");
    setMode("online");
  };

  const tile = (role: LoginRole) => {
    const Icon = ROLE_ICONS[role.key] ?? ClipboardList;
    return (
      <Button
        key={role.key}
        className={`login-tile${role.needsPassword ? " login-tile-admin" : ""}`}
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
        <img className="login-logo" src="/logo.png" alt="SSR" />
        <div className="login-welcome">Welcome, {username}</div>
        <Typography.Text type="secondary" className="login-subtitle">
          请选择登录身份
        </Typography.Text>

        <div className="login-row login-row-main">{mainRoles.map(tile)}</div>
        <div className="login-row login-row-admin">{adminRole ? tile(adminRole) : null}</div>

        {passwordOpen && adminRole ? (
          <div className="login-password">
            {adminStep === "password" ? (
              <>
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
              </>
            ) : adminStep === "mode" ? (
              <>
                <Typography.Text>
                  以 <b>{adminRole.label}</b> 登录 · 选择运行模式
                </Typography.Text>
                <Radio.Group
                  className="login-mode-options"
                  value={mode}
                  onChange={(event) => setMode(event.target.value as "online" | "local")}
                >
                  <Radio value="online">联网模式（默认）</Radio>
                  <Radio value="local">本地模式</Radio>
                </Radio.Group>
                <Typography.Text type="secondary" className="login-mode-hint">
                  {mode === "online"
                    ? "使用共享盘：导入即发布；Supply Chain 对齐后整合。"
                    : "不连接共享盘：只用本机数据，适合离线排查或单机使用。"}
                </Typography.Text>
                <div className="login-password-row">
                  <Button disabled={disabled} onClick={() => setAdminStep("password")}>
                    返回
                  </Button>
                  <Button type="primary" disabled={disabled} onClick={continueWithMode}>
                    继续
                  </Button>
                </div>
              </>
            ) : (
              <>
                <Alert
                  className="login-mode-warning"
                  type="warning"
                  showIcon
                  message="本地模式不会使用共享盘"
                  description="不会发布到共享盘、不会对齐，整合只使用本机数据。确认当前只在本机处理时再进入。"
                />
                <div className="login-password-row">
                  <Button disabled={disabled} onClick={cancelLocalMode}>
                    否（返回身份选择）
                  </Button>
                  <Button danger type="primary" disabled={disabled} onClick={() => finishAdminLogin("local")}>
                    确认进入本地模式
                  </Button>
                </div>
              </>
            )}
          </div>
        ) : null}
      </div>
    </div>
  );
}
