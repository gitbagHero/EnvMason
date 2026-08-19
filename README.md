# EnvMason（境匠）

EnvMason 是面向 macOS、Windows 和 Linux 的开发者工作站生命周期管理平台。它不重新实现包管理器，而是在 Homebrew、WinGet、apt、NVM、mise、jenv、Docker 等现有工具之上提供统一的发现、评估、规划、执行和验证能力。

## 当前状态

**I00：产品契约冻结** 至 **I20：macOS Profile 解析与 Lock** 已按顺序通过验收；I21-A 至 I21-C 已完成 Base Plan、Homebrew 安装事务 Review 和只读事实采集门禁，I21-D 已完成内部固定 Homebrew 写适配器的本地与远端客观验收。系统可以将本机、项目和新鲜官方版本事实组合成 Node/Java 的结构化建议，把一项合格的 Node 目标转换为可审查 Plan，并在 macOS 上通过现有 NVM 安装精确 Node 版本、独立切换 default alias、显式恢复原 alias，以及在目标 NVM Node 下选择性更新 npm、Corepack 与 pnpm。I18 的内部能力已覆盖多动作 DAG 失败隔离、检查点继续、恢复复核和三阶段独立确认编排；按 D-053 方案 A，新编排保持内部 API。I19/I20 新增严格 Profile `0.1.0`、单目标 Lock `0.1.0` 和纯 macOS 解析器，可规范化 Base/Frontend Node 并基于显式目录快照区分已满足、需安装、冲突和无法解析。I21-D 仍未接入 CLI，也未完成真实可恢复 macOS 环境验收、执行后 Lock 和差异报告，因此 I21 尚未完成。当前仍不能安装 NVM、迁移任意全局包、处理 Yarn 复杂策略、卸载旧版本或执行任意命令。

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

## 受控执行、NVM Node 安装与默认切换

I14 新增内部受控执行核心和 [`Plan 0.2.0`](./schemas/plan/v0.2.0.json)。I18-I2 将当前操作记录升级为 [`Operation Record 0.4.0`](./schemas/operation/v0.4.0.json)，每条新记录保存与确认凭据、步骤顺序和动作身份一致的完整 `confirmed_plan`，并允许 Plan `0.2.0`、`0.3.0` 或 `0.5.0`；冻结的 [`0.3.0`](./schemas/operation/v0.3.0.json) 继续保存 Plan `0.2.0`/`0.3.0`，而 [`0.2.0`](./schemas/operation/v0.2.0.json) 和 [`0.1.0`](./schemas/operation/v0.1.0.json) 仍可读取和验证但没有完整 Plan 来源。I18-D 新增审查态 [`Plan 0.4.0`](./schemas/plan/v0.4.0.json)，用于把来源记录、新鲜检查点证据和重新准备的剩余动作绑定成新的不可变 Plan。I18-I1 新增最终可确认的 [`Plan 0.5.0`](./schemas/plan/v0.5.0.json)，它由通过不可变校验的 `0.4.0` 草案确定性提升，完整保留来源、检查点、剩余动作和时间窗口并重新计算 ID；I18-I3 允许受控执行核心接收 `0.5.0`，但本阶段只有内部 Node tools 服务会在完成来源复核并构造固定适配器注册表后调用它。Plan 仍只携带声明式动作身份，不包含命令、参数或执行规范；NVM、HOME 和临时目录下的安装路径在进入 Plan 前转换为稳定占位符。实际进程规范只能来自核心内置注册表。执行前必须重新校验不可变 Plan ID、30 分钟有效期、环境摘要、NVM 控制文件摘要和绑定相同 Plan ID 的用户确认。

I15 在 macOS 上公开单个 R2 NVM 安装入口。NVM 必须已经存在并有可读取的 default alias；目标必须是高于当前生效版本、且能在本次 fresh Node.js 官方 release index 中精确验证的稳定版本。先用 dry-run 审查完整 Plan：

