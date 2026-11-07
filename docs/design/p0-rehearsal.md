# P0 演练环境：本机假共享盘 + 多实例

> 本文件是 `docs/design/multiuser-workflow.md` 的实施前准备（P0）。
> 目的：**不碰真实共享盘**，在一台电脑上把"多部门 → 发布 → 待整合 → 对齐 → 整合 → 异常"整条链路跑通。
> 本阶段只产出脚手架（脚本 + 样例配置 + 本文），不改程序行为、不打包新版本。

## 1. 产物

| 文件 | 作用 |
| --- | --- |
| `scripts/ssr-share-init.sh` | 建"假共享盘"骨架：`published/{ra,eama,udi}`、`state/state.json`、`consolidated`、`audit`、`README.txt` |
| `scripts/ssr-share-launch.sh` | 为每个角色准备独立实例目录（配置 + 数据 + 模板），`workflow.yaml` 的 `share.root` 指向假共享盘，可选直接启动 |
| `config/workflow.yaml.example` | 流程配置样例（`share.root`、`sync`、`consolidation`、`recipients`、`email` 占位） |
| `config/users.yaml.example` | 人员与角色样例（固定组别 RA / EAMA / UDI / SUPPLY CHAIN / ADMIN） |

## 2. 快速开始（macOS / Linux）

```bash
# 1) 建假共享盘（绝对路径）
./scripts/ssr-share-init.sh --root /tmp/ssr-share

# 2) 只准备实例目录（不启动）
./scripts/ssr-share-launch.sh --share /tmp/ssr-share --roles ra,sc,admin --prepare-only

# 3) 准备并启动（GUI 会各开一个窗口）
./scripts/ssr-share-launch.sh --share /tmp/ssr-share --roles ra,sc

# 关闭
kill $(cat /tmp/ssr-rehearsal/pids)
```

每个实例互相隔离：

```
/tmp/ssr-rehearsal/<role>/config/   四份业务配置 + workflow.yaml + users.yaml
/tmp/ssr-rehearsal/<role>/data/     udi_data.sqlite3、template、output（程序按 config 的父目录解析）
```

启动方式等价于（可手工执行）：

```bash
UDI_CONFIG_DIR='/tmp/ssr-rehearsal/ra/config' \
  'release/SingleSourceReady/SingleSourceReady.app/Contents/MacOS/ssr' &
```

## 3. 现在就能验证的内容（程序尚未实现共享盘功能）

| 检查 | 期望 |
| --- | --- |
| 假共享盘骨架 | `published/{ra,eama,udi}`、`state/state.json`、`consolidated`、`audit` 都存在 |
| `state.json` | 合法 JSON，`generation = 0`，四个来源键都在（值为 null） |
| 实例隔离 | 每个实例有自己的 `data/udi_data.sqlite3`、`config/.backup/`、`output/` |
| 配置生成 | `workflow.yaml` 的 `share.root` 指向假共享盘；`users.yaml` 含 5 个角色 |
| 模板就位 | 每个实例的 `data/template/SingleSource-DCF_Column-China-UDI.xlsx` 存在（导出要用） |
| 不影响真实运行包 | `release/SingleSourceReady/config`、`data/` 未被修改（脚本只读运行包、只写 /tmp） |

## 4. 后续阶段在这套环境上的验收清单

| 阶段 | 在演练环境里做什么 | 通过标准 |
| --- | --- | --- |
| P1 登录与权限 | 各实例登录不同角色 | 看不到别人的明细；越权动作被后端拒绝；审计记录角色 + 用户名 + 机器 |
| P2 发布与状态 | RA/EAMA/UDI 实例各导入并发布 | `published/<部门>/<version>.csv + manifest.json` 出现；`state.json` 的版本与发布人正确；`state.lock` 无残留 |
| P3 对齐与徽标 | SC 实例观察共享盘 | ≤30 秒出现"有更新"；差异清单显示 旧→新版本/发布人/时间/行数变化；对齐后徽标消失 |
| P4 整合与门禁 | SC 未对齐时点整合 → 对齐后再点 | 未对齐时按钮置灰且后端拒绝；对齐后可整合；结果写 `consolidated/<id>/` 与 `last_consolidation`（含四个版本） |
| P7 邮件（延后） | 用 outbox 模拟 | 发布通知落 outbox；整合附件落 outbox；同版本不重复 |

## 5. 故障演练

| 场景 | 做法 | 期望 |
| --- | --- | --- |
| 共享盘不可达 | 把 `/tmp/ssr-share` 改名 | 启动/整合前检查失败 → 进入只读锁定，写明原因 |
| 恢复 | 改回原名，点「重新检查」 | 解锁，操作恢复 |
| 只有读权限 | `chmod -R a-w /tmp/ssr-share`（测完改回） | 探针写失败 → 判定不可写（P2 起验证） |
| 版本损坏 | 手工改 `published/ra/<version>.csv` 的内容 | 对齐时 sha256 不符 → 中止并提示"版本损坏，请重新发布" |
| 整合互斥 | SC 实例整合时，另一实例再点整合 | 只放行一个，另一个提示"谁正在整合" |
| 发布中断 | 发布过程中断网/杀进程 | 共享盘无半份文件、`state.json` 不变 |

## 6. 决策记录（P0 需要确认的事项）

| 项 | 当前默认 | 状态 |
| --- | --- | --- |
| 真实共享盘路径 | 待定（UNC） | ⏳ 待确认 |
| 人员名单与角色 | 见 `config/users.yaml.example`（5 人） | ⏳ 待确认 |
| Admin 密码保管人 | 待定（密码写死在程序里，改密码需发新版） | ⏳ 待确认 |
| SC 自动对齐 | 默认开（可在参数设定关闭） | ✅ 默认（可改） |
| 差异粒度 | 版本级 + 行数变化（逐行 diff 不做） | ✅ 默认（可改） |
| 邮件通道 | 延后（P7），先 `disabled` | ⏳ 待确认 |

## 7. 注意事项

- 一台电脑上所有实例的**操作系统用户名相同**，审计里会显示"同一用户名、不同角色"——演练可接受；
  如需更真实，可在后续测试构建里加 `SSR_ACTOR_USERNAME` 覆盖（仅测试用）。
- 脚本只写 `/tmp`，**不会**改动 `release/SingleSourceReady`（配置与数据库）；删除演练环境：
  `rm -rf /tmp/ssr-share /tmp/ssr-rehearsal`。
- Windows 版脚本（PowerShell）随 P1 提供；目录结构、`workflow.yaml`/`users.yaml` 完全一致，
  用 `set UDI_CONFIG_DIR=...` 启动同一份 exe 即可。
