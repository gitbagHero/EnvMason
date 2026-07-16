# EnvMason 项目决策记录

本文件集中记录会长期影响产品范围、安全、兼容性、架构或开发流程的决定。普通实现细节不在此记录。项目维护者是所有产品决策的最终决定人；不单独维护虚拟负责人字段。

## 状态定义

- **Accepted**：维护者已经明确决定，后续增量必须遵守。
- **Planned**：已确定最迟决策增量，但具体方案尚未决定。
- **Superseded**：已被后续决定替代，并保留历史引用。

## 已接受决定

### D-001：产品标识与本地目录

- 状态：Accepted
- 决定：英文名为 EnvMason，中文名为境匠，CLI 命令为 `envmason`。
- 决定：配置、缓存、状态和数据目录遵循各平台原生规范；具体路径和迁移规则在相关实现增量中确定。

### D-002：许可证

- 状态：Accepted
- 决定：项目使用 MIT License。

### D-003：单人项目治理

- 状态：Accepted
- 决定：维护者承担产品决策和最终验收；AI 是开发协作者，不是独立审批人。
- 决定：不模拟多人团队角色、会议、Story Point、燃尽图或角色签字。
- 决定：每个增量只要求明确目标与边界、设计与风险、实现、测试、验收核对和维护者确认；不适用交付物直接记为 N/A。

### D-004：Git 与审查方式

- 状态：Accepted
- 决定：采用 AI 自检、自动化测试和维护者按需检查 diff。
- 决定：早期允许在 `main` 上小步开发；高风险执行、安全边界、公开 Schema 和正式发布优先使用分支及 PR 式审查。
- 决定：未经维护者授权，不提交、推送、创建 PR 或发布版本。

### D-005：版本路线与公开发布

- 状态：Accepted
- 决定：I00～I41 构成首个稳定版候选总体路线，I41 是首个稳定版候选，不要求等待 I41 才公开发布。
- 决定：I08 可发布 macOS 只读预览版，后续里程碑按 `0.x` 逐步发布；I42 以后属于 v1.x 完整能力扩展。

### D-006：首期平台和工具范围

- 状态：Accepted
- 决定：Linux 首个稳定版只正式支持 Ubuntu LTS。Debian 可观察和测试，但在建立独立验收矩阵前不声明为正式支持。
- 决定：PRD 中 Python、Go、Rust、Ruby、.NET、Docker、SDK 和运维工具是目标工具类别，实际版本承诺以已验收增量为准。
- 决定：macOS v1 首条垂直切片聚焦系统与 PATH、Homebrew、Node/NVM/npm/pnpm、Java/jenv/Maven/Gradle，以及报告、建议、Plan 和安全执行基础。

### D-007：风险确认策略

- 状态：Accepted
- 决定：R0 可以直接执行。v1 中 R1 写操作仍需要计划级确认，不允许配置静默授权。R2 需要计划级确认。R3 需要明确确认，必要时逐项确认。R4 需要单独确认，不适合首版开放的能力直接禁止。
- 决定：所有写操作必须来自可审查且未过期的 Plan。
- 决定：AI 不能替用户确认、降低风险等级、修改 Plan 后复用原 Plan ID 或绕过确定性核心执行任意命令。

### D-008：自动清理语义

- 状态：Accepted
- 决定：系统可以自动计算清理候选，但实际卸载永远需要生成 Plan，属于 R3，且必须由用户明确确认。
- 决定：不支持 AI 或配置进行无人值守删除。

### D-009：批量更新独立增量

- 状态：Accepted
- 决定：I42 是只读发现，不能作为批量更新完整化增量。
- 决定：在 I42 后增加独立的 macOS 受控选择性与批量更新增量；现有后续增量顺延，避免把写能力夹带到只读增量。

### D-010：I01 CLI 框架与公开接口

- 状态：Accepted
- 决定：CLI 使用 Cobra，I01 不引入 Viper 或任何配置解析能力。
- 决定：最低 Go 版本为 1.25；CI 在 Go 1.25 和 1.26 上覆盖 macOS、Windows 和 Linux。
- 决定：I01 公开 `envmason`、`help`、`-h`、`--help`、`version` 和 `--version`；不使用 `-v`，为未来 verbose 保留。
- 决定：成功和帮助返回 0，未知命令或非法参数返回 2，内部失败保留返回 1。帮助写 stdout，使用错误写 stderr。
- 决定：版本输出包含版本、提交、构建时间、Go 版本和目标平台。机器可读格式、completion 和配置读取不属于 I01。

