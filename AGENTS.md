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
| `frontend/src/__tests__/` | 前端**行为**测试（vitest + jsdom + testing-library），配置在 `frontend/vitest.config.ts`，垫片在 `frontend/src/test/setup.ts` |
| `config/` | 四份行为配置 + 可选的 `sample_mapping.yaml`（样本工厂专用）；打包时整目录复制成发布包的 `config/defaults/` |
| `testdata/` | 测试用样本工作簿与 golden 基线（`.tsv`） |
| `docs/archive/` | 迁移期（Python + PySide6 原型）对照材料，仅历史存档 |

## 3. 配置口径（最重要）

- **四份 YAML 是行为来源**：`setting.yaml`、`excel_import_mapping.yaml`、`consolidation_mapping.yaml`、`log_columns.yaml`。
  改行为优先改配置，不是改代码。
- `config/sample_mapping.yaml` 是**可选的第 5 份**，只给样本工厂（`ssr-core gensample`）用：
  导入 / 整合 / 导出都不读它。它按 `sources.<来源>.<db_field>` 存 `english_name` / `data_type` /
  `allowed_values`；`LoadImportMapping()` 会把条目**合并回列的 `sample` 子块**，
  所以 `internal/sample` 无需改动。老配置（把元数据内联在列里、或没有这个文件）照常可用。
- **升级不覆盖用户配置**：正式名文件缺失时，从 `config/defaults/*.yaml` 生成一份；发布包解压时**从不覆盖**已存在的
  `config/*.yaml`、`config/.backup/`、`data/` 下的数据库。
- **改配置参数要同时改两处**：仓库里的默认文件 `config/*.yaml`，以及本地运行包 `release/SingleSourceReady/config/`
  里**正在使用**的那份同名文件（发布脚本按设计不会覆盖它）。两边不一致时程序读的是正在使用的那份 ——
  只改默认文件等于没生效。同步走「参数设定」的安全通道或 `SaveConfigurationFile`（自动留 `.bak` 并整体校验）。
- `config/log_columns.yaml` 是**生成物**：头部注释与内容由 `cmd/ssr-core/tools.go` 的 `logColumnsComment` /
  `RenderLogColumns` 产出，两者必须逐字节一致 —— CI 用 `genlogcolumns --check` 把关。
- **`mandatory_status` 词表**：来源列与导出列都用 `optional` / `required` / `required_if_applicable`
  （导出列另有 `user_defined`，取自 `setting.yaml` 的用户自定义值）。空串是**旧写法**（等价 optional），
  校验仍接受以便老配置照跑，但仓库自己的配置不许再写空串（`mandatory_status_test.go` 会把关）。
- 四份配置的**分节横幅与"顺序即语义"注释**是刻意保留的阅读辅助：导入映射按来源分节，
  整合映射按模板 `category` 分节；列顺序 = 暂存表列顺序，导出列顺序 = 模板表头顺序
  （并决定 `Change Description` 里差异项的罗列顺序）。改配置时不要顺手重排。
- **取值白名单 `accepted_values`**（写在来源列上）：这一列的单元格**只接受**列出的原始写法，
  否则整份文件被拒（校验的是 Excel 里的写法，不是 `value_mapping` 归一化之后的值 ——
  两者分工不同：白名单管"输入允许哪些写法"，映射管"入库/导出用什么写法"）。
  空单元格归必填 / 条件必填管。样本工厂会**优先**用白名单第一项生成样本（样本必须能导入）。
- **条件必填 `required_when`**：列的状态必须写 `required_if_applicable`（否则配置校验不过），
  条件写在 `required_when` 里（列表 = AND；也支持 `all` / `any` 分组）。
  条件里的期望值会按**被引用列自己的 `value_mapping` 自动归一化**，所以可以按"人怎么填"写
  （`value: "是"` 等价于归一化后的 `Yes`）。导入侧先拦（`conditionalViolations`），
  导出侧的 `required_if_applicable` 再按同一条件判定缺失（`isRequiredForRow`）。
