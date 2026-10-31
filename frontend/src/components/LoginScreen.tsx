// 登录页（P1）：选择身份进入；Admin 需要密码，其余角色点一下即可。
//
// 口径（docs/design/multiuser-workflow.md §5）：
//   - 身份是声明的：不读域信息，登录后只记录本机用户名；
//   - 单机模式（没有 users.yaml）不会走到这里 —— 后端启动时已自动以 Admin 进入。

import { Button, Card, Input, Typography } from "antd";
import { LogIn, ShieldCheck, UserRound } from "lucide-react";
import { useMemo, useState } from "react";

import type { LoginUser } from "../types";

type LoginScreenProps = {
  users: LoginUser[];
  /** 选中身份后的回调：非 Admin 的 password 传空串 */
  onLogin: (name: string, password: string) => void;
  busy?: string | null;
};

/** 按登录页的显示顺序把人员分组（同一角色的人排在一起）。 */
function groupByRole(users: LoginUser[]): { roleLabel: string; users: LoginUser[] }[] {
  const groups: { roleLabel: string; users: LoginUser[] }[] = [];
  for (const user of users) {
    const group = groups.find((item) => item.roleLabel === user.roleLabel);
    if (group) {
      group.users.push(user);
    } else {
      groups.push({ roleLabel: user.roleLabel, users: [user] });
    }
  }
  return groups;
}

export function LoginScreen({ users, onLogin, busy }: LoginScreenProps) {
  const groups = useMemo(() => groupByRole(users), [users]);
  // 点了 Admin 之后才显示密码框；其余角色点一下直接进
  const [admin, setAdmin] = useState<LoginUser | null>(null);
  const [password, setPassword] = useState("");
  const disabled = Boolean(busy);

  const choose = (user: LoginUser) => {
    if (user.needsPassword) {
      setAdmin(user);
      setPassword("");
      return;
    }
    setAdmin(null);
    onLogin(user.name, "");
  };

  const submitAdmin = () => {
    if (!admin || disabled) return;
    onLogin(admin.name, password);
  };

  return (
    <div className="login-screen">
      <Card className="login-card" size="small">
        <div className="login-head">
          <div className="login-title">SSR - UDI数据整合平台</div>
          <Typography.Text type="secondary">
            请选择登录身份（Admin 需要密码；其余身份点一下即可）
          </Typography.Text>
        </div>

        {groups.map((group) => (
          <div className="login-group" key={group.roleLabel}>
            <div className="login-group-title">{group.roleLabel}</div>
            <div className="login-group-users">
              {group.users.map((user) => (
                <Button
                  key={user.name}
                  className={`login-user${admin?.name === user.name ? " login-user-active" : ""}`}
                  disabled={disabled}
                  icon={user.needsPassword ? <ShieldCheck size={16} /> : <UserRound size={16} />}
                  onClick={() => choose(user)}
                >
                  <span className="login-user-name">{user.name}</span>
                  {user.note ? <span className="login-user-note">{user.note}</span> : null}
                </Button>
              ))}
            </div>
          </div>
        ))}

        {admin ? (
          <div className="login-password">
            <Typography.Text>
              以 <b>{admin.name}</b>（Admin）登录
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
                icon={<LogIn size={16} />}
                disabled={disabled || password.trim() === ""}
                onClick={submitAdmin}
              >
                登录
              </Button>
            </div>
          </div>
        ) : null}
      </Card>
    </div>
  );
}