### D-011：I02 统一清单 Schema 契约

- 状态：Accepted
- 决定：统一清单采用 JSON Schema Draft 2020-12，初始 `schema_version` 为 `0.1.0`，Schema `$id` 为 `urn:envmason:schema:inventory:0.1.0`。
- 决定：对象默认使用 `additionalProperties: false`，未知字段、缺失必填字段、非法枚举和未知 Schema 版本必须被拒绝。
- 决定：激活状态、默认状态和安装原因使用包含 `unknown` 的显式枚举，不用无法表达不确定性的简单布尔值。
- 决定：公开 Schema 手工维护，不从 Go 结构体自动生成；Go 结构体、Schema、fixture 和 golden snapshot 通过契约测试保持一致。
- 决定：使用 `github.com/santhosh-tekuri/jsonschema/v6` v6.0.2 在本地嵌入并校验 Draft 2020-12 Schema，不在运行时联网获取 Schema。
- 决定：I02 不增加 CLI 命令，不扫描系统，不执行外部命令，不进行版本比较或生成建议。

### D-012：I03 macOS 系统只读探测与 Schema 演进

- 状态：Accepted
- 事实依据：macOS `sw_vers(1)` 将 ProductVersion 和 BuildVersion 定义为当前本机系统版本；`sysctl(8)` 在参数不含赋值时只读取内核状态。
- 事实依据：[Apple 官方 Rosetta 文档](https://developer.apple.com/documentation/apple-silicon/about-the-rosetta-translation-environment)规定 `sysctl.proc_translated` 返回 `0` 表示原生进程、`1` 表示转译进程，OID 不存在表示原生执行。
- 决定：Inventory Schema 当前版本升为 `0.2.0`，保留 `0.1.0` Schema 和原始 JSON 验证能力；不修改已经冻结的 `v0.1.0.json`。
- 决定：System 增加 OS build、转译状态、结构化 Shell 和有序 PATH 条目；转译状态使用 `native`、`translated`、`unknown`，PATH 状态使用 `exists`、`missing`、`unknown`。
- 决定：macOS 探测只调用 `sw_vers`、`sysctl` 和 `ps` 的只读查询形式，使用结构化参数、逐命令超时和输出上限，不通过 Shell 拼接命令，不保留原始 stderr。
- 决定：环境变量只读取 `SHELL`、`PATH`、`HOME`；`HOME` 只用于把报告中的用户主目录替换为 `$HOME`，不输出通用环境变量集合。
- 决定：I03 不新增 CLI 命令，不探测 Homebrew 或语言运行时，不联网，不修改 PATH、Shell、文件、包管理器或系统配置。

### D-013：I04 通用可执行文件发现器

- 状态：Accepted
- 决定：I04 只增加内部确定性发现能力，不新增 CLI 命令，不修改 Inventory Schema；公开报告映射留给后续报告整合增量。
- 决定：发现请求显式携带命令名、PATH 目录顺序、工作目录、HOME 和采集时间；核心不依赖隐式全局环境，便于 fixture 和跨平台测试。
- 决定：命令名必须是单一路径段，拒绝空值、`.`、`..`、路径分隔符、NUL 和控制字符，避免路径穿越及报告注入。
- 决定：发现器只使用 `Lstat`、`Stat` 和软链接解析读取文件元数据，绝不执行候选文件；空 PATH 条目按当前工作目录解释并产生 Finding。
- 决定：候选路径和解析后的真实路径分别记录；软链接、PATH 目录自身的链接、损坏链接和链接循环均显式表达。
- 决定：Mach-O 架构通过 Go 标准库 `debug/macho` 读取，支持 thin 和 universal binary；脚本或非 Mach-O 文件保留为有效候选，架构降级为 `unknown`。
- 决定：HOME 内路径只在输出和 Finding 证据中替换为 `$HOME`；文件访问始终使用未脱敏的内部路径。
- 决定：I04 不映射包管理器、不查询版本、不调用 Homebrew、不修改 PATH、权限或软链接；Homebrew 只读适配器属于 I05。

### D-014：低风险增量的批次串行自动推进

- 状态：Accepted
- 决定：维护者预先授权 AI 在增量全部客观验收、风险匹配测试、功能测试和 diff 自检通过后，将该增量标记为预授权验收、提交到 `main` 并推送触发 CI。
- 决定：只有当前增量远程 CI 通过后才能开始下一增量；本地或远程测试失败时继续停留在当前增量修复，不得以批次授权绕过门禁。
- 决定：每个批次串行推进 3～5 个增量；当前批次为 I04～I08，目标是在不扩大各增量范围的前提下完成 macOS 只读预览里程碑。
- 决定：授权不覆盖新的产品范围、安全边界、许可证、公开接口或公开 Schema 决策，也不覆盖高风险系统操作；遇到这些事项或存在实质歧义时必须暂停并由维护者决定。
- 决定：每个增量仍须单独记录范围、测试和验收证据，保持提交单一、可理解、可回退；不得把多个未验收增量合并成一个提交后一次性验证。

## 已规划、尚未决定的事项

| 事项 | 最迟决策增量 |
|---|---|
| YAML Profile 正式 Schema | I19 |
| 首批版本信息来源和缓存策略 | I10 |
| Windows 提权与 WinGet Configuration 边界 | I28 前 |
| Ubuntu LTS 具体版本范围 | I31 前 |
| 第三方适配器动态加载与隔离方案 | I51 |
| Lock 跨平台组织和兼容规则 | I19–I20 |
| 操作历史采用 JSON 或 SQLite 及迁移时点 | I14 |
| 首个支持的 AI Agent | I36 前 |

## I00 验收记录

- 增量：I00 产品契约冻结
- 检查日期：2026-07-15
- 客观检查状态：Passed
- 维护者最终验收：Accepted（2026-07-15）
- 自动化检查结果：
  - 仓库文件符合 I00 白名单，已存在的 `.DS_Store` 被保留并忽略。
  - Markdown 本地链接全部有效。
  - 增量标题 I00–I51 连续且唯一，I01–I51 依赖链连续。
  - 64 个 FR 与 5 个 AI 安全约束编号唯一。
  - 批量更新由独立 I43 承载，I42 保持只读。
  - 范围、平台、R0–R4、自动清理和 AI 安全语义检查通过。
  - MIT License 关键条款检查通过。
  - 未创建产品代码、Go 模块或 I01 实现。
- N/A：I00 不包含产品实现，因此单元测试、fixture、集成测试、CLI 帮助、构建产物和发布版本均不适用。
- 结论：I00 已完成并被接受为开发基线。I01 在 I00 验收后按顺序开始。

## I01 验收记录

- 增量：I01 可运行的空 CLI
- 开始日期：2026-07-15
- 检查日期：2026-07-15
- 客观检查状态：Passed
- 维护者最终验收：Accepted（2026-07-15）
- 自动化检查结果：
  - Go 1.25.6 和本机 Go 1.26.2 单元测试通过。
  - `go vet ./...`、gofmt 和 race 检测通过。
  - macOS、Linux、Windows 的 amd64/arm64 六个目标交叉编译通过；Go 1.25.6 的三平台代表目标构建通过。
  - 无参数、`help`、`-h`、`--help`、`version` 和 `--version` 的真实二进制行为通过。
  - 版本、提交、构建时间、Go 版本和目标平台注入验证通过。
  - 未知命令、未知参数和保留的 `-v` 均返回退出码 2，错误写入 stderr。
  - Cobra 默认 completion 和 `-v` 版本简写均未暴露。
  - GitHub Actions 工作流 YAML 语法检查通过。
  - [远程 CI #2](https://github.com/gitbagHero/EnvMason/actions/runs/29393737936) 通过：Ubuntu、macOS、Windows × Go 1.25/1.26 共六个任务全部成功。
- N/A：I01 不包含配置、扫描、网络、系统修改、fixture、Schema 或发布版本。
- 结论：I01 已完成并被接受。I02 已具备顺序依赖条件，但尚未开始。

## I02 验收记录

- 增量：I02 统一清单 Schema 与 fixture 框架
- 开始日期：2026-07-15
- 客观检查状态：本地与远程 CI 均通过（2026-07-15）
- 维护者最终验收：Accepted（2026-07-15）
- 验收项：合法 fixture 和公开示例通过 Draft 2020-12 Schema 校验；缺失必填字段、非法枚举、未知字段、非法时间和未知 Schema 版本均被拒绝。
- 验收项：相同对象重复序列化结果一致并通过 golden snapshot；公开示例成功表达同一 Tool 的两个 Installation。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./cmd/envmason` 均通过。
- 离线检查：`GOPROXY=off go test -count=1 ./internal/inventory ./schemas/inventory` 通过，运行时 Schema 校验无需网络。
- 远程检查：[GitHub Actions CI #4](https://github.com/gitbagHero/EnvMason/actions/runs/29397547420) 通过。
- 手动验收：维护者确认 I02 手动检测通过（2026-07-15）。
- N/A：I02 不包含 CLI 新命令、真实系统探测、网络查询、版本比较、建议、配置或系统修改。
- 结论：I02 已完成并被接受，已经提交到 `main` 且远程 CI 通过；I03 已具备顺序依赖条件，但尚未开始。

## I03 验收记录

- 增量：I03 macOS 系统只读探测
- 开始日期：2026-07-15
- 客观检查状态：本地与远程 CI 均通过（2026-07-16）
- 维护者最终验收：Accepted（2026-07-16）
- 真机检查：macOS 15.7.4（Build 24G517）Apple Silicon 设备正确识别系统架构 `arm64`、进程架构 `arm64` 和转译状态 `native`。
- 功能检查：fixture 覆盖原生 Apple Silicon、Rosetta、Intel、非 macOS 拒绝和命令失败降级；PATH 顺序、重复、存在、缺失、相对路径和空条目均有断言。
- 隐私检查：只读取 `SHELL`、`PATH`、`HOME`；测试令牌未进入结果或错误，HOME 路径被替换为 `$HOME`，原始 stderr 被丢弃。
- 只读检查：探测器只具备命令查询、环境读取和文件状态读取接口；固定命令列表不包含赋值、包管理器或写入命令；真机探测前后仓库状态一致。
- 可靠性检查：逐命令超时、64 KiB 输出上限、未知父进程不误报为 Shell，以及失败 Finding 均通过测试。
- Schema 检查：`0.2.0` 合法 fixture、公开示例和真机结果通过；缺少新增必填字段被拒绝；`0.1.0` fixture 继续通过原版本 Schema 校验。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./cmd/envmason`、gofmt 和 `git diff --check` 均通过。
- 离线与跨平台检查：`GOPROXY=off` 核心测试通过；macOS 探测测试包面向 Linux amd64 和 Windows amd64 交叉编译通过。
- 远程检查：[GitHub Actions CI](https://github.com/gitbagHero/EnvMason/actions/runs/29480948709) 通过。
- 手动验收：维护者确认 I03 手动检测通过（2026-07-16）。
- N/A：I03 不包含 CLI 新命令、Homebrew、语言运行时、网络查询、版本比较、建议、配置写入或系统修改。
- 结论：I03 已完成并被接受，已经提交到 `main` 且远程 CI 通过；I04 已具备顺序依赖条件，但尚未开始。

## I04 验收记录

- 增量：I04 通用可执行文件发现器
- 开始日期：2026-07-16
- 客观检查状态：本地与远程 CI 均通过（2026-07-16）
- 维护者最终验收：Accepted（依据维护者预授权，2026-07-16）
- 功能检查：fixture 中两个同名命令按 PATH 顺序全部发现，首个可执行且链接有效的候选被标为生效项，重复目录和后续遮蔽项被标记。
- 链接检查：最终文件软链接和 PATH 目录软链接均记录解析路径；模拟与真实临时文件系统中的损坏链接和链接循环都产生 Finding，扫描继续完成。
- 路径检查：包含空格和 Unicode 的目录及命令名正确处理；空 PATH 条目按工作目录解释并产生 Finding；HOME 内输出路径替换为 `$HOME`。
- 权限与失败检查：候选访问、目标访问和架构读取权限不足均降级为 Finding；非可执行文件不会成为生效项；脚本保留为架构 `unknown`。
- 安全检查：拒绝路径穿越、路径分隔符、NUL 和控制字符命令名；真实脚本候选未被执行，探测前后仓库状态一致。
- 架构检查：thin/universal Mach-O CPU 映射和稳定去重通过测试；本机测试二进制和当前 PATH 中的 Go 可执行文件成功识别架构。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./cmd/envmason`、gofmt 和 `git diff --check` 均通过。
- 离线与跨平台检查：`GOPROXY=off` 核心测试通过；发现器测试包面向 Linux amd64 和 Windows amd64 交叉编译通过。
- 远程检查：[GitHub Actions CI](https://github.com/gitbagHero/EnvMason/actions/runs/29483813206) 通过。
- N/A：I04 不包含 CLI 新命令、包管理器映射、版本获取、Homebrew、网络请求、配置读取或任何系统修改。
- 结论：I04 已依据维护者预授权完成验收，已经提交到 `main` 且远程 CI 通过；I05 已具备顺序依赖条件，但尚未开始。