```sh
envmason apply --tool runtime.node --version 24.14.0 --online --dry-run
```

dry-run 不确认、不安装且不创建操作记录。实际执行使用同样的参数但移除 `--dry-run`：

```sh
envmason apply --tool runtime.node --version 24.14.0 --online
```

CLI 会显示完整 Plan ID，并要求在交互终端逐字输入 `apply <完整 Plan ID>`。不支持 `--yes`、管道确认、配置授权、环境变量授权或 AI 代确认。安装只使用固定 `/bin/bash --noprofile --norc` 和内置 NVM 调用，采用已有 `nvm.sh` 的内容摘要、二进制安装和受控环境；安装后验证目标、原生效版本、default alias 和所有原安装。它不会修改 alias、改变当前 Shell、安装 NVM、清理失败残留或删除旧 Node。现有 `envmason plan` 继续生成 `0.1.0` 的不可执行预览。

操作记录遵循平台原生状态目录：macOS 为 `~/Library/Application Support/EnvMason/operations`，Windows 为 `%LOCALAPPDATA%/EnvMason/operations`，Linux 为 `$XDG_STATE_HOME/envmason/operations`，未设置时回退到 `~/.local/state/envmason/operations`。stdout、stderr 分别限制为 64 KiB；`0.2.0` 起记录执行前后事实摘要、确定性差异和幂等跳过状态，`0.3.0` 增加经确认的完整 Plan 来源，`0.4.0` 增加最终可确认继续 Plan `0.5.0` 的记录契约。完成状态要求动作成功且注册验证器通过，失败、取消和中断不会被报告为 Completed。

I18-C 增加内部只读检查点资格判定。只有 Operation Record `0.3.0` 中已经 Completed、验证 Passed、具有 After Snapshot，并通过注册适配器新鲜动作级复核的步骤，才可作为继续 Plan 的可复用候选；旧记录、活动记录、缺失证据或发生版本、provider、所有权、NVM 控制状态漂移的记录会以稳定原因码阻断。

I18-D 在 Eligible 判定上生成 Plan `0.4.0`。它绑定来源 Operation/Plan、重新观察的检查点 digest、来源动作顺序、重新准备的 Plan ID，以及由检查点满足的依赖边；动作列表只包含剩余 R1/R2 动作，并使用来源终态后新 Plan 的环境、策略和原 30 分钟时窗。Plan `0.4.0` 固定为 `"executable": false`，现有执行器会在注册表解析、历史写入和进程启动前拒绝；本阶段仍没有公开 CLI，也不能真正继续或恢复操作。

I18-E 将上述核心接入 Node tools 内部服务。调用方只提供已有 Operation ID；服务从已确认的 Node tools Plan 提取 Node 版本、工具目标和 provider，严格拒绝旧记录、活动或完成记录、非 Node tools 来源及来源元数据替换，并在一次当前扫描上复核检查点、重新准备剩余动作和生成 Plan `0.4.0`。该过程只读取历史、文件元数据和受控版本输出，不保存新记录、不启动包管理器写动作，也不提供公开继续命令。

I18-F 增加内部执行前只读复核。它先拒绝不可变内容非法、尚未生效、过期或非 `0.4.0` 的输入，再使用原 Plan 的创建时间重新运行一次只读准备；只有来源、检查点、当前环境、剩余动作和完整 Plan ID 均未变化才通过。复核不会刷新有效期、确认 Plan、写入历史或执行动作。

I18-I1 将通过复核的审查草案纯函数式提升为 Plan `0.5.0`。`0.5.0` 固定可执行标志、只允许 R1/R2、要求完整 continuation provenance 和新的 Plan 级确认；来源、检查点、目标、依赖、时间或环境任一变化都会产生不同 ID。I18-I1 交付时它只提供数据契约；执行能力由后续 I18-I3 独立开放。

