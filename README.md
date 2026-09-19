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
| `cmd/ssr-core` 子命令骨架（8 个子命令） | 骨架（除 `version` 外均为占位） |
| `internal/config`（R24 配置校验） | 待做（需要 `gopkg.in/yaml.v3`） |
| `internal/store` / `importer` / `consolidation` / `changedetect` / `excelio` | 待做 |
| Wails 界面 | 待做 |

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
