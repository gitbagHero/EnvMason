# EnvMason（境匠）

EnvMason 是面向 macOS、Windows 和 Linux 的开发者工作站生命周期管理平台。它不重新实现包管理器，而是在 Homebrew、WinGet、apt、NVM、mise、jenv、Docker 等现有工具之上提供统一的发现、评估、规划、执行和验证能力。

## 当前状态

**I00：产品契约冻结** 至 **I13：只读 Plan/Action 模型** 已按顺序通过验收。系统可以将本机、项目和新鲜官方版本事实组合成 Node/Java 的结构化建议，并把一项合格的 NVM Node 更新建议转换为只读计划预览，但仍不能执行系统变更。

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

I00～I41 是首个稳定版候选的总体路线，但各里程碑可以按 `0.x` 版本逐步公开。当前能力仍属于开发阶段，实际支持范围必须以已验收增量和版本兼容矩阵为准。

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

当前公开清单契约位于 [`schemas/inventory/v0.3.0.json`](./schemas/inventory/v0.3.0.json)，并保留 [`v0.2.0`](./schemas/inventory/v0.2.0.json) 和 [`v0.1.0`](./schemas/inventory/v0.1.0.json) 的验证能力。`0.3.0` 为 Finding 增加可选的状态、建议和影响字段；手工示例位于 [`examples/inventory-report.json`](./examples/inventory-report.json)。Schema 会嵌入核心并在本地校验，校验过程不需要联网。

## macOS 只读报告

I08 提供首个 macOS 综合只读报告。默认终端摘要以及 Markdown、JSON 输出都来自同一次确定性清单模型；当前 JSON 输出遵循 Inventory Schema `0.3.0`。

```sh
envmason report
envmason report --format markdown
envmason report --format json
envmason report --category runtime --category ecosystem
envmason report --severity warning --severity error
envmason report --online
envmason report --project /path/to/workspace
envmason report --project project-a --project project-b --exclude archived
envmason report --online --policy /path/to/envmason-policy.json
envmason report --format json > envmason-report.json
```

重复的类别或严重程度值采用 OR，类别与严重程度之间采用 AND。部分适配器失败时仍会生成报告，并用 `REPORT_SECTION_FAILED` 和 `REPORT_INCOMPLETE` 标记不完整；默认命令不会联网查询版本，也不会修改包管理器、配置或系统。

`--online` 是显式的只读联网入口，查询 Node.js 官方 release index/Release 工作组 schedule，以及 Adoptium available releases/Temurin support schedule。报告显示来源、数据时间和 fresh/stale 状态；超时、离线或外部数据异常不会阻止本地报告生成，过期数据不会冒充“已确认最新”。I10 不写持久化缓存，已有缓存仅通过可注入的只读契约使用。

I12 将 I09～I11 的本机版本、官方版本事实和显式项目引用组合为确定性评估。只有 fresh 数据或用户明确 Pin 才能产生确定更新结论；Current 与 LTS 保持不同语义，项目仍引用的安装会被标记为 `retain_required`，Unknown 不会被猜测为可更新。

策略文件只通过 `--policy` 显式读取，不自动搜索 HOME 或平台配置目录，不会被修改，最大 64 KiB。当前格式为严格 JSON，例如：

```json
{
  "schema_version": "0.1.0",
  "tools": {
    "runtime.node": {
      "channel": "lts",
      "pin": "22.22.0",
      "ignore_updates": false
    },
    "runtime.java": {
      "channel": "lts",
      "pin": "21",
      "ignore_updates": false
    }
  }
}
```

`channel` 仅接受 `lts` 或 `stable`。`ignore_updates` 只抑制普通更新建议，不隐藏 EOL、项目保留要求和运行时冲突。I12 仍不生成 Plan，也不执行安装、升级、卸载或默认版本切换。

I11 的项目入口只扫描用户通过 `--project` 明确选择的目录；未提供该参数时不会搜索工作目录、HOME 或整个磁盘。扫描器只读取首批 Node/Java 版本声明白名单，忽略 `node_modules`、版本库内部目录和常见构建产物，不跟随符号链接，也不执行项目脚本或构建工具。`--exclude` 可重复使用，并要求至少存在一个 `--project`。

## 只读 Plan 预览

I13 提供仅面向 `runtime.node` 的不可执行预览。命令必须显式联网取得 fresh 官方版本事实；只有存在可比较的更新建议、NVM 已存在且显式 Pin 能在 fresh Node.js 官方 release index 中验证时才会生成 Plan：

```sh
envmason plan --tool runtime.node --online
envmason plan --tool runtime.node --online --format json
envmason plan --tool runtime.node --online --policy /path/to/envmason-policy.json
envmason plan --tool runtime.node --online --project /path/to/workspace
```

没有合格建议时命令明确返回退出码 1，不会生成空计划或伪造目标；缺少 `--online`、未知工具或格式属于用法错误并返回退出码 2。

Plan JSON 遵循 [`schemas/plan/v0.1.0.json`](./schemas/plan/v0.1.0.json)，固定包含内容派生 Plan ID、30 分钟有效期、环境与策略摘要、R2 风险、计划级确认、前置条件、验证及恢复元数据。任何内容变化都会使原 Plan ID 失效。I13 的 Plan 固定为 `"executable": false`，没有 command、args、Shell、apply 或执行器入口，也不会写入 Plan、日志或系统状态。
