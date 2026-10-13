# 变更日志（CHANGELOG）

本文件记录**已发布版本**的变更。写法遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号与 Git tag 一致（`v` 前缀），发布日期取 Release 的发布时间。

> 版本号的唯一真源是 **git tag**：发布构建用 `-ldflags` 把构建期信息写进二进制，
> 运行期按「构建期常量 → git → `config/setting.yaml` 的 `APP_VERSION`」回退。
> `APP_VERSION` 只是最后的回退值，不随版本发布更新。

## [未发布]

（暂无）

## [2.3.5] - 2026-10-13

数据量大时数据整合结果表卡顿：加上分页与每页行数选择，数据回顾窗口同步，
并修掉一个选每页行数时才出现的多余滚动条。

### 新增

- **结果表分页**：默认 **100 行/页**，可切 **50 / 100 / 200 / 500**，带「上一页 / 下一页」与「共 N 行」。
  表格只渲染当前页 —— 以前一次把全部行交给 antd Table 渲染，上万行时页面会明显卡顿。
- **数据回顾窗口**同样可切换每页行数（50/100/200/500，默认 100）；它的分页在 Go 侧完成
  （`ReviewLog` / `ReviewSource` 接受任意正整数 pageSize）。
- 分页口径收敛成单一来源 `frontend/src/tablePagination.ts`（结果表、回顾窗口、bridge 共用）。

### 变更

- 结果表工具条：**查找框居左、三个状态筛选片居右**，同一行（不换行；查找框可压缩但不挤掉筛选片）。
- 交互细节：切换每页行数、改动查找关键字、点状态筛选片、重新整合，都回到第 1 页。

### 修复

- 在结果表里选每页行数时，窗口右侧会冒出一条**多余的垂直滚动条**：antd 的下拉弹层挂在 `body` 下
  （窗口外壳本来就是"刚好一屏 + 各面板内部各自滚动"），整窗不再出现文档级滚动条。

## [2.3.4] - 2026-10-13

配置能力补强与配置治理：把"条件必填、取值白名单、运行日日期列"都做成**配置可写**的能力，
补齐两个模板里一直空着的导出列，并顺手修掉身份列会出现 `MISSING` 的缺陷。

### 新增

- **取值白名单 `accepted_values`**（写在来源列上）：该列单元格**只接受**列出的原始写法，
  否则整份文件被拒（校验的是 Excel 里的写法，不是 `value_mapping` 归一化之后的值）。
  空单元格仍归必填 / 条件必填管。参数设定里可以增删条目。
- **条件必填的期望值自动归一化**：`required_when` 里可以按"人怎么填"写（`value: "是"`），
  程序按被引用列自己的 `value_mapping` 换算成归一化后的值再比较。
- **运行日日期列 `transform.type: date_offset`**：值 = 运行日（本地当天）+ `offset_days`，
  格式固定 `YYYY-MM-DD`。它必须写进变更比较的 `ignore_fields`（校验会强制检查），
  测试可用 `Service.Now` 冻结运行日。
- **导出列 `Sterilization Method Description`**：模板里本来就有这一列，配置里一直没有映射，
  导出时始终为空。现在取自 `ra_input.灭菌方式`，缺失判定跟随来源列的条件必填。
- **导出列 `DI Record Publish Date`**：同样是模板里有、配置里没有的列。现在按运行日 +1 天生成。

### 变更

- **⚠️ 升级注意：`ra_input` 的「使用前是否需要进行灭菌」只接受 `是` / `否`**。
  以前填 `Yes / No / Y / N` 的文件会被拒绝导入（把它加进 `accepted_values` 即可放宽）。
  样本工厂已按白名单生成样本。
- **⚠️ 升级注意：`global_udi_input` 的四列改为选填**（`device_category`、`device_generic_name`、
  `if_consistent_with_registration_di`、`product_description`）—— 最终文件用的是 `ra_input` 的同名列，
  这四个不再需要填写。
- `Catalog or Reference Number` 改取 `ra_input.material_code`（基准来源）。原来源在来源表没有
  匹配记录时会写成字面量 `MISSING`，**导出文件的身份列因此可能损坏**；现在不再出现。
- 另有三列改为选填：`global_udi_input.franchise`、`global_udi_input.if_sold_as_individual_units`、
  `ra_input.original_registration_filing_certificate_number`（它们在后续计算中并没有用到）。
- **配置治理**：两份映射加分节横幅与"顺序即语义"说明；`mandatory_status` 词表统一为
  `optional`（空串作为旧写法继续兼容）；`shared_mappings` 由流式改块状（diff 更细）；
  样本工厂专用元数据拆到 `config/sample_mapping.yaml`（可选文件，`excel_import_mapping.yaml`
  因此少 31% 篇幅，非样本内容逐字节不变）。

