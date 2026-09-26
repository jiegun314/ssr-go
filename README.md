# SSR —— UDI 数据整合平台（SingleSource Ready）

面向医疗器械 UDI 申报场景的**离线单机桌面工具**：把四类来源 Excel 导入本地 SQLite，按配置规则
整合成 SingleSource 导出模板，只导出可用的记录，并把每次导出写成可审计的操作日志。

- 完全离线：不联网、无账号、无遥测；
- 配置驱动：表名、字段、必填规则、匹配规则、去重规则、输出列、变更描述、日志列全部写在 `config/` 的
  YAML 里，**改规则不改代码**；
- 启动即校验：配置有任何问题程序拒绝启动，不会带着错误配置运行。

---

## 1. 技术架构

| 层 | 选型 |
| --- | --- |
| 桌面界面 | **Wails v2**（系统原生 WebView）+ 原生 JS/CSS 前端（Material Design） |
| 命令行 | 同一个 Go 模块下的 `cmd/ssr-core`（导入 / 整合 / 导出 / 快照 / 工具子命令） |
| Excel | `github.com/xuri/excelize/v2`（读输入、写模板副本，按文本写值，保留模板全部部件） |
| 数据库 | `database/sql` + `modernc.org/sqlite`（纯 Go，`CGO_ENABLED=0`；所有列均为 `TEXT`） |
| 配置 | `gopkg.in/yaml.v3`（四份 YAML） |
| 打包 | Wails 三平台原生构建；`ssr-core release` 组装发布目录与压缩包 |

**设计原则**：界面与命令行只做编排，业务规则集中在 `internal/`；配置是唯一的行为来源。

```
cmd/ssr-core/        无界面 CLI：import | consolidate | export | snapshot |
                     genlogcolumns | alignlogcolumns | gensample | preparedb | release
internal/config/     四份 YAML 的加载与整体校验（启动即校验）
internal/paths/      路径基准：配置目录 / 项目根 / 可执行文件同级
internal/rules/      单元格空值语义、required_when 条件求值
internal/store/      SQLite：通用仓库、来源表策略、80 列操作日志、启动清理、旧表对齐
internal/importer/   导入编排（归一 → 表头 → 必填 → 条件必填 → 写表）
internal/consolidation/  整合编排（匹配 / 拆行 / 合并 / 输出映射 / 导出前校验）
internal/changedetect/   变更检测（重复判定与变更描述共用一次比较）
internal/excelio/     excelize 封装：读工作表、写模板副本
internal/validation/ 整合前置检查（四张来源表必须存在且非空）
internal/buildinfo/  版本信息（构建期常量 → git → 配置回退）
internal/appicon/    图标（编进程序的资源优先，exe 同级文件回退）与 Windows 任务栏身份
frontend/dist/       前端页面（随二进制内嵌）
config/ data/template/ resource/   运行期配置、模板与图标资源
testdata/            样本工作簿与数据层黄金快照
```

**数据流**：

```
Excel 来源 ×4 → （配置校验：表头 / 必填 / 条件必填 / 值归一）→ SQLite 暂存表
      → 按 merge_rules 匹配 + 拆行 + 合并 + 输出列映射 → 结果表（status + 30 个导出列）
      → 与操作日志中同一身份的最新记录比较：无变化 = Duplicate（不重复导出）；
        有变化 = Ready 并自动生成 Change Description
      → 导出 Ready 行到模板副本（模板部件全保留）→ 写操作日志（按身份幂等）
```

| 结果状态 | 含义 | 是否导出 | 表格底色 |
| --- | --- | --- | --- |
| `Ready` | 数据完整（新增，或相对历史记录发生变更） | ✅ | 浅绿 |
| `Incomplete` | 来源缺失或必填输出列为空 | ❌ | 浅红 |
| `Duplicate` | 同一身份已存在且逐列一致 | ❌ | 浅黄 |
| `Conflict` | 同一来源内出现「去重键相同但内容不同」的记录 | ❌ | 浅粉 |

---

## 2. 主要功能

### 数据导入（四个来源）

- 四个来源：**UDI团队信息**、**RA信息**、**医保代码信息**、**产品类别**；每组有状态圆点、导入按钮、
  状态标签与「…」数据回顾按钮；导入区右下角是「清空导入数据」；