I18-I2 使 Operation Record `0.3.0` + Plan `0.2.0` 和 Operation Record `0.4.0` + Plan `0.2.0`/`0.5.0` 都能成为 R1/R2 只读继续来源。再次继续只绑定直接来源；更早的来源和检查点由 confirmed Plan `0.5.0` 的 continuation provenance 维持链路。Node tools 再次继续仍只做一次新鲜扫描，不改写任何已有记录；I18-I2 交付时尚未开放 `0.5.0` 执行。

I18-I3 新增内部 `ExecuteContinuation`。调用方必须提供最终 Plan `0.5.0` 及绑定其完整 ID 的新 Plan 级确认；服务先拒绝非法、尚未生效、过期或确认不匹配的输入，再用原创建时间和一次新鲜扫描重建 `0.4.0` 草案及最终 `0.5.0`。只有完整 ID 相同才复用该次扫描的固定 Node tools 适配器执行剩余动作并写新 Record `0.4.0`。失败后的下游保持 Pending，可再次走同一流程；没有公开 CLI、`--yes` 或旧确认复用。

I18-J 在每次内部继续执行产生终态 Record 后再做一次新鲜扫描。结果按本次动作返回 before/after/target 精确版本、provider、步骤终态与 verified；Completed 只有在记录验证通过且执行后版本/provider 仍匹配时才标记 verified，Failed/Pending 不会误报。结果不含绝对路径、命令、参数或 runner 输出；后扫描失败会保留已持久化的真实 Record，并与原执行错误一并返回。

I18-K 新增纯只读 `AssessRecovery`。它只接受带完整 confirmed Plan 的终态 Record `0.3.0`/`0.4.0`：合法非空 Snapshot diff 标记为 `changed`；写进程可能已经启动但最终差异无法证明时标记为 `uncertain`；skipped、空 diff、Pending 和从未启动的写动作不列为恢复候选。结果原样保留 Action 冻结的 `recovery.mode=plan|manual` 与摘要，但不生成、确认或执行恢复 Plan，也不读取当前环境。

I18-L 新增只读 `RevalidateRecovery`。它在 I18-K 候选之上，仅对具有合法 After Snapshot 的 `changed` 动作调用确定性核心已注册的 checkpoint revalidator，并输出 `current`、`drifted` 或 `verifier_unavailable`；缺少最终证据的候选保持 `uncertain` 且不触发适配器。结果不含 Snapshot facts 或适配器错误，也不构建命令、写历史、生成恢复 Plan 或声称已经恢复。

I18-M 将上述只读复核接入现有 NVM 默认版本内部服务。`ReviewRestore` 只接收来源 Operation ID：合法 `set_default` 变化最多触发一次 Inventory 扫描，并通过固定 NVM checkpoint revalidator 复核默认别名、解析版本、控制文件和安装集合；不确定来源不扫描，非 set-default 来源在扫描前拒绝。该入口不需要 Runner、不生成 R3 恢复 Plan，也不改变恢复仍须新 Plan 和明确确认的边界。

I18-N 为终态失败的 Node tools R2 操作新增内部 `ReviewRecovery`。只有历史中存在 `changed` 候选时才进行一次扫描和受控 `--version` 探针，用来源固定的目标/provider 注册表复核为 `current` 或 `drifted`；失败且没有最终差异的动作保持 `uncertain`，若全部候选均不确定则零扫描。非 Node tools 或不安全来源在扫描前拒绝，整个过程没有包管理器写调用、历史写入或恢复 Plan。

I18-O 将 Node tools `ReviewRecovery` 扩展到 Completed 记录：成功且 Snapshot diff 非空的动作同样通过一次新鲜扫描得到 `current/drifted`，成功但 skipped/unchanged 的操作返回空候选且零扫描。共同来源校验仍固定 confirmed Plan 的动作身份、provider、目标与 DAG；`PrepareContinuation` 继续只接受具有剩余动作的终态失败记录，Completed 不会获得继续资格。

