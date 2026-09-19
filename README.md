# SSR（SingleSource Ready）—— Go + Wails v2 版

把四份来源 Excel（RA 信息 / UDI 团队信息 / 医保编码信息 / 产品类别）导入本地 SQLite，按
`config/` 的 YAML 规则整合成 SingleSource 导出模板，并把导出的记录写成可审计操作日志的
**离线单机桌面工具**。本仓库是原 Python + PySide6 实现的 Go 重写（Wails v2 + excelize）。

原实现保留在 `/Users/zhouhui/Documents/Projects/ssr_go`，它是**行为规格与 golden baseline
生成器**，不在这里改动。

## 安装 / 构建

需要 Go ≥ 1.25；出桌面产物还需要 Wails CLI v2（`go install github.com/wailsapp/wails/v2/cmd/wails@latest`）。

```bash
go build ./...                    # 全部包
go test ./...                     # 155 个用例
go run ./cmd/ssr-core version     # 命令行
wails build                       # 桌面产物：build/bin/SingleSourceReady.app
```

发布构建（没有交叉编译，见 AGENTS.md §7.6）：三个平台各自原生构建 —— macOS
`wails build -platform darwin/universal`、Windows `wails build -platform windows/amd64`、
Linux `wails build -tags webkit2_41`。

## 使用

### 桌面界面

双击产物（macOS 是 `SingleSourceReady.app`）。界面结构与原版一致：左栏「数据导入」（四组：
UDI团队信息 / RA信息 / 医保代码信息 / 产品类别，每组有状态圆点、导入按钮、状态标签、
「…」数据回顾按钮，下面一行右对齐的是「清空导入数据」）与「记录导出」（起止日期 + 回顾）；
右栏「数据整合」（数据整合 / 生成文件）与「整合结果」（大表格，按状态着色、`MISSING` 加粗）；
底部整宽「操作日志」。菜单：`文件（打开 / 退出）`、`设置`、`关于`（关于窗口的图标 5 秒内
连点 8 次会打开彩蛋）。

### 命令行（`cmd/ssr-core`）

| 子命令 | 用途 |
| --- | --- |
| `version` | 打印版本（构建期常量 → git → `setting.yaml` 的 `APP_VERSION` 回退链） |
| `import --source <来源键> --file <xlsx>` | 导入一份来源（校验失败整份不写库） |
| `consolidate` | 按配置整合四份来源并写 `consolidation_staging` |
| `export [--file <名字>]` | 把 `Ready` 行写进模板副本，随后记操作日志 |
| `snapshot --input <valid 目录> --out <目录>` | 产出可逐格比对的行为快照（导入/整合/导出/幂等/变更/拒绝路径） |
| `genlogcolumns [--check]` | 由模板表头生成 `config/log_columns.yaml`；`--check` 只校验 |
| `alignlogcolumns [--database <db>] [--no-backup]` | 把旧库的 `operation_log` 对齐到当前列（先备份、失败回滚） |
| `gensample [--rows N] [--output <目录>]` | 生成本地样本 Excel（`valid/`、`invalid-conditions/`） |
| `preparedb --output-root <目录>` | 生成发布包用的空数据库（只含空的 `operation_log`） |

所有子命令都接受 `--config <目录>`，等价于环境变量 `UDI_CONFIG_DIR`（见下）。

### 配置与路径

行为**全部**来自 `config/` 的四份 YAML：`setting.yaml`（路径、表名、模板、启动策略）、
`excel_import_mapping.yaml`（来源、列、必填与条件必填、值归一）、
`consolidation_mapping.yaml`（匹配、拆行、去重、输出列、变更描述）、
`log_columns.yaml`（**生成物**，由 `genlogcolumns` 从模板表头生成，不要手改）。

路径基准：`project_root` = 配置目录的父目录；配置里的相对路径都相对它。设置
`UDI_CONFIG_DIR` 会同时改变数据库、模板与导出目录的解析基准。打包后配置随可执行文件同级
目录走，所以现场改配置/换模板**不需要重新打包**。

`SSR_CLEANUP_ON_STARTUP` 可以在一次运行里覆盖 `startup.cleanup_on_startup`
（`true/false`、`1/0`、`yes/no`、`on/off`，大小写不敏感；写错直接报配置错误）——发布包
必须保持 `true`，本地调试才用环境变量临时关掉启动清理。

## 发布包结构

```text
SingleSourceReady/
  ssr.exe (或 macOS 的 .app、Linux 可执行文件)
  config/                 四份 YAML（现场可改）
  data/template/*.xlsx    导出模板
  data/udi_data.sqlite3   空数据库（由 preparedb 生成，只有空的 operation_log）
  resource/logo.ico       exe 同级图标（回退用；主来源是编进程序的资源）
  output/export/          每次导出的带时间戳副本
  VERSION                 版本 / commit / 构建时间
```

## 迁移说明与对比报告

- 从 Python 版迁移（数据怎么带走、旧库怎么对齐）：[docs/迁移说明.md](docs/迁移说明.md)
- 现状 vs 新实现的对比（golden diff、性能、已知差异）：[docs/对比报告.md](docs/对比报告.md)

## 权威文档在哪里

| 内容 | 位置 |
| --- | --- |
| 决策、验收口径、目标架构、禁止事项（R1–R27） | `ssr_go/AGENTS.md`（§2 规则、§9 验收口径、§10.1 已拍板决策、§11 验收清单） |
| 现有行为的细节规格 | `ssr_go/README.md` |
| 行为的数据来源 | `ssr_go/config/*.yaml`（本仓库 `config/` 是随发布包走的副本） |
| golden baseline（必须逐字节对齐） | `ssr_go/baseline/`（由 `ssr_go/scripts/snapshot_baseline.py` 生成） |
| 性能记录 | `ssr_go/perf-baseline/` |
| 重写对照表 / 决策表 / 现状证据 | `ssr_go/docs/rewrite/` |

