# I21 macOS Base 可恢复实机验收

## 当前状态

I21-F1 只提供默认禁用的双阶段测试入口和本地安全拒绝验证。它没有在开发机或远程 CI 执行真实 Homebrew 安装，也不代表 I21 已完成。

真实写入属于独立的 I21-F2。开始 I21-F2 前，维护者必须先准备可恢复的 macOS VM 快照；`prepare` 输出最终 Plan/Review 后，还必须对这两个精确 ID 给出新的 R2 明确确认。环境变量只负责把已确认的 token 传给测试入口，不能代替维护者确认。

## 环境要求

- 原生 macOS arm64 或 amd64 VM，且能够恢复或销毁到运行前快照。
- 已安装一个原生、active 的系统级 Homebrew；本入口不 bootstrap、更新或换源 Homebrew。
- 目标架构下没有 Homebrew 安装的 Git 和 CMake formula；存在任意一个都会在网络和写入前停止。
- 从待验收提交的干净工作树运行；bundle 必须位于仓库外、扩展名为 `.json`。
- bundle 父目录必须是无符号链接的真实目录且权限为 `0700`，bundle 必须是权限为 `0600` 的真实普通文件。
- 启动进程不能携带额外 `HOMEBREW_*`、proxy、`SUDO_ASKPASS` 或 `XDG_CONFIG_HOME`；七个安全开关即使已存在也必须与入口固定值相同。若 shell 配置导出了 Homebrew 镜像或 proxy，应先在该次命令的隔离环境中移除，入口不会继承后再静默忽略。

推荐先创建专用私有目录，例如：

```sh
install -d -m 700 "$HOME/envmason-i21-evidence"
```

## 阶段一：prepare（只读）

把下面路径替换为仓库外的实际绝对路径：

```sh
env -i HOME="$HOME" PATH="$PATH" TMPDIR="${TMPDIR:-/tmp}" SHELL="${SHELL:-/bin/zsh}" \
  ENVMASON_I21_MODE=prepare \
  ENVMASON_I21_DISPOSABLE=I_UNDERSTAND_THIS_DISPOSABLE_VM_WILL_CHANGE \
  ENVMASON_I21_BUNDLE="$HOME/envmason-i21-evidence/bundle.json" \
  go test -tags envmason_live_i21 ./internal/baseapply \
  -run '^TestI21LiveHarness$' -count=1 -v
```

prepare 只执行固定的 macOS/Homebrew 只读查询，读取三层固定 `brew.env` 与 brew 可执行文件，GET 官方 `formula.json`，并对选中的官方 GHCR bottle 执行匿名 token challenge 和 HEAD。它不下载 bottle、不写 Homebrew cache、不创建 Operation，也不执行 `brew install`。

成功输出只包含脱敏的 Plan/Review 摘要、精确确认 token 和 bundle 路径。确认前应至少复核：

- target 是预期 macOS 版本和架构；
- action 只包含 Git/CMake，风险均为 R2；
- Plan ID、Review ID、过期时间、formula 版本、依赖数和总下载量符合预期；
- bundle 位于专用私有目录，且 VM 快照仍可恢复。

## 阶段二：apply（I21-F2，需另行授权）

仅在维护者明确确认 prepare 输出的精确 Plan ID 与 Review ID 后运行。把 `<EXACT_TOKEN_FROM_PREPARE>` 整体替换为 prepare 原样输出的 token；不要自行拼接或复用旧 token。

```sh
env -i HOME="$HOME" PATH="$PATH" TMPDIR="${TMPDIR:-/tmp}" SHELL="${SHELL:-/bin/zsh}" \
  ENVMASON_I21_MODE=apply \
  ENVMASON_I21_DISPOSABLE=I_UNDERSTAND_THIS_DISPOSABLE_VM_WILL_CHANGE \
  ENVMASON_I21_BUNDLE="$HOME/envmason-i21-evidence/bundle.json" \
  ENVMASON_I21_CONFIRM='<EXACT_TOKEN_FROM_PREPARE>' \
  go test -tags envmason_live_i21 ./internal/baseapply \
  -run '^TestI21LiveHarness$' -count=1 -v
```

apply 会在 Operation 或写进程前重新读取并验证私有 bundle，重新 GET 官方 catalog、重新取得 bottle HEAD 元数据，并重新采集 Inventory、brew 可执行文件和固定配置。Plan/Review 过期，或 Homebrew、配置、catalog、bottle、formula 状态任一漂移时都会停止。

通过全部复核后，入口只调用现有 I21-D 固定适配器：`brew install --formula --force-bottle git|cmake`。成功后它重新扫描 Inventory、调用 I21-E `Finalize`、验证第二次 Plan 为零安装动作，并在私有目录写入 Operation、最终 Lock 和 Outcome。

## 结束与恢复

- 保存需要审查的脱敏输出；bundle 和 Operation 目录仍含私有主机证据，不应提交到仓库或公开分享。
- 无论成功或失败，都记录 VM 快照标识和测试输出。
- 验收完成后恢复或销毁 VM。任何卸载/清理都属于新的 R3 操作，不能用本入口执行。
- 未完成真实 apply、最终 Lock、二次零动作和 VM 恢复前，不得把 I21 标记为完成或进入 I22。