- **运行日日期列 `transform.type: date_offset`**：不读来源表，值 = 运行日（本地当天）+ 天数，
  格式固定 `YYYY-MM-DD`（`base` 只允许 `today`）。天数二选一：`offset_days` 直接写整数，或
  `offset_days_from` 指向 `setting.yaml` 的 `user_defined_data` 键 —— 后者是**固定参数**
  （与 `social_credit_code` 同处，在「参数设定」里可改），当前用
  `di_record_publish_date_offset_days: 5`（DI Record Publish Date = 运行日 + 5 天）；
  两者同时写会被配置校验拦住。它**必须**写进变更比较的 `ignore_fields` ——
  日期每天都变，漏写会让每次整合都把全部行判成"有差异"；配置校验会直接拦住这种情况
  （`internal/config/date_offset_test.go`）。测试要冻结运行日：`Service.Now = func() time.Time {…}`。
- **合并多条记录 `merge_records`**（写在来源上，只支持 `many_to_one`，与 `duplicates` 互斥）：
  同一键的每条记录都留下（`deduplicate: true` = 整行完全相同才去重；`order: source | sorted`），
  **不判冲突**、也不产出单元格候选值 —— 多条不同的值在这里本来就合法。导出列用
  `transform.record_separator` 把多条连成一格（只允许用在 `concatenate` 列上，且它读取的来源必须
  声明了 `merge_records`，否则配置校验拦住）：每条记录先按列自己的字段与 `separator` 拼好、
  空值跳过，再按 `record_separator` 连接，所以不会出现尾部分隔符。当前用于「一个型号多个医保编码」：
  只合并**医保代码本身**（不带序列号），英文逗号连接。
- **多人协作版（P1）**：`config/users.yaml`（人员与角色，可选）与 `config/workflow.yaml`（共享盘地址、
  收件人等，可选）是新增的两份配置，缺失时程序按**单机模式 / 本地模式**运行（行为与旧版本一致）。
  有 `users.yaml` 时启动先登录（选择身份，不读域信息，只记录本机用户名）；除 Admin 外无密码，
  Admin 密码是 `internal/auth` 里的哈希常量（换密码 = 发新版）。权限矩阵（角色 × 动作）集中在
  `internal/auth`，`App` 的每个绑定方法用 `authorize`/`authorizeSource` 裁决，拒绝也写审计；
  审计表 `audit_log`（本地库）记录登录、拒绝、导入、整合、导出、设置保存等动作。
- **导入后自动发布（P2）**：配了 `workflow.yaml` 的 `share.root` 时，导入成功会把整张表发布成
  `published/<部门>/<version>.csv` + `<version>.manifest.json`（+ 原始 xlsx 留档），
  再在 `state/state.lock` 里更新 `state/state.json`（哪个来源、谁、何时、行数、sha256）。
  版本号 = 时间戳 + 内容哈希前 8 位；发布是"先写快照、后改状态、全程原子 rename"。
  发布前会复查共享盘：不健康就**拒绝导入**（避免"以为传上去了"），审计记 deny；
  没配 `share.root` = 本地模式，不发布也不拦。实现见 `internal/publish` 与 `app_publish.go`。
  **部署口径**：`share.root` 的根目录必须由 IT 预先建好（程序不替用户创建根目录，路径写错时
  会明确报"共享盘不可达"）；第一次检查通过后程序自动补齐骨架目录
  `published/{ra,eama,udi}`、`state`、`consolidated`、`audit`（每个进程只做一次），
  `state/state.json`、`published/` 下的版本文件、`consolidated/<id>/`、`log/` 分别在第一次发布、
  第一次导出、第一次日志同步时生成。
- **对齐与整合门禁（P3）**：配了共享盘时，SC 的本地数据要先"对齐"（读 `state.json` 取最新版本 →
  `publish.Fetch` 拉快照 → 校验 sha256 与行数 → **整表覆盖导入** → 记本地 `synced_versions` →
  RA 还要 `importer.RebuildCleaned` 重建清洗表）。整合前校验"四份都已对齐"
  （`sync.require_before_consolidate`，默认开），不通过就拒绝；整合成功后在 `state.state.json`
  写 `last_consolidation`（四个版本 + 计数），各端的「待整合」标志据此清除。
  `ShareOverview` 负责按角色下发：RA/EAMA/UDI 只看自己来源的版本/行数/发布人，其他来源只给
  `published_at`（前端只显示"最后更新：…"）。实现见 `app_align.go`、`internal/publish.Fetch`、
  `internal/store/sync.go`。