### 修复

- 导出文件的身份列在来源表没有匹配记录时不再写成 `MISSING`（见上）。
- 样本工厂生成的样本一定能通过当前配置的导入校验（白名单优先）。

## [2.3.3] - 2026-10-09

### 修复

- **导出文件的 `Choose Action` 与 `Change Description` 两列按修正后的数据要求生成**：
  - `Choose Action` 以前是固定文本 `Add or Modify`（不符合要求），现在**按行判定**：
    以 **Catalog or Reference Number + Primary DI** 为身份，操作日志里没有同一身份的最新记录 → `Add`，
    已经有 → `Modify`；该列**不可能为空**（配置里声明为派生列 `record_action`，且 `mandatory_status: required`）。
  - `Change Description` 在 `Modify` 时列出与同一身份记录的差异项，形如
    `Package Type变更；Package DI变更`（多项用全角分号连接，**顺序＝导出列顺序**）；
    `Add` 行留空。措辞与分隔符仍可在 `consolidation_mapping.yaml` 里配置。
  - 与历史记录完全一致的行照旧判为「重复」、不进导出，所以进导出的 `Modify` 行必然至少有一处差异。
  - 新增安全网：万一出现「判定为 Modify 但没有差异项」的**可导出**行，会在操作日志里留一条警告
    （不改行状态；重复 / 缺失行不会有这条警告）。

### 变更

- 差异项的罗列顺序由 map 的字母序改为**配置里导出列的声明顺序**（与导出、日志列同一套口径）。

> 注意：这一版修正的是**行为**，`config/consolidation_mapping.yaml` 的 `target_dataset.fields` 里
> `Choose Action` / `Change Description` 两块需要同步更新。发布包**不会覆盖**你已有的 `config/*.yaml`，
> 所以升级后请把这两块改成新写法（或删掉该文件让程序从 `config/defaults/` 重新生成一份）。


## [2.3.2] - 2026-10-06