- 导入顺序固定：读 Excel → 值归一 → 表头校验 → 必填校验 → 条件必填校验 → 写暂存表；
  **任一校验失败整份文件不写入**；
- 表头缺失、必填列为空、条件必填列为空会明确报出行号与列名；缺值类失败的弹窗只给汇总，
  逐行明细写进操作日志；
- 导入按 `replace_on_import` 覆盖整表；「清空导入数据」只清来源（保留医保编码、整合结果与操作日志）。

### 数据整合

- 以 RA 信息为基准，按 `consolidation_mapping.yaml` 的 `merge_rules` 匹配：一对一、一对多（按 DI 拆行）、
  多对一（取一条）；
- 输出列支持：直接取值、首个非空、拼接（空值跳过）、固定文本、用户自定义值、变更描述（自动生成）；
- 导出前最后一道校验：必填列为空写 `MISSING` 并把该行降级为 `Incomplete`（不导出）；
- 结果卡片右上角工具条显示 `Ready / Incomplete / Duplicate` 计数（圆点标识），出现 `Conflict` 时才显示它。

### 变更检测

- 身份 = **产品代码（Catalog or Reference Number）+ Primary DI**；医保编码不属于身份；
- 与操作日志中同一身份的最新记录逐列比较：无变化 → `Duplicate`（不重复导出）；有变化 → 保持 `Ready`
  并生成 `Change Description`（如 `Product Name/Generic Name changed`、`Medical Insurance Code changed`）。

### 导出

- 只导出 `Ready` 行；没有可导出行时提示 `No data to export, please check the consolidation result.`；
- 先复制模板再写值（模板的说明行、格式、图片、打印设置等部件全部保留），单元格**按文本写入**（前导零不丢）；
- 文件名 = `{模板名}-{YYYYmmdd_HHMMSS}.xlsx`，先在 `output/export/` 留一份副本，再复制到用户选择的位置；
- 导出成功后写操作日志（身份相同且各列一致时不会重复写入）。

### 回顾与导出

- **数据回顾**：点来源的「…」查看该来源已导入的全部数据（中文表头），每页 100 行，可分页、可导出
  （默认名 `{来源键}_imported_data.xlsx`）；
- **记录导出**：按起止日期回顾操作日志（每页 100 行、可分页、可导出 `operation_log_imported_data.xlsx`），
  时长任务期间显示载入图层。

### 其它

- 「关于」窗口显示图标、名称、版本（悬停看 commit 与构建时间）；
- 命令行提供无界面能力：批量导入/整合/导出、生成日志列、旧库对齐、生成样本与空库、出发布包。

---

## 3. 安装与运行

### 3.1 直接使用发布包

发布包是目录型产物（macOS 为 `SingleSourceReady.app`，Windows 为 `ssr.exe`），解压即用：

```text
SingleSourceReady/
  SingleSourceReady.app / ssr.exe     程序
  config/                             四份 YAML（可现场编辑）
  data/template/*.xlsx                导出模板
  data/udi_data.sqlite3               空数据库（只含一张空 operation_log）
  output/export/                      每次导出的带时间戳副本
  resource/logo.ico                   图标回退
  VERSION                             版本 / commit / 构建时间
```

**必须整体使用**：`config/`、`data/`、`resource/` 要跟程序放在同一层（配置文件读的是程序同级那份）；
不要只把 `.app` 或 `ssr.exe` 单独挪走。macOS 产物未做 Apple 公证，首次打开若被拦截请右键 →「打开」。

### 3.2 源码构建

需要 **Go ≥ 1.25**；出桌面产物还需要 Wails CLI v2：

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest

go build ./...                        # 编译全部包
go test ./...                         # 运行测试
go run ./cmd/ssr-core version         # 命令行版本信息
wails build                           # 桌面产物：build/bin/SingleSourceReady.app
```

出发布包（会用 `wails build` 注入版本，并组装目录与压缩包）：

```bash
go run ./cmd/ssr-core release --out release          # 正式发布（HEAD 必须打过 v<版本> tag、工作区干净）
go run ./cmd/ssr-core release --allow-dirty --no-zip # 本地自测
```

三个平台各自原生构建（没有交叉编译）：macOS `wails build -platform darwin/universal`、
Windows `wails build -platform windows/amd64`、Linux `wails build -tags webkit2_41`。

### 3.3 命令行

| 子命令 | 用途 |
| --- | --- |
| `import --source <来源键> --file <xlsx>` | 导入一份来源 |
| `consolidate` | 按配置整合四份来源 |
| `export [--file <名字>]` | 把 Ready 行写进模板副本并记日志 |
| `snapshot --input <valid 目录> --out <目录>` | 产出数据层快照（供逐格比对） |
| `genlogcolumns [--check]` | 由模板表头生成 `config/log_columns.yaml`；`--check` 只校验 |
| `alignlogcolumns [--database <db>] [--no-backup]` | 把旧库 `operation_log` 对齐到当前列（先备份、失败回滚） |
| `gensample [--rows N] [--output <目录>]` | 生成本地样本 Excel |
| `preparedb --output-root <目录>` | 生成发布包用的空数据库 |
| `release [--out release] [--skip-build] [--allow-dirty] [--no-zip]` | 发布构建 |

### 3.4 路径与开关

- `project_root` = 配置目录的父目录；配置里的相对路径都相对它解析；
- `UDI_CONFIG_DIR`：指定配置目录（会同时改变数据库、模板与导出目录的基准）；
- `SSR_CLEANUP_ON_STARTUP`：一次运行内覆盖启动清理开关（`true/false`、`1/0`、`yes/no`、`on/off`，
  大小写不敏感；写错直接报配置错误）。

---

## 4. 注意事项

**数据与规则**

1. **`NA` / `N/A` 是有效值**，不是空值：写在必填列上不会被拦下，会原样入库并导出；真正算空的是
   "没写、只有空格或不可见字符"的单元格；
2. **值归一不会清空已填内容**：把某种写法映射成空值时保留原文，避免"用户写了值却被判为空"；
3. **整行空不是数据行**：不触发必填校验，也不写进暂存表；
4. **医保编码不属于身份字段**：编码变化是同一产品的变更（结果保持 `Ready` 并带
   `Medical Insurance Code changed`），不会变成第二条记录；
5. **一个产品可能拆成多行**：`Primary DI` 来自 UDI 来源的一对多拆行；
6. **`Conflict` 是数据自身矛盾**：同一来源里"去重键相同但内容不同"的记录需要你回来源修数据。

**运行行为**

7. **重启不清空导入数据（默认）**：`setting.yaml` 的 `startup.cleanup_on_startup` 默认是
   `false`，四张来源表（含医保编码）与整合暂存表都跨会话保留；需要"每次启动都从空表开始"时，
   把它改成 `true`，或用 `SSR_CLEANUP_ON_STARTUP=true` 覆盖一次运行；
8. **「清空导入数据」范围更窄**：只清来源表，医保编码、整合结果与操作日志都不动；
9. **导出会在 `output/export/` 留副本**：这是刻意设计，长期使用可定期清理该目录；
10. **重复导出的行不会再次导出**：已一致的记录是 `Duplicate`；需要重发历史数据时只能改来源数据或清理
    `operation_log`（请先备份数据库）；
11. **日志只在导出成功后写入**：只做整合不产生日志记录。

**配置与维护**

12. **`config/` 是唯一行为来源**：表名、字段、规则都在 YAML 里；`setting.yaml` 的
    `startup.cleanup_on_startup` 默认 `false`（重启保留数据），可按现场需要改成 `true`；
13. **`config/log_columns.yaml` 是生成物**：不要手改，改模板或 `setting.yaml` 后运行
    `genlogcolumns` 重新生成（`--check` 可用于流水线守卫）；
14. **`operation_log` 不会自动迁移**：升级后需要对齐旧库时运行 `alignlogcolumns`（自动备份）；
15. **版本号来自 git tag**：构建期写入程序（exe 版本资源 / macOS `Info.plist` / `VERSION` / 关于窗口
    四处一致）；`setting.yaml` 的 `APP_VERSION` 只是回退值；
16. **界面固定亮色**，不跟随系统深色模式；导出目录、日志表列顺序等均按配置与模板决定。
