# EnvMason（境匠）

EnvMason 是面向 macOS、Windows 和 Linux 的开发者工作站生命周期管理平台。它不重新实现包管理器，而是在 Homebrew、WinGet、apt、NVM、mise、jenv、Docker 等现有工具之上提供统一的发现、评估、规划、执行和验证能力。

## 当前状态

**I00：产品契约冻结**、**I01：可运行的空 CLI**、**I02：统一清单 Schema 与 fixture 框架** 和 **I03：macOS 系统只读探测** 已通过维护者验收。I04 尚未开始；Homebrew、语言运行时、配置和系统修改能力也尚未开始。

核心原则：

- 默认只读。
- 所有写操作必须来自可审查的 Plan，并按风险等级由用户确认。
- AI 不能替用户确认、降低风险等级或绕过确定性核心执行任意系统命令。
- 同一时间只推进一个最小增量；当前增量测试和验收未通过前，不进入下一增量。
- 不重新实现包管理器，通过适配器编排现有管理器。

## 项目文档

- [产品需求](./PRODUCT_REQUIREMENTS.md)
- [增量开发计划](./INCREMENTAL_DEVELOPMENT_PLAN.md)
- [项目决策记录](./docs/PROJECT_DECISIONS.md)
- [AI 协作约定](./AGENTS.md)
- [MIT License](./LICENSE)

## 支持范围

I00～I41 是首个稳定版候选的总体路线，但各里程碑可以按 `0.x` 版本逐步公开。当前没有已发布或已实现的产品能力，实际能力必须以已验收增量和版本兼容矩阵为准。

首个 macOS 垂直切片聚焦系统与 PATH、Homebrew、Node/NVM/npm/pnpm、Java/jenv/Maven/Gradle，以及报告、建议、Plan 和安全执行基础。Linux 首个稳定版正式支持范围仅承诺 Ubuntu LTS；其他工具和平台能力按增量计划逐步扩展。

## 贡献

EnvMason 当前由项目维护者个人开发和维护。外部贡献前请先通过 Issue 或其他约定渠道确认范围，避免与当前唯一进行中的增量冲突。贡献必须遵守默认只读、Plan 先行、风险确认和测试门禁；未经维护者确认，不得夹带后续增量功能。

本项目采用 [MIT License](./LICENSE)。

## 开发

I01 使用 Go 1.25 或更高的受支持版本。当前最小命令可使用标准 Go 工具构建和测试：

```sh
go test ./...
go vet ./...
go build -o envmason ./cmd/envmason
```

发布或 CI 构建通过 `-ldflags` 注入版本、提交和构建时间；未注入时会明确显示 `devel` 或 `unknown`。

当前公开清单契约位于 [`schemas/inventory/v0.2.0.json`](./schemas/inventory/v0.2.0.json)，并保留 [`v0.1.0`](./schemas/inventory/v0.1.0.json) 的验证能力。手工示例位于 [`examples/inventory-report.json`](./examples/inventory-report.json)。Schema 会嵌入核心并在本地校验，校验过程不需要联网。
