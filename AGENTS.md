# AGENTS.md —— 本仓库的工程约定

面向在本仓库动手的人（也包括 AI 编码助手）。这里只写**可验证**的约定：每条都能在代码、配置或测试里找到依据。
业务功能的完整说明在 [README.md](README.md)，已发布版本的变更在 [CHANGELOG.md](CHANGELOG.md)。

## 1. 这是什么

- 桌面应用：**Go 1.25 + Wails v2 + React 18 + antd 5**，数据库是 SQLite（`modernc.org/sqlite`，纯 Go，无 CGO）。
- 两个二进制：
  - `ssr` —— 图形界面（名字来自 `wails.json` 的 `outputfilename`，产物为 `SingleSourceReady.app`）；
  - `ssr-core` —— 命令行（导入 / 整合 / 导出 / 样本工厂 / 发布打包 / 版本信息）。
- 界面文案一律中文；配置文件的注释保留英文（历史原因，见 §10）。
- 不做 Linux 桌面版：三个平台各自原生构建，没有交叉编译。

## 2. 目录职责

| 位置 | 职责 |
| --- | --- |
| `main.go` | 桌面入口：Wails 选项、窗口尺寸、资源与任务栏身份 |
| `app.go` / `app_methods.go` | `App` 类型与**前端可调用的绑定方法**（`window.go.main.App.*`）；`wireConfiguration` 负责装配依赖（启动与热重载共用） |
| `settings*.go` | 参数设定：树/表单读取、结构化行级写回、热重载 |
| `menu_actions.go` | 菜单动作（打开参数设定、关于、快照目录等） |
| `*_test.go`（根） | 集成与**跨语言契约**测试（前端源码文本、设计口径、发布包结构） |
| `internal/config` | 四份配置的读取、校验、路径解析（**行为来源**，不要在这里改口径） |
| `internal/rules` | 必填与条件必填（`required_when`）的判定 |
| `internal/importer` | Excel → 暂存表；`internal/store` SQLite 访问（暂存/整合/日志/对齐） |
| `internal/consolidation` / `internal/changedetect` | 整合流程与变更描述 |
| `internal/excelio` / `internal/sample` / `internal/validation` | Excel 读写、样本工厂、整合前的前置检查 |
| `internal/fileutil` / `internal/configsnapshot` | 文件复制（流式 + fsync）、启动时的配置快照与滚动清理 |
| `internal/buildinfo` / `internal/paths` / `internal/numfmt` / `internal/appicon` | 版本、路径解析、千分位、图标 |
| `cmd/ssr-core` | 命令行入口；`tools.go` 里的生成器常量与 `config/log_columns.yaml` 保持逐字节一致 |
| `scripts/` / `Makefile` | 本地发布、界面预览脚本；常用任务的固定入口（`make help`） |
| `frontend/` | React + antd 前端；`frontend/dist` 与 `frontend/wailsjs` **都提交入库**（前者供 `//go:embed`，后者供 TS 编译） |
| `config/` | 四份 YAML（同时是发布包 `config/defaults/` 的来源） |
| `testdata/` | 测试用样本工作簿与 golden 基线（`.tsv`） |
| `docs/archive/` | 迁移期（Python + PySide6 原型）对照材料，仅历史存档 |

## 3. 配置口径（最重要）

- **四份 YAML 是行为来源**：`setting.yaml`、`excel_import_mapping.yaml`、`consolidation_mapping.yaml`、`log_columns.yaml`。
  改行为优先改配置，不是改代码。
- **升级不覆盖用户配置**：正式名文件缺失时，从 `config/defaults/*.yaml` 生成一份；发布包解压时**从不覆盖**已存在的
  `config/*.yaml`、`config/.backup/`、`data/` 下的数据库。
- `config/log_columns.yaml` 是**生成物**：头部注释与内容由 `cmd/ssr-core/tools.go` 的 `logColumnsComment` /
  `RenderLogColumns` 产出，两者必须逐字节一致 —— CI 用 `genlogcolumns --check` 把关。
- 「参数设定」的结构化写回（`settings_write.go`）**只改被编辑的那几行**：注释、空行、键序、引号风格、锚点与别名都不动；
  先备份 `.bak`，再整体校验，失败自动还原。

## 4. 构建、测试、发布

```bash
go build ./...                                        # 编译全部包
go test -count=1 ./...                                # 全部测试（提交前必跑）
go run ./cmd/ssr-core genlogcolumns --check --config config   # 日志列生成物不许漂移
go run ./cmd/ssr-core version                         # 版本信息

npm ci --prefix frontend && npm run build             # 前端（build 里含 tsc --noEmit）
wails build                                           # 桌面产物（版本由 git tag / ldflags 注入）
wails dev                                             # 开发模式

./scripts/release-local.sh                           # 刷新本地运行包 release/SingleSourceReady/（make local）
go run ./cmd/ssr-core release --allow-dirty --no-zip   # 本地自测发布包组装
go run ./cmd/ssr-core release --out release            # 正式发布（HEAD 必须打过 v<版本> tag 且工作区干净）
```

