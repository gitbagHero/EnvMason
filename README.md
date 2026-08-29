# EnvMason（境匠）

EnvMason 是一个以安全和可审查性为核心的开发者工作站生命周期管理工具。它在 Homebrew、NVM 等现有管理器之上提供统一的环境发现、状态评估、变更计划、受控执行和结果验证能力，而不是重新实现包管理器。

> EnvMason 目前处于发布前开发阶段。现阶段经过验收的用户入口主要面向 macOS；Windows 11 和 Ubuntu LTS 尚未形成可用支持。路线图与准确进度以[增量开发计划](./INCREMENTAL_DEVELOPMENT_PLAN.md)为准。

## 项目定位

EnvMason 旨在将工作站变更拆分为可验证的确定性流程：

1. 只读发现系统、运行时、包管理器和项目约束。
2. 基于本机事实、显式策略和可信版本数据生成建议。
3. 将允许的变更固化为内容寻址、具有有效期的 Plan。
4. 由固定适配器执行已确认的动作，并记录结构化操作历史。
5. 执行后重新验证环境，明确成功、失败、漂移与恢复边界。

EnvMason 不接受任意 Shell、PowerShell、提权命令或由 AI 直接生成的系统命令作为执行入口。

## 当前能力

| 能力 | 状态 | 说明 |
|---|---|---|
| macOS 环境报告 | 可用 CLI | 发现系统、PATH、Homebrew、Node/NVM、npm/pnpm、Java/jenv、Maven/Gradle，并输出终端摘要、Markdown 或 JSON |
| Node/Java 状态评估 | 可用 CLI | 可结合显式项目引用、策略文件和新鲜官方数据生成结构化 Finding |
| Node 更新 Plan 预览 | 可用 CLI | 仅为 `runtime.node` 生成不可执行、具有有效期的只读 Plan |
| NVM Node 精确版本安装 | 可用 CLI | NVM 已存在时，可审查并确认安装单个版本；不会自动切换默认版本 |
| NVM 默认版本切换与恢复 | 可用 CLI | 独立 R3 Plan、独立确认，并拒绝覆盖已经漂移的 alias |
| npm、Corepack、pnpm 更新 | 可用 CLI | 更新被绑定到指定的既有 NVM Node，且按工具分别验证 |
| Profile、Lock 与 macOS Base 配装 | 内部能力 | Profile/Lock 契约和 Base 配装核心已实现；真实可恢复 VM 写入验收尚未完成，不提供公开 CLI |
| Windows 11、Ubuntu LTS | 尚未支持 | 仍属于后续增量，不应视为当前产品能力 |

当前不提供 NVM 自动安装、任意全局包迁移、旧版本卸载、自动清理、软件源切换、任意 formula 安装或通用命令执行。

## 安全模型

- 默认只读；联网和写操作都必须通过明确入口触发。
- 所有写操作必须来自未过期、内容不可变且可审查的 Plan。
- R1/R2 需要计划级确认；R3 需要明确确认；R4 需要单独确认，不适合当前版本的能力直接禁止。
- 确认必须绑定完整 Plan ID。配置、环境变量、管道输入、`--yes` 或 AI 都不能代替用户确认。
- Plan 只描述声明式动作身份；命令、参数和受控环境只能来自核心内置适配器。
- 执行前重新校验环境和 Plan，执行后重新扫描并记录真实结果；失败、取消或验证失败不会伪装为完成。
- 安装、卸载、换源和系统修改优先在可恢复环境中验证。

更完整的产品与权限边界见[产品需求](./PRODUCT_REQUIREMENTS.md)和[项目决策记录](./docs/PROJECT_DECISIONS.md)。

## 构建与验证

开发需要 Go 1.25 或更高的受支持版本：

```sh
go test ./...
go vet ./...
go build -o envmason ./cmd/envmason
./envmason version
```

CI 或发布构建可通过 `-ldflags` 注入版本、提交和构建时间；未注入时程序会明确显示 `devel` 或 `unknown`。

## 使用方法

### 只读环境报告

```sh
envmason report
envmason report --format markdown
envmason report --format json > envmason-report.json
envmason report --category runtime --severity warning
envmason report --project /path/to/workspace
envmason report --online --policy /path/to/envmason-policy.json
```