I18-P 为 I15 NVM Node 安装补齐同一只读恢复复核。固定 install adapter 会比较 active/default/NVM script、安装集合和目标存在状态，并通过目标二进制 `--version` 验证精确版本；apply 内部 `ReviewRecovery` 对 Completed changed 输出 `current/drifted`，对失败无差异输出 `uncertain`，对 skipped/unchanged 输出空候选。它不调用在线 Assess、安装 Runner、Plan builder 或历史写入，也不会删除目标；任何卸载仍需独立 R3 Plan 和明确确认。

I18-Q1 新增只读 [`Workflow Manifest 0.1.0`](./schemas/workflow/manifest-v0.1.0.json) 与 [`Workflow Record 0.1.0`](./schemas/workflow/record-v0.1.0.json)。Manifest 用内容派生 ID 固定“安装 Node（R2/Plan 0.2.0）→ 设置 NVM default（R3/Plan 0.3.0）→ 更新 Node tools（R2/Plan 0.2.0）”及精确版本目标，自身不可执行、不可确认；Record 以纯函数按顺序绑定每阶段独立 Plan ID、Operation ID 和 checkpoint，异常终态立即停止后续。本阶段不扫描环境、不生成或加载子 Plan、不确认、不执行、不持久化记录，也没有公开 CLI。

I18-Q2 增加内部 `PrepareNext`：它只为 Workflow Record 中唯一 Ready 的阶段调用现有 apply、defaultversion 或 nodetools 准备服务，严格校验新鲜子 Plan 的 Schema、风险、动作与 Manifest 精确目标后，把该 Plan ID 绑定到返回的新 Record。前序未完成、工作流异常终止、Plan 陈旧或阶段/目标不匹配时不会触发后序准备。公开 Plan 与封存的原生 Prepared 上下文深度隔离；本阶段仍不确认、不执行、不写 Workflow 文件，也没有公开 CLI。

I18-Q3 增加内部 `ExecutePrepared`：每个阶段只接受绑定当前子 Plan 完整 ID 的独立确认，然后把 Q2 封存上下文交给现有 apply、defaultversion 或 nodetools 执行服务。只有合法终态 Operation Record 才会按其 ID、状态和规范化 SHA-256 digest 推进 Workflow；执行前漂移、过期或确认拒绝若尚未创建 Operation，阶段保持 Planned。失败、超时、取消或中断会立即终止工作流，不会准备后续阶段；编排层不生成确认、命令或任意执行入口。

I18-R1 新增只读 [`Recovery Manifest 0.1.0`](./schemas/recovery/manifest-v0.1.0.json)，把一个或多个已经完成当前状态复核的来源 Operation 汇总为内容派生清单。它固定不可执行、不可确认，并将候选确定性分类为“准备新 Plan 审查”“人工处置”“调查不确定性”“重新评估漂移”或“缺少复核器时人工审查”。Manifest 不含 Plan 内容、命令、确认、Snapshot、diff、输出或路径，也不主动读取历史、扫描环境或生成恢复 Plan。

I18-R2 增加内部恢复 Plan 白名单入口。只有 Recovery Manifest 中仍为 `changed/current`、`recovery_mode=plan` 的 `runtime.node/set_default` item，才会调用现有 defaultversion `PrepareRestore` 重新加载来源 Operation、扫描当前 NVM 状态并生成全新独立 Plan `0.3.0`；新 Plan 仍需单独明确确认。manual、uncertain、drifted、无复核器、Node 安装和 Node tools 候选都不能进入该入口；本阶段不确认或执行恢复 Plan。

I16 在 macOS 上增加 [`Plan 0.3.0`](./schemas/plan/v0.3.0.json) 的单动作 R3 默认版本切换。目标必须已由 NVM 安装，不需要联网。先只读审查原 alias、原解析版本和精确目标：

```sh
envmason default set --tool runtime.node --version 24.14.0 --dry-run
```