- 只想看界面、不启 Go 后端：`./scripts/preview.sh --build`（注入模拟的 Wails 桥接；`make preview` 等价）。
- 常用任务也可以走 `make help`：`make test` / `make check` / `make frontend` / `make local` / `make preview` / `make package`。
- 装 Wails CLI 请与 `go.mod` 的版本对齐：`go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0`。

## 5. 测试约定

- 分层：`internal/*` 放单元与 golden 测试（样本在 `testdata/`，比对基线是 `.tsv`）；根 `package main` 放集成与契约测试。
- 根目录的 `layout_test.go` 是**对前端源码文本**的契约断言（类名、CSS 规则、中文文案）。改界面时它必须一起改；
  它的定位是"设计口径护栏"（颜色、字号、模块直角、固定宽度），**不是行为测试**。
- 断言里的取值要从**唯一来源**取（`cmd/ssr-core/tools.go` 的常量、`config/*.yaml`、`frontend/src/theme.ts`），
  不要在测试里复制魔法值。
- 契约测试的意义：这些口径跨语言（Go 断言 TS/CSS），一旦有人"顺手简化"就会报错。

## 6. 版本与发布

- **版本号的唯一真源是 git tag**：发布构建用 `-ldflags` 把构建期信息写进二进制；运行期按
  「构建期常量 → git → `config/setting.yaml` 的 `APP_VERSION`」回退。`APP_VERSION` 只是回退值。
- 流程：`git tag v<版本>` 并推送 → CI（`.github/workflows/ci.yml` 校验 + `release.yml` 构建）→
  两个平台的 zip 挂到 Release（`SingleSourceReady-<版本>-<构建日期>-<平台>.zip`）。
- 平台事实：macOS 包是 **arm64**（CI runner 决定）、Windows 可执行文件**未签名**（首次运行会有 SmartScreen 提示）。

## 7. 命名对照（同一件事的几种叫法）

| 场合 | 名字 |
| --- | --- |
| 产品名 / 发布包目录 / macOS 应用 | `SingleSourceReady` |
| 界面里的缩写 | `SSR` |
| GUI 可执行文件 | `ssr` |
| 命令行 | `ssr-core` |
| Go module | `github.com/jiegun314/ssr-go` |
| 前端包名 | `ssr-frontend` |

## 8. 界面口径

- **一律用 antd 默认值**（字号、圆角、间距、控件高度、表格内边距），只覆盖品牌色：
  `BRAND_RED = #DA291C`（强生企业红）、`DANGER_RED = #CF1322`，见 `frontend/src/theme.ts`。
- 功能模块：**直角** + `1px solid #d9d9d9` 描边（比 antd 默认的 `#f0f0f0` 更深）。
- 数字一律千分位（`internal/numfmt`）；文案中文；日期格式统一。
- 具体宽度与位置口径由 `layout_test.go` 固定 —— 改 CSS 时别只看截图，先看契约。

## 9. 提交与协作

- 提交信息用中文，一行说清"做了什么"；正文里写清影响与验证方式。
- **UI 改动一律用显式路径 `git add` 提交，不要 `git add -A`**（避免把本地实验性改动一并带进提交）。
- 提交前的最低验证：`go test -count=1 ./...`、`genlogcolumns --check`、`npm run build`。
- 改了 `config/*.yaml` 或 `frontend/dist` 要一并提交（发布包与 `//go:embed` 依赖它们）。

## 10. 历史遗留与悬空引用

- 代码注释里的 `R10/R15/R16/R17/R26/R27`、`§5.1/§6/§7.2/§9.2` 等编号来自**迁移前的需求与设计文档**，
  那些文档不在本仓库。遇到时以代码与测试的**实际行为**为准，本文件不复制那套编号。
- `docs/archive/` 下是迁移期对照材料（会引用已删除的 Python 脚本），仅作历史存档，不要按它操作。
- 配置注释里若出现"Python 版"的说法，属于与旧仓库共享的历史描述；本仓库真实可用的命令见 §4。
- `config/setting.yaml` 的 `APP_VERSION` 与 Python 版共享，**不随发布更新**。

## 11. 已知欠账（欢迎逐步偿还）

- 前端没有单元测试：行为断言目前借用 Go 的源码契约（见 §5）；计划引入 vitest + @testing-library/react。
- 根目录 `package main` 偏重：`App` 与 Wails 绑定方法都在根，搬迁受 `window.go.main.App` 命名空间约束。
- 错误文案中英混用（校验器英文、界面中文）：计划在 UI 边界统一包装，内部错误保持英文便于检索。
- 日志级别/颜色的口径分散在 Go、TS、CSS 三处，目前靠契约测试兜住。
