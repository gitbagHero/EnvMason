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
