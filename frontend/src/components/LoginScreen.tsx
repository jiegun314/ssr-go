// 登录页（P1）：按**组别**选择，不显示具体人名。
//
// 布局（用户口径）：
//   - 左侧品牌区：Logo、产品名、产品说明和“导入 / 对齐 / 整合”三步；
//   - 右侧：Welcome, <本机用户名> + 2×2 组别卡；管理员入口单独放在分隔线下方，
//     点它才出现密码框；
//   - 除 Admin 外点一下即进入（不读域信息，只记录本机用户名）。

import { Alert, Button, Input, Radio, Typography } from "antd";
import {
  Barcode,
  ChevronRight,
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
        className="login-tile"
        aria-label={role.label}
        disabled={disabled}
        onClick={() => choose(role)}
      >
        <span className="login-tile-icon" aria-hidden="true">
          <Icon size={22} strokeWidth={1.7} />
        </span>
        <span className="login-tile-copy">
          <span className="login-tile-label">{role.label}</span>
          {role.hint ? <span className="login-tile-hint">{role.hint}</span> : null}
        </span>
      </Button>
    );
  };

  return (
    <div className="login-screen">
      <div className="login-panel">
        <aside className="login-brand">
          <div className="login-brand-head">
            <img className="login-logo" src="/logo.png" alt="SSR" />
            <div>
              <div className="login-brand-name">SingleSourceReady</div>
              <div className="login-brand-sub">SSR · UDI 数据整合平台</div>
            </div>
          </div>
          <h1 className="login-brand-title">把来源数据，收成一份可提交文件。</h1>
          <p className="login-brand-desc">
            RA、EAMA、UDI 与产品类别在同一个工作区汇合，减少重复核对与手工拼接。
          </p>
          <div className="login-brand-steps" aria-label="工作流程">
            <span className="login-brand-step"><b>01</b>导入来源</span>
            <span className="login-brand-step"><b>02</b>对齐版本</span>
            <span className="login-brand-step"><b>03</b>整合导出</span>
          </div>
        </aside>

        <section className="login-access">
          <div className="login-access-head">
            <div className="login-welcome">Welcome, {username}</div>
            <Typography.Text type="secondary" className="login-subtitle">
              请选择登录身份
            </Typography.Text>
          </div>

          <div className="login-role-grid">{mainRoles.map(tile)}</div>

          {adminRole ? (
            <div className="login-admin">
              <span className="login-admin-label">管理员入口</span>
              <Button
                type="text"
                className="login-admin-entry"
                aria-label={adminRole.label}
                disabled={disabled}
                onClick={() => choose(adminRole)}
              >
                <span className="login-admin-icon" aria-hidden="true">
                  <ShieldCheck size={18} strokeWidth={1.7} />
                </span>
                <span className="login-admin-copy">
                  <span className="login-admin-name">{adminRole.label}</span>
                  <span className="login-admin-hint">{adminRole.hint ?? "需要密码"}</span>
                </span>
                <ChevronRight size={16} aria-hidden="true" />
              </Button>
            </div>
          ) : null}

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
        </section>
      </div>
    </div>
  );
}