确认无误后移除 `--dry-run`。CLI 要求在交互终端逐字输入 `set-default <完整 Plan ID>`；不支持 `--yes`、管道确认、配置授权或 AI 代确认。动作只修改 NVM `default` alias，不切换当前已打开的 Shell，不修改 Shell profile，不删除 Node 版本。验证在不读取用户 profile 的隔离 Shell 中 source 已摘要绑定的 `nvm.sh`。

设置操作的 Operation ID 可用于审查恢复 Plan：

```sh
envmason default restore --operation op-00000000000000000000000000000000 --dry-run
envmason default restore --operation op-00000000000000000000000000000000
```

恢复是新的 R3 Plan，具有新 Plan ID，并要求逐字输入 `restore-default <完整 Plan ID>`。验证失败时系统只输出恢复建议，不会自动回滚；如果 alias 在原操作后被外部修改，恢复 Plan 会拒绝覆盖。

## Node 附属工具更新

I17 在 macOS 和已有 NVM 的前提下，将更新绑定到一个已经安装的精确 Node 版本。每个目标版本都必须显式提供；省略某个参数即把对应工具排除在 Plan 外：

```sh
envmason update node-tools \
  --node-version 24.14.0 \
  --npm 12.0.1 \
  --corepack 0.35.0 \
  --pnpm 11.1.0 \
  --dry-run
```

移除 `--dry-run` 后，CLI 仍要求在交互终端逐字输入 `apply <完整 Plan ID>`。不支持 `latest`、版本范围、降级、`--yes`、管道确认或 AI 代确认；至少选择 npm、Corepack、pnpm 中的一项，目标可以等于当前版本以完成幂等验证。Plan 固定使用已有 `0.2.0` 声明式 Action 和 R2 计划级确认，不包含 Shell 或用户提供的命令。

npm 与 Corepack 由目标 Node 自己的 npm 使用固定官方 registry 和关闭 lifecycle scripts 的受控环境更新；独立安装的 pnpm 也通过该 npm 更新。Corepack 代理的 pnpm 使用目标 Node 自己的 Corepack 执行精确 `install --global`，与独立 pnpm 使用不同适配器。Corepack 的 Known Good Release 存放在其用户级 `COREPACK_HOME`，因此 Plan 会明确标记 Corepack provider，但不承诺不同 NVM Node 之间的 Corepack 缓存隔离；动作不会改写其他 Node 的 `bin` 目录。

执行前重新扫描 Inventory，并重新校验目标 Node、工具提供方、包元数据摘要、`nvm.sh` 和 default alias。每项动作后都验证目标工具路径、版本、目标 Node、当前生效 Node 和 default alias；任一项失败时后续动作保持 Pending，Operation Record 分别保留已完成、失败与未执行状态。I17 不修改项目 `package.json` 或 lockfile，不读取用户 npm/Corepack 配置，不迁移全局包，不处理 Yarn，也不提供 I18 的继续 Plan、检查点恢复或自动回滚。

## Profile 声明

I19 新增正式的 [`Profile 0.1.0 Schema`](./schemas/profile/v0.1.0.json) 与内部严格解析器。YAML 和 JSON 使用同一个数据契约；解析后会补齐变体、版本策略与布尔选项默认值，并把模块稳定排序为 Base、Frontend Node。示例位于 [`Base Profile`](./examples/profiles/base.yaml) 和 [`Frontend Node Profile`](./examples/profiles/frontend-node.yaml)。

首版只接受 `base` 与 `frontend_node`，两者的变体都是 `minimal` 或 `standard`。Frontend Node 支持 `lts`、`stable` 和 `exact`；只有 `exact` 携带不含 `v`、预发布或 build metadata 的精确 SemVer `pin`。未知字段、重复/冲突模块、YAML alias/显式 tag/多文档、非法策略和任意 `command`/`shell`/`args` 字段都会被拒绝。