这一版没有新的业务功能，做的是**工程规范与结构治理**（第 1–4 批），外加一项用户可见的改进：
界面上的报错统一成中文。完整说明见 [Release v2.3.2](https://github.com/jiegun314/ssr-go/releases/tag/v2.3.2)。

### 变更

- **界面报错统一为中文**：内部错误与操作日志仍保留英文原文（便于检索、对照上游库），
  但给用户看的对话框文案统一经 `userMessage` 包装 —— 认识的错误（文件类型不支持、文件不存在/被移动、
  没有权限、数据库被占用/损坏、不是有效的 Excel、工作表找不到、没有可读数据行、源=目标文件、
  配置校验类）给中文说法；不认识的用「操作失败：」起头并附原文；已经含中文的原样显示。
- **日志级别口径合一**：级别标签以 Go 的 `logLevelLabels` 为真源，前端逐条比对；
  四个级别色只在 `frontend/src/design-tokens.ts` 定义，CSS 用 `var(--log-*)` 引用。
- **设计 token 单一来源**：颜色与关键尺寸（品牌红、危险红、模块描边、日志标题条灰底、
  时间列宽 / 级别槽位 / 日志卡下限 / 状态片宽）集中到 `frontend/src/design-tokens.ts`，
  `styles.css` 用 `:root` 变量引用，由一个前端测试校验两者一致。
- **前端类型改为从 Wails 生成模型派生**（`wailsjs/go/models.ts`），不再手写字段清单：
  Go 结构体加字段后前端会自动跟上，不一致由 `tsc --noEmit` 立刻发现。
- **结构整理**：纯工具函数移入 `internal/`（`fileutil` / `configsnapshot` / `excelio.review`）；
  本地发布与界面预览脚本收进 `scripts/`；新增 `Makefile` 任务入口（`make help`）；
  前端契约测试按主题拆成 7 个文件。
- **CI 拆分**：`ci.yml`（push main / PR：Go 测试 + 日志列一致性 + gofmt + go vet + 前端测试与构建）
  与 `release.yml`（tag：两个平台原生构建并发布）；固定 `wails@v2.16.0` 保证构建可复现，
  加 Go/npm 缓存与并发控制。

### 新增

- **前端行为测试**：引入 vitest + jsdom + @testing-library/react，26 条用例覆盖操作日志（筛选、
  置顶、详情弹窗与复制内容）、数据整合结果表（状态片筛选、查找、空态）、参数设定表单
  （控件渲染、只读项、注释口径、列表增删、列清单抽屉、保存载荷）与设计 token 一致性。
- 工程文件：`LICENSE`（MIT）、`.editorconfig`、`CHANGELOG.md`、`AGENTS.md`（工程约定索引）。

### 修复

- `gofmt` 检查在"索引里仍列着已删除路径"时误报（重构期间会出现）。


## [2.3.1] - 2026-10-05

完整说明见 [Release v2.3.1](https://github.com/jiegun314/ssr-go/releases/tag/v2.3.1)。

### 新增

- **参数设定：YAML 完整转成标准控件**（默认视图「表单」）：
  - 每一层 YAML 渲染成一个直角描边分块、嵌套缩进，**所有注释都显示**（显示时去掉行首 `#`，缩进与空行保留）；
  - 标量按控件类型渲染（输入框 / 数字框 / 开关 / 下拉），枚举候选值取自程序里的校验常量与配置自身；
  - 只读项显示值与原因：DB 字段名、样本工厂专用块、条件必填表达式、生成物（日志列整份）、共享映射的别名引用；
  - 列清单用表格 + 抽屉编辑，**不支持新增 / 删除 / 排序**（顺序即暂存表列顺序）；标量列表可增删条目；
  - 「结构树」「原文」两个视图保留为查看与兜底；改动攒起来一次性保存，可逐项或整批撤销。
- **写回只改被编辑的那几行**：按 YAML 节点行号定位、只替换目标值（或对列表增删整行），
  注释、空行、键序、引号风格、锚点与别名全部原样保留。
- **保存后热重载**：重建导入规则 / 整合服务 / 操作日志句柄，不需要重启程序
  （只有数据库路径这类需要换连接的改动会提示"重启后切换"）。
- 操作日志：警告 / 错误行可以点开看完整正文，并一键复制到剪贴板。

### 变更

- 整合错误信息改为「错误类型 + 该类型的行数」汇总，不再逐条展开（报错上千行时日志与悬浮提示不再卡顿）。
- 导入状态与日志里的行数统一千分位显示（如 `现有3,072条`、`共 1,248 行`）。

## [2.3.0] - 2026-10-04

完整说明见 [Release v2.3.0](https://github.com/jiegun314/ssr-go/releases/tag/v2.3.0)。

### 变更

- **布局**：左列顺序为 **数据导入 → 记录导出 → 操作日志**；记录导出改成与其他模块同构的 86px 模块；
  操作日志吃掉左列剩余高度；右列只服务结果表（可视高度 +58px）；两列之间是窄的收起 / 展开按钮，收起后结果表吃满宽度。
- **数据整合**：「整合」主按钮进入标题栏正中（白底红字、28px）；「生成文件」为无边框白色图标（18px、与「整合」同高）；
  工具条一行（左侧查找框 + 右侧三个固定 86px 等宽状态片，片内数字加粗）；状态片文案为 **合格 / 缺失 / 重复**。
- 日志列宽压缩（时间 64px 左对齐、级别 32px 居中槽位、单元格内边距 4px）。
- 导入结果弹窗改为 antd `Descriptions`；整合日志只留摘要。

### 修复

- 修复表格单元格内边距覆盖失效的问题（antd 的 `:where()` 前缀需要更高特异性的选择器）。
- 修复记录导出行在窄左列里溢出换行的问题（改成单行 + 纯图标「回顾」）。

## 迁移期（0.2.x，Python + PySide6 原型 → Go + Wails v2）

`v0.2.1` … `v0.2.23` 是原型迁移与对齐阶段，逐条变更见 GitHub Releases。

| 版本 | 发布时间 | 说明 |
| --- | --- | --- |
| [v0.2.23](https://github.com/jiegun314/ssr-go/releases/tag/v0.2.23) | 2026-10-03 | 2.3.0 之前的最后一个原型版本 |
| [v0.2.20](https://github.com/jiegun314/ssr-go/releases/tag/v0.2.20) | 2026-09-28 | |
| [v0.2.14](https://github.com/jiegun314/ssr-go/releases/tag/v0.2.14) | 2026-09-26 | |
| [v0.2.10](https://github.com/jiegun314/ssr-go/releases/tag/v0.2.10) | 2026-09-20 | |
| [v0.2.5](https://github.com/jiegun314/ssr-go/releases/tag/v0.2.5) | 2026-09-19 | 迁移后的早期可运行版本 |
| [v0.2.1](https://github.com/jiegun314/ssr-go/releases/tag/v0.2.1) | — | 最早的打标签版本 |

[未发布]: https://github.com/jiegun314/ssr-go/compare/v2.3.3...HEAD
[2.3.3]: https://github.com/jiegun314/ssr-go/compare/v2.3.2...v2.3.3
[2.3.2]: https://github.com/jiegun314/ssr-go/compare/v2.3.1...v2.3.2
[2.3.1]: https://github.com/jiegun314/ssr-go/compare/v2.3.0...v2.3.1
[2.3.0]: https://github.com/jiegun314/ssr-go/compare/v0.2.23...v2.3.0