- **整合租约与结果归档（P4）**：共享盘模式下整合前抢 `state/consolidate.lock`（O_EXCL）并在
  `state.json` 写 `consolidation_running`（谁、何时、心跳），第二个人会被拦下；运行崩溃留下的锁
  30 分钟后可回收。整合期间有人发布时按 `consolidation.on_new_version_during_run` 处理：
  `warn`（默认，写提示）/ `auto_rerun`（自动重对齐再整合一次）/ `block`（整合前 10 分钟内有发布就拒绝）。
  导出成功后把文件复制到 `consolidated/<整合id>/` 并写 `summary.json`，再把相对路径写回
  `state.last_consolidation.result`（`ExportResult.SharedPath` 回给界面）。
- **运行状态、审计与旧版本清理（P5）**：状态栏的「运行状态」入口打开弹窗（SC/Admin 可见），
  展示共享盘状态、四份来源的版本/发布时间/发布人/行数与本机对齐情况、最后一次整合（含结果文件）、
  整合锁（谁在整合）；「查看审计」走 `ReviewAudit`（最新的在前、分页），数据来自本地 `audit_log`；
  「清理」走 `CleanupShareVersions`（只 Admin）：每个来源保留最新 N 个版本，且当前版本、
  `last_consolidation` 用到的版本与整合目录永远保留。参数设定新增两个页签：**「共享盘」**编辑
  `workflow.yaml`（`share.root` 留空 = 本地模式，相对路径会被校验拦下）、**「人员名单」**编辑
  `users.yaml`；两份文件不存在时给模板，保存即创建（重开程序生效）。「共享盘」页签里的
  `recipients.ra_receivers` / `supply_chain` / `admins` 是收件人列表（可增删，邮箱格式校验）。
- **记录导出全量同步**：「记录导出」所有人可见，所以操作日志要与共享盘合并成全量：
  本机 `operation_log`（未变判定用）保持原样，并集写进回顾表 `operation_log_all`
  （`ReviewLog` 读它），并集再发布到 `<share>/log/operation_log.csv` + manifest（sha256/行数校验）；
  登录、轮询（60 秒）、打开记录导出、导出成功后都会同步，全程在 `state.lock` 里合并。
  这样"别人导出的记录"不会被误当成"本机已导出"而把整份数据判成重复。
- **试点与加固（P6）**：`ExportDiagnostics`（SC/Admin）把版本、日志、审计、共享盘状态与
  **邮箱打码**后的配置打成 `output/diagnostics-*.zip`（不含数据库）；`ConsolidateForce` 是
  "强制重跑"（未变化的行也不判重复，连同 `Change Description = 无变更` 一起导出，用于数据没变
  但需要重新出一份）；整合/清理前先查共享盘，断盘时直接给中文提示；同一来源 10 分钟内被
  别人发布覆盖时写日志与 `publish/warning` 审计。EAMA 试点手册见 `docs/pilot/eama-pilot.md`。
- **导入第一步删除失效注册证**（来源的 `drop_expired.field`，当前只用于 RA）：导入时先按
  `RA失效日期` 过滤 —— 日期早于今天的行整行丢弃（今天到期仍保留；空值/无法解析的值保留），
  原文表、清洗表、发布快照都不再包含这些行；**对齐共享盘快照时也会再过滤一次**
  （旧版本发布的快照可能带过期行），保证整合输入一致；导入结果会回传删除行数，
  弹窗与操作日志各显示一次。
- **导入后生成清洗表 `clean`**（写在来源上；导入与整合共用这份配置）：原文照旧写进 `target_table`
  （`数据回顾` 读它），另按规则生成一张派生表（`clean.target_table`，**整合读它**，
  `consolidation_mapping` 不用改）。规则按顺序生效：`dedupe.by` 分组只留 `dedupe.latest_by` 最新的一行
  （同值取靠后的记录）→ `group_by` 分组 → `unique` 列组内必须唯一（`conflict: latest` 取最新那一行的值 /
  `error` 整份文件被拒）→ `join_comma` / `join_backslash` 列去重后连接（空值跳过，不会出现首尾分隔符）→
  其余列取最新那行的值；`drop` 的列不进清洗表（仍参与去重/挑最新）。当前 RA 用它把「注册证 × 规格型号」
  收成「一个规格型号一行」：`注册证编号/备案凭证编号` 用英文逗号连接、5 个文本项用反斜杠连接，
  `注册证唯一码` / `BU` 等 8 列只用于挑选与追溯。
  ⚠️ 有 `clean` 时，**条件必填**校验看的是**清洗后**的行（被淘汰的旧行不该拦住整份文件），
  表头 / 必填 / 取值白名单仍看原文；「清空导入数据」会把原文表与清洗表一起删。
