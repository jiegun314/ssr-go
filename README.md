# SSR（SingleSource Ready）—— Go + Wails v2 重写

本仓库是 UDI 数据整合工具的 **Go + Wails v2 + excelize 重写**。原 Python + PySide6 实现
保留在 `/Users/zhouhui/Documents/Projects/ssr_go`，它是**行为规格与 baseline 生成器**，
不在这里改动。

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
| `cmd/ssr-core` | `version` / `import` / `consolidate` / `export` / `snapshot` 已实现；`genlogcolumns` / `alignlogcolumns` / `gensample` / `preparedb` 待做 |
| Wails 界面 | 待做 |

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