Profile 只声明期望能力，不是 Plan 或安装授权。当前没有公开 Profile CLI；解析过程不会读取本机、项目、网络或历史，不会生成 Lock、调用适配器或修改系统。macOS 实现解析与 Lock 属于下一增量 I20。

I20 已冻结 [`Lock 0.1.0 Schema`](./schemas/lock/v0.1.0.json)、内部确定性构造器和 macOS 纯解析器。一份 Lock 只对应一个 Profile digest 和一个 OS/版本/架构目标，来源使用无凭据的 HTTPS URI、快照时间与内容 digest；解析项区分 `satisfied`、`install_required`、`conflict` 与 `unresolved`。解析器只消费调用方提供的显式版本目录与当前 Inventory，已安装的兼容 manager/版本不会重复列为需要安装，缺失/歧义来源或平台不匹配会给出稳定 reason。Lock 固定不可执行、不可确认，也不包含路径、机器身份、命令、Plan 或输出；当前仍未提供公开 Profile/Lock CLI，也不会执行 Base 配装。

I21-A 新增内部 Base Plan 准备器，只为已校验 Lock 中确需安装的 Git/CMake Homebrew formula 生成 Plan `0.2.0` R2 Action；已满足项跳过，冲突/无法解析项阻断。Homebrew 必须已存在且与 Lock 目标 Inventory 一致，系统不会 bootstrap Homebrew。当前尚未注册 Homebrew 写适配器，因此该 Plan 即使获得正确确认也会在历史写入和进程启动前被执行器拒绝。

I21-B 新增内部只读 Homebrew Transaction Review `0.1.0`。它把仍在有效期内的 I21-A Plan、原 Lock、Homebrew/配置/catalog 摘要，以及 Git/CMake 根 formula 的精确传递依赖闭包封存为不可执行、不可确认的内容派生记录。Action 漏项、来源或配置漂移、依赖版本冲突、未知下载量和总量溢出都会停止；共享依赖按唯一 formula 去重汇总。Review 不含 HOME、brew 路径、源 URI、环境值、命令、参数、确认或凭据，也不会注册或运行 `brew install`。

I21-C 增加上述 Review 的内部纯只读事实采集核心。调用方必须显式提供同一观察时刻的当前 Inventory、精确 brew 可执行文件字节、当前环境及 system/prefix/user 三层 `brew.env` 内容、与 Lock digest 完全一致的官方 formula catalog JSON，以及按目标 macOS bottle tag 绑定 SHA-256 的下载大小事实。核心按 Homebrew 固定优先级解析配置但不执行 Shell，要求关闭自动更新、隐式升级/cleanup、dependents 检查、analytics、ask 和环境提示；未批准的 Homebrew/proxy 配置全部停止。catalog 只接受 `homebrew/core`，解析目标 variation、formula revision、required/recommended 传递依赖并对照当前已安装版本，缺失/多余/零大小或 digest 不匹配的 bottle 都停止。输出只含 I21-B 所需摘要和公式事实；本阶段仍不读任意路径、不联网、不调用 Homebrew、不开放 CLI、不注册 `brew install`。

I21-D 增加内部固定 Homebrew 写适配器和执行编排。I21-A 候选 Plan 必须先与 I21-C Review 派生出新的最终 Plan ID，用户确认同时绑定该 Plan 与 Review；执行前重新采集完整事务事实，并再次核对 brew 可执行文件、HOME/TMPDIR 和受控 Homebrew 配置摘要。当前只注册 Git/CMake 的 `brew install --formula --force-bottle` R2 动作，环境不继承 proxy，关闭自动更新、隐式升级和 cleanup；预检、幂等判断、执行后精确闭包验证、Operation Record 和恢复检查点复核均复用现有确定性执行核心。它仍是内部 API，不 bootstrap/升级 Homebrew、不接受任意 formula、不卸载、不提权、不读取任意路径，也不生成最终 Lock/diff。