- 两列是**运行期派生**的（由变更比较填值，不读来源表）：`Choose Action`（`transform.type: record_action`，
  只能是 Add 或 Modify）与 `Change Description`（`change_description`）。判定键与重复检查共用
  `duplicate_check.identity_fields`，不允许另写一份；比较顺序 = 导出列的**配置声明顺序**。
- 「参数设定」的结构化写回（`settings_write.go`）**只改被编辑的那几行**：注释、空行、键序、引号风格、锚点与别名都不动；
  先备份 `.bak`，再整体校验，失败自动还原。

## 4. 构建、测试、发布

```bash
go build ./...                                        # 编译全部包
go test -count=1 ./...                                # 全部测试（提交前必跑）
go run ./cmd/ssr-core genlogcolumns --check --config config   # 日志列生成物不许漂移
go run ./cmd/ssr-core version                         # 版本信息

npm ci --prefix frontend                              # 前端依赖
npm run build --prefix frontend                       # 前端类型检查与构建（含 tsc --noEmit）
npm test --prefix frontend                            # 前端行为测试（vitest + jsdom）
wails build                                           # 桌面产物（版本由 git tag / ldflags 注入）
wails dev                                             # 开发模式

./scripts/release-local.sh                           # 刷新本地运行包 release/SingleSourceReady/（make local）
go run ./cmd/ssr-core release --allow-dirty --no-zip   # 本地自测发布包组装
go run ./cmd/ssr-core release --out release            # 正式发布（HEAD 打过 v<版本> tag 且工作区干净）；只对空白目录用，会重建目录
```

- **本地刷新运行包只用 `./scripts/release-local.sh`**（= `make local`）：它按设计**保留**
  `config/*.yaml`、`config/.backup/`、`data/udi_data.sqlite3*`、`output/` 以及其中的用户数据。
- ⚠️ **不要对"正在使用"的目录执行 `release --out <目录>`**：该命令会**重建整个目标目录**，
  连同其中的工作配置与数据库一起清掉（本仓库已经因此清空过一次 52MB 的运行库）。
  它只适合输出到**空白目录**；给本地在用目录打包一律走 `release-local.sh`。
- 只想看界面、不启 Go 后端：`./scripts/preview.sh --build`（注入模拟的 Wails 桥接；`make preview` 等价）。
- 常用任务也可以走 `make help`：`make test` / `make check` / `make frontend` / `make frontend-test` /
  `make local` / `make preview` / `make package`。`make check` = Go 测试 + 前端测试 + 生成物一致 + gofmt + vet。
- 装 Wails CLI 请与 `go.mod` 的版本对齐：`go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0`。

## 5. 测试约定

**两套测试，各管一段，别互相替代：**

| | 位置 | 管什么 |
| --- | --- | --- |
| 前端行为测试 | `frontend/src/__tests__/*.test.tsx`（vitest + jsdom + testing-library） | 渲染出来的东西与交互结果：筛选、排序、点开弹窗、复制内容、控件改值与**保存载荷**、抽屉内容、空态文案 |
| Go 单元/golden | `internal/*`（样本在 `testdata/`，基线是 `.tsv`） | 业务逻辑、配置校验、导入/整合/导出结果 |
| Go 契约测试 | 根目录 `layout_*_test.go`（对前端**源码文本**断言） | **设计口径与跨语言一致性**：颜色 token、固定宽度、中文文案、产物内嵌、`frontend/dist` 与模板一致 |

- 能写成行为测试的，就写进行为测试：源码字符串断言只能证明"某段文字还在"，证明不了"跑起来对"。
- Go 契约测试里的取值要从**唯一来源**取（`cmd/ssr-core/tools.go` 的常量、`config/*.yaml`、`frontend/src/theme.ts`），
  不要在测试里复制魔法值。
- 改界面时的最低要求：`npm test --prefix frontend` 与 `go test -count=1 ./...` 都要过；改了设计口径要同步 Go 契约。

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
  `BRAND_RED = #DA291C`（强生企业红）、`DANGER_RED = #CF1322`（真源在 `frontend/src/design-tokens.ts`，
  `theme.ts` 只做转出）。