## 硬约束（摘自 AGENTS.md）

- **不要发明新行为**，也不要「顺手」实现配置里那些声明但未生效的键（§3.6 / §10.1）；
- 导出验收口径：**数据口径① + 文件口径②**（每个数据单元格文本与现状一致；模板部件必须保留）；
- 所有表列都是 `TEXT`，读出来一律当字符串；
- 界面文案、日志文案、环境变量名逐字复刻；
- 没有 golden diff 通过之前，不声称任何一步完成。

## 当前进度

| 模块 | 状态 |
| --- | --- |
| `internal/rules`（R1 空值语义、R6 条件规则） | 已实现 + 单测 |
| `internal/buildinfo`（R26 版本与构建信息） | 已实现 + 单测 |
| `internal/paths`（§4.3 路径基准） | 已实现 |
| `internal/config`（R24 配置整体校验） | 已实现 + 单测（setting / import / consolidation / log columns 四份配置全量校验） |
| `internal/store`（R8/R9/R22/R23/R25 的 SQL 层） | 已实现 + 单测（通用仓库、来源表导入策略、启动清理、80 列操作日志与按身份幂等写入） |
| `internal/excelio`（读取侧） | 已实现（只读第一个工作表、单元格一律按文本、尾部空行裁剪） |
| `internal/importer`（R2–R8） | 已实现 + 单测，**导入阶段 golden diff 通过**（Go 暂存表与 Python 现状逐格一致） |
| `internal/validation`（R10） | 已实现（四张来源表的存在性与非空检查、缺表/空表文案） |
| `internal/changedetect`（R17/R18） | 已实现（身份命中判定、按存储真实列比较、auto/source 两种描述模式） |
| `internal/consolidation`（R11–R16/R19） | 已实现 + 单测，**整合阶段 golden diff 通过**（9 行结果逐格一致） |
| `internal/excelio` 导出侧（R20/R21 + 模板保真） | 已实现 + 单测（按文本写值、模板部件全保留、缺列上报、无 Ready 行时报原文） |
| `cmd/ssr-core` | 8 个子命令全部实现：`version` / `import` / `consolidate` / `export` / `snapshot` / `genlogcolumns`（含 `--check`）/ `alignlogcolumns`（含 `--no-backup`）/ `gensample` / `preparedb` |
| Wails 界面 | 待做 |

`gensample` 让 Go 侧**不再依赖 Python 生成样本**：用 Go 生成的 `valid/` 与
`invalid-conditions/` 跑完整链路，12 个行为快照仍与 Python baseline 逐字节一致。

`config/` 与 Python 仓库**逐字节一致**（含生成物的注释），所以 `genlogcolumns` 生成出来的
`config/log_columns.yaml` 与 Python 版生成物完全相同；`genlogcolumns --check` 可直接当
CI 守卫用（模板表头变了、忘了重新生成就会失败）。Go 的等价命令是
`go run ./cmd/ssr-core genlogcolumns`。

### 与 Python 现状的逐格比对（数据口径①）

```bash
go run ./cmd/ssr-core snapshot --config <工作区>/config \
    --input <工作区>/data/input/sample/valid --out baseline-go
```

用 Python 侧 `ssr_go/baseline/` 作对照物时，以下文件**逐字节一致**：
四个 `来源表_*.tsv`、`整合结果.tsv`、`计数断言.json`、`导出文件_逐格.tsv`、`操作日志.json`、
`幂等路径.json`、`变更路径.json`、`拒绝路径.json`、`拒绝路径.txt`（共 12 个）。

唯一有意不同的文件是 `导出文件_部件清单.tsv`：openpyxl 会丢 24 个部件（`docMetadata/LabelInfo.xml`
敏感度标签、两张 png、drawings、printerSettings、customXml），excelize 全部保留 ——
这就是验收口径②（模板保真）的落地证据，见 AGENTS.md §10.1 #2。

`snapshot` 覆盖三条路径：幂等（再整合一次全部 Duplicate、不新增日志、无可导出行）、
变更（改字段 → `{列名} changed`、改医保编码 → 同一身份换描述）、拒绝
（`invalid-conditions/` 的 UDI 文件被拒，行号 13/14 与命中条件逐字记录，暂存表不受影响）。
`--input` 指向 `valid/` 目录，命令会自动在同一父目录下找 `invalid-conditions/`。

> 说明：一次 `snapshot` 会导入 → 整合 → 导出 → 记日志，所以在同一份数据库上重跑会得到
> 「全部 Duplicate、无可导出行」。要比对请用干净工作区（与 Python 快照脚本同口径）。

依赖：`gopkg.in/yaml.v3`（配置解析）、`modernc.org/sqlite`（纯 Go SQLite，`CGO_ENABLED=0`）。
后续步骤会加入 `github.com/xuri/excelize/v2`。

## 构建与测试

```bash
go build ./...
go test ./...
go run ./cmd/ssr-core version
go run ./cmd/ssr-core --help
```

沙箱/CI 里如果 `GOCACHE` 不可写，用：

```bash
GOCACHE=/private/tmp/go-cache GOPATH=/private/tmp/go-path go test ./...
```

发布构建（后续步骤）：`wails build` 三个平台各自原生构建（没有交叉编译，见 AGENTS.md §7.6）。