默认报告不联网，也不修改包管理器、配置或系统。`--online` 只读取 Node.js 与 Adoptium 的官方版本和生命周期数据；联网失败不会阻止本地报告生成，过期数据也不会被标记为已确认最新。

`--project` 只扫描用户明确选择的目录，不会搜索当前目录、HOME 或整个磁盘；扫描器不跟随符号链接，也不执行项目脚本或构建工具。

策略文件仅通过 `--policy` 显式读取，采用严格 JSON，且不会被修改：

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

### Node Plan 预览

```sh
envmason plan --tool runtime.node --online
envmason plan --tool runtime.node --online --format json
envmason plan --tool runtime.node --online --policy /path/to/envmason-policy.json
```

该命令只生成不可执行的 Plan。没有合格建议时返回退出码 1；缺少必要参数或参数非法时返回退出码 2。Plan ID 由内容派生，任何内容变化都会使原 ID 失效。

### 安装一个 NVM Node 版本

先执行 dry-run 审查完整 Plan：

```sh
envmason apply --tool runtime.node --version 24.14.0 --online --dry-run
```

确认目标与风险后，使用相同参数移除 `--dry-run`：

```sh
envmason apply --tool runtime.node --version 24.14.0 --online
```

CLI 会要求在交互终端逐字输入 `apply <完整 Plan ID>`。该动作不会安装 NVM、修改 default alias、切换当前 Shell 或删除已有 Node 版本。

### 切换与恢复 NVM 默认版本

```sh
envmason default set --tool runtime.node --version 24.14.0 --dry-run
envmason default set --tool runtime.node --version 24.14.0

envmason default restore --operation op-00000000000000000000000000000000 --dry-run
envmason default restore --operation op-00000000000000000000000000000000
```

设置和恢复分别生成新的 R3 Plan，并分别要求 `set-default <完整 Plan ID>` 或 `restore-default <完整 Plan ID>`。如果来源操作之后 alias 已被外部修改，恢复会停止而不是覆盖当前状态。

### 更新 Node 附属工具

```sh
envmason update node-tools \
  --node-version 24.14.0 \
  --npm 12.0.1 \
  --corepack 0.35.0 \
  --pnpm 11.1.0 \
  --dry-run
```

移除 `--dry-run` 后进入确认和执行。npm、Corepack、pnpm 均为可选目标，但至少需要指定一项；不支持 `latest`、版本范围、隐式降级或 Yarn 策略。

## 数据契约

公开 JSON 报告使用 [Inventory Schema 0.3.0](./schemas/inventory/v0.3.0.json)，示例见 [inventory-report.json](./examples/inventory-report.json)。Schema 嵌入核心并在本地验证，无需联网。

仓库还维护版本化的 [Plan](./schemas/plan)、[Operation Record](./schemas/operation)、[Profile](./schemas/profile)、[Lock](./schemas/lock)、[Workflow](./schemas/workflow) 和 [Recovery](./schemas/recovery) 契约。其中 Profile、Lock、Workflow 和 Recovery 当前是内部接口，不代表已经开放同名 CLI 或完整用户流程。

操作记录默认保存在平台原生状态目录：macOS 为 `~/Library/Application Support/EnvMason/operations`，Windows 为 `%LOCALAPPDATA%/EnvMason/operations`，Linux 为 `$XDG_STATE_HOME/envmason/operations`，未设置时回退到 `~/.local/state/envmason/operations`。

## 项目文档

- [产品需求](./PRODUCT_REQUIREMENTS.md)：产品范围、术语、功能与安全需求。
- [增量开发计划](./INCREMENTAL_DEVELOPMENT_PLAN.md)：路线图、依赖、当前进度与后续门禁。
- [项目决策记录](./docs/PROJECT_DECISIONS.md)：长期架构、安全、兼容性决定及验收证据。
- [I21 真实环境验收手册](./docs/I21_LIVE_ACCEPTANCE.md)：macOS Base 配装的隔离环境验收边界。
- [AI 协作约定](./AGENTS.md)：维护者与 AI 的开发、验收和 Git 规则。

## 贡献与许可

EnvMason 当前由项目维护者个人开发和维护。提交外部贡献前，请先通过 Issue 或约定渠道确认范围，避免与唯一进行中的增量冲突。变更必须遵守默认只读、Plan 先行、风险确认和测试门禁，不得夹带尚未批准的后续能力。

本项目采用 [MIT License](./LICENSE)。