- 功能模块：**直角** + `1px solid #d9d9d9` 描边（比 antd 默认的 `#f0f0f0` 更深）。
- 数字一律千分位（`internal/numfmt`）；文案中文；日期格式统一。
- **错误文案分两层**：内部错误保持英文（便于检索与对照上游库），**给用户看的对话框文案统一走
  `userMessage`**（`user_message.go`：认识的错误给中文说法，不认识的用"操作失败："起头并附原文；
  已经含中文的原样显示）。操作日志里仍记英文原文（技术记录）。新增错误码/库错误时，
  往 `uiErrorRules` 里加一条整串匹配的规则，并补 `user_message_test.go` 的用例。
- 颜色与关键尺寸只在 `frontend/src/design-tokens.ts` 定义一次，`styles.css` 用 `:root` 变量引用；
  具体宽度与位置口径由根目录 `layout_*_test.go` 固定（按主题拆分）—— 改 CSS 时别只看截图，先看契约。

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

- 设计口径已收敛成 **design token 单一来源**（`frontend/src/design-tokens.ts` ↔ `styles.css` 的
  `:root` 变量，一致性由 `frontend/src/__tests__/designTokens.test.ts` 校验）；
  Go 契约只断言"规则里用了哪个变量"，不再断言颜色字面量。
- **冲突的单元格级标识（五步已实现）**：行级的 `Conflict` 与 `Outcome.ConflictData`
  （来源 + 键 + 文字说明）之外，还要标出"哪些**导出列**冲突、各是什么候选值"：
  1. `groupOneToMany` / `groupManyToOne` 额外产出 `source → key → map[导出列名]候选值集合`
     （来源的 `db_field` 经 `Config.Fields[col].source.{file,field}` 反查导出列名）；
     只有值多于一个的字段才算冲突的格，反查不到导出列的字段没有可标记的单元格；
     验证：`internal/consolidation/conflict_cells_test.go`。
  2. `mapResultRow` 调 `conflictNotes(...)`，得到本行 `map[导出列名]提示文本`
     （简版口径：`来源 + 键 + 候选值前 3 条`，键里的 `\x00` 换成 `+`）。
  3. `Outcome` 增加 `ConflictNotes [][]string`（与 `Rows` 同形同序，列顺序 = `Config.FieldNames`，
     空串 = 该格不冲突；`Rows`/`Statuses`/`Counts` 语义不变），golden 基线不变。
  4. `app_methods.go` 的整合返回值带上该矩阵（`ConsolidateResult.Conflicts`，与 `Rows`/`Columns`
     同形同序 —— status 那一格永远是空串），并**重新生成** Wails 模型
     （`frontend/wailsjs/go/models.ts`）；验证：`npm run build --prefix frontend`
     与根包契约测试 `consolidate_conflicts_test.go`。
  5. 前端 `ConsolidationPanel` 增加 `conflicts?: string[][]`，冲突格渲染
     `<Tooltip title={note} mouseEnterDelay={0.1}><span className="cell-conflict">…</span></Tooltip>`；
     **不用原生 `title`**（延迟由系统控制、且要精确停在文字上），`.cell-conflict` 用 `display: block`
     撑满整格；`layout_result_table_test.go` 加样式契约；
     验证：前端行为测试（标记 + 悬停出提示）+ `make check` 退出码 0。
- 前端类型的来源：`frontend/src/types.ts` 从生成的 `frontend/wailsjs/go/models.ts` **派生**
  （`Data<>` 剥掉生成类上的 `convertValues`，需要收窄的用 `Omit` + 交叉类型补联合类型），
  只有纯前端的 `SourceDefinition` 手写（`AboutInfo` 已由 Go 结构体生成）。
  改 Go 结构体后重新生成模型即可，前端不必同步字段清单（`tsc --noEmit` 会立刻发现不一致）。
- 根目录 `package main` 偏重：`App` 与 Wails 绑定方法都在根，搬迁受 `window.go.main.App` 命名空间约束。
- 日志级别的口径已合一：标签以 Go 的 `logLevelLabels` 为真源（`log_test.go` 逐条比对 `log.ts`），
  四个级别色只在 `frontend/src/design-tokens.ts` 定义（CSS 用 `var(--log-*)`）。
  还差一块：**值类型着色**（`theme.ts` 的 `VALUE_COLORS` 与 CSS 的 `.type-*`）仍是两处，可按同样办法收敛。
