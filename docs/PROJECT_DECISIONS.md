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

### D-015：I05 Homebrew 只读适配器

- 状态：Accepted
- 事实依据：[Homebrew 官方手册](https://docs.brew.sh/Manpage)定义 `info --json=v2 --installed` 和 `outdated --json=v2` 的结构化只读查询形式，并提供 `--prefix`、`--repository`、`--cellar` 和 `--caskroom` 路径查询。
- 决定：I05 只调用固定白名单：`brew --version`、`--prefix`、`--repository`、`--cellar`、`--caskroom`、`info --json=v2 --installed`、`outdated --json=v2`，以及 `git -C <repository> remote get-url origin`；不接受外部命令参数，也不调用任何变更命令。
- 决定：`brew` 和 `git` 都必须由 I04 按请求中的 PATH 顺序确定实际可执行路径；报告字段只保留脱敏路径，未脱敏路径仅供确定性核心内部执行。
- 决定：所有查询设置 `HOMEBREW_NO_AUTO_UPDATE=1`、`HOMEBREW_NO_ANALYTICS=1` 和 `HOMEBREW_NO_ENV_HINTS=1`，使用结构化参数、30 秒超时、32 MiB stdout 与 64 KiB stderr 上限，不经 Shell 拼接。
- 决定：formula 与 cask 分别映射为统一 Tool/Installation；formula 的 `installed_on_request=true` 记为直接安装，否则记为依赖安装；`linked_keg` 用于表达生效和默认版本，keg-only 未链接版本保守记为未知。
- 决定：Homebrew 仓库远端移除 URL 用户信息、查询参数和 fragment；命令错误与原始 stderr 不进入结果，只输出固定 Finding，锁占用单独分类。
- 决定：I05 不新增 CLI 命令或公开 Schema，不运行 `brew update`、安装、升级、卸载、清理、tap/untap、换源或修复；报告整合属于 I08，变更能力属于后续增量。

### D-016：I06 Node.js 生态只读适配器

- 状态：Accepted
- 事实依据：[NVM 官方 README](https://github.com/nvm-sh/nvm)说明 NVM 是按用户、按 Shell 生效的版本管理器，会修改 PATH，并在 `$NVM_DIR/versions/node` 保存安装版本、通过 alias 表达默认版本；默认安装目录还受 `XDG_CONFIG_HOME` 影响。
- 事实依据：[Corepack 官方 README](https://github.com/nodejs/corepack/blob/main/README.md)说明 pnpm/Yarn 代理会按项目配置选择版本，缺失时可能访问网络并写入缓存；`COREPACK_ENABLE_NETWORK=0` 可禁止网络访问。Node.js 25 的[官方文档](https://nodejs.org/download/release/v25.8.0/docs/api/corepack.html)还明确 Corepack 从 Node.js 25 起不再随 Node 分发。
- 决定：I06 不 source Shell 配置、不调用 `nvm` 函数；通过明确的 `NVM_DIR`、`XDG_CONFIG_HOME/nvm` 或 `$HOME/.nvm` 候选，以只读文件遍历发现已安装版本和 default alias。
- 决定：NVM alias 只接受受限名称，单文件上限 4 KiB，递归解析深度上限 16；支持具体版本、数字前缀、`node`/`stable` 和 alias 链，循环、越界或无已安装匹配均降级为 Finding。
- 决定：Node 与 npm/Corepack/pnpm/Yarn 候选复用 I04 的 PATH 顺序、软链接和架构发现；内部同时保留未脱敏调用路径和解析目标，公开结果只使用 `$HOME` 脱敏路径。
- 决定：版本优先从 NVM 目录名或本地受限 `package.json` 元数据读取；必要执行只允许固定 `--version` 参数、10 秒超时和 64 KiB 双向输出上限，不经 Shell。
- 决定：版本进程使用最小受控环境，不继承 `NODE_OPTIONS`、npm token 或用户钩子；Corepack 网络、自动 pin、项目版本选择、下载提示和 latest 查询均关闭。识别为 Corepack 的 pnpm/Yarn 代理不执行，版本保守记为动态未知并记录 Corepack provider 版本。
- 决定：I06 内部模型显式记录当前 Node、NVM 默认版本、管理来源、PATH 生效状态以及每个包管理器所属 Node Installation ID；I08 再负责公开报告映射。
- 决定：I06 不新增 CLI 命令或公开 Schema，不安装/删除 Node，不修改 NVM alias，不运行 Corepack enable/disable，不升级全局包，也不执行网络请求或配置写入。

### D-017：I07 Java 生态只读适配器

- 状态：Accepted
- 事实依据：macOS 自带 `java_home(1)` 的 `-X` 选项以 XML plist 列出匹配 JVM 及其属性；默认调用只返回适合 `JAVA_HOME` 的路径，`--exec` 才会执行 JDK 工具，I07 不使用后者。
- 事实依据：[jenv 官方 README](https://github.com/jenv/jenv)说明 jenv 不安装 Java，只登记既有 JDK；版本选择优先级为 shell、local、global，local 通过项目或父目录中的 `.java-version` 表达。
- 事实依据：[Maven 官方 CLI 参考](https://maven.apache.org/ref/3.9.6/maven-embedder/cli.html)定义 `--version` 只显示版本信息；Gradle 即使仅查询版本，也可能在首次运行时初始化用户目录，因此 I07 不执行 Gradle，而从已安装分发包的本地元数据和现有 `gradle.properties` 读取有限信息。
- 决定：系统注册 JDK 使用固定 `/usr/libexec/java_home -X` 结构化输出；Homebrew JDK 只读遍历明确前缀下的 `opt/openjdk*`/`opt/java` 链接并解析 JDK `release` 文件，按规范化 home 路径去重。
- 决定：jenv 不通过命令查询，更不调用 add/global/local 等写操作；只读解析 `versions` 注册链接、根 `version`、请求显式提供的 shell 版本和从实际存在工作目录向父级查找的最近 `.java-version`。失效或循环链接产生 Finding。
- 决定：实际 `java` 只允许 `-XshowSettings:properties -version`；只保留 `java.home`、`java.version`、`java.vendor` 和 `os.arch` 白名单字段，忽略其余属性及原始输出。
- 决定：Maven 只允许 `--version` 并设置 `MAVEN_SKIP_RC=1`；Gradle 不执行任何命令或 Wrapper，其版本从分发目录的 `gradle-core-*.jar` / `gradle-runtime-api-info-*.jar` 文件名读取，JVM 选择仅从已有用户级或项目级 `gradle.properties` 的 `org.gradle.java.home`、显式 `JAVA_HOME` 或当前 Java 推导。允许执行的命令使用 15 秒超时、512 KiB 合并输出上限和最小受控环境，不继承 `JAVA_TOOL_OPTIONS`、`MAVEN_OPTS` 或用户秘密。
- 决定：内部模型分别记录 JDK 安装、当前 Java、JAVA_HOME、jenv 选择、Maven runtime 与 Gradle Launcher/Daemon JVM；JAVA_HOME、jenv、Maven 或 Gradle 与当前 Java 不一致时产生独立 Finding，单项失败不阻断其他字段。
- 决定：I07 不新增 CLI 命令或公开 Schema，不安装、升级或删除 JDK，不修改 jenv/JAVA_HOME/项目文件，不运行构建任务、Wrapper、Daemon 管理或网络请求；I08 再负责公开报告映射。

### D-018：I08 macOS 综合只读报告接口

- 状态：Accepted（维护者于 2026-07-17 明确确认）
- 决定：公开 `envmason report`，默认输出终端摘要；`--format summary|markdown|json` 选择格式，`--category` 和 `--severity` 可重复使用。
- 决定：同一过滤维度内多个值按 OR，类别与严重程度之间按 AND；系统信息始终保留，类别过滤作用于 Tool 及其关联 Finding，严重程度过滤作用于 Finding。
- 决定：部分适配器失败仍生成报告并返回成功；整个 section 失败使用 `REPORT_SECTION_FAILED`，任一关键探测失败增加 `REPORT_INCOMPLETE`，共同显著标记不完整。只有平台不支持、无法建立扫描上下文、无法编码或无法写出报告等整体失败返回非零。
- 决定：三种渲染器只消费同一个 `inventory.Inventory`；JSON 继续使用公开 Inventory Schema `0.2.0` 并在输出前本地校验，不升级 Schema。现有 Schema 没有专门字段的 NVM/jenv 选择和 Maven/Gradle Java runtime 事实映射为 `info` Finding。
- 决定：扫描范围固定为系统、PATH、Homebrew、Node.js/NVM/npm/Corepack/pnpm/Yarn、Java/JDK/jenv/Maven/Gradle；扫描时间、范围、失败项和来源均进入报告。输出仅写 stdout，保存由 Shell 重定向完成。
- 决定：I08 仅编排 I03～I07 的只读能力，不联网查询最新版或 EOL，不比较版本，不生成更新建议，不修改配置或系统；版本规范化与比较仍属于 I09。

### D-019：I09～I10 一小时安全微批次

- 状态：Accepted（维护者于 2026-07-17 明确确认）
- 决定：本批次仅串行推进 I09～I10，作为 D-014 每批 3～5 个增量的一次时间盒例外；任何增量未通过本地门禁和远程 CI 时不得进入下一增量，达到一小时时保留当前安全进度并暂停，绝不进入 I11。
- 决定：I10 默认本地报告不联网，只有维护者确认的 `envmason report --online` 显式入口可以访问远程只读数据源。
- 决定：I10 不允许默认或在线报告写入磁盘；只建立可注入缓存契约、fresh/stale/corrupt 策略和已有缓存的只读行为，生产持久化缓存写入延后到具备 Plan 的后续增量。
- 决定：Node 使用 Node.js 官方 release index 与 Release 工作组 schedule；Java 使用 Adoptium available releases，Temurin 生命周期只适用于能确认属于 Temurin 的数据，其他厂商 EOL 保守为 Unknown。
- 决定：发布索引 TTL 为 6 小时，生命周期 TTL 为 24 小时；来源并发查询且单来源 5 秒超时、2 MiB 响应上限。过期数据必须标 stale，不能表达为“已确认最新”。

### D-020：I09 通用版本规范化与比较

- 状态：Accepted
- 决定：I09 建立独立确定性核心，不新增 CLI、网络请求或公开 Schema；输出保留原始值、规范化值、scheme 和显式 Comparable 状态。
- 决定：SemVer 遵循 2.0.0 优先级规则并接受 Node 常见小写 `v` 前缀；build metadata 不影响比较，非法前导零和不完整版本返回 Unknown。
- 决定：Java 支持现代数值版本、`1.8.0_361`、`8u361`、`-ea`、build number 及受限厂商/支持标签；Java GA 的厂商 build 不用于跨厂商更新排序，EA build 可用于同一 EA line 排序。
- 决定：跨 scheme、非法、歧义或超长输入一律返回 Unknown，不进行字符串兜底排序；I09 不解析 npm/Maven 范围，也不生成升级或清理结论。

### D-021：I11 显式项目引用扫描

- 状态：Accepted（维护者于 2026-07-17 明确确认）
- 事实依据：[NVM 官方 README](https://github.com/nvm-sh/nvm#nvmrc)定义 `.nvmrc` 作为项目 Node 版本声明；[npm 官方 package.json 文档](https://docs.npmjs.com/cli/v11/configuring-npm/package-json/#engines)定义 `engines.node`；[asdf 官方配置文档](https://asdf-vm.com/manage/configuration.html#tool-versions)定义 `.tool-versions`；[Maven Compiler Plugin 官方文档](https://maven.apache.org/plugins/maven-compiler-plugin/examples/set-compiler-release.html)定义 `maven.compiler.release`，并保留 `source`/`target` 属性；[Gradle 官方 JVM Toolchains 文档](https://docs.gradle.org/current/userguide/toolchains.html)定义静态 Java toolchain 声明。
- 决定：本批次作为 D-014 的一小时时间盒例外，只实施 I11；达到一小时即保留安全进度并暂停，I11 的本地门禁和远程 CI 未通过前绝不进入 I12。
- 决定：公开可重复 `report --project <目录>` 与 `--exclude <相对子树>`；未提供 `--project` 时不访问项目目录，单独使用 `--exclude` 是用法错误。项目扫描始终本地只读，可与显式 `--online` 正交组合。
- 决定：只读取 `.nvmrc`、`.node-version`、`package.json` 的 `engines.node`、`.java-version`、`.tool-versions` 的 Node/Java 条目、`pom.xml` 静态 Java 属性及 Gradle 静态 compatibility/toolchain 表达；不执行构建工具、脚本、Wrapper 或项目命令。
- 决定：固定忽略版本库内部目录、依赖目录和常见构建产物，不跟随目录或文件符号链接；用户排除按每个根目录内精确相对子树处理。目录深度、目录数、文件数和单文件大小均有确定上限，超限或部分失败显式降级。
- 决定：建立内部 Project→Runtime→Constraint→Source 关系；I11 通过现有 Finding 表达引用和冲突，不升级 Inventory Schema。只报告精确版本不等价或简单 Node 范围与精确版本确定不相容的冲突；动态、复杂或无法解析的声明为 Unknown，不回显未知原文。

### D-022：I12～I13 两小时只读评估与 Plan 预览批次

- 状态：Accepted（维护者于 2026-07-17 明确确认）
- 决定：本批次仅串行实施 I12～I13，作为 D-014 每批 3～5 个增量的两小时时间盒例外；I12 的本地门禁和远程 CI 未通过前不得进入 I13，达到时间盒时保留安全进度并暂停，绝不进入 I14。
- 决定：I12 的规则核心直接消费结构化 Inventory、VersionData、ProjectReference、显式 Policy 和扫描期保留的 Java vendor，不从自然语言 message 反向推断版本；每项建议必须给出 status、evidence、confidence、recommendation 和 impact。
- 决定：Inventory Schema 升级到 `0.3.0`，Finding 新增可选的 `status`、`recommendation` 和 `impact`；继续原样保留并支持验证 `0.2.0` 与 `0.1.0`，不修改历史 Schema 文件。
- 决定：公开 `report --policy <file>`，仅显式读取版本化严格 JSON，不自动发现默认配置；文件上限 64 KiB，未知字段、工具、通道和非法 Pin 被拒绝。首批工具仅为 `runtime.node` 与 `runtime.java`，通道仅为 `lts`/`stable`。
- 决定：`ignore_updates` 只抑制普通更新建议，不隐藏 EOL、项目保留或冲突；只有 fresh 官方数据或用户明确 Pin 可以产生确定更新比较，stale/unavailable/不可比较数据必须输出 Unknown。Temurin 生命周期只应用于扫描期能确认 vendor 的 JDK。
- 决定：项目精确引用匹配既有安装时输出 `retain_required`，无论是否存在更新都不得建议删除；多来源、active/default 不一致以及 Maven/Gradle/Shell Java 不一致输出独立可解释冲突。
- 决定：I12 全程只读，不生成 Plan，不执行命令，不修改策略或系统。I13 的 Plan 接口和 Schema 按本批次已确认契约在 I12 验收后实施；I14 的执行器和操作历史存储仍不属于本批次。

### D-023：I13 不可执行 Plan/Action 契约

- 状态：Accepted（依据维护者对 D-022 接口与 Schema 的明确确认）
- 决定：公开 `envmason plan --tool runtime.node --online --format summary|json`；可复用显式 `--policy`、`--project` 与 `--exclude` 只读输入。I13 只接受 `runtime.node`，必须显式 `--online`，无合格建议返回操作失败而非生成空计划。
- 决定：Plan Schema 首版为 `0.1.0`，固定 `executable=false`；Plan 包含内容派生 SHA-256 ID、UTC 创建时间、恰好 30 分钟的过期时间、环境/策略 digest、环境摘要和至少一个 Action。
- 决定：首个 Action 只表达 NVM `install_version`，不携带 command、args、Shell 或可调用 executor；NVM 必须已存在，目标必须来自 fresh LTS/Stable 建议，显式 Pin 还必须在 fresh Node.js 官方 release index 中精确验证。
- 决定：Node 安装属于 R2，要求计划级确认，不提权、不要求重启；Schema 显式表达下载量 Unknown、依赖、前置条件、验证及手动恢复说明。风险可以由后续平台策略上调但不能低于 R2。
- 决定：Plan ID 根据除 ID 外的完整规范化内容计算；Action、风险、验证、恢复、环境、策略或时间变化后旧 ID 校验失败，必须产生新 Plan。I13 不保存 Plan，不接受确认，不提供 apply，不创建操作日志。
- 决定：环境 digest 覆盖 Node 安装 ID、版本、路径、manager、active/default 状态及相关系统摘要；策略 digest 覆盖解析并补齐默认值后的 Policy。后续执行前必须重新验证，但执行器属于 I14 之后范围。

### D-024：I14 受控执行、确认与本地操作记录契约

- 状态：Accepted（维护者于 2026-07-17 明确确认推荐方案）
- 决定：保留 I13 Plan Schema `0.1.0` 及其 `executable=false` 语义；新增 Plan Schema `0.2.0` 表达可执行的声明式 R1/R2 Plan。两个版本均不允许 command、args、Shell 或可执行路径字段，任何动作必须通过 `(tool_id, operation, adapter)` 命中确定性核心的内置注册表，且注册表最低风险只能上调、不能下调。
- 决定：I14 不改变公开 `envmason plan` 命令，它继续输出 I13 的不可执行预览。I14 只提供内部受控执行核心和一个项目自带的无害 R1 `envmason version` 测试动作；测试动作仍要求绑定完整 Plan ID、确认时间和 `scope=plan` 的计划级确认，不开放 apply、任意命令或真实包管理器写操作。
- 决定：执行器只使用绝对可执行路径和结构化参数直接启动进程，不解析 Shell 字符串；动作注册表固定提供参数、环境、工作目录、最长 30 秒超时、前置检查和验证器。stdout 与 stderr 分别最多保留 64 KiB，超限显式标记，敏感值和常见 Token/Password/Secret/Authorization 赋值在持久化前替换为 `[REDACTED]`。
- 决定：Operation Record Schema 首版为 `0.1.0`；记录 Operation ID、Plan ID 与版本、确认凭据、动作身份与风险、已脱敏调用信息、前置检查、输出、退出码、标准化错误、验证和完整状态迁移。只有进程成功且注册验证器通过才能成为 Completed；遗留 Running/Verifying 记录只能恢复为 Interrupted，不能推断为成功。
- 决定：I14 操作历史采用本地版本化 JSON，不引入 SQLite。每次状态更新先在同目录写入权限受限的临时文件并安全替换当前记录；macOS 使用 `~/Library/Application Support/EnvMason/operations`，Windows 使用 `%LOCALAPPDATA%/EnvMason/operations`，Linux 使用 `$XDG_STATE_HOME/envmason/operations` 或 `~/.local/state/envmason/operations`。Unix 目录/文件权限分别为 `0700`/`0600`，符号链接目的地被拒绝。
- 决定：在 I18 前根据历史查询、并发、迁移和 GUI 需求重新评估 SQLite；I14 不提供删除历史的公开入口，后续删除或恢复仍必须遵循对应增量的 Plan 和确认语义。

### D-025：I15 单个 NVM Node 安装接口与固定 Shell 例外

- 状态：Accepted（维护者于 2026-07-17 明确确认推荐方案）
- 事实依据：[NVM 官方 README](https://github.com/nvm-sh/nvm/blob/master/README.md)说明 `nvm` 是 sourced Shell function 而不是独立可执行文件，正确存在性检查使用 `command -v nvm`；NVM 的[固定版本脚本](https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.4/nvm.sh)支持 `--no-use`、二进制安装 `-b`、`--skip-default-packages` 和 `--no-progress`。
- 决定：I15 公开 `envmason apply --tool runtime.node --version <精确稳定版本> --online [--dry-run]`。目标必须高于当前生效版本，并精确存在于本次 fresh Node.js 官方 release index；I13 的 `envmason plan` 继续输出不可执行 Plan `0.1.0`。
- 决定：dry-run 只在内存中构建并显示 Plan `0.2.0`，不确认、不执行且不创建操作记录。真实 apply 显示同一不可变 Plan，用户必须在交互终端逐字输入 `apply <完整 Plan ID>`；不提供 `--yes`、配置静默授权、环境变量授权或 AI 代确认，错误输入、EOF 和非交互输入均拒绝且不执行。
- 决定：I15 只公开支持 macOS、已存在且可读取 default alias 的 NVM 和单个 Node 版本安装；不安装 NVM，不修改 default alias，不切换当前 Shell，不升级 npm/pnpm，不删除旧版本，不清理失败残留，也不提权。Linux/Windows 只维持编译和 fixture 兼容，不声明可执行支持。
- 决定：为 NVM 建立唯一固定 Shell 例外：只调用绝对路径 `/bin/bash --noprofile --norc`，脚本文本编译在内置适配器中，只 source 已确认的 `$NVM_DIR/nvm.sh --no-use` 并执行固定 `nvm install -b --skip-default-packages --no-progress <精确版本>`。目录和版本只作为位置参数传递，Plan、AI 和用户输入均不能提供 Shell 文本、可执行路径或附加参数。
- 决定：Plan 记录 `nvm.sh` 与 default alias 的 SHA-256；二者必须是非符号链接普通文件，并在确认后、进程启动前重新校验。适配器只传入 HOME、NVM_DIR、固定系统 PATH、临时目录和标准代理变量，不读取 Shell profile，不继承 NVM mirror、认证、第三方 hook、`BASH_ENV`、`NODE_OPTIONS` 或包管理器秘密；主目录、NVM 目录、临时路径和代理值在操作记录中脱敏。
- 决定：NVM 动作超时为 10 分钟，执行器注册上限扩展为 15 分钟；Unix 取消或超时时终止整个新进程组，避免 curl/tar 后代残留。失败不自动删除部分目录或缓存，因为该清理会形成新的 R3 操作。
- 决定：Operation Record 当前版本升为 `0.2.0`，记录动作执行前后事实快照、内容 digest、确定性差异和幂等跳过状态；完整保留 `0.1.0` Schema 和读取验证能力。目标已安装时不再次运行 NVM，但仍执行验证并写入新的已确认操作记录。

### D-026：I16 Node 默认版本切换与恢复契约

- 状态：Accepted（维护者于 2026-07-20 明确确认推荐方案）
- 事实依据：[NVM 官方 README](https://github.com/nvm-sh/nvm/blob/master/README.md#set-default-node-version)定义 `nvm alias default <version>` 为新 Shell 的默认 Node 选择；NVM 是按用户、按 Shell 加载的 sourced function，实际新 Shell 效果依赖 profile 正确加载 `nvm.sh`。
- 决定：本批次只实施 I16，不进入 I17。公开 `envmason default set --tool runtime.node --version <已安装精确版本> [--dry-run]` 与 `envmason default restore --operation <Operation ID> [--dry-run]`；不要求联网，不安装目标或 NVM。
- 决定：新增 Plan Schema `0.3.0`，只允许单个 Node/NVM `set_default` 或 `restore_default` R3 Action，不提权、不下载。完整保留 `0.2.0` 和 `0.1.0`。设置和恢复分别要求逐字输入 `set-default <完整 Plan ID>` 和 `restore-default <完整 Plan ID>`；单动作 Plan 的计划级确认即为该 R3 动作的显式逐项确认。
- 决定：将 D-025 的固定 NVM Shell 例外扩展到 default alias 动作。只运行内置固定脚本、绝对路径 `/bin/bash --noprofile --norc`、已校验 digest 的 `nvm.sh` 和位置参数；不接受用户或 AI 提供的 Shell 文本、路径或附加参数，不读取 Shell profile。
- 决定：默认 alias 只接受 NVM 规范单行格式；设置目标固定为 `vX.Y.Z`，不使用会漂移的 `node`、主版本或次版本前缀。执行前重新扫描并校验 Inventory、`nvm.sh` 和 alias digest；动作后验证 alias 值、解析版本、目标可执行性、当前 Shell 版本未变及隔离新 Shell 默认版本。
- 决定：Operation Record 继续使用 `0.2.0`，快照记录原/新 alias、解析版本和 digest。验证失败不自动回滚；恢复必须从源记录生成新 Plan ID 并再次 R3 确认。当前 alias 与源操作的 after 快照不一致时拒绝覆盖；不支持自动、AI 或配置无人值守恢复。

### D-027：I17 Node 附属工具更新契约

- 状态：Accepted（维护者于 2026-07-28 确认两小时时间盒方案并指示开始开发）
- 事实依据：[npm 官方文档](https://docs.npmjs.com/try-the-latest-stable-version-of-npm)使用全局 `npm install npm@latest -g` 更新 npm；I17 将浮动的 `latest` 收紧为用户明确审查的精确稳定版本，并固定目标 Node 的 npm、prefix、registry 与安全参数。
- 事实依据：[Corepack 官方 README](https://github.com/nodejs/corepack)规定 Corepack 本身通过 npm 安装或更新，`corepack install --global <name@version>` 更新项目外使用的 Known Good Release；`COREPACK_HOME` 默认是用户级缓存，`COREPACK_ENABLE_PROJECT_SPEC=0`、`COREPACK_DEFAULT_TO_LATEST=0`、`COREPACK_ENV_FILE=0` 和 `COREPACK_ENABLE_NETWORK` 可分别控制项目约束、远程漂移、环境文件与联网。
- 事实依据：[pnpm 官方 self-update 文档](https://pnpm.io/cli/self-update)说明 `pnpm self-update` 在项目上下文可能更新 `package.json`，全局模式又取决于安装上下文；I17 不使用该命令，避免修改项目或把更新路由到错误的全局安装。
- 决定：公开 `envmason update node-tools --node-version <已安装精确版本> [--npm <精确版本>] [--corepack <精确版本>] [--pnpm <精确版本>] [--dry-run]`。至少选择一项，省略即排除；不接受 `latest`、版本范围、降级、隐式当前 Node、Yarn、`--yes` 或非交互确认。目标等于当前版本时允许生成幂等验证 Plan。
- 决定：I17 复用 Plan `0.2.0`、Operation Record `0.2.0` 和现有确定性执行器，不新增公开 Schema。每个选中工具是独立 R2 Action，按 npm、Corepack、pnpm 的实际提供方建立依赖并统一要求 `apply <完整 Plan ID>` 计划级确认；失败后停止，后续动作保持 Pending。继续 Plan、跨运行检查点和恢复 Plan 仍属于 I18。
- 决定：目标必须是已有 NVM Node。执行前绑定并复核 Inventory、`nvm.sh`、default alias、目标 Node 根目录、目标工具软链接解析、package.json 名称/版本/摘要及提供方；软链接或包入口解析到目标 Node 根目录外时拒绝。Corepack 代理 pnpm 的当前版本通过关闭联网和项目选择的目标代理 `--version` 读取，以阻止隐式降级。执行后通过目标工具的绝对路径验证精确版本，同时验证目标 Node、当前生效 Node 和 default alias 未改变。
- 决定：npm、Corepack 和独立 pnpm 只通过目标 Node 的 npm 运行固定 `install --global` 模板，使用固定官方 registry、目标 Node prefix、关闭 lifecycle scripts/audit/fund/update notifier，并忽略用户和全局 npm 配置。Corepack 代理 pnpm 只通过目标 Node 的 Corepack 运行固定 `install --global pnpm@<精确版本>`，关闭项目规范、auto-pin、远程 latest 漂移、自定义 URL 和 `.corepack.env`。
- 决定：Corepack 的 Known Good Release 按其官方契约存放在用户级 `COREPACK_HOME`，不是 NVM 版本私有缓存。I17 的 Node 作用域保证是“命令、代理所有权、bin 目录和验证都绑定目标 Node”，不承诺 Corepack 缓存在不同 NVM Node 之间隔离；Plan 必须显式标记 `corepack` provider，且动作不得改写其他 Node 的 `bin` 目录。
- 决定：I17 不安装 NVM/Node，不修改 default alias 或当前 Shell，不迁移任意全局 npm 包，不修改项目 `package.json`/lockfile，不读取用户包管理器配置或认证，不支持 Homebrew/standalone/未知 provider，不清理缓存或失败残留，不自动回滚，不提权。

### D-028：I18 分段交付与失败隔离基线

- 状态：Accepted（维护者于 2026-07-30 接受一小时时间盒方案，授权在 I17 远程门禁通过后继续开发）
- 决定：I18 先交付最小 I18-A，不在一个时间盒中同时引入混合风险 Plan、跨运行检查点、继续 Plan 和恢复 Plan。I18-A 的用户价值是证明多动作流程中任一步失败都不会启动其后续依赖，且动作进程成功但验证失败时不能产生 Completed。
- 决定：I18-A 复用 Plan `0.2.0`、Operation Record `0.2.0` 和现有确定性执行器，以 npm → Corepack → pnpm 三动作 R2 DAG 建立六场景失败注入矩阵：每一步分别注入进程失败和验证失败。每个场景必须同时证明上游已验证完成、当前动作失败、下游保持 Pending 且没有启动痕迹、终态失败已持久化并通过 Operation Record 语义校验。
- 决定：若现有执行器已满足上述契约，I18-A 只增加回归证据，不为制造代码改动而重写核心。新增公开 Schema、CLI、写适配器或确认语义均不属于本增量。
- 非范围：从中断记录生成继续/恢复 Plan、跨运行检查点重新验证、环境关键状态漂移策略、R2/R3 混合 DAG 的逐项确认、“安装 Node → 切换默认 → 更新工具”完整流程和通用跨管理器事务。以上仍需在后续 I18 增量中分别冻结契约。

### D-029：I18-B 确认 Plan 来源固化

- 状态：Accepted（维护者于 2026-07-30 确认一小时增量方案并指示开始开发）
- 决定：Operation Record 当前 Schema 升为 `0.3.0`，每条新记录必须保存执行前通过内容校验并由确认凭据绑定的完整 `confirmed_plan`。记录中的 Plan ID、Plan Schema 版本、确认凭据、步骤数量、拓扑顺序和动作身份必须与该 Plan 一致；任一不一致均拒绝持久化或读取。
- 决定：执行器对确认 Plan 做独立深拷贝，操作记录不得与调用方后续可变切片或指针共享状态。`confirmed_plan` 仍只包含已有声明式 Plan 字段，不加入命令、参数、Shell 或执行规范；进入 Plan 前，NVM、HOME 和临时目录下的安装路径分别规范化为 `$NVM_DIR`、`$HOME` 和 `$TMPDIR` 占位符，Plan ID 绑定规范化后的内容。若确认 Plan 仍命中请求或注册执行规范声明的敏感值，必须在写入操作记录前拒绝执行。
- 决定：Operation Record `0.2.0` 和 `0.1.0` 保持完整读取与语义验证能力，但因缺少可由 Plan ID 复核的完整来源 Plan，不得作为未来通用继续 Plan 的来源。I16 已有的单动作 default restore 仍按其专用快照契约工作，不被本决定追溯禁止。
- 非范围：本增量不生成继续/恢复 Plan，不重新扫描或验证检查点，不增加 CLI，不升级 Plan Schema，不定义 R2/R3 混合 DAG 确认，也不执行新的系统写操作。

### D-030：I18-C 检查点资格判定

- 状态：Accepted（维护者于 2026-07-30 确认三小时时间盒方案并指示开始开发）
- 决定：继续 Plan 生成前必须先完成独立的只读资格判定。来源只接受带完整 `confirmed_plan` 的 Operation Record `0.3.0`，且必须是 Failed、TimedOut、Cancelled 或 Interrupted 终态；活动记录必须先经 `RecoverInterrupted` 固化，Completed、旧 Schema、篡改记录或没有剩余动作的记录均不得产生可继续结论。
- 决定：只有原记录中状态为 Completed、验证为 Passed、具有合法 After Snapshot，且其依赖检查点已经通过新鲜复核的动作，才能列为可复用检查点。Pending、Failed、TimedOut、Cancelled 和 Interrupted 动作始终列入待执行集合，不得因当前环境看似已满足目标而追认成已完成。
- 决定：检查点复核由确定性注册表中的只读 `RevalidateCheckpoint` 能力完成；核心不得调用动作 `Build`、启动写执行或写入历史。复核器必须按动作作用域比较目标版本、provider、所有权、控制摘要和关键环境不变量，返回不含原始路径或命令输出的证据摘要。为取得精确工具版本，适配器可以复用已有受控只读版本探测。
- 决定：不直接比较完整 After Snapshot，因为后续合法动作可能改变同一广域快照中的其他工具字段。任一已完成检查点缺少复核器、缺少快照、证据非法或发生漂移时，整个继续候选按稳定原因码阻断；本增量不允许独立分支绕过漂移继续。
- 非范围：本增量不新增 Plan 或 Operation Record Schema，不生成、确认或执行继续/恢复 Plan，不增加 CLI，不支持 R2/R3 混合 DAG，不自动回滚，不迁移历史存储，也不声称完成 FR-046。

### D-031：I18-D 继续 Plan 来源绑定与执行隔离

- 状态：Accepted（维护者于 2026-07-30 确认开始并继续）
- 决定：新增 Plan Schema `0.4.0` 表达审查态继续 Plan，固定 `executable=false`。Plan 必须绑定来源 Operation ID、来源 confirmed Plan ID、在来源终态之后重新准备的 Plan `0.2.0` ID、来源动作顺序、动作级检查点的记录/新鲜观察 digest，以及由检查点满足而从待执行 DAG 中移除的依赖边；以上任一内容变化都会改变 Plan ID。
- 决定：继续 Plan 只接受 Operation Record `0.3.0` 中 Failed、TimedOut、Cancelled 或 Interrupted 的来源，来源 confirmed Plan 当前只允许 R1/R2 Plan `0.2.0`，并要求 I18-C 资格判定为 Eligible 且与来源身份、动作分区和检查点证据完全一致。首个动作失败时允许零个复用检查点，但仍绑定来源并重新准备全部动作。
- 决定：不得从失败记录直接复制可能过期的剩余动作。调用方必须在来源终态之后重新扫描并生成一份只含剩余动作的 Plan `0.2.0`；动作 ID、tool、operation、adapter、精确目标、风险和执行约束身份必须与来源一致，待执行动作间依赖必须保留，指向已复核动作的依赖必须显式转成 checkpoint-satisfied dependency。Plan `0.4.0` 复用新 Plan 的环境、策略和 30 分钟时窗，不延长其有效期。
- 决定：现有通用 Executor 只接受 Plan `0.2.0`/`0.3.0`，必须在注册表解析、Operation Record 写入和进程启动前拒绝 Plan `0.4.0`。继续 Plan 生成只做确定性内存计算，不调用 Action Build、写历史或执行系统修改。
- 非范围：本增量不增加公开 CLI，不确认或执行继续 Plan，不升级 Operation Record，不支持 R3/R4、混合风险 DAG、自动回滚、恢复 Plan、完整“安装 Node → 切换默认 → 更新工具”流程或通用跨管理器事务。

### D-032：I18-E Node tools 继续 Plan 端到端只读准备

- 状态：Accepted（维护者于 2026-07-30 确认三小时时间盒方案并指示开始）
- 决定：新增 Node tools 内部 `PrepareContinuation` 服务，从确定的 Operation ID 只读加载 Operation Record `0.3.0`，严格验证来源 confirmed Plan 为只含 npm、Corepack、pnpm `update_version` 动作的 R1/R2 Plan `0.2.0`。Node 版本、精确工具目标和 provider 必须从来源 Plan 一致提取，调用方不得用新参数扩大或替换目标。
- 决定：服务必须在一次当前环境扫描上完成目标 NVM Node 检查、工具归属检查、必要的 Corepack-managed pnpm 只读版本探测、I18-C 检查点复核和剩余 Plan 重新准备。失败步骤即使当前状态已经等于目标，仍属于剩余动作；新 Plan 只更新其 current-state 前置事实，不能追认历史完成。
- 决定：重新准备的 Plan `0.2.0` 只包含资格判定的剩余动作，创建时间取本次服务时间且必须晚于来源终态；随后由 I18-D 核心生成不可执行 Plan `0.4.0`。实现可以抽取 I17 `Prepare` 的单次扫描纯辅助逻辑，但相同输入的既有 I17 Plan 和执行语义不得变化。
- 决定：本服务只允许历史读取、文件元数据检查和受控 `--version` 类探测；不得调用 Action Build、执行包管理器写动作、保存 Operation Record 或创建新的历史文件。漂移、旧/活动/完成/篡改记录、非 Node tools 来源或无剩余动作均阻断。
- 非范围：本增量不增加 CLI，不确认或执行 Plan `0.4.0`，不升级 Operation Record，不支持 R3/R4、混合风险 DAG、恢复 Plan、自动回滚或完整 Node 工作流。

### D-033：I18-F Node tools 继续 Plan 执行前只读复核

- 状态：Accepted（维护者于 2026-07-30 授权两小时时间盒内在客观验收后自动规划并执行下一最小增量）
- 决定：新增 Node tools 内部 `RevalidateContinuation` 服务，输入必须是通过不可变内容校验的审查态 Plan `0.4.0`。服务先按当前时间拒绝尚未生效或已过期的 Plan，再从其绑定的来源 Operation ID 重新执行 I18-E 的只读准备流程。
- 决定：复核必须复用已审查 Plan 的原 `created_at`，从而在来源、检查点、当前环境和剩余动作均未变化时重建相同 Plan ID；不得通过刷新创建时间或有效期让旧审查继续有效。重建 ID 不一致时只返回重新生成 Plan 的稳定提示。
- 决定：过期、不可变内容非法或非 Plan `0.4.0` 的输入必须在加载历史和扫描环境前拒绝；来源或检查点漂移继续沿用 I18-C/I18-E 的阻断语义。任一路径不得保存历史、调用 Action Build 或启动写动作。
- 非范围：本增量不增加公开 CLI、确认或执行入口，不改变 Plan/Operation Record Schema，不支持 R3/R4、恢复 Plan、自动回滚或完整 Node 工作流。

### D-036：I18-I 真正继续执行的 Schema 与确认对象

- 状态：Accepted（维护者于 2026-07-30 明确批准方案 A）
- 已确认约束：Plan `0.4.0` 已冻结为 `executable=false`，现有 Executor 和 Operation Record `0.3.0` 只接受可执行 Plan `0.2.0`/`0.3.0`；Operation Record 要求 `plan_id`、`confirmed_plan` 和确认凭据全部指向同一不可变 Plan。真正继续执行不能复用来源操作的旧确认，也不能让确认 ID 与持久化执行 Plan ID 不同。
- 方案 A（推荐）：新增可执行继续 Plan `0.5.0`，保留来源 Operation/Plan、重新准备的 Plan、检查点和 satisfied dependency 绑定，固定只允许 R1/R2 剩余动作并要求新的 Plan 级确认；新增 Operation Record `0.4.0`，保存完整 confirmed Plan `0.5.0` 和仅本次剩余动作的步骤。Plan `0.4.0` 保持只读兼容，不原地改变语义。
- 方案 A 的确认对象：Plan `0.5.0` 是最终呈现并确认的唯一执行 Plan；Plan `0.4.0` 只是确定性只读草案，不收集确认。`0.5.0` 可由合格草案确定性提升并重新计算 ID，但无需把草案 ID 作为第二确认对象；其自身已完整包含来源、检查点、剩余动作和新鲜环境事实。
- 方案 A 的记录兼容：Operation Record `0.4.0` 的 `confirmed_plan` 允许当前可执行 Plan `0.2.0`、`0.3.0` 和 `0.5.0`，普通新操作与继续操作统一写当前记录版本；`0.3.0` 继续按原 Schema 读取验证并可作为首次继续来源，`0.2.0`/`0.1.0` 继续只读兼容但不能作为通用继续来源。不得修改任何历史 Schema 文件。
- 方案 A 的重复继续：首次来源可为 Record `0.3.0` + Plan `0.2.0`；继续执行失败后，Record `0.4.0` + Plan `0.5.0` 也必须可作为下一次来源。新 Plan 绑定直接来源，前一轮复用检查点通过 confirmed Plan 的 continuation provenance 保持可追溯链，不复制或改写更早记录。
- 方案 A 的实现边界：先只实现内部 Node tools Plan `0.5.0` 提升、Record `0.4.0` 兼容和重复继续的确定性核心，不决定公开 CLI；实际写执行作为其后的独立增量，执行前必须在同一调用中用原时间窗口重建并比较最终 Plan `0.5.0` 完整 ID，再由现有固定适配器执行剩余动作。
- 方案 B（不推荐）：确认 Plan `0.4.0` 后派生 Plan `0.2.0` 执行。该方案会让用户确认 ID 与执行记录 Plan ID 不同；若要求第二次确认，又会丢失本次记录中的继续来源语义，因此不采用除非维护者明确接受双 Plan、双确认及审计缺口。
- 方案 C（不推荐）：将 Plan `0.4.0` 原地改为可执行并让 Operation Record `0.3.0` 接受它。该方案破坏已发布 Schema 的 `executable=false` 契约、I18-D 执行隔离和历史兼容原则，因此不建议。
- 方案 A 获批后的最小增量顺序：
  1. I18-I1 只新增 Plan `0.5.0` Schema、严格语义校验和从合格 Plan `0.4.0` 确定性提升的纯函数；Executor 继续拒绝 `0.5.0`，不写历史、不执行动作。
  2. I18-I2 新增 Operation Record `0.4.0` Schema 与 `0.3.0`/`0.2.0`/`0.1.0` 兼容解码，允许 confirmed Plan `0.5.0`，并扩展只读来源判定支持首次和重复继续；Executor 仍不执行 `0.5.0`。
  3. I18-I3 才实现 Node tools 内部继续执行：确认绑定最终 Plan `0.5.0`，同一调用内重新复核最终 ID，再解析固定注册表并写新的 Record `0.4.0`。公开 CLI 继续保持 N/A。
- 方案 A 的客观门禁：旧 Plan/Record Schema fixture 必须逐版本继续通过；Plan `0.5.0` 的来源、检查点、目标、动作顺序、依赖、时间或环境任一变化必须改变 ID；错误确认、过期、来源漂移和复核 ID 不同必须在注册表解析、历史创建和进程启动前拒绝；首次继续与再次继续均覆盖首、中、末失败位置；来源记录和此前记录逐字节保持不变。
- 决定：采用方案 A；公开 CLI 命令形状、确认短语和发布版本仍作为后续独立决定，不在本次批准中自动确定。

### D-037：I18-I1 可执行继续 Plan 的确定性提升

- 状态：Accepted（依据维护者对 D-036 方案 A 的明确批准）
- 决定：新增 Plan Schema `0.5.0`，固定 `executable=true`、必须包含现有 continuation provenance、只允许 R1/R2 剩余动作和 Plan 级确认。Plan `0.4.0`、`0.3.0`、`0.2.0`、`0.1.0` 保持原文件和语义不变。
- 决定：新增纯函数从通过不可变校验的 Plan `0.4.0` 确定性提升 Plan `0.5.0`。提升保留创建/过期时间、环境、策略、动作、来源、检查点和 satisfied dependency，只改变 Schema、可执行标志、面向执行的摘要并重算完整 Plan ID；输入与输出不得共享可变切片或指针。
- 决定：Plan `0.5.0` 在本增量仍由现有 Executor 于注册表解析、历史写入和进程启动前拒绝。该 Schema 只是最终确认对象的数据契约，确认校验、Operation Record `0.4.0`、重复继续和真实执行必须按 D-036 的 I18-I2/I3 后续增量交付。
- 非范围：本增量不新增或修改 Operation Record，不接受确认、不解析注册表、不执行动作、不增加 CLI，不支持 R3/R4、恢复 Plan或自动回滚。

### D-038：I18-I2 Operation Record 兼容与重复继续来源

- 状态：Accepted（依据维护者对 D-036 方案 A 的明确批准及时间盒内继续开发授权）
- 用户价值：一条未来由 Plan `0.5.0` 执行产生的失败记录可以保留完整确认来源，并与现有 Plan `0.2.0` 失败记录一样成为下一轮只读继续准备的直接来源，从而形成可审计而不改写历史的重复继续链。
- 范围：新增 Operation Record Schema `0.4.0`，保存完整 confirmed Plan `0.2.0`、`0.3.0` 或 `0.5.0`；普通新操作统一写 `0.4.0`。冻结的 `0.3.0` 继续保存并校验 Plan `0.2.0`/`0.3.0`，`0.2.0`/`0.1.0` 继续只读兼容且不得携带 confirmed Plan。
- 来源资格：通用继续只接受终态失败的 Record `0.3.0` + Plan `0.2.0`，或 Record `0.4.0` + Plan `0.2.0`/`0.5.0`；Plan `0.3.0`、旧记录、完成记录、活动记录和没有剩余动作的记录不能成为 R1/R2 继续来源。每次只绑定直接来源，Plan `0.5.0` 自身的 continuation provenance 保留更早链路。
- Node tools 语义：首次和再次继续都只从 confirmed Plan 解析精确 Node/工具目标、provider、安全检查和当前记录的动作 DAG；每次仍只扫描一次当前环境，重新复核当前记录中已完成步骤，并生成新的审查态 Plan `0.4.0`。首、中、末失败位置均须覆盖；来源和此前记录必须逐字节不变。
- 兼容与隔离：不得修改 `v0.3.0.json`、`v0.2.0.json`、`v0.1.0.json` 或历史 Plan Schema。现有 Executor 在本增量仍只接受 Plan `0.2.0`/`0.3.0`，必须在确认 Plan `0.5.0` 的注册表解析、Record 创建和进程启动前拒绝。
- 风险：Schema 版本提升可能误把 `0.3.0` 当作无 confirmed Plan 的旧记录，或让 `0.5.0` 的上轮 satisfied dependency 错误回灌到当前步骤 DAG；通过逐版本 codec fixture、直接 confirmed Plan/步骤绑定篡改测试、重复继续三失败位置矩阵和副作用断言控制。
- 依赖：D-036、D-037、Plan `0.5.0`、Operation Record `0.3.0` confirmed Plan 绑定、I18-C 检查点资格、I18-E/F Node tools 单次扫描准备与复核。
- 验收：四版 Operation Record 均可按冻结语义解码；`0.4.0` 的 Plan/确认/步骤身份任一篡改均拒绝；Record `0.3.0` 首次继续与 Record `0.4.0` + Plan `0.5.0` 再次继续的首、中、末失败位置均产生正确动作分区、检查点和新 Plan ID；旧记录不变；全量、race、vet、build、离线核心测试及 Linux/Windows 目标构建通过。
- 非范围：本增量不确认或执行 Plan `0.5.0`，不解析其执行注册表、不从生产路径创建一条 Plan `0.5.0` 记录、不增加 CLI，不支持 R3/R4 继续、恢复 Plan、自动回滚或完整 Node 工作流。

### D-039：I18-I3 Node tools 内部继续执行

- 状态：Accepted（依据维护者对 D-036 方案 A 的明确批准及时间盒内继续开发授权）
- 用户价值：内部调用方可以把最终呈现给维护者并获得新确认的 Plan `0.5.0` 安全执行为一条新的 Operation Record `0.4.0`，只运行当前记录中尚未完成的 Node tools 动作，同时保留完整继续来源和失败隔离。
- 入口与确认：新增内部 `ExecuteContinuation(ctx, finalPlan, confirmation)`；唯一确认对象是完整 Plan `0.5.0` ID。非法内容、非 `0.5.0`、尚未生效、过期、错误 scope/ID/时间的确认必须在读取历史、扫描环境、解析执行动作、创建记录或启动进程前拒绝。
- 同调用复核：执行调用必须使用 final Plan 原 `created_at` 在一次新鲜 Inventory 扫描上重新加载直接来源、复核检查点、重新准备剩余 Plan `0.2.0`、生成草案 `0.4.0` 并提升最终 `0.5.0`；完整 ID 不同即要求重新生成和确认，不刷新 30 分钟时窗。
- 执行上下文：复核通过后必须复用该次扫描得到的 NVM/Node/tool baseline、精确目标、provider 和固定适配器注册表，不得为了执行再次扫描或改用 Plan 中的命令。通用确定性 Executor 可接受通过不可变校验与新 Plan 级确认的 `0.5.0`，但 Node tools 服务是唯一在本增量构造其执行注册表的生产入口。
- 记录与失败：执行前保存新的 Record `0.4.0`，`confirmed_plan`、确认凭据和步骤只绑定 final Plan `0.5.0` 及其剩余动作。动作继续遵守拓扑顺序、幂等检查、逐项验证和首个失败停止；失败点之后保持 Pending，可由 I18-I2 再次继续。直接来源及更早记录不得改写。
- 风险：复核与执行之间可能错误重复扫描、确认草案而非最终 Plan、在 ID 不同时仍执行、把来源已完成步骤复制到新记录，或让失败后的下游启动；通过扫描/Build/Runner/Store 调用计数、错误确认与漂移矩阵、Record 身份断言和首次/再次继续执行测试控制。
- 依赖：D-036～D-038、Plan `0.5.0`、Operation Record `0.4.0`、I18-E/F 单次扫描准备与复核、Node tools 固定适配器及通用 Executor。
- 验收：首次继续至少覆盖成功及中间失败，再次继续覆盖成功收敛；确认、过期、来源/检查点/当前状态漂移和最终 ID 不同均在 Action Build、Record 创建和写进程前拒绝；成功/失败 Record 只含本次剩余步骤且保存 final Plan `0.5.0`；整个来源历史逐字节不变；全量、race、vet、build、离线核心和跨平台构建通过。
- 非范围：本增量不新增 CLI 或确认短语，不复用旧确认，不执行 Plan `0.4.0`，不支持 R3/R4 继续、恢复 Plan、自动回滚、无人值守授权、任意命令或完整 Node 工作流。

### D-040：I18-J 继续执行后的最终环境证据

- 状态：Accepted（依据 I18 路线图“执行后重新扫描并输出差异”及维护者时间盒内继续开发授权）
- 用户价值：内部调用方在继续执行成功或部分失败后，不只得到过程记录，还能看到一次执行后新鲜扫描所证明的每个本次动作的前后版本、目标、provider、步骤终态和完成验证结论。
- 范围：`ExecuteContinuation` 在 Executor 产生终态 Record 后恰好再执行一次 Inventory 扫描，重新检查相同目标 Node、NVM 控制状态和 Node tools provider；返回不含绝对路径的结构化 `ContinuationOutcome`。执行前仍只扫描一次，执行中不重复扫描。
- 结果语义：每个 final Plan 动作按拓扑顺序输出 action/tool、before/current after/target version、provider、步骤 state 和 `verified`。Completed 只有在 Record 验证 Passed 且执行后版本与精确目标、provider 均匹配时才为 verified；Failed/Pending 不伪报完成，但保留可观察 after version。
- 失败语义：执行后扫描或最终验证失败不能改写已经持久化的真实 Record，也不能把失败操作报告为成功；返回值必须保留 Record/路径并组合执行错误与最终证据错误。来源及此前记录继续逐字节不变。
- 隐私：结果只包含稳定 action/tool 身份、版本、provider 和状态，不包含 HOME/NVM/临时绝对路径、命令、参数、代理值或 runner 原始输出。
- 风险：第二次扫描可能被误用于执行前 ID 复核、掩盖原执行错误、因动态时间戳产生虚假差异，或泄漏安装路径；通过扫描顺序/次数、双错误保留、字段白名单和序列化敏感值断言控制。
- 依赖：D-039 的单次执行前复核上下文、Operation Record `0.4.0`、Node tools 只读 inspect 与 adapter 完成验证。
- 验收：成功、首/中/末失败和再次继续场景均证明每次调用只有一次执行前及一次执行后扫描；Completed 精确匹配，Failed/Pending 不误报；后扫描失败保留终态 Record；结果不含私有路径；全量、race、vet、build、离线核心及跨平台构建通过。
- 非范围：本增量不生成通用 Inventory diff Schema，不修改 Operation Record，不新增 CLI/文件输出，不支持混合 R2/R3、恢复 Plan、自动回滚或完整工作流。

### D-041：I18-K 终态操作的只读恢复候选评估

- 状态：Accepted（依据 I18 路线图恢复要求及维护者时间盒内继续开发授权）
- 用户价值：在决定生成何种恢复 Plan 前，维护者可以从一条不可变终态记录中区分哪些动作有已验证状态差异、哪些写进程失败后结果仍不确定、哪些幂等动作没有变化，以及恢复元数据要求新 Plan 还是人工处置。
- 范围：新增纯函数 `AssessRecovery(record)`，只接受带合法 confirmed Plan 的 Record `0.3.0`/`0.4.0` 终态；按 confirmed Plan 拓扑和步骤证据输出稳定候选，不读环境、不解析注册表、不写历史。
- 候选规则：Before/After Snapshot 的合法非空 diff 表示 `changed`；失败、超时、取消或中断步骤只要已经启动非幂等写进程而没有可证明的最终差异，就表示 `uncertain`；Completed 且 skipped 或空 diff 表示 `unchanged`，Pending 和从未启动的步骤不产生候选。
- 恢复分类：保留 Plan Action 冻结的 `recovery.mode` 与摘要；`plan` 表示后续必须生成新 Plan ID 并重新确认，`manual` 表示核心不能自动逆转。评估不得把 manual 改成 plan、把 uncertain 当作 changed，或声称已经恢复。
- 安全与隐私：结果只包含 Operation/Plan/Action 身份、tool/operation、步骤终态、证据分类、恢复模式和冻结摘要；不包含 Snapshot facts、diff 值、Invocation、输出、绝对路径或敏感值。
- 风险：仅凭 Completed 误判发生变化、忽略验证失败后的部分写入、把 skipped 动作列为恢复候选或从记录中泄漏事实值；通过证据矩阵、字段白名单、输入逐字节不变和敏感标记断言控制。
- 依赖：Operation Record `0.3.0`/`0.4.0` confirmed Plan 绑定、Snapshot diff 语义、Plan Recovery 元数据和终态状态机。
- 验收：覆盖 completed changed、failed changed、failed uncertain、completed skipped/unchanged、pending、四种部分终态、Completed 操作、Plan/manual 两种模式、旧记录与篡改记录；无回调和副作用；全量、race、vet、build、离线核心及跨平台构建通过。
- 非范围：本增量不生成、确认或执行恢复 Plan，不判断当前环境是否仍可恢复，不自动回滚，不新增 CLI/Schema，不决定混合 R2/R3 的公开确认形状。

### D-042：I18-L 恢复候选的当前 checkpoint 只读复核

- 状态：Accepted（依据 I18 路线图恢复要求及维护者时间盒内继续开发授权）
- 用户价值：在为历史变化生成恢复 Plan 前，维护者可以区分“当前环境仍匹配操作结束时的状态”“环境已经漂移”“历史结果本来就不确定”和“核心没有对应复核能力”，避免依据过期证据恢复。
- 范围：新增只读 `RevalidateRecovery(ctx, record, registry)`；它先复用 I18-K 的候选判定，再按 confirmed Plan 拓扑处理候选。只有具有合法 After Snapshot 的 `changed` 候选可以调用已注册动作的 `RevalidateCheckpoint`；`uncertain` 候选不调用适配器。
- 结果语义：每个候选只输出稳定的 `current`、`drifted`、`uncertain` 或 `verifier_unavailable` 状态。复核成功仅表示当前动作作用域仍匹配历史 After checkpoint，不表示恢复可执行、已获确认或已经恢复；适配器错误内容和 Snapshot facts 不进入结果。
- 安全与不可变性：复核不得调用 Action Build、Preflight、Capture、Satisfied 或 Verify，不得写历史或执行进程。传给回调的 Action 与 Snapshot 必须是副本，错误回调不能修改 confirmed Plan、来源步骤或输入 Registry；取消上下文必须原样返回，不输出部分成功结论。
- 风险：把 `uncertain` 当成可复核状态、把未注册动作当成安全、暴露适配器错误/路径、复用被回调修改的输入，或将“仍匹配”误述为“可自动恢复”；通过稳定枚举、字段白名单、回调矩阵、序列化敏感标记、上下文和逐字节不可变断言控制。
- 依赖：D-041 的恢复候选分类、Operation Record confirmed Plan、动作级 `RevalidateCheckpoint` 注册契约和 Snapshot 完整性校验。
- 验收：覆盖 changed/current、changed/drifted、uncertain、验证器缺失、回调返回非法证据、取消上下文、多个候选拓扑顺序、回调输入变异、禁止其他动作回调、来源逐字节不变和结果隐私；全量、race、vet、build、离线核心及跨平台构建通过。
- 非范围：本增量不新增适配器复核器，不生成、确认或执行恢复 Plan，不自动回滚，不修改 Operation Record/Plan Schema，不新增 CLI，也不决定混合 R2/R3 的公开确认形状。

### D-043：I18-M NVM 默认恢复的只读复核入口

- 状态：Accepted（依据 I18 路线图恢复要求及维护者时间盒内继续开发授权）
- 用户价值：维护者在生成 NVM 默认版本恢复 Plan 前，可以仅凭来源 Operation ID 得到当前默认别名是否仍与来源 After checkpoint 一致的结构化结论，而不必通过“尝试准备恢复 Plan”间接判断。
- 范围：为固定 NVM `set_default`/`restore_default` 定义补充 `RevalidateCheckpoint`，并在 defaultversion 内部服务新增 `ReviewRestore(ctx, options)`。服务验证 Operation ID、加载一条不可变记录并复用 I18-K/L；只有存在 `changed` 候选时执行一次 Inventory 扫描、构造固定注册表并复核。
- 当前状态契约：复核器验证动作身份与精确目标、Plan 冻结的 NVM script digest，以及 recorded/current 的 active version、default alias/value/digest/resolution、installed versions 和 target version。完全匹配返回 `current`；外部别名、NVM script、安装集合或动作身份变化由 I18-L 收敛为 `drifted`。
- 不确定与空候选：`uncertain` 候选不扫描环境、不调用适配器；无实际变化的合法来源返回空候选。非法、非 NVM default 或篡改来源在扫描前拒绝。
- 安全、隐私与不可变性：入口不调用 Plan builder、Action Build 或 Runner，不创建 Operation Record/文件，不返回 Snapshot facts、路径或底层错误。来源历史和默认别名逐字节不变，读取失败不能被报告为 current。
- 风险：把通用 R3 动作误接入、扫描结果与文件状态竞态、忽略 NVM script/安装集合漂移、或让只读入口成为绕过确认的执行路径；通过固定动作身份、复核时再次读取、零 Runner/历史写入断言和原有 PrepareRestore/Execute 回归控制。
- 依赖：D-041/D-042、I16 NVM default 固定适配器、Operation Record confirmed Plan、defaultversion 历史加载和只读 Inventory/NVM Inspect。
- 验收：适配器覆盖匹配及 alias/script/安装/目标漂移；服务覆盖 changed current、外部漂移、uncertain 零扫描、非法来源扫描前拒绝、恰好一次扫描、来源/别名不可变和结果隐私；原有 R3 PrepareRestore/Execute 行为不变；全量、race、vet、build、离线核心及跨平台构建通过。
- 非范围：本增量不生成、确认或执行恢复 Plan，不改变 R3 明确确认，不自动回滚，不新增 CLI/Schema，不支持 Node 安装或 Node tools 的恢复入口，也不决定混合 R2/R3 工作流。

### D-044：I18-N Node tools 失败操作的只读恢复复核入口

- 状态：Accepted（依据 I18 路线图恢复要求及维护者时间盒内继续开发授权）
- 用户价值：Node tools 多动作更新失败后，维护者可以按来源 Operation ID 同时看到已发生变化的上游动作是否仍匹配历史 checkpoint，以及失败动作是否因结果不明仍需人工检查，而不生成或执行恢复操作。
- 范围：在 nodetools 内部服务新增 `ReviewRecovery(ctx, operationID)`；只接受现有 continuation 来源契约已经支持的终态失败 R1/R2 Node tools confirmed Plan。服务复用 I18-K/L 和冻结的动作身份、provider、目标 Node、目标版本、安全检查及 DAG 校验。
- 扫描与注册表：若候选全部为 `uncertain`，不扫描环境、不构造动作注册表；若存在 `changed`，恰好执行一次 Inventory 扫描和现有受控版本探针，按来源目标/provider 构造固定 Node tools 注册表，再逐项复核。Runner 只允许 `--version` 只读探针，不能出现包管理器写形状调用。
- 状态与隐私：已变化上游输出 `current`/`drifted`，失败但无最终差异的动作保持 `uncertain`，Pending 不出现。结果不含 Node/NVM/package/executable 路径、命令、参数、Snapshot facts、探针输出或底层适配器错误。
- 安全与不可变性：非法 Operation ID、旧记录、非 Node tools、Completed 或不安全/篡改动作在 Scan 前拒绝；不调用 Plan builder/Action Build、不写历史、不改变工具文件。来源历史逐字节不变。
- 风险：把 continuation 资格误当成恢复授权、对 uncertain 动作运行探针、执行写形状命令、使用新目标替换来源目标或泄漏包路径；通过来源契约复用、扫描/Runner 计数、历史快照和序列化敏感值断言控制。
- 依赖：D-041/D-042、I18-C/E 的 Node tools 来源/检查点校验、固定 Node tools adapter revalidator、一次扫描 inspectTargets 和 Operation Record confirmed Plan。
- 验收：覆盖中间失败的 changed/current + uncertain、上游版本漂移、首动作失败 uncertain 零扫描、非法非 Node tools 来源扫描前拒绝、一次扫描、零写形状 Runner、历史不可变和结果隐私；全量、race、vet、build、离线核心及跨平台构建通过。
- 非范围：本增量不支持 Completed Node tools 操作，不生成、确认或执行恢复 Plan，不自动回滚，不新增 CLI/Schema，不支持 Node 安装或 R3，也不决定混合 R2/R3 工作流。

### D-045：I18-O Completed Node tools 变化的只读恢复复核

- 状态：Accepted（依据 I18 路线图恢复要求及维护者时间盒内继续开发授权）
- 用户价值：一次 Node tools 更新成功后，只要记录证明实际状态发生变化，维护者仍可在未来恢复决策前确认该成功后的 checkpoint 是否保持当前，而不把“操作成功”错误解释为“不需要恢复评估”。
- 范围：抽取 Node tools confirmed Plan 的共同来源元数据校验，并让 `ReviewRecovery` 同时接受 Completed 及原有四种部分失败终态。共同校验继续固定 Record/Plan 兼容、动作身份、provider、目标 Node/版本、安全元数据和 DAG。
- 边界保持：`PrepareContinuation` 仍只接受 Failed/TimedOut/Cancelled/Interrupted 且具有 remaining actions 的来源；Completed 来源不得因共同校验抽取而获得 continuation 资格。Completed skipped/空 diff 返回空候选且零扫描；Completed changed 才执行一次扫描与 checkpoint revalidator。
- 安全、隐私与不可变性：复用 D-044 的一次扫描、只读版本探针、零写形状调用、结果字段白名单和历史不可变契约；成功记录不构成恢复授权，也不会自动生成反向目标。
- 风险：重构时放宽 continuation、把 skipped 成功列为候选、将当前 target 自动变成恢复 target或改变旧错误顺序；通过现有 completed-continuation 拒绝回归、changed/unchanged 对照及全仓测试控制。
- 依赖：D-041/D-042/D-044、Node tools confirmed Plan 来源校验、Operation Record 终态和 Snapshot diff。
- 验收：真实 Completed changed 来源经一次扫描输出 current 且零写形状调用；外部漂移沿用 drifted；Completed skipped/unchanged 返回空候选且零扫描；PrepareContinuation 对 Completed 继续在扫描前拒绝；失败来源测试不变；全量、race、vet、build、离线核心及跨平台构建通过。
- 非范围：本增量不生成、确认或执行恢复 Plan，不自动选择旧版本、不自动回滚，不新增 CLI/Schema，不支持 Node 安装或 R3，也不决定混合 R2/R3 工作流。

### D-046：I18-P NVM Node 安装的只读恢复复核

- 状态：Accepted（依据 I18 路线图恢复要求及维护者时间盒内继续开发授权）
- 用户价值：一次 NVM Node 版本安装成功或失败后，维护者可以按 Operation ID 判断安装后的 checkpoint 是否仍保持、目标是否已被外部改变，或失败结果是否仍不确定，而不触发卸载。
- 范围：为固定 `runtime.node/install_version/nvm` 定义补充 `RevalidateCheckpoint`，并在 apply 内部服务新增 `ReviewRecovery(ctx, operationID)`。只接受带完整 confirmed Plan 的合法 I15 单动作终态记录。
- checkpoint：复核动作身份/精确目标及 Plan 冻结的 nvm.sh digest，比较 recorded/current active version、default alias digest、installed versions 和 target_installed，并用固定目标二进制 `--version` 只读验证精确版本。匹配为 `current`，目标删除/替换、active/default/script 或安装集合变化为 `drifted`。
- 扫描与失败：Completed changed 执行一次 Inventory 扫描并复核；下载/磁盘/取消等写进程失败但 diff 为空时保持 `uncertain` 且零扫描；skipped/unchanged 返回空候选。非法或非 install_version 来源在 Scan 前拒绝。
- 安全、隐私与不可变性：不调用 Assess 网络证据、Plan builder、Action Build 或安装 Runner，不写历史、不删除目录。结果不含 HOME/NVM/二进制路径、Snapshot facts、命令、参数、输出或底层错误；来源历史逐字节不变。
- 风险：把“目标仍存在”误当作可安全卸载、漏检 NVM 控制状态/其他版本变化、运行写命令或将失败误判为已安装；通过完整 checkpoint、只读 `--version`、扫描/文件/历史断言和 I15 回归控制。
- 依赖：D-041/D-042、I15 apply 服务、固定 NVM install adapter、Operation Record confirmed Plan 和安装 Snapshot。
- 验收：覆盖真实 Completed changed/current、外部删除目标 drifted、失败 uncertain 零扫描、skipped/unchanged 空候选、非法来源扫描前拒绝、来源/工具状态不变和结果隐私；原 I15 Prepare/Execute 不变；全量、race、vet、build、离线核心及跨平台构建通过。
- 非范围：本增量不生成卸载或恢复 Plan、不删除部分/完整安装、不自动回滚、不改变 R3 确认，不新增 CLI/Schema，也不决定混合 R2/R3 工作流。

### D-047：I18 收尾的恢复对象与混合 R2/R3 工作流

- 状态：Accepted（维护者于 2026-07-31 批准方案 A）
- 决策原因：I18 剩余两项是从恢复复核生成可审查恢复对象，以及“安装 Node（R2）→ 切换 default（R3）→ 更新 Node tools（R2）”完整工作流。现有 Plan `0.2.0` 只支持 R1/R2，Plan `0.3.0` 只支持单个 NVM default R3 动作，Plan `0.5.0` 只支持 R1/R2 continuation；manual recovery 又不能伪装成可执行 Action。因此下一步必须先决定公开对象和确认边界。
- 方案 A（推荐）：分阶段子 Plan + 只读 Workflow/Recovery Manifest。完整工作流只编排现有确定性核心：先生成并确认 Node 安装 Plan `0.2.0`，验证和新鲜扫描后生成独立 set-default Plan `0.3.0` 并执行现有 R3 明确确认，再重新扫描并生成 Node tools Plan `0.2.0`。新的只读 Workflow Manifest 绑定阶段、直接来源 Plan/Operation ID、checkpoint 和下一阶段状态，但自身不可执行、不可确认；每个子 Plan 使用自己的 ID、有效期和确认，不能预先确认后续阶段。
- 方案 A 的恢复：新的只读 Recovery Manifest 汇总 I18-K～P 的 candidate/current-state。`recovery.mode=plan` 且 current 的 NVM default 项只引用重新生成的现有独立 Plan `0.3.0`；manual、uncertain、drifted 和 verifier_unavailable 项只输出不可执行处置说明，不能进入任何可执行 Plan。Node 安装删除仍留给独立 R3 卸载能力，Node tools 只在未来有相称反向适配器时才能生成子 Plan。
- 方案 A 的优点与代价：复用已验证的 Plan/Executor/确认边界，不让一次 R3 确认覆盖尚未生成的 R2 动作，也不把 manual 项伪装成自动恢复；代价是完整流程需要分阶段确认，并新增只读 Manifest/Workflow Record 的版本化契约和跨记录来源链。
- 方案 B：新增混合风险可执行 Plan 和 Operation Record，让一个 DAG 同时携带 R2、R3 及逐项确认。它需要新的确认收据集合、动作级过期/再确认、部分确认状态机、混合恢复语义和 Executor/Schema 升级；能力集中但安全与兼容面显著扩大。
- 方案 C（不推荐）：把完整工作流整体提升为 R3 并只做一次计划级确认，同时丢弃或仅在摘要中描述 manual 恢复项。它实现较少，但会让确认覆盖尚未经过前序执行后新鲜扫描的动作，并可能造成“有恢复 Plan”的误解，不符合现有不可复用确认和确定性重建原则。
- 若批准 A，建议后续最小顺序：
  1. I18-Q1：只读 Workflow Manifest/Record 契约、严格校验与三阶段状态机；不生成命令、不执行。
  2. I18-Q2：内部 staged workflow 只准备首个子 Plan，并在每个阶段终态后用新扫描生成下一个全新 Plan；不新增公开 CLI。
  3. I18-Q3：三阶段成功及每阶段失败/中断/漂移端到端编排，每个子 Plan 独立确认，来源记录不可变。
  4. I18-R1：只读 Recovery Manifest 契约，只汇总候选、current-state、mode 和处置类别。
  5. I18-R2：仅为 current + plan 的 NVM default 项生成/引用现有独立 R3 restore Plan；manual/uncertain/drifted 永不执行。
  6. I18-S：I18 全链路验收、文档和是否开放公开 CLI 的独立决策。
- 无论方案：不得自动确认、复用旧确认、把 manual/uncertain 当作可执行、自动删除 Node 版本、自动回滚、在一个 ID 下修改内容，或绕过固定适配器执行任意命令。

### D-048：I18-Q1 只读 Workflow Manifest/Record 0.1.0

- 状态：Accepted（依据维护者批准的 D-047 方案 A）
- 用户价值：在任何子 Plan 被生成或确认前，用一个不可执行、不可确认且内容派生 ID 的 Manifest 固定“安装 Node → 切换 default → 更新 Node tools”目标；用独立 Record 审计三个阶段各自 Plan、Operation、checkpoint 和当前状态，避免跨阶段复用确认或跳过前序。
- Manifest 范围：新增 Workflow Manifest `0.1.0`，固定 `executable=false`、`confirmable=false`、精确目标 Node 版本、至少一个精确 npm/Corepack/pnpm 目标，以及三个固定阶段：`install_node`（R2/Plan `0.2.0`）→ `set_default`（R3/Plan `0.3.0`）→ `update_node_tools`（R2/Plan `0.2.0`）。阶段顺序、风险、子 Plan Schema 和依赖进入内容派生 Manifest ID。
- Record 范围：新增 Workflow Record `0.1.0`，绑定 Manifest ID、固定三阶段、各阶段状态、独立 Plan ID/Schema、Operation ID、完成 checkpoint、时间戳和逐项转换历史。Workflow ID 使用独立 `wf-<32 hex>` 身份；本增量只提供内存纯函数，不持久化文件。
- 状态机：初始仅 `install_node=ready`；阶段必须按 `ready → planned → running → terminal` 前进。`planned` 绑定符合该阶段固定 Schema 的新 Plan ID，`running` 再绑定 Operation ID；Completed 必须绑定 checkpoint digest并使下一阶段 ready，最后阶段完成才使工作流 Completed。Failed/TimedOut/Cancelled/Interrupted 立即终止工作流，后续保持 Pending。
- 不可执行与确认边界：Manifest/Record 模型和 Schema 不含确认凭据、command、args、Shell、环境、Runner 或进程输出；状态转换不读取 Plan/Operation 文件，也不声称绑定身份已经通过 Q2 的内容校验。Workflow Manifest 本身没有确认或执行入口，每个子 Plan 的新鲜生成和独立确认属于 Q2/Q3。
- 不可变性：所有构造和转换返回新值，不修改调用方 Manifest/Record；修改目标、阶段、风险、Schema、依赖、时间或 ID 会使严格校验失败。JSON 严格拒绝未知字段、未知版本、尾随值和不合法枚举/格式。
- 风险：把 Manifest 误当作可执行 Plan、允许后序提前规划、让 R3 确认覆盖其他阶段、在失败后继续、记录身份与转换历史不一致，或把未来命令/秘密带入契约；通过固定三阶段 Schema、历史重放、字段白名单、负例矩阵和不可变断言控制。
- 依赖：D-047 方案 A、Plan `0.2.0`/`0.3.0` 独立确认语义、Operation Record `0.4.0` 身份格式和现有严格 JSON Schema 编解码方式。
- 验收：确定性 Manifest ID；合法全成功链和每阶段失败终态；后序提前、错误 Plan Schema/ID、错误 Operation ID、缺失 checkpoint、非法时间、终态后继续、篡改历史/阶段/Manifest ID全部拒绝；Manifest/Record JSON Schema 直接负例、输入逐字节不变、结果不含执行/确认字段；全量、race、vet、build、离线核心及跨平台构建通过。
- 非范围：本增量不扫描环境、不准备或加载子 Plan、不确认、不执行、不持久化 Workflow Record、不新增 CLI/Skill/MCP、不生成 Recovery Manifest、不改变现有 Plan/Operation Schema，也不完成 Q2/Q3。

### D-049：I18-Q2 当前阶段的新鲜子 Plan 准备

- 状态：Accepted（依据维护者批准的 D-047 方案 A 及时间盒内客观验收后继续授权）
- 用户价值：调用方只需提交已校验的 Manifest/Record，内部编排就会为唯一 Ready 阶段调用现有确定性服务生成一个新的可审查子 Plan，并把其完整 ID 绑定到返回的新 Record；不会提前准备后续阶段，也不会把一个阶段的确认边界扩散到另一个阶段。
- 范围：新增内部 `PrepareNext` 纯编排入口和原生准备器。`install_node` 调用现有 apply `Prepare` 并要求新鲜官方版本证据；`set_default` 调用现有 defaultversion `PrepareSet`；`update_node_tools` 调用现有 nodetools `Prepare`。三个入口继续各自执行现有扫描/检查，并返回其原生 Prepared 上下文供 Q3 后续使用；编排层不自行扫描、重写 Plan 或构建命令。
- 顺序与新鲜度：只有 Record 中唯一 Ready 阶段可以准备；初始只准备安装 Plan，前一阶段经 Q1 合法 Completed 并使后继 Ready 后，下一次调用才准备后继。返回 Plan 的 `created_at` 不得早于当前 Record `updated_at`，每次只调用一个阶段准备器一次；Planned/Running、异常终态、Completed 工作流或无 Ready 阶段均在调用准备器前拒绝。
- 子 Plan 绑定：准备器返回值必须先通过现有 Plan 完整不可变校验，再严格匹配 Manifest 冻结的阶段 Schema、风险、动作身份、精确 Node/工具目标和 Node tools 所属 Node 版本；不匹配时 Record 保持不变。通过后复用 Q1 `BindStagePlan` 返回新的 Planned Record，转换时间绑定子 Plan `created_at`。
- 不可变与隔离：Manifest、输入 Record、返回的公开 Plan 与原生 Prepared 上下文不共享可由调用方修改的 Plan 切片；失败不产生部分 Record。结果不含确认收据、命令、参数或 Runner 输出，不创建 Operation Record 或 Workflow 文件。
- 风险：准备后序阶段过早、接受错误类型或错误目标 Plan、复用陈旧 Plan、一次调用准备多个阶段、调用方篡改公开 Plan 后影响未来执行上下文，或在准备动作中误确认/执行；通过 Ready 门、阶段语义白名单、时间门、调用计数、深复制和无副作用断言控制。
- 依赖：D-047/D-048、I15 apply Prepare、I16 defaultversion PrepareSet、I17 nodetools Prepare，以及现有 Plan `0.2.0`/`0.3.0` 严格校验。
- 验收：全三阶段依次只准备一个新 Plan并绑定正确 Schema/ID；每阶段参数与 Manifest 精确一致；后序提前、非 Ready/终态、陈旧时间、错误 Schema/风险/动作/目标/Node 所属和准备错误全部拒绝且输入不变；公开 Plan 修改不影响封存的原生上下文；准备路径不调用执行、确认或历史存储；全量、race、vet、build、离线核心及跨平台构建通过。
- 非范围：本增量不确认或执行子 Plan、不接收 Operation Record、不自动推进 running/terminal、不持久化 Workflow、不新增公开 CLI/Skill/MCP、不生成 Recovery Manifest、不修改现有 Plan/Operation Schema，也不完成 Q3。

### D-050：I18-Q3 独立确认的三阶段执行编排

- 状态：Accepted（依据维护者批准的 D-047 方案 A、时间盒自动过门授权及继续任务指令）
- 用户价值：内部调用方可把 Q2 当前阶段的封存 Prepared 与只绑定该子 Plan ID 的新确认交给现有执行服务；返回的终态 Operation Record 会确定性推进 Workflow Record，而确认错误、执行前漂移或未创建 Operation 时不会伪报阶段已经开始或完成。
- 范围：新增内部 `ExecutePrepared`、原生阶段执行器和纯 `AttachOperation`。原生执行器只把封存的 apply/defaultversion/nodetools Prepared 与调用方提供的确认收据转交现有 `Execute`；编排层不生成确认、不构建命令、不绕过服务的执行前重建、注册表、验证或历史写入。
- 确认边界：每次调用必须是 `scope=plan`，ID 精确等于当前 Prepared 子 Plan，确认时间不得早于该 Plan；实际服务继续拒绝未来确认和过期 Plan。前一阶段收据用于后一阶段、Manifest/Record/公开 Plan/封存上下文任一不匹配，均在阶段执行器前拒绝。
- Operation 绑定：只接受通过现有 Operation Record 完整校验、Plan ID/Schema/confirmed Plan 与当前子 Plan一致的终态记录。Operation `created_at`/ID 形成 Q1 planned→running 绑定，`finished_at` 形成 terminal 转换；Completed/Failed/TimedOut/Cancelled/Interrupted 一一映射。完整规范化 Operation Record JSON 的 SHA-256 作为阶段 checkpoint digest，使来源记录内容与 Workflow 审计链绑定。
- 错误与漂移：若现有服务在执行前因环境/Plan 漂移、过期或确认错误而返回空 Operation Record，Workflow 保持 Planned 且不产生部分转换；若服务已产生合法失败终态 Record，则 Workflow 同步进入相同异常终态并保留原错误。非法、活动或不匹配的 Operation Record 不得推进 Workflow。
- 不可变与隐私：Prepared、Manifest、Workflow Record、确认收据和来源 Operation Record 均不修改；返回的公开 Plan/Operation/Workflow 值深复制。Workflow 只保存 Operation ID 和 digest，不复制 command、args、输出、环境或确认凭据。
- 风险：把旧确认用于新阶段、在执行前拒绝后仍标记 Running、Operation 与 Plan 身份错绑、把失败误报 Completed、允许异常后续继续、调用方修改公开值影响封存上下文，或从 Operation 泄漏过程数据进 Workflow；通过前置身份门、终态映射、规范化摘要、错误/记录双返回测试和不可变断言控制。
- 依赖：D-048/D-049、Q2 封存原生 Prepared、现有 apply/defaultversion/nodetools `Execute`、Operation Record `0.4.0` 严格验证和 Q1 状态机。
- 验收：三个阶段以三个不同 Plan ID/确认收据完整成功；每个阶段 Failed/TimedOut/Cancelled/Interrupted 后 Workflow 同步终止且后续不执行；错误/复用确认、过期或执行前漂移在无 Operation 时保持 Planned；篡改、活动、错误 Plan/Schema/时间 Operation 拒绝；checkpoint 可复算，全部输入及既有 Operation 不变；全量、race、vet、build、离线核心及跨平台构建通过。
- 非范围：本增量不替用户确认、不增加 `--yes` 或配置授权、不自动重试/恢复/继续、不持久化 Workflow、不新增公开 CLI/Skill/MCP、不生成 Recovery Manifest、不修改 Plan/Operation Schema，也不执行真实用户环境修改作为测试。

### D-051：I18-R1 只读 Recovery Manifest 0.1.0

- 状态：Accepted（依据维护者批准的 D-047 方案 A、时间盒自动过门授权及继续任务指令）
- 用户价值：维护者可把 I18-K～P 已完成当前状态复核的多个来源操作汇总成一个不可执行、不可确认、内容可校验的恢复清单，清楚区分哪些候选仅可进入新 Plan 审查、哪些需要人工处置、哪些必须先调查不确定性或重新评估漂移。
- 契约：新增 Recovery Manifest `0.1.0`，固定 `executable=false`、`confirmable=false`、内容派生 ID 和创建时间；`sources` 记录已复核的 Operation/Plan ID，`items` 记录来源、动作/tool/operation、步骤终态、changed/uncertain 证据、current/drifted/uncertain/verifier_unavailable 当前状态、`plan|manual` 恢复模式、冻结摘要和确定性处置类别。
- 处置映射：`current + plan → prepare_new_plan`，仅表示后续可以进入新的 Plan 审查；`current + manual → manual_action`；`uncertain → investigate_uncertain`；`drifted → reassess_drifted`；`verifier_unavailable → review_without_verifier`。Manifest 不携带恢复 Plan ID，R2 才会限制真正可生成/引用的对象。
- 输入与排序：纯构造器只接受现有 `RecoveryRevalidation`；验证来源 ID、候选字段、终态、mode 以及 evidence/current-state 一致性，拒绝重复来源或同来源重复 action。来源按 Operation ID 排序，items 按来源和 action 排序，使输入顺序不影响内容 ID；空候选来源仍保留在 sources。
- 不可执行与隐私：模型和 Schema 不含 Plan 内容、确认、command、args、Runner、Snapshot facts/diff、Invocation、输出、路径、错误详情或当前探针原始值；只保留已由恢复复核公开的稳定字段及冻结恢复摘要。构造、编解码和验证不扫描环境、不调用适配器、不读写历史。
- 风险：把处置类别误当作已生成恢复 Plan、把 uncertain/drifted 降格为可执行、遗漏空候选复核来源、输入顺序导致不稳定 ID、重复来源覆盖或过程证据泄漏；通过固定映射、严格 Schema、排序、重复门和字段白名单控制。
- 依赖：D-047、I18-K `AssessRecovery`、I18-L `RevalidateRecovery` 及 I18-M～P 三类只读复核入口。
- 验收：五种处置类别、空候选来源、乱序输入确定性 ID；非法 ID/终态/mode/evidence-state/重复项全部拒绝；目标、来源、状态、摘要、时间或 ID 篡改拒绝；严格 JSON 直接拒绝未知版本/字段、尾随值及 command/confirmation/Snapshot 等注入；输入不变；全量、race、vet、build、离线核心及跨平台构建通过。
- 非范围：本增量不加载 Operation 文件、不主动调用 ReviewRecovery/ReviewRestore、不生成/引用/确认/执行恢复 Plan、不自动回滚或删除、不持久化 Manifest、不新增公开 CLI/Skill/MCP，也不完成 R2。

### D-052：I18-R2 current NVM default 候选的独立恢复 Plan

- 状态：Accepted（依据维护者批准的 D-047 方案 A、时间盒自动过门授权及继续任务指令）
- 用户价值：维护者可以从 Recovery Manifest 中明确选择一个仍为 current、恢复模式为 plan 的 NVM set-default 候选，并得到现有 defaultversion 核心重新加载来源、重新扫描当前环境后生成的全新独立 R3 restore Plan；其他候选永远不能借此进入可执行 Plan。
- 入口：新增内部 `PrepareRestore`，调用方只传 Recovery Manifest、source Operation ID 和 action ID；函数必须从已校验 Manifest 查找原始 item，不能接受调用方自行提供或修改的候选字段。只有 `runtime.node/set_default + changed/current + plan + prepare_new_plan` 精确组合进入准备器。
- 原生复用：原生准备器只调用现有 `defaultversion.PrepareRestore(operationID)`，因此继续由其加载并校验来源 Operation、扫描 NVM 当前状态、检查 alias 未漂移并构建 Plan `0.3.0`；本层不读取历史、不扫描、不重新实现恢复 Plan builder。
- 返回绑定：返回值绑定 Recovery Manifest ID、source Operation/Plan、source action 和新 Plan。新 Plan 必须是单动作 `restore-node-default/runtime.node/restore_default/nvm` R3，`recovery.mode=manual`，创建时间不早于 Manifest，ID 不等于来源 Plan，并包含 `source_operation_matches` 前置条件精确绑定来源 Operation/Plan。公开 Plan 与封存原生 Prepared 深复制隔离。
- 拒绝边界：manual、uncertain、drifted、verifier_unavailable、非 set-default、错误/缺失 item、准备器错误、陈旧/错误 Schema/动作/来源 Plan 均在返回前拒绝；取消在调用准备器前拒绝。拒绝不修改 Manifest，不生成确认、不执行、不写新的 Workflow/Recovery 对象。
- 风险：把处置提示当作授权、让 manual/uncertain/drifted 进入 Plan、为 Node 安装删除或 Node tools 伪造反向动作、接受与来源不匹配的 restore Plan、或让公开 Plan 修改封存执行上下文；通过精确白名单、现有 R3 builder、来源前置条件、时间/ID门和深复制控制。
- 依赖：D-051 Recovery Manifest、I18-M current NVM default 复核、I16 `PrepareRestore` 及 Plan `0.3.0` 独立确认语义。
- 验收：唯一合法候选生成并绑定全新现有 R3 restore Plan；五类非白名单、非 NVM default、错误 key 和取消均在准备器调用前拒绝；错误/陈旧/来源不匹配 Plan 拒绝；Manifest/输入/封存上下文不变且无执行/确认/历史写入；全量、race、vet、build、离线核心及跨平台构建通过。
- 非范围：本增量不确认或执行 restore Plan、不新增确认短语或公开 CLI、不自动选择候选、不批量恢复、不生成 Node 卸载或 Node tools 反向 Plan、不自动回滚、不持久化 Recovery Manifest，也不完成 I18-S。

### D-053：I18 staged workflow / recovery 是否开放公开 CLI

- 状态：Accepted（维护者于 2026-08-10 批准方案 A）
- 现状：I18-Q1～Q3 已提供内存 Workflow Manifest/Record、逐阶段准备和独立确认执行；I18-R1/R2 已提供只读 Recovery Manifest 和 current NVM default 的独立 R3 restore Plan 准备。现有单功能 apply/default/node-tools CLI 及其确认边界保持可用，但 Workflow/Recovery Manifest 未持久化，原生 Prepared 上下文仅在进程内封存，进程重启后的安全恢复/重新准备语义尚未定义。
- 方案 A（推荐）：当前里程碑保持 I18 新编排为内部 API，完成 I18 内部全链路验收；把公开 CLI 延后到单独增量，先冻结 Workflow/Recovery 的存储目录、原子写入、重启恢复、过期 Plan 清理、逐阶段交互提示和状态展示。优点是不会把内存编排误表述为可恢复的用户工作流，也不改变现有逐 Plan 确认；代价是用户暂时仍需调用现有单功能命令。
- 方案 B：只开放只读 `workflow inspect` / `recovery inspect`，不提供执行。优点是较早展示 Manifest/Record；代价是仍需定义输入来源、存储和脱敏输出，且如果没有持久化 Workflow，inspect 对真实用户价值有限。
- 方案 C：立即开放完整 staged workflow CLI，在同一进程内依次展示并确认三个子 Plan。优点是体验连贯；代价是中断/重启无法安全恢复封存 Prepared，上一步 Operation 与下一步新扫描的状态展示、R3 提示、记录落盘和过期处理均没有公开契约，容易让用户误解一次启动或一次确认覆盖完整流程。
- 推荐理由：方案 A 保持已验证的每个子 Plan 独立 ID/确认、执行前新鲜重建和异常停止边界，同时避免在缺少持久化/恢复契约时承诺公开工作流。它不阻塞现有 apply/default/node-tools 能力，也为未来 CLI 留出可审查的最小增量。
- 决定：I18 以内部 staged workflow / recovery API 完成里程碑验收，不新增公开 CLI。公开入口进入 Backlog，必须在独立增量中先冻结持久化、进程重启恢复、过期清理、逐阶段交互和状态展示契约，再重新提请维护者决定。
- 无论选择：不得复用前一阶段确认、增加 `--yes`、序列化命令/秘密、在重启后复用过期 Prepared、自动选择恢复候选、自动执行 R3 恢复或把 manual/uncertain/drifted 项转为可执行。

### D-054：I19 Profile 0.1.0 声明契约

- 状态：Accepted（依据 I19 路线图及维护者于 2026-08-10 的继续开发指令）
- 用户价值：用户可用同一份严格、平台无关的 YAML/JSON Profile 声明 Base 与 Frontend Node 能力，由核心补全明确默认值并给出稳定规范化结果，而无需维护 Homebrew、NVM 或其他平台命令。
- 元数据：首版固定 `schema_version: 0.1.0`，要求稳定的 `name`，允许简短 `description`；版本缺失或不支持时必须说明当前支持版本及尚无迁移路径，不能静默猜测或升级。
- 模块：首版仅接受 `base` 与 `frontend_node`，每类至多一次并规范化为固定顺序。两类模块都支持 `minimal`/`standard`，省略时为 `standard`；重复模块拒绝，重复模块使用不同变体时给出冲突变体错误。
- 默认能力：Base 始终表达 Git/SSH；`standard` 默认选择基础编译工具与终端基础配置，`minimal` 默认不选择二者，用户可用显式布尔项覆盖。Frontend Node 始终表达 Node/npm；`standard` 默认选择 Corepack、pnpm 与浏览器测试基础，`minimal` 默认不选择三者，用户同样可显式覆盖。
- 版本策略与 Pin：Frontend Node 省略策略时为 `lts`，另支持最新稳定版 `stable` 与 `exact`。只有 `exact` 必须且只能携带不含 `v`、预发布或 build metadata 的 SemVer `pin`；`lts`/`stable` 不接受 Pin。Base 不接受版本策略，精确平台实现与版本解析属于 I20。
- 输入与输出：JSON Schema Draft 2020-12 是 YAML/JSON 共用的正式数据契约；解析器限制文档大小，拒绝未知字段、重复键、YAML alias/显式 tag、多文档与尾随 JSON，并在纯内存中输出带完整默认值的规范化 YAML/JSON。输入值保持不变。
- 安全边界：Profile 视为不可信且不可执行；模型与 Schema 不含 command、args、shell、script、URL、来源、凭据、路径、确认或 Plan/Action。解析和规范化不读取主机、项目、网络或历史，不生成 Lock/Plan，也不调用任何适配器或执行器。
- 风险：宽松 YAML 类型转换、alias 扩张、未知选项被静默忽略、变体默认漂移、Pin 与策略含义冲突、模块顺序影响后续解析，或把意图误作安装授权；通过严格解码、白名单 Schema、显式默认、语义校验和固定排序控制。
- 验收：合法 YAML/JSON 和 Base/Frontend 示例得到稳定规范化输出；未知模块/字段、冲突变体、非法策略/Pin、任意命令、alias、多文档和版本错误均有明确拒绝；Schema 可独立编译并验证；全量、race、vet、build、离线相关包和跨平台构建通过。
- 非范围：I19 不解析 macOS 包或来源，不生成 Lock/Plan，不安装或修改系统，不新增 CLI/Skill/MCP，不支持 Java/DevOps/其他模块，也不决定 Lock 的跨平台组织和兼容规则。

### D-055：I20-A Lock 0.1.0 单目标组织与隐私契约

- 状态：Accepted（依据 I19 客观验收、维护者既有时间盒自动过门授权及继续开发指令）
- 用户价值：一次 Profile 解析可以产生只对应一个明确 OS/架构目标、内容可校验的 Lock；用户能审查精确实现、版本、来源及解析状态，后续其他平台可从同一 Profile 生成各自 Lock，而不会把不同平台的包名强行合并。
- 组织方式：Lock `0.1.0` 采用“一份文件、一个目标”。顶层固定 Schema、内容派生 ID、生成时间、规范化 Profile 的版本/名称/SHA-256、目标 OS/版本/架构、来源快照、解析项和状态计数。跨平台复现保存多份 Lock；多目标聚合文件如有需要另开版本，不修改已冻结 `0.1.0`。
- 平台兼容：数据契约允许 `macos|windows|linux` 及受支持架构枚举，解析项的条件必须包含与顶层一致的 OS 和目标架构；I20 实际解析只开放 macOS arm64/amd64。不同平台可以选择功能等价但 manager/package ID 不同的实现。
- 来源：每个来源记录稳定 ID、`package_catalog|runtime_catalog|platform_catalog` 类型、无凭据/查询/fragment 的 HTTPS URI、快照时间和内容 SHA-256。Lock 不自行联网；I20-B 解析器必须消费调用方提供且新鲜度已判定的明确快照，不能从已安装 Inventory 猜测未安装包的版本。
- 解析项：每项绑定模块、能力 ID 和 `satisfied|install_required|conflict|unresolved`。除 unresolved 外必须携带精确 tool ID、manager、package kind/ID、版本、来源引用和平台条件；unresolved 不伪造实现。状态只描述解析结果，不是 Plan、Action 或安装授权。
- 确定性：构造器深复制并规范排序来源、解析项、架构和观察版本，拒绝重复身份、悬空来源、状态/实现不一致及计数篡改；完整规范化内容（ID 字段置空）计算 SHA-256。相同 Profile、目标和数据快照得到逐字节相同内容。
- 隐私与安全：Lock 不含路径、主机名、用户名、机器 ID、环境、凭据、命令、参数、Shell、脚本、确认、Plan/Action、stdout/stderr 或 Inventory 原始来源。已安装事实最多保留经白名单的 manager 与版本字符串，不复制 Installation ID、Path 或来源详情。
- 验收：Schema/严格 JSON codec、确定性构造与 ID 校验通过；四种状态、来源引用、跨平台条件和 Profile digest 被覆盖；未知字段/版本、命令或路径注入、带凭据来源、重复/悬空/篡改内容均拒绝；输入不变；全量、race、vet、build、离线及跨平台构建通过。
- 非范围：I20-A 不解析 Profile/Inventory/目录快照，不选择 macOS 包，不联网，不生成 Plan，不执行安装，不持久化 Lock，不新增 CLI/Skill/MCP，也不开放 Homebrew bootstrap。

### D-056：I20-B macOS Profile 纯解析规则

- 状态：Accepted（依据 I20-A 客观验收及维护者既有时间盒自动过门授权）
- 用户价值：内部调用方可把规范化 Profile、当前 Inventory 和显式版本目录快照解析为 Lock，准确区分已满足、需要安装、冲突与无法解析，不重复安排已有兼容实现。
- 目录快照：解析器只接受调用方提供的 Lock 来源列表及 capability catalog entries；每个 entry 固定 capability、`stable|lts` channel 和完整 Lock implementation。目录必须给出精确版本、manager/package/source/platform 条件；解析器不联网、不查询 Homebrew，也不猜测缺失版本。
- 能力展开：Base 始终包含 Git/SSH，按规范化选项加入 CMake 与 terminal configuration；Frontend Node 始终包含 Node/npm，按选项加入 Corepack、pnpm 与 browser testing。Node 使用 Profile 的 `lts|stable|exact`；其余首批目录项使用 `stable`。`exact` 只选择版本相同 entry。
- 当前首批边界：terminal configuration 与 browser testing 在 I20-B 固定输出 `unresolved/unsupported_capability`，不因目录中出现同名 entry 就伪装为已支持；它们的实际平台实现不属于 I21/I22 首批配装范围。
- 平台边界：只有 `macos` + `arm64|amd64` 进入目录选择；其他 OS/架构为每个期望能力输出稳定的 unsupported reason，不返回部分伪实现。implementation 条件必须包含目标 OS/架构，歧义或缺失候选输出 `version_source_unavailable`。
- Inventory 映射：只读取 tool ID、manager、规范化版本和 architecture；Homebrew formula 通过固定 `homebrew.formula.<package>` ID 映射，Node/npm/Corepack/pnpm 使用现有规范 tool ID。匹配 manager、精确规范化版本且架构未冲突即 `satisfied`；无观察为 `install_required`；有观察但无匹配为 `conflict`。不复制 Installation ID、Path 或来源。
- 确定性与安全：Profile、Inventory、catalog、来源和嵌套切片保持不变；能力、观察与最终 Lock 由 I20-A 统一排序/摘要。解析不读取文件/环境/网络/历史，不构建命令、Plan 或 Action，不执行或持久化任何内容。
- 验收：Base/Frontend Node 默认与覆盖、三种 Node 策略、四种状态、已安装兼容/冲突/无候选、非 macOS/架构及歧义目录均覆盖；相同乱序快照得到相同 Lock；私有路径和来源不进入输出；全量、race、vet、build、离线及跨平台构建通过。
- 非范围：I20-B 不提供公开 CLI，不采集目录，不保证目录新鲜度，不安装 Homebrew/NVM/包，不生成 Plan，不实现 terminal/browser 配装，也不执行 I21/I22。

### D-057：I21-A macOS Base 安装 Plan 准备边界

- 状态：Accepted（依据 I20 客观验收及维护者既有时间盒自动过门授权）
- 用户价值：用户可先得到只包含当前确需安装的 Base formula 的可审查 R2 Plan，Homebrew 缺失、Lock/Inventory 不一致或现有冲突会在任何执行能力开放前停止。
- Schema 复用：继续使用已冻结 Plan `0.2.0` 和 Operation Record `0.4.0`，不新增版本。Plan `0.2.0` 已支持多个 R1/R2 声明 Action；Base Plan 的 environment 以当前 `manager.homebrew` active installation 为锚，`policy_digest` 绑定规范化 Profile digest，precondition 绑定完整 Lock ID/来源/状态。
- 首批动作：只允许 Lock 中 `base.git` 与 `base.cmake` 的 `install_required` + Homebrew formula implementation，分别生成 `homebrew.formula.git|cmake / install / homebrew` R2 Action。动作按 ID 排序且互不伪造依赖；satisfied 项省略，conflict 项阻断整个准备，unresolved 的 SSH/terminal 或其他非首批能力不转成动作。
- 前置条件：Lock 必须完整通过内容 ID 校验、目标为当前 macOS arm64/amd64、生成时间不晚于 Plan、Profile 包含 Base，且 Inventory 当前 Schema/系统与 Lock 目标一致。Inventory 必须有唯一 active `manager.homebrew` installation；缺失时明确要求用户自行安装 Homebrew，不执行 bootstrap。
- 安全边界：Plan 仍只保存声明式 tool/operation/adapter/target 和检查元数据，不含 brew 路径之外的新命令或参数；每项要求绑定完整 Plan ID 的计划级确认、无提权、无重启、下载大小 unknown、恢复为 manual。I21-A 不创建 Registry Definition，因此执行器必须以 action_unregistered 在历史/进程前拒绝。
- 验收：单 Git、单 CMake、两项、部分 satisfied、全部 satisfied、冲突、错误 Lock/目标/Profile、Homebrew 缺失/重复 active、输入不可变和确定性 ID 均覆盖；执行隔离证明没有命令构建、历史或进程；全量、race、vet、build、离线及跨平台构建通过。
- 非范围：I21-A 不新增 CLI/确认短语，不注册或运行 `brew install`，不查询/更新 Homebrew，不执行 bootstrap，不安装 SSH/terminal config，不持久化 Plan/Lock，不生成最终差异 Lock，也不完成 I21。

### D-058：I21-B Homebrew 安装事务只读预检边界

- 状态：Accepted（依据 I21-A 客观验收、维护者批准方案 A 并继续开发的指令及既有时间盒自动过门授权；不包含提交或推送授权）
- 用户价值：在开放任何 Homebrew 写动作前，用户能审查与已确认候选 Plan/Lock 严格绑定的实际公式事务闭包，避免 `brew install` 隐式处理传递依赖或读取漂移配置后，真实变更超出 Plan 所表达的范围。
- 契约：新增内部只读 Transaction Review `0.1.0`，固定 `executable=false`、`confirmable=false` 和内容派生 ID。输入必须是完整通过不可变校验、仍在 30 分钟窗口内的 I21-A Plan 与其原 Lock，并提供同一时刻的 Homebrew 安装身份/版本、可执行文件摘要、配置摘要与安全评估，以及每个 Plan Action 的根 formula、精确版本、状态、下载字节和完整传递依赖预览。
- 绑定与规范化：Plan 的 policy、环境、Action、Lock/source 前置条件及精确验证必须与 Lock 逐项一致；预览必须恰好覆盖全部 Action。根 formula 固定 `git|cmake` 且为 `install_required`；依赖只允许 `satisfied|install_required`、稳定精确版本和同一已锁定 catalog。Action 与依赖按身份排序；跨 Action 的重复 formula 必须逐字段一致，总下载量按唯一 formula 计算并防止溢出。
- 安全与隐私：配置评估非 safe 或发现任何未批准配置键时停止；封存结果只保留摘要和安全事实，不保存 HOME、brew 路径、源 URI、环境变量值、命令、参数、确认或凭据。未知/零下载大小、目录/可执行文件摘要不合法、Homebrew/来源漂移、遗漏/额外 Action、公式冲突均拒绝。
- 验收：Git/CMake 单项与双项、共享依赖去重、乱序确定性、输入不变、Plan/Lock/来源/Homebrew/配置漂移、Action 漏项/额外项、根公式或精确版本不符、重复依赖冲突、未知大小和总量溢出均覆盖；结果篡改后严格验证失败；全量、race、vet、build、离线及跨平台构建通过。
- 非范围：I21-B 不执行或注册 `brew install`，不自行调用/更新 Homebrew，不解释 shell 或 Homebrew 环境文件，不新增公开 Schema/CLI/确认，不写 Operation/Lock，不 bootstrap、不卸载，也不声称完成 I21。实际只读采集器与固定写适配器分别在后续最小增量开放。

### D-059：I21-C Homebrew 事务事实采集边界

- 状态：Accepted（依据阶段二合并完成及维护者进入阶段三的明确指令；不包含提交或推送授权）
- 用户价值：I21-B 不再依赖调用方手工拼接 Homebrew Baseline 和依赖预览；在任何写适配器开放前，确定性核心可从一组可审查的同刻快照生成完整事务事实，并对配置、catalog、当前安装和 bottle 成本漂移提前失败。
- 输入与副作用：核心只接收调用方显式提供的当前 Inventory、brew 可执行文件路径与内容、进程环境、system/prefix/user 三层 `brew.env` 内容、完整官方 catalog JSON、目标 bottle tag 以及带 catalog SHA-256 的下载大小事实。它不自行发现或读取路径，不启动 `brew`/Shell，不访问网络，不写历史、Plan、Lock 或系统状态。
- 配置策略：只把 `HOMEBREW_*`、大小写 proxy 和 `SUDO_ASKPASS` 视为影响面；环境文件严格按 `KEY=VALUE` 数据解析并依次覆盖 environment → system → prefix → user。任何未批准键或非固定安全值停止；当前白名单要求 `HOMEBREW_NO_AUTO_UPDATE=1`、`HOMEBREW_NO_INSTALL_UPGRADE=1`、`HOMEBREW_NO_INSTALL_CLEANUP=1`、`HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1`、`HOMEBREW_NO_ANALYTICS=1`、`HOMEBREW_NO_ASK=1`、`HOMEBREW_NO_ENV_HINTS=1`。不保存或回显配置值。
- catalog 与闭包：catalog 原始字节 digest 必须等于 Lock 的唯一 `homebrew-core` package catalog source，并固定官方 URI。完整 catalog 只建立有界、名称唯一的索引；严格的 `homebrew/core`、版本、revision、依赖和 bottle 校验只作用于 Git/CMake 的可达闭包，避免无关 formula 的合法版本格式阻断事务。精确版本包含 formula revision，依赖使用目标 bottle tag 的 variation 覆盖和 required+recommended 传递闭包，拒绝缺项、重复、循环、disabled 和超过 256 个 formula 的闭包。当前 Inventory 中同 manager/目标架构的精确版本才标记 satisfied；目标架构存在其他版本时作为冲突停止，完全缺失时才要求与 catalog bottle SHA-256、版本和目标 tag 一致的正下载大小事实；其他架构安装不计为满足或冲突，多余 artifact 同样拒绝。
- 隐私与边界：输出只含 I21-B 的 `TransactionBaseline` 与 `ActionPreview`，不含文件路径、环境键值、catalog URI、bottle URL、命令、参数、确认或凭据。I21-C 不新增公开 Schema/CLI，不注册或运行 `brew install`，不完成 I21；固定写适配器和执行后 Lock/diff 继续后移。
- 验收：双 formula、共享/已满足依赖、catalog/Artifact 乱序、target variation、formula revision、输入不可变、Plan/Lock/Inventory/Homebrew/配置/catalog/bottle 漂移、恶意/超大配置、缺失/多余/零大小 artifact、依赖缺失/循环/过大、输出无敏感信息及 I21-B 端到端接受均覆盖；全量、race、vet、build、离线和跨平台构建必须通过。

### D-060：I21-D 固定 Homebrew 写适配器与重新确认边界

- 状态：Accepted（依据 I21-C 合并完成及维护者进入阶段四的明确指令；不包含提交、推送、发布或真实机器写入授权）
- 用户价值：已审查的 Git/CMake Homebrew 事务可以进入现有确定性执行器；任何候选 Plan、Review、brew 身份/字节、HOME/TMPDIR、配置、catalog、依赖闭包或安装状态漂移都会在写入前停止或被执行后精确验证捕获，失败不会误报完成。
- 双重绑定与确认：I21-A 输出继续作为候选 Plan；`BindBaseTransactionReview` 为每项动作加入 Review ID 并派生不同的最终 Plan ID。执行适配器同时持有候选 Plan、Review 和最终 Plan，重新派生后逐字段比较。用户确认固定绑定最终 Plan ID 与 Review ID；Operation Record 复用 `0.4.0`，保存最终 Plan，因而保留 Review 前置条件与确认来源。
- 固定动作：只注册 Review 恰好覆盖的 `homebrew.formula.git|cmake / install / homebrew` R2 Action，命令固定为绝对 active brew 路径加 `install --formula --force-bottle <git|cmake>`，不接受调用方参数、任意 formula、Shell、提权或 bootstrap。环境仅含经 Review 摘要绑定的 HOME/TMPDIR、brew/system PATH 及七个固定 Homebrew 安全变量，不继承 proxy、凭据或用户任意变量。
- 重新采集、幂等与验证：确认后、历史和写进程前，使用新鲜显式快照重新运行 I21-C，忽略观察时间变化但要求全部执行事实一致。每项写动作再组合固定 `brew list --formula --versions` 与 `brew list --formula --full-name` 两个紧凑探针，拒绝旧版、多个 keg、重复/非 core tap、输出截断和已满足依赖漂移；根及完整 Review 闭包都精确满足时跳过写进程。进程成功后要求根与全部依赖恰有一个精确版本，否则记录验证失败。
- 失败与恢复：复用 Operation Record 的 pending/running/verifying/completed/failed 状态和 before/after/diff；preflight 或探针失败不会启动写命令，进程失败和验证失败不会标记 completed，后续 Action 保持 pending。Action 恢复仍为 manual；固定检查点可将已记录变更复核为 current/drifted，不生成或执行卸载，因为卸载仍是独立 R3 Plan。
- 非范围：不新增公开 Schema/CLI/Skill/MCP，不自动读取文件或环境，不自行下载 catalog/artifact，不更新/安装 Homebrew，不换源、不安装 cask、不开放任意版本或 formula，不生成执行后 Lock/diff，不进行真实机器安装验收，也不完成 I21。
- 验收：最终 Plan 内容派生和输入不可变；确认/平台/快照/配置漂移在历史及写进程前停止；固定 spec 与隔离环境；根/依赖 absent/exact/conflict、多版本、重复/恶意或截断探针输出；幂等跳过、安装成功、preflight/进程/验证失败、Operation 隐私和恢复 current/drifted 均覆盖；全量、race、vet、build、module、离线及 Linux/Windows 跨平台构建通过。

### D-061：I21-E 执行后最终 Lock 与差异收敛边界

- 状态：Accepted（依据 I21-D 合并完成及维护者开始下一阶段任务的明确指令；不包含提交、推送、PR、合并、发布或真实机器写入授权）
- 用户价值：只有一次完整成功且身份绑定的 Base Homebrew Operation 与执行后精确 Inventory 同时成立时，调用方才能得到新的最终 Lock 和最小结构化差异；失败、部分成功或当前状态漂移不会产生误导性的成功 Lock。
- 入口与证据：新增内部纯 `Finalize` 入口，只接受显式 `finalized_at`、I21-D `Prepared`、Operation Record `0.4.0` 和同一时刻的当前 Inventory。入口重新派生 Prepared，要求 Record 为 Completed、保存的 confirmed Plan 与最终 Plan 逐字段一致、结束时间不晚于收敛时间，并核对每个 Action 的 before/after Snapshot 与 Review 公式闭包；不读取历史文件、环境、Homebrew、网络或主机状态。
- 执行后状态：Inventory 必须完整通过当前 Schema，时间恰好等于 `finalized_at`，目标 OS/版本/架构与 Lock 相同，并且唯一 active Homebrew installation 的 ID、版本、manager、路径和架构仍与确认 Plan 一致。Review 中全部根与传递依赖必须在目标或 unknown 架构下各有且仅有一个 Homebrew installation，并匹配精确版本；缺失、旧版、多版本或身份漂移全部停止。
- Lock 派生：新增 `lockfile.DeriveState`，只能更新原 Lock 中已有 item 的 state、observed 和 reason；Profile reference、target、sources、item/module/capability 和 implementation 全部继承，重新计算 summary、规范顺序和内容 ID。I21-E 只把本次 Review 对应的 `base.git|base.cmake` 从 `install_required` 收敛为 `satisfied`，预先 satisfied 或非本次项保持逐字段不变。
- 差异与隐私：结果只含 Operation/Plan/Review ID、完整最终 Lock，以及按 item ID 排序的 capability/tool/manager/version 与 before/after state。它不包含原始 Inventory、Installation ID/path、命令、参数、环境、Snapshot facts、stdout/stderr 或错误详情；输出中的 Lock 继续遵守既有无路径隐私契约。
- 失败边界：非 Completed、旧 Schema、缺失状态快照、Snapshot 闭包伪造、最终时间倒退、Inventory/目标/Homebrew/公式闭包漂移均返回空结果，不为部分完成动作生成部分 Lock。自动扫描、持久化及恢复/卸载决策仍由后续入口负责。
- 验收：单动作与 Git+CMake 双动作、确定性 ID、输入不可变、已满足项不变、差异排序、二次 Plan 无安装动作、输出隐私，以及 Prepared/Record/Snapshot/时间/目标/Homebrew/root/dependency 缺失、旧版、重复和漂移失败均覆盖；全量、race、vet、build、module、离线及 Linux/Windows 跨平台构建通过。
- 非范围：不新增或修改公开 Schema/CLI/Skill/MCP，不自动扫描或持久化 Inventory/Lock，不调用 Homebrew 或执行真实安装，不处理失败后的恢复/卸载，不 bootstrap/更新/换源 Homebrew，也不完成 I21 的可恢复 macOS 环境验收。

### D-062：I21-F1 默认禁用的可恢复 macOS 双阶段验收入口

- 状态：Accepted（依据 I21-E 合并完成及维护者开始下一步任务的明确指令；不包含提交、推送、发布或真实 Homebrew 写入授权）
- 用户价值：维护者可以先在一次完全只读的准备阶段审查真实 macOS VM 上生成的 Base Plan 与 Homebrew Review，再用精确 Plan/Review 身份启动独立应用阶段；测试入口不会因普通单元测试、默认 CI 或发行构建而意外修改机器。
- 形态：入口仅存在于 `internal/baseapply` 的测试构建，要求 `darwin && envmason_live_i21` build tag、固定测试名、显式 `prepare|apply` 模式、仓库外绝对 bundle 路径和一次性 disposable-VM 声明。默认 `go test ./...`、`go build ./...`、CLI 和发布产物不包含或运行实机入口。
- 两阶段边界：`prepare` 只执行固定只读系统/Homebrew 查询、读取固定配置文件和 brew 可执行文件、获取官方 `formula.json` 及 GHCR bottle HEAD 元数据；它生成内容派生、权限受限的私有 bundle，并输出不含环境值或原始 Inventory 的 Plan/Review 摘要及精确确认 token。`apply` 重新严格解码和校验 bundle，要求 token 同时绑定最终 Plan ID 与 Review ID，重新采集全部新鲜事实后才调用现有 I21-D 服务。
- 环境安全门：只允许 macOS arm64/amd64、系统级 active Homebrew 和仓库外 bundle；Homebrew 缺失、Git/CMake 任一目标架构 Homebrew formula 已安装、多个安装版本、Homebrew/config/catalog/bottle 漂移、Plan 过期或确认错误均在 Operation 历史和写进程前停止。当前开发机已安装 Git/CMake，因此只能验证拒绝路径，不能成为 I21-F2 写入环境。
- 网络与来源：catalog URL 固定为无查询/凭据的 `https://formulae.brew.sh/api/formula.json`，大小有界并以原始 SHA-256 绑定 Lock。Bottle URL 只接受官方 `ghcr.io/v2/homebrew/core/.../blobs/sha256:<digest>`；只通过受限匿名 token challenge 和 HEAD 读取正 `Content-Length`/digest，不下载 bottle、不写 Homebrew cache、不跟随任意 realm/host。
- 私有证据：bundle 保存规范化 Profile、完整 Prepared、原始 catalog 和白名单 artifact 元数据，但不保存确认 token、GHCR token、进程输出或环境继承；只能在仓库外创建真实普通文件，父目录/文件权限分别为 `0700`/`0600`，符号链接拒绝。Operation 历史、最终 Lock 和 Outcome 同样写在该私有 VM 目录，最终可分享证据仅使用既有脱敏模型。
- I21-F2 边界：F1 只交付并模拟验证入口，不在当前机器执行安装。真实 `brew install` 仍是独立 I21-F2：必须在 Homebrew 已存在而 Git/CMake formula 均缺失的可恢复 VM 快照中，再次获得绑定输出 Plan/Review 的 R2 明确确认；成功后验证最终 Lock、二次零动作，并恢复或销毁 VM。
- 风险：build tag/环境变量误触发、确认复用、私有 bundle 被篡改或落入仓库、prepare 暗中下载/写 cache、外部 URL/token 泄漏、非空环境被误作首次配装，或把 F1 误报为真实验收；通过多重门、内容 ID、严格 codec、固定网络白名单、HEAD-only 客户端、零写 runner 断言和明确 F1/F2 状态控制。
- 验收：默认不可达与 tagged 显式入口、mode/path/VM 声明/确认矩阵、bundle 内容 ID/严格解码/权限/符号链接、非空 Git/CMake 拒绝、固定 catalog/GHCR URL 与 HEAD/token challenge、准备零安装/历史、apply 漂移零写、输出隐私和输入不变均覆盖；全量、race、vet、build、module、离线、Linux/Windows 构建及 macOS tagged 安全拒绝测试通过。
- 非范围：不新增公开 CLI/Schema/Skill/MCP，不自动创建/启动/恢复 VM，不安装/bootstrap/update/cleanup/uninstall/换源 Homebrew，不执行 cask/任意 formula，不把环境变量当作用户确认，不在默认 CI 运行 live 模式，也不完成 I21 或进入 I22。

## 已规划、尚未决定的事项

| 事项 | 最迟决策增量 |
|---|---|
| I18 可执行继续 Plan 与 Operation Record Schema | I18-I |
| I18 恢复对象与混合 R2/R3 工作流 | I18-Q 前（D-047） |
| Windows 提权与 WinGet Configuration 边界 | I28 前 |
| Ubuntu LTS 具体版本范围 | I31 前 |
| 第三方适配器动态加载与隔离方案 | I51 |
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

## I05 验收记录

- 增量：I05 Homebrew 只读适配器
- 开始日期：2026-07-16
- 客观检查状态：本地与远程 CI 均通过（2026-07-16）
- 维护者最终验收：Accepted（依据 D-014 维护者预授权，2026-07-16）
- 功能检查：Homebrew 缺失返回 `not_installed` 且扫描成功；fixture 覆盖 Apple Silicon `/opt/homebrew` 与 Intel `/usr/local` 前缀、formula、cask、outdated、pin 和多版本安装。
- 映射检查：formula 和 cask 映射到统一 Tool/Installation；直接安装与依赖安装可区分，`linked_keg` 对应生效与默认版本；映射结果通过 Inventory `0.2.0` Schema 校验。
- 失败与隐私检查：命令失败和无效 JSON 降级为 Finding 后继续扫描；锁占用单独识别；测试令牌、原始错误、远端凭据、查询参数和 fragment 均未进入结果。
- 安全检查：测试逐项断言八种固定只读查询及三项 Homebrew 防副作用环境变量；未发现 update、安装、升级、卸载、清理、tap/untap 或换源调用。
- 真机检查：本机 Homebrew 6.0.9 的版本、前缀、仓库、formula、cask 和 outdated 查询成功；适配器运行前后 Homebrew 仓库状态与已安装包版本清单完全一致。
- 可靠性检查：命令使用 30 秒超时、stdout/stderr 独立上限和不经 Shell 的参数调用；超限与失败路径测试通过。
- 自动检查：`go test ./...`、`go test -race ./...`、`go vet ./...`、`go build ./...`、gofmt 和 `git diff --check` 均通过。
- 跨平台检查：全部包面向 Linux amd64 和 Windows amd64 编译通过；Windows CI 不执行依赖 POSIX 文件执行权限的 Homebrew 集成 fixture，但继续执行解析器、缺失场景、runner 和全仓测试。
- CI 回归检查：首次远程运行暴露 Windows 无法用 POSIX 权限位构造 `brew` fixture；修复为 POSIX 路径跨宿主解析并限定该集成 fixture 的适用平台后，[GitHub Actions CI](https://github.com/gitbagHero/EnvMason/actions/runs/29485534755) 六个任务全部通过。
- N/A：I05 不包含 CLI 新命令、公开 Schema 变更、网络版本查询、更新、安装、卸载、清理、换源、修复或系统修改。
- 结论：I05 已依据维护者预授权完成验收，已经提交到 `main` 且远程 CI 通过；I06 已具备顺序依赖条件，但尚未开始。

## I06 验收记录

- 增量：I06 Node.js 生态只读适配器
- 开始日期：2026-07-16
- 客观检查状态：本地与远程 CI 均通过（2026-07-16）
- 维护者最终验收：Accepted（依据 D-014 维护者预授权，2026-07-16）
- 场景检查：fixture 覆盖仅系统 Node、仅 NVM、多来源和无 Node；另覆盖 Homebrew 来源、Node 22 保留且 Node 24 为默认、NVM 未加载时从默认磁盘目录降级发现。
- NVM 检查：安装版本按版本目录稳定发现；default 支持数字前缀、`node` 和多级 alias，循环 alias 被拒绝；当前 Shell 生效版本与默认版本分别表达。
- 归属检查：npm 和 Corepack pnpm 代理均关联到明确的 Node Installation ID；PATH 生效、遮蔽、多来源和离线 NVM 包管理器实例可区分。
- Corepack 安全检查：pnpm/Yarn Corepack 代理不执行；动态版本记为 `unknown` 并记录 provider 版本；所有允许的版本进程只使用 `--version`，且 Corepack 网络、latest、auto-pin、项目选择和下载提示均关闭。
- 失败与隐私检查：版本命令失败、非法输出、NVM 未加载、alias 无法解析和候选异常均降级为固定 Finding；runner stderr、测试令牌、`NODE_OPTIONS` 和 npm token 未进入结果或子进程环境，HOME 路径被替换为 `$HOME`。
- 真机检查：本机发现 6 个 NVM Node 版本和 20 个 npm/Corepack/pnpm/Yarn 实例，正确识别当前与默认 Node v26.5.0；扫描前后 NVM alias、版本目录的路径、时间和权限快照一致。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、gofmt 和 `git diff --check` 均通过。
- 离线与跨平台检查：`GOPROXY=off` I06 核心测试通过；全部包面向 Linux amd64 和 Windows amd64 编译通过。Windows CI 不执行依赖 POSIX 执行权限、软链接和 `lts/*` 文件名的 NVM 集成 fixture，但继续执行 runner 和全仓测试。
- CI 回归检查：首次远程运行暴露 Windows 不能构造 NVM 的 POSIX `lts/*` alias 文件；限定该文件系统集成 fixture 的适用平台后，[GitHub Actions CI](https://github.com/gitbagHero/EnvMason/actions/runs/29487223348) 六个任务全部通过。
- N/A：I06 不包含 CLI 新命令、公开 Schema、项目 packageManager 评估、联网版本查询、安装、删除、alias 修改、全局包升级或任何系统修改。
- 结论：I06 已依据维护者预授权完成验收，已经提交到 `main` 且远程 CI 通过；I07 已具备顺序依赖条件，但尚未开始。

## I07 验收记录

- 增量：I07 Java 生态只读适配器
- 开始日期：2026-07-16
- 客观检查状态：本地与远程 CI 均通过
- 维护者最终验收：Accepted（依据 D-014 预授权）
- JDK 检查：fixture 覆盖单 JDK、系统与 Homebrew 多 JDK、JAVA_HOME 去重和架构/厂商元数据；系统 plist、Homebrew `release` 和 jenv 注册指向同一 home 时合并为一个安装。
- jenv 检查：global/local/shell 优先级可分别表达；local 从存在项目目录向父级查找最近 `.java-version`，不存在项目目录不产生虚假 local；断裂注册产生 Finding 后继续扫描。
- 运行时检查：当前 `java` 只保留四个白名单属性并关联到 JDK ID；Maven 的工具版本、Java 版本和 runtime home 独立表达；Gradle 只根据本地分发元数据及已有配置表达可确定字段。
- 冲突检查：fixture 验证 jenv local 与实际 Java 不一致、Maven Java 与实际 Java 不一致分别产生 Finding；Gradle Java 一致时不误报。
- 失败与隐私检查：系统注册、当前 Java 或 Maven 命令失败只影响对应字段，Gradle 元数据缺失也只影响 Gradle；固定错误不包含 runner 原始输出或测试令牌，HOME 路径脱敏，子进程不继承 Java/Maven 注入钩子。
- 真机检查：本机去重识别 10 个 JDK，当前 Java、jenv global 和 Maven runtime 均为 25.0.3；正确保留 Java 8/17/21/23/24 等其他安装。扫描前后 jenv、Maven 与 Gradle 用户目录的路径、时间和权限快照一致。
- 安全检查：仅调用 `java_home -X`、`java -XshowSettings:properties -version` 和 `mvn --version`；Gradle 完全不执行，未发现安装、删除、jenv 写入、构建任务、Wrapper 或 Daemon 管理命令。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、gofmt 和 `git diff --check` 均通过。
- 离线与跨平台检查：`GOPROXY=off` I07 核心测试通过；全部包面向 Linux amd64 和 Windows amd64 编译通过。
- CI 回归检查：首次远程运行发现 macOS 预装 Gradle 即使仅查询版本也会初始化用户目录；改为只读解析本地分发元数据与已有配置后，[GitHub Actions CI](https://github.com/gitbagHero/EnvMason/actions/runs/29489120547) 六个任务全部通过。
- N/A：I07 不包含 CLI 新命令、公开 Schema、远程版本/EOL、安装、升级、删除、配置写入或系统修改。
- 结论：I07 已依据维护者预授权完成验收，已经提交到 `main` 且远程 CI 通过；按维护者要求在此暂停，I08 尚未开始。

## I08 验收记录

- 增量：I08 macOS 首份综合 Markdown/JSON 报告
- 开始日期：2026-07-17
- 客观检查状态：本地与远程 CI 均通过
- 维护者最终验收：Accepted（依据 D-014 维护者预授权，2026-07-17）
- 接口检查：`envmason report` 默认输出 summary；`--format summary|markdown|json`、可重复 `--category` 和 `--severity` 按维护者确认语义工作。非法格式返回退出码 2，整体运行失败返回退出码 1。
- 一致性检查：fixture 从同一个 Inventory 分别渲染三种格式，并逐项确认过滤后的 Tool、Installation 和 Finding 事实一致；终端摘要和 Markdown 都标注扫描时间、范围、完整状态、来源及失败项。
- Schema 检查：真实与 fixture JSON 均由 `inventory.Marshal` 在输出前通过嵌入式 Inventory Schema `0.2.0` 校验；本增量没有修改或升级公开 Schema。
- 过滤检查：重复类别、重复严重程度去重；同维度 OR、跨维度 AND；系统信息始终保留，带 Tool ID 的 Finding 跟随类别过滤，不带 Tool ID 的全局 Finding 保留。
- 降级检查：Homebrew 与 Java section 同时失败时 Node 结果仍被保留，三种格式仍可生成并显示 incomplete；单个 Node 版本探测失败也会增加 `REPORT_INCOMPLETE`。适配器原始错误及测试令牌不进入报告。
- Markdown 检查：固定一级标题和 System、PATH、Tools、Findings、Data Sources 二级结构；表格单元格中的竖线、换行和反引号被转义或规整。
- 真机检查：编译后的真实二进制识别 macOS 15.7.4 arm64、zsh、38 个 PATH 条目、90 个工具和 140 个安装实例；逐项对照 Homebrew formula 74/74、cask 8/8、NVM Node 6/6，并识别 10 个 JDK、Maven 及当前 Node/Java。未发现已知漏报或误报。
- 只读与隐私检查：真实扫描前后 `.nvm/alias`、`.jenv`、`.m2`、`.gradle` 的路径、修改时间、权限和大小快照一致；JSON、summary 和 Markdown 均未出现真实 HOME、`127.0.0.1:10090`、URL 认证信息或常见凭据标记。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、gofmt 和 `git diff --check` 均通过。
- 离线与跨平台检查：`GOPROXY=off` report、CLI、inventory 测试通过；全部包面向 Linux amd64 和 Windows amd64 编译通过，非 macOS 调用 report 在任何适配器执行前返回明确的不支持错误。
- CI 检查：[I08 分支 CI](https://github.com/gitbagHero/EnvMason/actions/runs/29550292286)和合入后的 [main CI](https://github.com/gitbagHero/EnvMason/actions/runs/29550372566)均为 macOS、Ubuntu、Windows × Go 1.25/1.26 六个任务全部成功。
- N/A：I08 不包含远程最新版、EOL、版本比较、建议、Plan、安装、升级、卸载、配置写入或任何系统修改。
- 结论：I08 已依据维护者预授权完成验收并合入 `main`；I04～I08 批次及 macOS 首个只读预览里程碑完成，I09 尚未开始。

## I09 验收记录

- 增量：I09 通用版本规范化与比较
- 开始日期：2026-07-17
- 客观检查状态：本地门禁与远程 CI 均通过
- 维护者最终验收：Accepted（依据 D-014 维护者预授权，2026-07-17）
- 接口检查：新增独立 `internal/version` 确定性核心，解析结果保留 Raw、Normalized、Scheme 和 Comparable；非法输入、跨 scheme 或缺失内部解析状态均返回 Unknown。
- SemVer/Node 检查：覆盖 SemVer 2.0.0 核心版本、预发布优先级、build metadata、小写 `v` 前缀、任意长度数值及非法前导零；build metadata 不影响比较。
- Java 检查：覆盖现代数值版本、`1.8.0_361`、`8u361`、EA、build number 和受限厂商/支持标签；传统 Java 8 表达归一到同一比较线，GA 厂商 build 不参与跨厂商更新排序。
- 比较性质检查：表驱动测试验证等价、边界值、反对称性和传递性；模糊测试验证任意输入不会 panic，成功解析值自反且交换参数后关系反转。
- 保守失败检查：空白、不完整、歧义、超长、未知标签和非法分隔符输入均不可比较，不进行字符串兜底排序，也不产生升级或清理结论。
- 自动检查：`go test -count=1 ./internal/version`、`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、gofmt 和 `git diff --check` 均通过。
- 离线与跨平台检查：`GOPROXY=off go test -count=1 ./internal/version` 通过；全部包面向 Linux amd64 和 Windows amd64 编译检查通过。
- CI 检查：[I09 分支 CI](https://github.com/gitbagHero/EnvMason/actions/runs/29557979075)和合入后的 [main CI](https://github.com/gitbagHero/EnvMason/actions/runs/29558052726)均为 macOS、Ubuntu、Windows × Go 1.25/1.26 六个任务全部成功。
- N/A：I09 不新增 CLI、公开 Schema、网络访问、建议、Plan、缓存、安装、升级、卸载、配置写入或系统修改。
- 结论：I09 已依据维护者预授权完成验收并合入 `main`；I10 已具备顺序依赖条件，但尚未开始。

## I10 验收记录

- 增量：I10 远程版本与 EOL 数据提供器
- 开始日期：2026-07-17
- 客观检查状态：本地功能测试、全量门禁与远程 CI 均通过
- 维护者最终验收：Accepted（依据 D-014 维护者预授权，2026-07-17）
- 接口检查：`envmason report` 仍默认完全离线；只有显式 `envmason report --online` 才并发查询四个官方只读来源。
- 来源检查：Node.js 使用官方 release index 和 Release 工作组 schedule；Java 使用 Adoptium available releases API，生命周期数据明确限定为 Eclipse Temurin，其他 JDK 厂商保守返回 Unknown。
- 数据语义检查：区分 Latest Stable、Latest LTS、stable、LTS、EOL 和 Unknown；报告显示来源 URL、数据获取时间及 fresh/stale/unavailable 状态。
- 降级检查：正常网络、并发超时、无网络、损坏缓存、过期缓存和 fresh 缓存测试通过；过期缓存只标 stale 且明确“not confirmed latest”，远程异常不阻止本地报告生成。
- 安全检查：每来源 5 秒超时和 2 MiB 响应上限；错误输出使用固定代码且不包含响应体或底层网络错误；来源 URL 移除认证信息、查询和 fragment。
- 缓存与写入检查：I10 缓存契约只有 Read，不提供 Write；默认与 `--online` 生产路径均不创建或修改磁盘缓存。持久化缓存写入延后到具备 Plan 的后续增量。
- 真实来源 smoke：2026-07-17 通过本机代理成功读取四个官方来源，识别 Node latest stable `v26.5.0`、latest LTS `v24.18.0`、Java latest feature `26` 和 latest LTS `25`，并解析 Temurin 生命周期表。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、gofmt 和 `git diff --check` 均通过；目标包在 `GOPROXY=off` 下通过，全部包面向 Linux amd64 和 Windows amd64 编译检查通过。
- CI 检查：[I10 分支 CI](https://github.com/gitbagHero/EnvMason/actions/runs/29558808460)和合入后的 [main CI](https://github.com/gitbagHero/EnvMason/actions/runs/29558918699)均为 macOS、Ubuntu、Windows × Go 1.25/1.26 六个任务全部成功。
- N/A：I10 不比较本机项目约束，不生成升级/清理建议，不新增或修改公开 Inventory Schema，不包含 Plan、安装、升级、卸载、配置写入或系统修改。
- 结论：I10 已依据维护者预授权完成验收并合入 `main`；I09～I10 一小时安全微批次完成并按约定暂停，I11 尚未开始。

## I11 验收记录

- 增量：I11 项目版本引用扫描
- 开始日期：2026-07-17
- 客观检查状态：本地功能测试、全量门禁与远程 CI 均通过
- 维护者最终验收：Accepted（依据 D-014 维护者预授权，2026-07-17）
- 接口检查：可重复 `report --project` 和 `--exclude` 已接入现有 summary、Markdown 和 JSON 报告；默认不扫描项目，`--exclude` 缺少 `--project` 时真实二进制返回退出码 2，项目扫描可与 `--online` 正交组合。
- 格式与冲突检查：覆盖 `.nvmrc`、`.node-version`、`package.json engines.node`、`.java-version`、`.tool-versions`、`pom.xml`、`build.gradle`/`build.gradle.kts` 的静态正例、缺失字段、损坏和动态表达式；Node 简单范围与精确版本、Java 不等价精确版本形成独立冲突 Finding，等价版本和 Unknown 不误报。
- 遍历、安全与隐私检查：内置依赖/构建/版本库目录、用户排除路径和符号链接均不被扫描；深度、目录数、文件数和单文件大小上限均有测试。未知声明不回显原文，测试令牌不进入结果，HOME 路径使用 `$HOME` 脱敏。
- 功能检查：真实 fixture 输出 9 条结构化项目引用，并分别识别 Node 与 Java 冲突；扫描前后 fixture 的路径、修改时间、权限和大小摘要一致。项目 Finding 的 JSON 继续通过 Inventory Schema `0.2.0`。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、gofmt 和 `git diff --check` 均通过；目标包在 `GOPROXY=off` 下通过，全部包面向 Linux amd64 和 Windows amd64 编译检查通过。
- CI 检查：[I11 分支 CI](https://github.com/gitbagHero/EnvMason/actions/runs/29560937178)和合入后的 [main CI](https://github.com/gitbagHero/EnvMason/actions/runs/29561069909)均为 macOS、Ubuntu、Windows × Go 1.25/1.26 六个任务全部成功。
- N/A：I11 不执行项目脚本或构建工具，不修改项目配置，不生成升级、删除或清理建议，不新增公开 Schema，不包含 Plan 或系统修改。
- 结论：I11 已依据维护者预授权完成验收并合入 `main`；I11 单增量一小时时间盒完成并按约定暂停，I12 尚未开始。

## I12 验收记录

- 增量：I12 首版建议与冲突规则
- 开始与检查日期：2026-07-17
- 客观检查状态：Passed
- 维护者最终验收：Accepted（依据 D-014 与 D-022 维护者预授权，2026-07-17）
- 规则检查：Node 决策表覆盖 LTS、Stable、Pin、推荐、可更新、通道不匹配、忽略和 Unknown；Current 不会被表达为 LTS，stale/unavailable 数据不会产生确定更新结论。
- 保留检查：项目精确主版本引用可匹配已安装 patch 版本并输出 `retain_required`；建议文本不会产生删除、移除或卸载指令。
- Java 检查：Temurin EOL 只应用于扫描期确认的 Eclipse Adoptium/Temurin vendor；未知 vendor 不产生确定 EOL。Maven、Gradle、JAVA_HOME 和 jenv 的运行时不一致会产生结构化冲突建议。
- 策略检查：`--policy` 仅显式读取最大 64 KiB 的严格 JSON；未知字段、工具、通道、非法 Pin、尾随 JSON 和超大文件均被拒绝，读取前后文件大小与修改时间不变。`ignore_updates` 不隐藏 EOL、项目保留和冲突。
- Schema 检查：Inventory `0.3.0` 的 `status`、`recommendation` 和 `impact` 可选字段通过 Draft 2020-12 校验；`0.2.0` 与 `0.1.0` 保持原文件和验证能力，公开示例与 golden 已更新。
- 输出检查：summary、Markdown 和 JSON 均显示结构化评估语义；JSON 在输出前通过嵌入式 Schema 校验。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./cmd/envmason`、gofmt 和 `git diff --check` 均通过。
- 远程检查：[I12 分支 CI #31](https://github.com/gitbagHero/EnvMason/actions/runs/29563334632) 与 [main CI #32](https://github.com/gitbagHero/EnvMason/actions/runs/29563470099) 均通过 Ubuntu、macOS、Windows × Go 1.25/1.26 六任务矩阵。
- N/A：I12 不生成 Plan，不执行安装、升级、卸载、清理、默认切换、Shell、提权或系统写入。
- 结论：I12 已依据维护者预授权完成验收并进入 `main`；I13 已具备顺序依赖条件，但尚未开始。

## I13 验收记录

- 增量：I13 只读 Plan/Action 模型
- 开始与检查日期：2026-07-17
- 客观检查状态：Passed
- 维护者最终验收：Accepted（依据 D-014、D-022 与 D-023 维护者预授权，2026-07-17）
- 接口检查：公开 `envmason plan --tool runtime.node --online --format summary|json`，并复用只读 `--policy`、`--project`、`--exclude`；未知工具/格式、缺少 `--online` 或孤立 `--exclude` 在调用生成器前返回退出码 2。
- 建议门禁：只有安全可比较且较旧的 Node 版本、fresh LTS/Stable 或已由 fresh 官方 release index 精确验证的 Pin、可识别的 NVM 能力同时成立时才生成 Plan；stale、ignored、已推荐、通道更高或无 NVM 均不伪造动作。
- Plan 契约：Plan Schema `0.1.0` 固定 `executable=false`，恰好 30 分钟有效；包含内容派生 ID、环境/策略 digest、结构化环境摘要，以及带依赖、R2 风险、计划级确认、下载状态、前置条件、验证与恢复元数据的 NVM `install_version` Action。
- 不可变检查：固定输入与时间生成字节一致的 JSON 和相同 Plan ID；修改目标、风险、验证、恢复、环境、策略或时间会使旧 ID 失效。环境摘要与 digest 不一致也会被拒绝。
- 安全验证：未知风险、R1 降级、提权、可执行标志、非法目标、重复/未知依赖、依赖环、缺失 verifier、缺失恢复说明和未知 Schema 字段均被 Schema 或语义校验拒绝。
- 执行边界：Plan JSON 不含 command、args、Shell 或 executor；源码能力审计确认生产 `internal/plan` 不直接导入 `os/exec`、`syscall`、`unsafe`，也不调用文件创建、修改、删除或重命名 API。
- 真机 smoke：真实二进制帮助正确显示 `plan`；缺少 `--online` 返回退出码 2；本机当前无合格 Node 更新建议时返回退出码 1 和明确错误，没有生成空 Plan 或系统变更。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./cmd/envmason`、gofmt、`git diff --check` 及 `GOPROXY=off` Plan/Schema/Assessment/Report 核心测试均通过。
- 远程检查：[I13 分支 CI #34](https://github.com/gitbagHero/EnvMason/actions/runs/29564907148) 与 [main CI #35](https://github.com/gitbagHero/EnvMason/actions/runs/29565075895) 均通过 Ubuntu、macOS、Windows × Go 1.25/1.26 六任务矩阵。
- N/A：I13 不保存或接受 Plan、不接受用户确认、不提供 apply、执行器、进程调用、提权、安装、升级、卸载、清理、操作日志或系统写入。
- 结论：I13 已依据维护者预授权完成验收并进入 `main`；I12～I13 两小时时间盒批次完成，按约定停在 I14 之前。

## I14 验收记录

- 增量：I14 通用受控执行器与操作日志
- 开始与检查日期：2026-07-17
- 客观检查状态：Passed
- 维护者最终验收：Accepted（依据维护者对 I14 推荐方案的明确确认及 D-014 预授权，2026-07-17）
- Plan 兼容检查：新增 Plan Schema `0.2.0` 并完整保留 `0.1.0` 文件、验证和现有 CLI 输出；`0.2.0` 只允许声明式 R1/R2 Action，拒绝 command/args/Shell 字段、不可执行标志、未知风险、风险下调、提权和内容修改后复用旧 Plan ID。
- 确认与注册表检查：执行前重新校验 Plan 内容 ID、30 分钟有效期以及绑定相同 Plan ID 的计划级确认；未确认、确认时间非法、Plan 过期、内容被修改、动作未注册或风险低于注册表下限时，在创建日志或启动进程前失败。
- 进程检查：唯一生产进程边界使用 `exec.CommandContext`、绝对可执行路径、结构化参数、由注册表提供的最小环境和最长 30 秒超时，不调用 Shell。真实子进程测试覆盖成功、启动失败、非零退出、超时、取消、自杀式异常退出和 stdout 超限。
- 注入检查：包含空格、中文、Emoji、分号、管道、`$()` 和 `&` 的参数逐字传递，未创建注入标记文件；Plan Schema 不携带任何进程命令或参数，执行规范只来自确定性注册表。
- 状态与验证检查：Operation Record `0.1.0` 记录 Pending、Running、Verifying 和终态迁移；只有进程成功且注册验证器通过才能 Completed。进程失败、验证失败、存储失败以及恢复的遗留 Running/Verifying 记录均不会伪装为 Completed。
- 日志与隐私检查：stdout/stderr 分别限制 64 KiB 并标记截断；显式敏感值及常见 Token/Password/Secret/Authorization 赋值在落盘前脱敏。测试逐字节确认模拟 Token 不存在于任何中间或最终 JSON 文件。
- 持久化检查：Operation Record 通过嵌入式 Draft 2020-12 Schema 校验；同目录临时文件、同步和安全替换覆盖状态更新，Unix 目录/文件权限为 `0700`/`0600`，记录及备份符号链接、非法 Operation ID、路径穿越和损坏 JSON 被拒绝。
- 平台目录检查：macOS、Windows 和 Linux 使用 D-024 确定的原生状态目录；Windows/Linux amd64 目标测试二进制编译通过，跨平台差异由 CI 验证。
- 真机 smoke：本机重新构建 CLI 后 `envmason version` 成功输出 `envmason devel`、Go 版本和 `darwin/arm64`，与内置无害测试动作的固定调用及验证契约一致。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、gofmt、`git diff --check`、`GOPROXY=off` I14 核心测试以及 Windows/Linux amd64 目标编译均通过。
- 远程检查：[I14 分支 CI #37](https://github.com/gitbagHero/EnvMason/actions/runs/29579093567) 与 [main CI #38](https://github.com/gitbagHero/EnvMason/actions/runs/29579204448) 均通过 Ubuntu、macOS、Windows × Go 1.25/1.26 六任务矩阵。
- N/A：I14 不新增公开 CLI 命令或 apply，不执行 NVM、Homebrew 或其他包管理器，不安装、升级、卸载、切换默认版本、换源、提权或修改系统服务；I15 尚未开始。
- 结论：I14 已依据维护者明确方案和预授权完成验收并进入 `main`；本次一小时时间盒批次完成，按约定停在 I15 之前。

## I15 验收记录

- 增量：I15 单个 NVM Node 版本安装
- 开始日期：2026-07-17；完成与检查日期：2026-07-20
- 客观检查状态：Passed
- 维护者最终验收：Accepted（依据 D-014 预授权及维护者对 D-025 接口和安全边界的明确确认）
- 接口检查：公开 `envmason apply --tool runtime.node --version <精确版本> --online [--dry-run]`；缺少 fresh 联网证据、未知工具、缺失版本和未定义的 `--yes` 在准备 Plan 前拒绝。I13 `envmason plan` 继续生成不可执行 Plan `0.1.0`。
- Plan 与确认检查：dry-run 在内存中生成并显示可执行 Plan `0.2.0`，不确认、不运行 NVM 且不创建历史目录；真实 apply 只接受交互终端逐字输入 `apply <完整 Plan ID>`。错误输入、拒绝、EOF、非交互输入、错误确认 ID、过期或内容变化均不执行动作。
- 目标与漂移检查：目标必须高于当前生效版本并精确存在于 fresh Node.js 官方 release index；确认后重新扫描环境并重建同一 Plan ID。系统摘要、`nvm.sh` 或 default alias 在审查后变化时分别在历史写入前或进程启动前拒绝。
- 固定适配器检查：唯一 NVM 写适配器固定调用 `/bin/bash --noprofile --norc` 和编译期脚本，只 source 已摘要绑定的 `nvm.sh --no-use`，执行 `nvm install -b --skip-default-packages --no-progress`；目录和版本仅作为位置参数传递，不接受 Shell 文本、可执行路径或附加参数。
- 环境与隐私检查：只传入 HOME、NVM_DIR、固定系统 PATH、临时目录和标准代理变量，不继承 NVM mirror、认证、`BASH_ENV`、`NODE_OPTIONS` 或包管理器秘密；NVM 目录、HOME、临时路径、代理值和模拟令牌在操作记录中脱敏。`nvm.sh` 与 default alias 必须是受大小限制的非符号链接普通文件。
- 成功与幂等检查：fixture 和真实 NVM 均验证目标 Node 可执行、原 Node 22 仍可执行、原生效 Node 未改变、default alias 内容摘要未改变且原安装全部保留；目标已安装时跳过 NVM 进程，但仍验证并写入新的已确认操作记录。部分目标目录不被误判为已安装。
- 失败与取消检查：下载失败和磁盘不足 fixture 产生 Failed 与标准化非零退出错误；用户取消产生 Cancelled，Unix 进程组终止测试确认后代进程不会残留。失败不自动清理部分目录或缓存，不夹带 R3 删除。
- 操作记录检查：当前 Operation Record Schema 升为 `0.2.0`，新增执行前后事实快照、内容 digest、确定性 diff 和 skipped 状态；`0.1.0` Schema、解码和语义验证继续通过。只有进程或幂等检查成功且目标、旧版本、active 和 default 验证全部通过才能 Completed。
- 真实可恢复环境检查：一次性 Docker Linux/arm64 容器使用 NVM v0.40.4 安装 Node v22.0.0 并设为 default，I15 适配器随后以官方二进制和 checksum 安装 v24.14.0；测试确认 v22.0.0/default 保留、v24.14.0 可执行且二次运行识别为已满足。宿主机 NVM 未被修改。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、gofmt、`git diff --check`、`GOPROXY=off` I15 核心测试以及 Linux/Windows amd64 目标构建均通过。
- 远程检查：[I15 分支 CI #40](https://github.com/gitbagHero/EnvMason/actions/runs/29721966589) 与 [main CI #41](https://github.com/gitbagHero/EnvMason/actions/runs/29722093239) 均通过 Ubuntu、macOS、Windows × Go 1.25/1.26 六任务矩阵。
- N/A：I15 不安装 NVM，不修改 default alias，不切换当前 Shell，不升级 npm/Corepack/pnpm，不卸载或清理旧 Node，不提权，不支持无人值守确认，也不开放 Linux/Windows apply。
- 结论：I15 已依据维护者明确方案和预授权完成验收并进入 `main`；当前停在 I16 之前，未实现默认版本切换。

## I16 验收记录

- 增量：I16 Node 默认版本切换
- 开始、完成与本地检查日期：2026-07-20
- 客观检查状态：本地功能测试、全量门禁与远程 CI 均通过
- 维护者最终验收：Accepted（依据维护者对 D-026 接口、公开 Schema 和安全边界的明确确认及 D-014 预授权）
- 接口检查：公开 `envmason default set --tool runtime.node --version <已安装精确版本> [--dry-run]` 与 `envmason default restore --operation <Operation ID> [--dry-run]`。未安装目标、未知工具、浮动版本、非法 Operation ID 和未定义的 `--yes` 在写入前拒绝；不需要联网。
- Plan 与确认检查：Plan Schema `0.3.0` 只允许单个 Node/NVM `set_default` 或 `restore_default` R3 Action，下载为 0、无依赖、不提权；`0.2.0` 和 `0.1.0` 保持验证能力。set/restore 分别只接受 `set-default <完整 Plan ID>` 和 `restore-default <完整 Plan ID>`，错误确认、非交互输入、过期、内容变更和环境漂移均不执行。
- 设置检查：fixture 验证仅将规范 default alias 从 `22` 改为 `v24.14.0`；目标和原默认版本都必须是可执行 NVM 安装。当前 Shell Node 保持 v22，隔离新 Shell 选择 v24.14.0；重新生成的相同目标 Plan 幂等跳过写动作但仍验证并记录。
- 恢复检查：Operation Record `0.2.0` 的 before/after 快照记录 alias、解析版本、digest、已安装版本和当前 Shell 版本。新恢复 Plan 绑定源 Operation/Plan ID 和 after digest，可将 `v24.14.0` 恢复为 `22`；外部 alias 变更时拒绝覆盖。
- 失败路径检查：dry-run 不修改 alias 或创建操作记录；错误确认和确认后漂移无写入。隔离 Shell 验证失败记为 Failed 并输出恢复建议，不自动回滚；失败记录可生成并显式确认恢复 Plan。
- 固定适配器检查：只使用 `/bin/bash --noprofile --norc`、内置脚本、摘要绑定的 `nvm.sh` 和位置参数运行 `nvm alias default`；不读取用户 Shell profile，不接收 Shell 文本或附加参数。I16 专用 `InspectDefault` 的严格 alias 解析未收紧 I15 原有 digest-only 安装契约。
- 真实 NVM 和本机只读检查：一次性临时 NVM 目录直接使用本机真实 `nvm.sh`，完成 `v22.0.0 → v24.14.0 → v22.0.0`，源 NVM 未修改。真实 CLI dry-run 正确识别宿主 `node → v26.5.0`，非交互真实执行返回 1，宿主 alias 仍为 `node` 且未新增操作记录。
- 稳定性检查：全量 race 首轮暴露 Unix 进程组取消与后代 fork 的时序竞争；取消器在有限 100 ms 内重复向同一进程组发送 SIGKILL。目标 race 测试连续 10 次与后续全量 race 均通过。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、gofmt、`git diff --check`、`GOPROXY=off` I16 核心测试以及 Linux/Windows amd64 目标构建均通过。
- 远程检查：[main CI #43](https://github.com/gitbagHero/EnvMason/actions/runs/29725948971) 的 Ubuntu、macOS、Windows × Go 1.25/1.26 六个任务全部成功。
- N/A：I16 不安装或升级 NVM/Node/npm/Corepack/pnpm，不卸载或清理任何版本，不修改 Shell profile，不切换当前 Shell，不提权，不提供无人值守确认或自动回滚，不进入 I17。
- 结论：I16 已依据维护者明确决策和 D-014 预授权完成验收并进入 `main`。本批次到 I16 结束，I17 尚未开始。

## I17 验收记录

- 增量：I17 Node 附属工具更新
- 开始与本地检查日期：2026-07-28；远程门禁完成日期：2026-07-30
- 客观检查状态：本地实现、fixture、全量回归、构建与远程 CI 门禁通过
- 维护者最终验收：Accepted（维护者于 2026-07-28 确认本地候选没有问题并授权提交、推送）
- 用户价值与范围检查：新增 `envmason update node-tools`，在一个已安装 NVM Node 下分别选择 npm、Corepack、pnpm 精确目标；省略任一参数即可排除。范围外保持为 NVM/Node 安装、default/current Shell 修改、全局包迁移、Yarn、清理、自动回滚、I18 继续/恢复 Plan 和任意命令。
- Plan 与确认检查：复用 Plan `0.2.0` 和 Operation Record `0.2.0`，所有 Action 为 R2、声明式身份和依赖，不新增公开 Schema。dry-run 不确认、不执行、不创建操作记录；真实入口只接受交互式 `apply <完整 Plan ID>`，没有 `--yes`。浮动版本、版本范围、降级、缺少选择和未知 provider 在写入前拒绝。
- 归属与固定适配器检查：目标 Node、npm/Corepack/pnpm 的入口、解析路径、package.json、provider 和控制摘要均由核心检查；越出目标 Node 根目录的链接被拒绝。npm provider 使用目标 npm 的固定全局安装模板；Corepack provider 使用目标 Corepack 的固定 Known Good Release 模板。命令均为绝对路径和结构化参数，不接受 Shell 文本、外部参数或项目配置。
- 受控环境检查：固定官方 registry 和目标 prefix，关闭 npm lifecycle scripts/audit/fund/update notifier，关闭 Corepack project spec/auto-pin/default latest/unsafe URL/环境文件；不继承用户 npm/Corepack 配置、Token 或 hook，只传递受控 HOME、PATH、临时目录和标准代理。敏感路径与代理值进入已有 Operation Record 脱敏链路。
- 成功、幂等与失败检查：fixture 覆盖 npm/Corepack/Corepack-proxy pnpm 归属、独立 npm provider 与 Corepack provider 的不同命令、相同版本幂等跳过、确认后环境漂移拒绝、目标路径逃逸拒绝，以及 npm 已验证完成、Corepack 失败、后续 pnpm 保持 Pending 的三种真实步骤状态。
- 必要基线修复：真实只读扫描暴露 I06 在 PATH 重复出现同一生效 Node 时会被后续非生效重复项覆盖；改为合并 `Effective` 状态并增加回归测试，保证 I17 能稳定绑定 active Node，未扩大 I17 产品范围。
- 本机只读检查：宿主 NVM Node v26.5.0 的 dry-run 正确识别 npm v12.0.1 和 Corepack-managed pnpm v11.13.0；相同 npm 版本生成幂等 R2 Plan，较低 pnpm 目标在准备阶段拒绝，较高精确 pnpm 目标显示 `corepack` provider。所有 dry-run 均未创建 Operation Record，也未执行更新。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、gofmt、`git diff --check`、`GOPROXY=off` I17 核心测试及 Linux/Windows amd64 目标构建均通过。
- 稳定性检查：同时并行叠加普通全量与 race 全量时，普通套件中的既有 I15 安装 fixture 一次验证失败，而同轮 race 通过；该 I15 目标测试随后独立连续 10 次通过，顺序运行的普通全量也通过，未复现 I17 回归。
- 远程检查：首次 [main CI](https://github.com/gitbagHero/EnvMason/actions/runs/30338105851) 的 Ubuntu 和 macOS 任务通过，Windows 因 POSIX fixture 执行位不适用而失败；修复仅在测试层跳过依赖 Unix 权限与符号链接的 fixture，并保留 Windows 平台在环境扫描前拒绝写执行的独立测试，没有放宽生产安全校验。修复后的 [main CI](https://github.com/gitbagHero/EnvMason/actions/runs/30509363380) 在 Ubuntu、macOS、Windows × Go 1.25/1.26 六个任务中全部成功。
- 限制：未对宿主执行真实 npm/Corepack/pnpm 更新；联网下载、远端包不存在、真实缓存/磁盘失败和真实进程中断仍应优先在可恢复环境验证。该限制不开放宿主写入或扩大 I17 范围。
- 结论：I17 已由维护者确认验收，远程 CI 门禁已通过并进入 I18-A。

## I18-A 验收记录

- 增量：I18-A 多动作 DAG 失败隔离基线（不是完整 I18）
- 开始、完成与检查日期：2026-07-30
- 客观检查状态：Passed
- 范围确认：Accepted（维护者接受一小时时间盒方案，并在 I17 远程门禁通过后授权继续开发和推送）
- 用户价值与范围检查：多动作流程中的失败不会启动后续依赖，进程成功但验证失败也不会产生虚假 Completed。本增量只固化已有确定性执行语义，不新增公开命令、Schema、确认方式或写能力。
- 失败矩阵检查：使用 npm → Corepack → pnpm 三动作 R2 DAG，在每一步分别注入进程失败和验证失败，共六个场景。每个场景都断言此前步骤为 Completed 且验证 Passed，当前步骤为 Failed，后续步骤保持 Pending、没有开始时间或调用记录，最终 Failed 已持久化且 Operation Record 语义验证通过。
- 实现检查：现有执行器的拓扑顺序、首个失败停止和最终验证门禁已满足 D-028，因此本增量只增加回归证据，没有重写或放宽生产执行核心。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、gofmt、`git diff --check`、`GOPROXY=off go test -count=1 ./internal/execution` 以及 Linux/Windows amd64 目标构建均通过。
- 远程检查：[main CI](https://github.com/gitbagHero/EnvMason/actions/runs/30509651704) 的 Ubuntu、macOS、Windows × Go 1.25/1.26 六个任务全部成功；新增 DAG 矩阵在两个 Windows 任务中实际运行通过。
- N/A：本增量不生成继续/恢复 Plan，不重新验证跨运行检查点，不定义环境漂移策略，不混合 R2/R3 Action，不执行真实包管理器写入，也不声称完成整个 I18。
- 结论：I18-A 客观验收完成；项目仍停留在 I18，下一最小增量必须先冻结检查点与新 Plan 生成契约。

## I18-B 验收记录

- 增量：I18-B 确认 Plan 来源固化
- 开始、本地检查与远程门禁日期：2026-07-30
- 客观检查状态：Passed
- 维护者验收：Accepted（维护者确认开始开发，并在本地候选通过后分别授权提交和推送）
- 来源固化检查：Operation Record `0.3.0` 保存经确认的完整 Plan；Plan ID、Schema、确认凭据、步骤数量、拓扑顺序和动作身份任一不一致时拒绝持久化或读取。记录持有独立深拷贝，调用方后续修改不会改变审计来源。
- 兼容与边界检查：Operation Record `0.2.0` 和 `0.1.0` 继续读取验证，但不能作为通用继续来源；本增量未生成继续/恢复 Plan，未新增 CLI 或写能力。
- 敏感信息检查：NVM、HOME 和临时目录下的安装路径在进入 Plan ID 前使用稳定占位符；确认 Plan 命中请求或注册执行规范声明的敏感值时，在创建 Operation ID、写历史或启动进程前拒绝。Node tools 的代理控制开关不再被错误当作用户代理秘密。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、gofmt、JSON、`git diff --check`、`GOPROXY=off` 核心测试及 Linux/Windows amd64 目标构建均通过。
- 远程检查：[main CI #30511127028](https://github.com/gitbagHero/EnvMason/actions/runs/30511127028) 的 Ubuntu、macOS、Windows × Go 1.25/1.26 六个任务全部成功。
- 结论：I18-B 已完成验收；I18-C 只能在该来源契约上做只读检查点资格判定，不能直接开放继续执行。

## I18-C 验收记录

- 增量：I18-C 检查点资格判定
- 开始、完成与本地检查日期：2026-07-30
- 客观检查状态：Passed
- 维护者验收：Accepted（维护者确认开始开发，并在客观门禁通过后授权提交和继续任务）
- 来源与状态检查：只接受带完整 confirmed Plan 的 Operation Record `0.3.0` 失败、超时、取消或中断终态；Completed、活动状态、旧 Schema、篡改记录和没有剩余动作均不能成为继续来源。
- 检查点检查：只有 Completed、验证 Passed、具有合法 After Snapshot、全部依赖已复核且通过注册适配器新鲜动作级只读复核的动作可复用。缺失复核器、缺失快照、非法证据或版本、provider、所有权、NVM 控制摘要漂移均以稳定原因码阻断整个候选。
- 副作用与隐私检查：资格判定不调用 Action Build、不写 Operation Record、不启动写动作；动作级证据不包含原始私有路径或复核器原始错误。测试确认来源 Plan、记录和 Snapshot 不与回调或结果共享可变状态。
- 失败矩阵检查：npm → Corepack → pnpm 的每一步分别覆盖进程失败和验证失败；复用集合始终只含失败点之前已验证并新鲜复核的动作，当前及后续动作全部留在待执行集合。另覆盖四种终态、零复用、依赖未复核、上下文取消和 Corepack-managed pnpm 精确只读探测。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、gofmt、`git diff --check`、`GOPROXY=off` 核心测试及 Linux/Windows amd64 目标构建均通过。
- 远程检查：提交 `8f62198` 已进入 `main`；包含该提交及后续 I18-D 的 [main CI #30519955250](https://github.com/gitbagHero/EnvMason/actions/runs/30519955250) 在 Ubuntu、macOS、Windows × Go 1.25/1.26 六个任务中全部成功。
- N/A：本增量不生成、确认或执行继续/恢复 Plan，不新增 CLI 或 Schema，不支持 R2/R3 混合 DAG，不自动回滚。
- 结论：I18-C 已完成并提交；I18-D 只能在其 Eligible 结论上生成不可执行的新继续 Plan，不能直接复用旧确认或开放执行。

## I18-D 验收记录

- 增量：I18-D 继续 Plan 来源绑定与执行隔离
- 开始、完成与本地检查日期：2026-07-30
- 客观检查状态：Passed
- 维护者最终验收：Accepted（维护者确认开始并继续，随后明确授权提交和推送）
- Schema 与不可变性检查：新增 Plan `0.4.0`，固定 `executable=false`；来源 Operation/Plan、重新准备的 Plan、来源动作顺序、记录/观察检查点 digest 和 checkpoint-satisfied dependency 全部进入内容派生 Plan ID。`0.3.0`、`0.2.0`、`0.1.0` 的嵌入 Schema、严格解码和语义校验继续通过。
- 新鲜准备与动作分区检查：只接受来源终态之后重新生成的 Plan `0.2.0`，且必须恰好包含 I18-C 判定的待执行动作。动作身份、adapter、精确目标和风险必须与来源一致；待执行依赖保持，指向复用检查点的依赖从 DAG 中移除并写入来源绑定。首个动作失败的零检查点场景生成带来源的全量重试草案，不追认完成动作。
- 篡改与隔离检查：来源/评估身份不匹配、记录证据 digest 被替换、准备时间不晚于终态、遗漏待执行动作、目标变化、动作重叠、分区遗漏、未知依赖和顺序变化均被拒绝。输入记录、Plan、动作、环境和证据切片与结果不共享可变状态。
- 执行边界检查：Plan `0.4.0` 送入现有 Executor 时在注册表解析、Operation Record 写入和进程启动前返回 `plan_invalid`；生成过程不解析注册表、不调用 Action Build、不写历史、不运行进程。JSON Schema 和模型均没有 command、args、Shell 或执行规范字段。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、gofmt、`git diff --check`、`GOPROXY=off` Plan/Execution/Schema 核心测试以及 Linux/Windows amd64 目标构建均通过。
- 远程检查：提交 `5321ad1` 已进入 `main`；[main CI #30519955250](https://github.com/gitbagHero/EnvMason/actions/runs/30519955250) 的 Ubuntu、macOS、Windows × Go 1.25/1.26 六个任务全部成功。
- N/A：本增量不新增 CLI，不确认或执行继续/恢复 Plan，不升级 Operation Record，不支持 R3/R4 或混合风险 DAG，不自动回滚，也不完成整个 I18。
- 结论：I18-D 已完成、提交并通过远程门禁；真正的继续执行必须作为后续独立增量重新冻结确认、过期、执行前复核和新 Operation Record 语义。

## I18-E 验收记录

- 增量：I18-E Node tools 继续 Plan 端到端只读准备
- 开始、完成与本地检查日期：2026-07-30
- 客观检查状态：Passed
- 维护者最终验收：Accepted（维护者确认本地候选并明确授权提交）
- 用户价值与入口检查：新增内部 `PrepareContinuation` 服务，调用方只提供 Operation ID，即可从一条真实 Node tools 失败记录生成不可执行的审查态 Plan `0.4.0`。本增量没有新增 CLI、确认入口或执行入口。
- 来源约束检查：只读加载 Operation Record `0.3.0`，并在环境扫描前拒绝旧 Schema、活动或完成记录、非 Node tools Plan、无剩余动作和非法 Operation ID。Node 版本、npm/Corepack/pnpm 精确目标及 provider 只从 confirmed Plan 提取；动作身份、风险、目标、provider、控制摘要、安全检查和验证范围必须符合 Node tools 白名单。
- 单次扫描与失败位置检查：npm、Corepack、pnpm 首、中、末三个失败位置均只执行一次新鲜 Inventory 扫描，分别产生全部、后两项和最后一项剩余动作；复用检查点及其满足的依赖准确进入 Plan `0.4.0`。失败动作即使当前版本已等于目标仍保留，只更新新 Plan 的 current-state 事实。
- 漂移与失败路径检查：default alias 等检查点事实漂移时以稳定 `checkpoint_drifted` 原因阻断；不向错误或 Plan 泄漏原始 NVM 路径。来源元数据替换、浮动目标、provider 替换、安全检查缺失或作用域变化均被拒绝。
- 副作用检查：准备过程只读取历史、文件元数据和受控 `--version` 输出；测试逐字节比较准备前后的历史目录，并断言没有 Action Build、包管理器写动作、Operation Record 保存或新历史文件。
- 兼容性检查：I17 `Prepare` 只抽取单次扫描后的共享检查与 Plan 构建辅助逻辑，既有公开 CLI、Plan `0.2.0` 内容和执行路径不变；全量回归通过。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、`GOPROXY=off go test -count=1 ./internal/nodetools ./internal/execution ./internal/plan` 以及 Linux/Windows amd64 目标构建均通过。
- 远程检查：N/A；本地提交 `c786722` 尚未获得推送授权。
- N/A：本增量不确认或执行 Plan `0.4.0`，不升级 Operation Record，不支持 R3/R4、混合风险 DAG、恢复 Plan、自动回滚或完整 Node 工作流。
- 结论：I18-E 已完成并提交，等待后续推送授权；继续 Plan 的执行前只读复核由 I18-F 独立交付，确认、新 Operation Record 和安全执行语义仍未开放。

## I18-F 验收记录

- 增量：I18-F Node tools 继续 Plan 执行前只读复核
- 开始、完成与本地检查日期：2026-07-30
- 客观检查状态：Passed
- 维护者验收：Accepted（依据维护者对两小时时间盒内客观验收后自动进入下一最小增量的预授权；不包含提交或推送授权）
- 用户价值与范围检查：新增内部 `RevalidateContinuation`，证明审查态 Plan `0.4.0` 在未来确认/执行前仍与来源记录、检查点、当前环境及剩余动作一致。本增量不新增 CLI、确认入口、执行入口、写能力或 Schema。
- 不可变性与时间检查：输入先通过完整 Plan ID 校验并限定为非执行 Plan `0.4.0`；尚未到创建时间或当前时间等于/晚于过期时间时拒绝。复核使用原 `created_at` 和原 30 分钟窗口重建 Plan，不刷新有效期；稳定事实可重建相同完整 Plan ID。
- 漂移检查：default alias 检查点漂移继续以稳定 `checkpoint_drifted` 阻断；首个动作失败且没有复用检查点时，npm 当前版本变化会导致重建 ID 不同并要求生成新 Plan。错误不包含原始 NVM 路径。
- 前置拒绝与副作用检查：非法内容、Plan `0.2.0`、尚未生效和过期输入均在历史加载与环境扫描前拒绝。成功和两种漂移路径都恰好扫描一次，测试逐字节确认来源历史与输入 Plan 未变化，并断言未启动写动作。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、`GOPROXY=off go test -count=1 ./internal/nodetools ./internal/execution ./internal/plan` 以及 Linux/Windows amd64 目标构建均通过。
- 远程检查：N/A；当前修改尚未获得提交或推送授权。
- N/A：本增量不生成新 Plan 类型，不确认或执行继续 Plan，不保存新 Operation Record，不支持 R3/R4、恢复 Plan、自动回滚或完整 Node 工作流。
- 结论：I18-F 客观验收完成；根据维护者的时间盒预授权，检查实际耗时后决定是否进入下一最小增量。

## I18-I1 验收记录

- 增量：I18-I1 可执行继续 Plan 的确定性提升
- 开始、完成与本地检查日期：2026-07-30
- 客观检查状态：Passed
- 维护者验收：Accepted（维护者明确批准 D-036 方案 A，并预授权时间盒内在客观验收后继续下一最小增量；不包含本增量的提交或推送授权）
- 用户价值与范围检查：新增严格 Plan Schema `0.5.0` 和纯函数 `BuildExecutableContinuation`，将合法审查态 Plan `0.4.0` 提升为唯一的最终可确认对象。本增量没有确认、记录或执行入口。
- Schema 与语义检查：Plan `0.5.0` 固定 `executable=true`、必须携带 continuation provenance、只允许 R1/R2 剩余动作并要求 Plan 级确认；JSON Schema 与模型分别拒绝非执行形状、缺失来源和 R3 动作。历史 Plan `0.4.0`、`0.3.0`、`0.2.0`、`0.1.0` 文件及语义保持不变并继续通过嵌入和解码检查。
- 确定性与不可变性检查：提升完整保留创建/过期时间、环境、策略、动作、来源、检查点和 satisfied dependency，只改变 Schema、执行标志、摘要和内容派生 ID。来源 Operation/Plan、prepared Plan、检查点、目标、依赖、时间或环境任一变化都会改变最终 ID；输入与输出不共享可变状态。
- 执行隔离检查：合法且已确认的 Plan `0.5.0` 仍被现有 Executor 在注册表解析、历史加载/创建和进程启动前以 `plan_invalid` 拒绝。提升过程不接受确认、不解析注册表、不调用 Action Build、不写历史、不运行进程。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、`GOPROXY=off go test -count=1 ./internal/nodetools ./internal/execution ./internal/plan ./schemas/plan` 以及 Linux/Windows amd64 目标构建均通过；补充 JSON Schema 反例后目标包复测通过。
- 远程检查：N/A；当前修改尚未获得提交或推送授权。
- N/A：本增量不新增 Operation Record `0.4.0`，不实现重复继续、确认、执行、CLI、R3/R4、恢复 Plan 或自动回滚。
- 结论：I18-I1 客观验收完成；根据 D-036 和时间盒预授权，检查累计有效耗时后进入 I18-I2。

## I18-I2 验收记录

- 增量：I18-I2 Operation Record 兼容与重复继续来源
- 开始、完成与本地检查日期：2026-07-30
- 客观检查状态：Passed
- 维护者验收：Accepted（维护者明确批准 D-036 方案 A，并预授权时间盒内在客观验收后继续下一最小增量；不包含本增量的提交或推送授权）
- 用户价值与范围检查：新增 Operation Record Schema `0.4.0`，使普通 Plan `0.2.0`/`0.3.0` 和未来最终继续 Plan `0.5.0` 使用同一当前记录契约；失败的 Record `0.4.0` + Plan `0.5.0` 可作为下一轮只读继续准备的直接来源。本增量没有 Plan `0.5.0` 生产执行路径。
- Schema 与兼容检查：`0.4.0` 严格要求完整 confirmed Plan 并允许 Plan `0.2.0`、`0.3.0`、`0.5.0`；冻结的 Record `0.3.0` 仍只接受 Plan `0.2.0`/`0.3.0`，Record `0.2.0`/`0.1.0` 仍不得携带 confirmed Plan。四版 Schema 均继续嵌入、严格解码并按各自语义验证，历史 Schema 文件没有修改。
- 身份绑定检查：Record `0.4.0` 的 `plan_id`、`plan_schema_version`、确认凭据、完整 confirmed Plan、拓扑步骤顺序和动作身份保持一致；把 Plan `0.5.0` 放入 Record `0.3.0`，或篡改 Plan ID、步骤身份、目标及依赖都会拒绝。
- 首次与重复继续检查：Record `0.3.0` + Plan `0.2.0` 首次继续保持兼容；Record `0.4.0` + Plan `0.5.0` 在 npm、Corepack、pnpm 首、中、末失败位置均正确区分已复核检查点和剩余动作，转换当前直接来源满足的依赖，并生成绑定当前 Record/Plan 的新审查态 Plan `0.4.0`。
- Node tools 与副作用检查：首次及再次继续都从 confirmed Plan 解析精确目标、provider、安全检查和当前动作 DAG，只执行一次新鲜 Inventory 扫描；测试断言不调用 Action Build 或写动作，并逐字节确认直接来源、更早记录和整个历史目录均未变化。
- 执行隔离检查：普通现有执行统一写 Record `0.4.0`，但合法且已确认的 Plan `0.5.0` 仍由 Executor 在注册表解析、历史创建和进程启动前以 `plan_invalid` 拒绝。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、`GOPROXY=off go test -count=1 ./internal/nodetools ./internal/execution ./internal/plan ./schemas/operation ./schemas/plan` 以及 Linux/Windows amd64 目标构建均通过。
- 远程检查：N/A；当前修改尚未获得提交或推送授权。
- N/A：本增量不确认或执行 Plan `0.5.0`，不从生产路径创建其 Operation Record，不新增 CLI，不支持 R3/R4 继续、恢复 Plan、自动回滚或完整 Node 工作流。
- 结论：I18-I2 客观验收完成；根据 D-036 和时间盒预授权，检查累计有效耗时后决定是否进入 I18-I3。

## I18-I3 验收记录

- 增量：I18-I3 Node tools 内部继续执行
- 开始、完成与本地检查日期：2026-07-30
- 客观检查状态：Passed
- 维护者验收：Accepted（维护者明确批准 D-036 方案 A，并预授权时间盒内在客观验收后继续下一最小增量；不包含本增量的提交或推送授权）
- 用户价值与入口检查：新增内部 `ExecuteContinuation`，以最终 Plan `0.5.0` 和绑定其完整 ID 的新 Plan 级确认执行当前来源记录中的剩余 Node tools 动作。没有新增 CLI、无人值守授权或旧确认复用。
- 前置拒绝检查：非法内容、Plan `0.4.0`、尚未生效、过期、错误 Plan ID 和未来确认均在历史读取与环境扫描前拒绝。来源检查点漂移和当前工具事实导致的最终 ID 变化均在 Action Build、新 Record 创建和写进程前拒绝，不泄漏 NVM 私有路径。
- 同调用复核检查：服务使用 final Plan 原 `created_at`，在一次新鲜 Inventory 扫描上重新加载直接来源、复核检查点、生成剩余 Plan `0.2.0`、草案 `0.4.0` 和最终 `0.5.0`；只有完整 ID 相同才继续，且不刷新 30 分钟时窗。
- 固定执行上下文检查：复核与执行复用同一次扫描得到的 NVM/Node/tool baseline、精确目标、provider 和固定适配器注册表，不再次执行 Inventory 扫描。通用 Executor 接受合法 Plan `0.5.0`，但无注册动作仍在历史和进程前以 `action_unregistered` 拒绝；本增量只有 Node tools 服务构造生产注册表。
- 记录与失败检查：成功执行只写 final Plan 中的剩余 Corepack/pnpm 步骤，Record `0.4.0` 的 `confirmed_plan`、确认和步骤全部绑定 Plan `0.5.0`。三动作继续在 Corepack 失败时 npm 为 Completed、Corepack 为 Failed、pnpm 保持 Pending；随后从该真实 Record `0.4.0` + Plan `0.5.0` 再次准备，只执行 Corepack/pnpm 并成功完成。
- 来源不变性检查：成功、失败和再次继续均逐字节保留原始来源及此前全部历史记录；每次执行只增加一条新 Operation Record，final Plan 输入也保持不变。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、`GOPROXY=off go test -count=1 ./internal/nodetools ./internal/execution ./internal/plan ./schemas/operation ./schemas/plan` 以及 Linux/Windows amd64 目标构建均通过。
- 远程检查：N/A；当前修改尚未获得提交或推送授权。
- N/A：本增量不新增 CLI 或确认短语，不执行 Plan `0.4.0`，不支持 R3/R4 继续、恢复 Plan、自动回滚、任意命令或“安装 Node → 切换默认 → 更新工具”完整混合风险工作流。
- 结论：D-036 方案 A 的 I18-I1～I3 已全部完成客观验收；根据时间盒预授权，检查累计有效耗时后决定下一最小增量。

## I18-J 验收记录

- 增量：I18-J 继续执行后的最终环境证据
- 开始、完成与本地检查日期：2026-07-30
- 客观检查状态：Passed
- 维护者验收：Accepted（依据 I18 路线图及维护者时间盒内客观验收后继续下一最小增量的预授权；不包含本增量的提交或推送授权）
- 用户价值与范围检查：`ExecuteContinuation` 在终态 Record 持久化后恰好再执行一次新鲜 Inventory 扫描，返回仅覆盖 final Plan 本次动作的 `ContinuationOutcome`；执行前 ID 复核仍恰好扫描一次，执行中不扫描。
- 证据语义检查：结果按拓扑顺序记录 action/tool、before/after/target 精确版本、provider、步骤 state 和 verified。Completed 只有 Record 验证 Passed 且后扫描版本/provider 精确匹配时才 verified；npm、Corepack、pnpm 首、中、末失败矩阵分别保持此前 Completed、当前 Failed、下游 Pending，Failed/Pending 均不误报。
- 成功与再次继续检查：Corepack/pnpm 成功继续的前后版本分别为 `0.34.5 → 0.35.0` 与 `10.0.0 → 11.1.0`，provider 分别保持 npm/Corepack。真实三动作中间失败后再次继续成功时，两轮各自产生匹配其步骤集的最终证据。
- 失败语义检查：执行成功但后扫描失败时返回错误，同时保留可加载的 Completed Record；执行失败且后扫描也失败时，组合错误同时保留原执行失败和后扫描失败，Record 仍准确保留 Completed/Failed/Pending。
- 安全与隐私检查：后扫描重新验证目标 Node、active Node、`nvm.sh`、default alias、精确版本和 provider；Outcome JSON 不含 NVM/包根目录、可执行路径、command、args、代理值或 runner 输出。来源及此前记录继续逐字节不变。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、`GOPROXY=off go test -count=1 ./internal/nodetools ./internal/execution ./internal/plan ./schemas/operation ./schemas/plan` 以及 Linux/Windows amd64 目标构建均通过。
- 远程检查：N/A；当前修改尚未获得提交或推送授权。
- N/A：本增量不增加通用 Inventory diff Schema，不修改 Operation Record，不新增 CLI/文件输出，不支持混合 R2/R3、恢复 Plan、自动回滚或完整工作流。
- 结论：I18-J 客观验收完成；根据时间盒预授权，检查累计有效耗时后决定下一最小增量。

## I18-K 验收记录

- 增量：I18-K 终态操作的只读恢复候选评估
- 开始、完成与本地检查日期：2026-07-30
- 客观检查状态：Passed
- 维护者验收：Accepted（依据 I18 路线图及维护者时间盒内客观验收后继续下一最小增量的预授权；不包含本增量的提交或推送授权）
- 用户价值与范围检查：新增纯函数 `AssessRecovery`，从合法终态 Record `0.3.0`/`0.4.0` 的 confirmed Plan 和步骤证据中输出恢复候选；不读取环境、不解析注册表、不调用回调、不写历史。
- 证据分类检查：Before/After Snapshot 的合法非空 diff 标记 `changed`；写进程返回非零、异常、超时、取消、中断、验证失败或日志失败但没有可证明最终差异时标记 `uncertain`。Completed skipped/空 diff、Pending、预检失败和进程未启动均不列候选；终态记录中仍含 Running/Verifying 步骤时拒绝。
- 恢复模式检查：Node tools changed/uncertain 候选保持 `manual`；NVM set-default 的已变化候选保持冻结的 `plan` 模式及“生成并明确确认新 R3 recovery Plan”摘要。评估不降低模式、不生成 Plan，也不声称已经恢复。
- 状态与兼容检查：覆盖 Failed、TimedOut、Cancelled、Interrupted 四种部分终态及 Completed changed；Record `0.2.0`/`0.1.0` 因没有 confirmed Plan 被拒绝，活动记录和篡改 confirmed Plan 被拒绝。
- 不可变与隐私检查：评估前后来源 Record 逐字节一致；结构化结果仅包含来源/动作身份、tool/operation、步骤状态、证据分类、恢复模式和冻结摘要，不包含 Snapshot facts/diff 值、Invocation、输出、绝对路径或测试敏感标记。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、`GOPROXY=off go test -count=1 ./internal/nodetools ./internal/execution ./internal/plan ./schemas/operation ./schemas/plan` 以及 Linux/Windows amd64 目标构建均通过。
- 远程检查：N/A；当前修改尚未获得提交或推送授权。
- N/A：本增量不生成、确认或执行恢复 Plan，不判断当前环境是否仍可恢复，不自动回滚，不新增 CLI/Schema，不决定混合 R2/R3 的公开确认形状。
- 结论：I18-K 客观验收完成；根据时间盒预授权，检查累计有效耗时后决定下一最小增量。

## I18-L 验收记录

- 增量：I18-L 恢复候选的当前 checkpoint 只读复核
- 开始、完成与本地检查日期：2026-07-30
- 客观检查状态：Passed
- 维护者验收：Accepted（依据 I18 路线图及维护者时间盒内客观验收后继续下一最小增量的预授权；不包含本增量的提交或推送授权）
- 用户价值与范围检查：新增只读 `RevalidateRecovery`，在 I18-K 终态候选之上提供新鲜、动作作用域的 checkpoint 匹配结论；不生成恢复 Plan、不确认、不执行动作、不写历史。
- 状态检查：合法 `changed` 候选在注册复核器成功时标记 `current`，复核器拒绝或返回非法 Snapshot 时标记 `drifted`，未注册或缺少复核器时标记 `verifier_unavailable`；`uncertain` 候选保持 `uncertain` 且不调用复核器。多个候选严格保持 confirmed Plan 拓扑顺序。
- 副作用检查：只调用 `RevalidateCheckpoint`；测试证明 Build、Preflight、Capture、Satisfied 和 Verify 均未调用。调用前复制 Action 与 recorded After Snapshot，回调修改嵌套检查项或事实 map 不会影响来源 Record。
- 取消、不可变与隐私检查：预先取消的上下文原样返回且没有回调；回调内触发取消时即使回调返回合法 Snapshot，也丢弃全部部分结论并原样返回取消。复核前后来源 Record 逐字节一致。结果仅包含来源/候选身份、证据、恢复元数据和稳定状态，不含 Snapshot facts、diff 值、Invocation、适配器错误、绝对路径或敏感标记。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、`GOPROXY=off go test -count=1 ./internal/nodetools ./internal/execution ./internal/plan ./schemas/operation ./schemas/plan` 以及 Linux/Windows amd64 目标构建均通过。
- 远程检查：N/A；当前修改尚未获得提交或推送授权。
- N/A：本增量不新增适配器复核器，不生成、确认或执行恢复 Plan，不自动回滚，不修改 Operation Record/Plan Schema，不新增 CLI，也不决定混合 R2/R3 的公开确认形状。
- 结论：I18-L 客观验收完成；根据时间盒预授权，检查累计有效耗时后决定下一最小增量。

## I18-M 验收记录

- 增量：I18-M NVM 默认恢复的只读复核入口
- 开始、完成与本地检查日期：2026-07-30
- 客观检查状态：Passed
- 维护者验收：Accepted（依据 I18 路线图及维护者时间盒内客观验收后继续下一最小增量的预授权；不包含本增量的提交或推送授权）
- 用户价值与范围检查：defaultversion 内部服务新增 `ReviewRestore(ctx, OperationID)`，加载一条来源记录并复用 I18-K/L 输出当前恢复 checkpoint 结论；不准备恢复 Plan、不确认、不执行、不写历史。
- 适配器检查：固定 NVM `set_default`/`restore_default` 定义新增动作级复核器。它验证动作/目标、Plan 冻结的 script digest，以及 active version、default alias/value/digest/resolution、installed versions 和 target version；匹配返回有效动作作用域 Snapshot，alias、script、安装集合或目标漂移均拒绝。
- 服务状态检查：真实 completed changed `set_default` 经恰好一次 Inventory 扫描得到 `current`，外部 alias 改变得到 `drifted`；写进程失败且最终差异不确定时得到 `uncertain`，不调用 Scan。`restore_default` 记录作为来源在 Scan 前拒绝。
- 只读、不可变与隐私检查：Review 不依赖或调用 Runner，不调用 Plan builder/Action Build，不创建记录；复核前后来源记录逐字节一致，默认别名不被改变。JSON 结果不含 NVM 目录、Snapshot 字段名/事实、底层适配器错误或测试敏感值。
- 回归检查：原有 set/restore 独立 R3 Plan、确认、执行前重建、外部漂移拒绝和恢复执行测试保持通过。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、`GOPROXY=off go test -count=1 ./internal/adapter/nvm ./internal/defaultversion ./internal/nodetools ./internal/execution ./internal/plan ./schemas/operation ./schemas/plan` 以及 Linux/Windows amd64 目标构建均通过。
- 远程检查：N/A；当前修改尚未获得提交或推送授权。
- N/A：本增量不生成、确认或执行恢复 Plan，不改变 R3 明确确认，不自动回滚，不新增 CLI/Schema，不支持 Node 安装或 Node tools 的恢复入口，也不决定混合 R2/R3 工作流。
- 结论：I18-M 客观验收完成；根据时间盒预授权，检查累计有效耗时后决定下一最小增量。

## I18-N 验收记录

- 增量：I18-N Node tools 失败操作的只读恢复复核入口
- 开始、完成与本地检查日期：2026-07-30
- 客观检查状态：Passed
- 维护者验收：Accepted（依据 I18 路线图及维护者时间盒内客观验收后继续下一最小增量的预授权；不包含本增量的提交或推送授权）
- 用户价值与范围检查：nodetools 内部服务新增 `ReviewRecovery(ctx, OperationID)`，只接受现有 continuation 来源契约支持的终态失败 R1/R2 Node tools confirmed Plan，并复用 I18-K/L 输出恢复 checkpoint 状态；不生成、确认或执行恢复 Plan。
- 状态检查：真实三动作来源中 npm 已写入并验证、Corepack 写失败、pnpm Pending；恰好一次扫描后 npm 为 `current`、Corepack 为 `uncertain`、Pending 不出现。外部修改 npm package 版本后，上游候选为 `drifted`；首动作写失败只有 uncertain 时不扫描环境。
- 来源与安全检查：复用来源动作身份、provider、目标 Node/版本、安全检查和 DAG 校验；非 Node tools 来源在 Scan 前以稳定概括错误拒绝。Runner 只处理受控 `--version` 探针，测试确认恢复复核的包管理器写形状调用为 0。
- 不可变与隐私检查：复核前后来源历史逐字节一致；结果不含 NVM/package/executable 路径、命令、参数、Snapshot facts、探针输出或适配器错误。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、`GOPROXY=off go test -count=1 ./internal/adapter/nvm ./internal/defaultversion ./internal/nodetools ./internal/execution ./internal/plan ./schemas/operation ./schemas/plan` 以及 Linux/Windows amd64 目标构建均通过。
- 远程检查：N/A；当前修改尚未获得提交或推送授权。
- N/A：本增量不支持 Completed Node tools 操作，不生成、确认或执行恢复 Plan，不自动回滚，不新增 CLI/Schema，不支持 Node 安装或 R3，也不决定混合 R2/R3 工作流。
- 结论：I18-N 客观验收完成；根据时间盒预授权，检查累计有效耗时后决定下一最小增量。

## I18-O 验收记录

- 增量：I18-O Completed Node tools 变化的只读恢复复核
- 开始、完成与本地检查日期：2026-07-30
- 客观检查状态：Passed
- 维护者验收：Accepted（依据 I18 路线图及维护者时间盒内客观验收后继续下一最小增量的预授权；不包含本增量的提交或推送授权）
- 用户价值与范围检查：Node tools `ReviewRecovery` 现在接受 Completed 及原有部分失败终态；成功操作只有在记录中存在真实 Snapshot diff 时才成为恢复候选，仍不生成或执行恢复 Plan。
- 共同来源检查：Record/Plan 兼容、Node tools 动作身份、provider、目标 Node/版本、安全元数据和 DAG 校验抽取为共享纯校验；Review 接受全部终态，PrepareContinuation 在共享校验后仍单独要求部分失败及 remaining actions。
- Completed 状态检查：真实 npm 版本写入并验证成功的 Record 经恰好一次扫描输出 `changed/current`，受控 Runner 写形状调用为 0，历史不变且结果不含 package 路径。成功但目标已满足而 skipped/unchanged 的 Record 返回非 nil 空候选且 Scan 为 0。
- 边界回归：既有 Completed PrepareContinuation 测试继续在 Scan 前以“必须是终态失败”拒绝；I18-N 中间失败 current/uncertain、漂移、首失败零扫描和非法来源测试保持通过。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、`GOPROXY=off go test -count=1 ./internal/adapter/nvm ./internal/defaultversion ./internal/nodetools ./internal/execution ./internal/plan ./schemas/operation ./schemas/plan` 以及 Linux/Windows amd64 目标构建均通过。首次并行门禁中一个既有 NVM fixture 在 10 秒验证边界超时；该包随后连续 5 次通过，全仓普通测试单独重跑通过。
- 远程检查：N/A；当前修改尚未获得提交或推送授权。
- N/A：本增量不生成、确认或执行恢复 Plan，不自动选择旧版本、不自动回滚，不新增 CLI/Schema，不支持 Node 安装或 R3，也不决定混合 R2/R3 工作流。
- 结论：I18-O 客观验收完成；根据时间盒预授权，检查累计有效耗时后决定下一最小增量。

## I18-P 验收记录

- 增量：I18-P NVM Node 安装的只读恢复复核
- 开始、完成与本地检查日期：2026-07-30
- 客观检查状态：Passed
- 维护者验收：Accepted（依据 I18 路线图及维护者时间盒内客观验收后继续下一最小增量的预授权；不包含本增量的提交或推送授权）
- 用户价值与范围检查：固定 NVM install adapter 新增 checkpoint revalidator，apply 内部服务新增 `ReviewRecovery(ctx, OperationID)`；只读取 I15 单动作终态记录和当前 NVM 状态，不生成卸载/恢复 Plan。
- checkpoint 检查：复核动作身份、精确目标和 Plan 冻结的 nvm.sh digest，比较 active version、default alias digest、installed versions 与 target_installed；目标存在时额外运行固定目标二进制 `--version` 并要求精确版本。完整匹配为 current，目标删除/替换或控制状态变化为 drifted。
- 状态检查：真实 Completed changed 安装经一次 Scan 输出 current；外部删除目标目录后输出 drifted。下载失败且 Before/After 无差异时保持 uncertain 并零 Scan；第二次幂等 skipped/unchanged 记录返回非 nil 空候选并零 Scan；非法 Operation ID 在 Scan 前拒绝。
- 只读、不可变与隐私检查：Review 在 Assess 和安装 Runner 均为 nil 时通过，证明没有在线评估或安装命令；来源历史逐字节不变。JSON 结果不含 NVM 目录、Snapshot facts、命令、参数或输出；复核不创建或删除目标。
- 回归检查：原 I15 Prepare/Execute 的安装、验证、确认、漂移、下载/磁盘失败、取消和幂等测试保持通过。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、`GOPROXY=off go test -count=1 ./internal/adapter/nvm ./internal/apply ./internal/defaultversion ./internal/nodetools ./internal/execution ./internal/plan ./schemas/operation ./schemas/plan` 以及 Linux/Windows amd64 目标构建均通过。
- 远程检查：N/A；当前修改尚未获得提交或推送授权。
- N/A：本增量不生成卸载或恢复 Plan、不删除部分/完整安装、不自动回滚、不改变 R3 确认，不新增 CLI/Schema，也不决定混合 R2/R3 工作流。
- 结论：I18-P 客观验收完成；根据时间盒预授权，检查累计有效耗时后决定下一最小增量。

## I18-Q1 验收记录

- 增量：I18-Q1 只读 Workflow Manifest/Record `0.1.0`
- 开始、完成与本地检查日期：2026-07-31
- 客观检查状态：Passed
- 维护者验收：Accepted（维护者明确批准 D-047 方案 A，并授权时间盒内在客观验收后继续下一最小增量；不包含本增量的提交或推送授权）
- 用户价值与范围检查：新增只读 Workflow Manifest/Record 严格契约和内存纯状态机，先固定精确 Node/Node tools 目标及三阶段确认边界，再审计每阶段 Plan、Operation、checkpoint 和状态；不扫描、不准备或读取子 Plan、不确认、不执行、不写文件。
- Manifest 检查：固定 `executable=false`、`confirmable=false`，精确稳定版本和至少一个工具目标；阶段严格为 `install_node`（R2/Plan `0.2.0`）→ `set_default`（R3/Plan `0.3.0`）→ `update_node_tools`（R2/Plan `0.2.0`）。目标、阶段、风险、Schema、依赖或时间变化都会使内容派生 ID 不匹配。
- Record 检查：初始仅首阶段 Ready，三个阶段只能依次 `ready → planned → running → terminal`；Plan ID/Schema、Operation ID、checkpoint、时间戳和完整转换历史相互校验。最终成功链为 Completed；3 个阶段 × Failed/TimedOut/Cancelled/Interrupted 均立即终止，后续保持 Pending。
- 严格与不可变检查：Manifest/Record Schema 拒绝未知版本、字段、枚举、风险、阶段 Schema 和尾随 JSON；直接注入 command、args、confirmation 或 runner 被拒绝。构造和每次转换返回深复制新值，测试证明输入 Manifest/Record 不变；Record 不能用不同 Manifest 解码。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、`GOPROXY=off go test -count=1 ./internal/adapter/nvm ./internal/apply ./internal/defaultversion ./internal/nodetools ./internal/execution ./internal/plan ./internal/workflow ./schemas/operation ./schemas/plan ./schemas/workflow` 以及 Linux/Windows amd64 目标构建均通过。
- 远程检查：N/A；当前修改尚未获得提交或推送授权。
- N/A：本增量不新增 fixture（测试值由确定性构造器产生），不生成/加载/确认/执行子 Plan，不持久化 Workflow Record，不新增 CLI/Skill/MCP，不生成 Recovery Manifest，也不进入 I18-Q2/Q3。
- 结论：I18-Q1 客观验收完成；依据时间盒预授权检查累计有效耗时后，先冻结 I18-Q2 再继续。

## I18-Q2 验收记录

- 增量：I18-Q2 当前阶段的新鲜子 Plan 准备
- 开始、完成与本地检查日期：2026-07-31
- 客观检查状态：Passed
- 维护者验收：Accepted（依据维护者批准的 D-047 方案 A、时间盒自动过门授权及继续任务指令；不包含本增量的提交或推送授权）
- 用户价值与入口检查：新增内部 `PrepareNext` 和原生准备器，只为合法 Workflow Record 中唯一 Ready 阶段调用一次现有准备服务，返回一个可审查子 Plan及绑定其完整 ID 的新 Planned Record。
- 顺序检查：完整测试依次准备 install `0.2.0`、set-default `0.3.0` 和 Node tools `0.2.0`；每次前一阶段必须经 Q1 Completed 才能调用后继。Planned、Completed 和异常终态均在准备器前拒绝，一次调用不会准备多个阶段。
- 新鲜与语义检查：子 Plan `created_at` 不早于 Record `updated_at`；现有 Plan 完整 ID 校验通过后，再逐阶段检查固定 Schema、可执行标志、风险、动作 ID/tool/operation/adapter、精确 Node/工具目标、Node tools provider/DAG 及所属 NVM Node。陈旧、错误阶段、错误 Node/工具目标和错误 Node 所属均拒绝且不绑定 Record。
- 原生复用与隔离检查：原生准备器只适配现有 apply `Prepare`（固定 online fresh evidence）、defaultversion `PrepareSet` 和 nodetools `Prepare`，不重写它们的扫描或 Plan builder。公开 Plan 与封存原生 Prepared 的 actions、checks、installations、continuation 切片及 download 指针深复制隔离；错误原生上下文类型拒绝。
- 取消、失败与不可变检查：预先取消在准备器前返回；准备错误原样保留稳定阶段上下文。所有拒绝路径保持 Manifest/Record 不变，不创建部分转换；成功路径同样不修改输入。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、`GOPROXY=off go test -count=1 ./internal/adapter/nvm ./internal/apply ./internal/defaultversion ./internal/nodetools ./internal/execution ./internal/plan ./internal/workflow ./schemas/operation ./schemas/plan ./schemas/workflow` 以及 Linux/Windows amd64 目标构建均通过。
- 远程检查：N/A；当前修改尚未获得提交或推送授权。
- N/A：本增量不确认或执行子 Plan、不接收 Operation Record、不自动推进 running/terminal、不持久化 Workflow、不新增公开 CLI/Skill/MCP、不生成 Recovery Manifest、不修改现有 Plan/Operation Schema，也不完成 Q3。
- 结论：I18-Q2 客观验收完成；依据时间盒预授权检查累计有效耗时后，先冻结 I18-Q3 再继续。

## I18-Q3 验收记录

- 增量：I18-Q3 独立确认的三阶段执行编排
- 开始、完成与本地检查日期：2026-07-31
- 客观检查状态：Passed
- 维护者验收：Accepted（依据维护者批准的 D-047 方案 A、时间盒自动过门授权及继续任务指令；不包含本增量的提交或推送授权）
- 用户价值与入口检查：新增内部 `ExecutePrepared`、`AttachOperation` 和原生阶段执行器。每个 Q2 Prepared 只携带当前子 Plan，调用方必须另行提供绑定该完整 ID 的 Plan 级确认；编排层不创建、扩大或复用确认。
- 完整成功检查：受控内存端到端依次准备并执行 install、set-default 和 Node tools，三个阶段具有不同 Plan ID、不同确认绑定和不同 Operation ID；最终 Workflow Completed。原生执行器只调用现有三个服务 `Execute`，没有新增命令构造或任意 Runner 入口。
- 异常与漂移检查：3 个阶段 × Failed/TimedOut/Cancelled/Interrupted 均产生同名 Workflow 异常终态、绑定来源 Operation，并保持后续 Pending；3 个阶段各自的执行前漂移在返回空 Operation 时均保持 Planned。异常终态后不能再准备下一 Plan。
- Operation 来源检查：仅完整通过 Operation Record `0.4.0` 校验、终态且 Plan ID/Schema/confirmed Plan 与当前子 Plan一致的记录可附加；活动、其他 Plan、篡改 Schema 或时间顺序非法记录拒绝。Operation `created_at`/ID 重放 Running，`finished_at` 重放 terminal；规范化完整 Record JSON 的 SHA-256 可从返回 Operation 复算并等于阶段 checkpoint。
- 确认与隔离检查：scope、Plan ID 或确认时间错误，以及把前一阶段确认用于后一阶段，均在阶段执行器前拒绝。公开 Plan 或封存原生 Plan 任一修改均拒绝；Manifest、Prepared、Workflow Record、确认和来源 Operation 在成功及失败路径均保持不变，返回 Operation 为深复制。
- 身份强化：Q1 Record 校验同步拒绝跨阶段重复使用 Plan ID 或 Operation ID，防止形式合法但并非独立的来源绑定。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、`GOPROXY=off go test -count=1 ./internal/adapter/nvm ./internal/apply ./internal/defaultversion ./internal/nodetools ./internal/execution ./internal/plan ./internal/workflow ./schemas/operation ./schemas/plan ./schemas/workflow` 以及 Linux/Windows amd64 目标构建均通过。
- 远程检查：N/A；当前修改尚未获得提交或推送授权。
- N/A：测试未修改真实用户环境；本增量不代替用户确认、不增加 `--yes`、不自动重试/恢复/继续、不持久化 Workflow、不新增公开 CLI/Skill/MCP、不生成 Recovery Manifest，也不修改 Plan/Operation Schema。
- 结论：I18-Q1～Q3 客观验收完成；依据时间盒预授权检查累计有效耗时后，先冻结 I18-R1 再继续。

## I18-R1 验收记录

- 增量：I18-R1 只读 Recovery Manifest `0.1.0`
- 开始、完成与本地检查日期：2026-07-31
- 客观检查状态：Passed
- 维护者验收：Accepted（依据维护者批准的 D-047 方案 A、时间盒自动过门授权及继续任务指令；不包含本增量的提交或推送授权）
- 用户价值与契约检查：新增不可执行、不可确认、内容派生 ID 的 Recovery Manifest Schema/模型/严格 codec，汇总一个或多个现有 `RecoveryRevalidation` 的来源和候选；空候选来源仍保留，表示该来源已复核但无需处置。
- 分类检查：完整覆盖 `current+plan → prepare_new_plan`、`current+manual → manual_action`、`uncertain → investigate_uncertain`、`drifted → reassess_drifted`、`verifier_unavailable → review_without_verifier`。changed/uncertain 与 current-state 不一致、活动步骤或未知 mode 均拒绝，处置不能手工降级。
- 确定性与身份检查：来源按 Operation ID、候选按来源/action 排序；乱序 sources/candidates 产生逐字段相同 Manifest 和相同 ID。非法 Operation/Plan/action/tool/operation ID、重复来源、同来源重复 action、空摘要和 ID/时间/内容篡改均拒绝。
- 严格与隐私检查：Schema/codec 拒绝未知版本/字段、错误处置、证据状态不一致和尾随 JSON；command、args、confirmation、Snapshot/facts/diff、Invocation、stdout/stderr 注入直接拒绝。构造和校验不调用扫描、适配器或历史，输入逐字段不变。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、`GOPROXY=off go test -count=1 ./internal/adapter/nvm ./internal/apply ./internal/defaultversion ./internal/nodetools ./internal/execution ./internal/plan ./internal/workflow ./internal/recoverymanifest ./schemas/operation ./schemas/plan ./schemas/workflow ./schemas/recovery` 以及 Linux/Windows amd64 目标构建均通过。
- 远程检查：N/A；当前修改尚未获得提交或推送授权。
- N/A：本增量不加载来源文件、不主动调用恢复复核、不生成/引用/确认/执行恢复 Plan、不自动回滚或删除、不持久化 Manifest、不新增公开 CLI/Skill/MCP，也不完成 R2。
- 结论：I18-R1 客观验收完成；依据时间盒预授权检查累计有效耗时后，先冻结 I18-R2 再继续。

## I18-R2 验收记录

- 增量：I18-R2 current NVM default 候选的独立恢复 Plan
- 开始、完成与本地检查日期：2026-07-31
- 客观检查状态：Passed
- 维护者验收：Accepted（依据维护者批准的 D-047 方案 A、时间盒自动过门授权及继续任务指令；不包含本增量的提交或推送授权）
- 用户价值与入口检查：新增内部 `PrepareRestore`，调用方只提供已校验 Recovery Manifest 的 source Operation/action key；入口从 Manifest 查找原 item，不接受调用方重述或修改候选字段。
- 白名单检查：只有 `runtime.node/set_default + changed/current + plan + prepare_new_plan` 组合调用一次恢复准备器。current+manual、uncertain、drifted、verifier_unavailable、非 set-default 和不存在 item 全部在准备器前拒绝；预先取消同样零调用。
- 原生复用检查：`NativeRestorePreparer` 只调用现有 `defaultversion.PrepareRestore(operationID)`，由其负责加载来源、扫描、漂移检查及 Plan builder；本层不访问 Store、Scan、Runner 或确认。
- Plan 来源检查：只接受新鲜、ID 不同于来源、单动作可执行 Plan `0.3.0`，动作严格为 `restore-node-default/runtime.node/restore_default/nvm` R3、`recovery.mode=manual`，并具有 `source_operation_matches` 前置条件精确绑定 Manifest 的来源 Operation/Plan。陈旧、set-default、错误来源、错误原生上下文及准备器漂移错误均拒绝。
- 不可变与隔离检查：Manifest、item key 和恢复 Plan输入不变；公开 Plan 与封存 defaultversion Prepared 通过严格 Plan codec 深复制，修改 action 或 download 指针不会影响未来原生上下文。结果只引用 Manifest/source/action 和新 Plan，不含确认或执行结果。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、`GOPROXY=off go test -count=1 ./internal/adapter/nvm ./internal/apply ./internal/defaultversion ./internal/nodetools ./internal/execution ./internal/plan ./internal/workflow ./internal/recoverymanifest ./schemas/operation ./schemas/plan ./schemas/workflow ./schemas/recovery` 以及 Linux/Windows amd64 目标构建均通过。
- 远程检查：N/A；当前修改尚未获得提交或推送授权。
- N/A：本增量不确认或执行 restore Plan、不新增确认短语或公开 CLI、不自动选择/批量恢复、不生成 Node 卸载或 Node tools 反向 Plan、不自动回滚、不持久化 Recovery Manifest，也不完成 I18-S。
- 结论：I18-R2 客观验收完成；依据时间盒预授权进入 I18-S 全链路审计，公开 CLI 仍需维护者独立决策。

## I18-S1 验收记录

- 增量：I18 内部全链路安全与交付审计
- 开始、完成与本地检查日期：2026-07-31
- 客观检查状态：Passed
- 维护者验收：Accepted（2026-08-10；维护者批准 D-053 方案 A）
- 范围检查：I18-A～R2 的内部链路已覆盖 DAG 失败隔离、检查点继续、最终 Plan `0.5.0`、Record `0.4.0`、执行后证据、恢复评估/复核、三阶段 Workflow 及 Recovery Manifest/独立 default restore Plan；I18-Q/R 未接入 `cmd` 或 `internal/cli`。
- 安全边界检查：Workflow/Recovery Schema 不含 command、args、confirmation、Snapshot/facts/diff、Invocation 或输出字段。代码中 ConfirmationReceipt 只作为 Q3 临时执行函数参数并转交现有服务，不进入 Workflow/Recovery Manifest/Record。Node 删除、Node tools 反向动作、manual/uncertain/drifted 自动执行和 `--yes` 均不存在。
- Schema 兼容检查：冻结的 Plan `0.1.0`～`0.4.0` 和 Operation Record `0.1.0`～`0.3.0` 文件无 diff；新增版本只包括已验收的 Plan `0.5.0`、Operation Record `0.4.0`、Workflow `0.1.0` 和 Recovery Manifest `0.1.0`。
- 测试充分性检查：新增 workflow、recoverymanifest、workflow schema、recovery schema 包语句覆盖率分别为 82.8%、84.7%、100%、100%；无 TODO/FIXME/HACK/panic。全量、race、vet、build、离线核心和 Linux/Windows 构建在 Q3、R1、R2 门禁中分别通过，最终 `git diff --check` 通过。
- 文档检查：README 已同步 I18-Q1～Q3、R1/R2 能力与内部 API 边界；D-047～D-052 及逐增量验收完整。D-053 方案 A 已接受，公开 CLI 明确进入独立 Backlog，不把未实现接口写成已支持能力。
- 远程检查：N/A；当前修改尚未获得提交或推送授权。
- N/A：本审计不新增代码、Schema、fixture、CLI、持久化或系统修改；维护者已独立选择公开接口边界。
- 结论：I18 内部确定性核心与安全边界完成最终验收；按 D-053 方案 A 保持内部 API，公开 staged workflow / recovery CLI 延后到独立增量。

## I19 验收记录

- 增量：I19 Profile 最小 Schema 与解析器
- 开始、完成与本地检查日期：2026-08-10
- 客观检查状态：Passed
- 维护者验收：Accepted（依据维护者批准方案 A 并继续开发的指令及既有时间盒内客观验收后自动过门授权；不包含提交或推送授权）
- 用户价值与契约检查：新增 Profile `0.1.0` Draft 2020-12 Schema、严格 YAML/JSON codec 和纯规范化核心。用户可声明 Base/Frontend Node、`minimal|standard` 变体、模块选项及 Frontend Node 的 `lts|stable|exact` 策略；`exact` 绑定稳定 SemVer Pin。
- 默认与确定性检查：省略变体统一补为 `standard`；Base standard 默认 build tools/terminal configuration，Frontend Node standard 默认 Corepack/pnpm/browser testing，Frontend Node 省略版本策略默认 `lts`。显式布尔值可覆盖变体默认；模块规范化为 Base、Frontend Node 固定顺序，重复编码结果逐字节稳定，输入结构不变。
- 严格输入检查：只接受 1 MiB 内的单份文档。JSON 拒绝未知字段和尾随值；YAML 额外拒绝重复键、alias/anchor、显式 tag、非字符串键和多文档。未知模块/变体、重复或冲突模块、跨模块选项、非法策略、缺失/浮动/带 `v`/预发布 Pin 及 command/shell/args 注入均在解析或规范化阶段拒绝。
- 版本与 Schema 检查：缺失 `schema_version` 会列出唯一支持版本；未知版本明确说明尚无迁移路径。嵌入 Schema 返回独立副本，可独立编译并验证合法/恶意样例；Base 与 Frontend Node YAML 示例均通过严格解析和确定性再编码。
- 安全与副作用检查：Profile 模型和 Schema 不含 command、args、shell、script、URL、来源、凭据、路径、确认、Plan 或 Action；解析、规范化和编码不读取主机、项目、网络或历史，不调用适配器/执行器，不生成 Lock/Plan，也不写系统状态。
- 依赖检查：新增直接依赖 `gopkg.in/yaml.v3 v3.0.1`，模块内容与 go.mod 校验和均锁定；`go mod verify` 通过。首次在线 sumdb 请求超时后，所有最终验收均在 `GOPROXY=off`、`GOSUMDB=off` 下使用已缓存且校验和固定的依赖完成。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、`go mod verify`、`GOPROXY=off go test -count=1 ./internal/profile ./schemas/profile` 以及 Linux/Windows amd64 目标构建均通过。`internal/profile` 与 `schemas/profile` 语句覆盖率分别为 86.5% 和 100%，`git diff --check` 通过。
- 远程检查：N/A；当前修改尚未获得提交或推送授权。
- N/A：本增量不新增公开 CLI/Skill/MCP，不解析平台实现，不生成 Lock/Plan，不安装/修改系统，不支持 Java/DevOps/其他模块，也不决定 Lock 跨平台组织和兼容规则。
- 结论：I19 全部客观验收完成；下一顺序增量是 I20 macOS Profile 解析与 Lock，开始前必须先冻结 Lock `0.1.0` 的跨平台组织和兼容规则。

## I20-A 验收记录

- 增量：I20-A Lock `0.1.0` 单目标契约与确定性构造
- 开始、完成与本地检查日期：2026-08-10
- 客观检查状态：Passed
- 维护者验收：Accepted（依据既有时间盒内客观验收后自动过门授权；不包含提交或推送授权）
- 组织与身份检查：新增 Lock `0.1.0` Draft 2020-12 Schema、模型、严格 JSON codec 和纯构造器。一份 Lock 只绑定一个 Profile digest 与一个 OS/版本/架构目标；完整规范化内容计算 SHA-256 ID，固定 `executable=false`、`confirmable=false`。
- 状态与来源检查：四种 `satisfied|install_required|conflict|unresolved` 状态全部覆盖。除 unresolved 外均绑定 tool、manager、package kind/ID、精确版本、来源 ID 和包含目标架构的平台条件；unresolved 明确省略伪实现。来源固定无凭据/查询/fragment 的 HTTPS URI、快照时间与内容 digest。
- 确定性与不可变检查：构造器从规范化 Profile JSON计算 digest，深复制并排序来源、items、观察版本和架构，重算状态计数；乱序但内容相同的输入产生逐字段相同 Lock/ID。Profile、来源、item、implementation 及嵌套切片输入均不变。
- 语义与篡改检查：拒绝重复来源/item/能力/观察/架构、悬空来源、条件排除目标、来源晚于 Lock、状态/实现/观察/reason 不一致、计数或顺序篡改及内容 ID 篡改。Schema/codec 另拒绝未知版本/字段、可执行/可确认标志、command/path 注入、尾随值与超大文档。
- 隐私与副作用检查：模型和 Schema 不含主机名、用户名、机器 ID、路径、环境、凭据、命令、参数、Shell、脚本、确认、Plan/Action 或输出；构造和编解码不读取 Profile 之外的文件、主机、网络或历史，不解析/安装任何包。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、`go mod verify`、`GOPROXY=off go test -count=1 ./internal/lockfile ./schemas/lock` 以及 Linux/Windows amd64 目标构建均通过。`internal/lockfile` 与 `schemas/lock` 语句覆盖率分别为 87.1% 和 100%，`git diff --check` 通过；无 TODO/FIXME/HACK/panic。
- 远程检查：N/A；当前修改尚未获得提交或推送授权。
- N/A：本增量不解析 Profile/Inventory/目录快照，不选择 macOS 包，不联网，不生成 Plan，不执行安装，不持久化 Lock，不新增 CLI/Skill/MCP，也不开放 Homebrew bootstrap。
- 结论：I20-A 客观验收完成；依据时间盒自动过门授权进入 I20-B macOS 纯解析器。

## I20-B 验收记录

- 增量：I20-B 基于显式目录快照的 macOS Profile 纯解析器
- 开始、完成与本地检查日期：2026-08-10
- 客观检查状态：Passed
- 维护者验收：Accepted（依据既有时间盒内客观验收后自动过门授权；不包含提交或推送授权）
- 能力展开检查：规范化 Base 固定展开 Git/SSH，并按布尔选项加入 CMake/terminal configuration；Frontend Node 固定展开 Node/npm，并按选项加入 Corepack/pnpm/browser testing。minimal/standard 默认与显式覆盖均通过测试；terminal/browser 在首批范围内稳定标记 unsupported，不被目录条目误开放。
- 版本目录检查：解析器只消费调用方提供的来源快照和 capability entries，不联网或猜版本。Node `lts|stable|exact` 分别选择匹配 channel 或精确版本，其余首批能力选择 stable；缺失或歧义候选稳定输出 `unresolved/version_source_unavailable`，悬空来源、未知 capability/channel 和重复来源拒绝。
- 平台检查：只有 macOS arm64/amd64 选择 implementation；Windows 及 macOS 386 fixture 为每项能力输出明确 unsupported reason。Inventory 的 Schema、生成时间和 OS/版本/架构必须与 Lock 目标一致，未来或不匹配 Inventory 在解析前拒绝。
- 状态与幂等检查：当前 Inventory 中 manager、规范化版本和未冲突架构与目录实现匹配时为 satisfied，不列为安装需要；无观察为 install_required；存在版本/manager/架构冲突为 conflict；未知 manager 或不安全版本保守归一为 unknown 观察，不会被静默当作未安装。
- 确定性、隐私与不可变检查：catalog source/entry 和 Inventory tool 顺序变化不影响最终 Lock/ID；Profile、Inventory、目录、来源及嵌套 architecture 切片均不变。输出仅保留白名单 manager/版本，不复制 Installation ID、Path、Inventory Source 或 fixture 私有字符串。
- 安全与副作用检查：解析器不读取文件、环境、网络或历史，不调用 Homebrew/NVM/包管理器，不生成命令、Plan、Action 或确认，不执行、不持久化，也不把 Lock 状态解释为安装授权。
- 自动检查：目标包覆盖率 `internal/profileresolver` 86.6%、`internal/lockfile` 87.1%、`schemas/lock` 100%。`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、`go mod verify`、离线目标包及 Linux/Windows amd64 构建均通过，`git diff --check` 通过。并发同时运行普通/race 全量曾使既有 NVM fixture 的固定 10 秒验证超时；按门禁顺序串行重跑两套全量均通过，确认是资源争用而非回归。
- 远程检查：N/A；当前修改尚未获得提交或推送授权。
- N/A：本增量不提供公开 CLI，不采集/持久化目录，不保证目录新鲜度，不安装 Homebrew/NVM/包，不生成 Plan，不实现 terminal/browser 配装，也不执行 I21/I22。
- 结论：I20 的单目标 Lock 契约与 macOS Base/Frontend Node 纯解析全部完成客观验收；下一顺序增量为 I21 macOS Base 配装，必须从新的可审查 Plan 开始，且真实写测试优先使用可恢复环境。

## I21-A 验收记录

- 增量：I21-A macOS Base Homebrew Plan 准备
- 开始、完成与本地检查日期：2026-08-10
- 客观检查状态：Passed
- 维护者验收：Accepted（依据既有时间盒内客观验收后自动过门授权；不包含提交或推送授权）
- Plan 与范围检查：新增纯 `BuildBaseInstall`，复用 Plan `0.2.0`。只把 Lock 中 `base.git`/`base.cmake` 的 install_required Homebrew formula 转成排序稳定的 `install/homebrew` R2 Action；satisfied 跳过，conflict/unresolved 阻断，SSH/terminal/Frontend 项不转为动作。全部已满足时明确返回无需 Plan。
- 身份与前置检查：Plan `policy_digest` 绑定 Profile digest，environment 绑定当前唯一 active `manager.homebrew` installation，每项 precondition 绑定 Lock ID、来源 digest、package state 和 Homebrew available；verification 绑定精确 formula 版本及 Lock OS/架构。相同输入产生相同 ID，输入 Lock/Inventory 不变。
- 平台与 Homebrew 检查：Lock 必须通过内容 ID 校验、生成时间不晚于 Plan、目标为当前 macOS arm64/amd64且含 Base；Inventory 必须为当前 Schema、时间不晚于 Plan且系统匹配。Homebrew 缺失、多个 active installation 或元数据不完整均停止，错误明确说明不支持 bootstrap。
- 风险与恢复检查：每项固定 R2、Plan 级确认、无提权/重启、下载大小 unknown、manual recovery；Plan 不含命令或参数。Git+CMake、单项、部分/all satisfied、冲突、错误时间/目标/Inventory/Homebrew 和篡改 Lock 均覆盖。
- 执行隔离检查：使用正确 Plan ID 的新确认调用通用 Executor 时，空 Registry 对 Base Action 返回 `ACTION_UNREGISTERED`；runner 调用数和 store 保存数均为零，证明 I21-A 没有隐式开放 `brew install`、历史或进程能力。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、`go mod verify`、离线相关包及 Linux/Windows amd64 构建均通过，`git diff --check` 通过。`internal/plan` 和 `internal/execution` 语句覆盖率分别为 80.3% 与 80.4%。
- 远程检查：N/A；当前修改尚未获得提交或推送授权。
- N/A：本增量不新增 CLI/确认短语，不注册或运行 `brew install`，不查询/更新 Homebrew，不执行 bootstrap，不安装 SSH/terminal config，不持久化 Plan/Lock，不生成最终差异 Lock，也不完成 I21。
- 结论：I21-A 客观验收完成；下一最小增量必须先冻结 Homebrew 的依赖闭包、配置、自动更新、精确版本复核、幂等与失败边界。依据官方行为复核，D-058 将 I21-B 收窄为只读事务预检，固定写命令继续后移。

## I21-B 验收记录

- 增量：I21-B Homebrew 安装事务只读预检
- 开始、完成与本地检查日期：2026-08-10
- 客观检查状态：Passed
- 维护者验收：Accepted（依据维护者批准方案 A、继续开发指令及时间盒内客观验收后自动过门授权；不包含提交或推送授权）
- 事务绑定检查：新增独立 `internal/baseinstall` 纯函数，输入必须是完整有效且仍在 30 分钟窗口内的 I21-A Plan `0.2.0` 和原 Lock `0.1.0`。Plan policy、macOS 目标、active Homebrew 身份/版本、四项前置条件、两项精确验证、Lock/source digest 和每个 Git/CMake Action 均逐字段绑定；遗漏、额外、重复或篡改 Action 全部拒绝。
- 闭包与成本检查：每项预览固定一个 install-required 根 formula 和完整传递依赖；依赖只允许 satisfied/install-required、受限 formula 身份、精确版本和已知非零下载量。Action/依赖乱序产生相同 Review/ID，共享依赖逐字段一致时去重，版本/状态/大小冲突停止；总下载量按唯一 formula 计算并检查 `int64` 溢出。Git、CMake 单项和双项均覆盖。
- 配置、漂移与隐私检查：Homebrew 观察时间不得早于 Plan 或晚于准备时间，可执行文件、配置和 catalog 使用 SHA-256 摘要；Homebrew 身份、版本、catalog digest/时间或配置安全评估漂移均停止。结果固定 `executable=false`、`confirmable=false`，只保留稳定身份、摘要、安全事实和公式事实；序列化断言不含 HOME/brew 路径、catalog URI、环境键值、命令、参数、确认或凭据。
- 不可变检查：输入 Plan、Lock、Baseline 和嵌套依赖切片保持不变；输出 Action/依赖规范排序并使用完整内容派生 ID。修改 ID、只读标志、配置安全事实、摘要、目标、formula 或顺序后严格验证失败。
- 自动检查：`internal/baseinstall` 语句覆盖率 87.4%。`GOSUMDB=off GOPROXY=off go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、`go mod verify`、Linux/Windows amd64 构建及 `git diff --check` 全部通过。
- 远程检查：N/A；当前修改尚未获得提交或推送授权。
- N/A：本增量不执行/注册或自行调用 `brew install`，不读取/解释 Homebrew 环境文件，不更新 Homebrew，不新增公开 Schema/CLI/确认，不写 Operation/Lock，不 bootstrap、不卸载，也不完成 I21。
- 结论：I21-B 客观验收完成。按维护者新的一小时时间盒要求停在已通过门禁的边界；下一安全增量是 I21-C 只读采集器，负责从固定 Homebrew JSON/配置文件白名单生成本 Review 所需 Baseline/Preview，写适配器继续保持未注册。

## I21-C 验收记录

- 增量：I21-C Homebrew 事务事实只读采集
- 开始、完成与本地检查日期：2026-08-12
- 客观检查状态：Passed
- 维护者验收：Accepted（维护者在客观门禁通过后明确授权审查与提交；不包含推送授权）
- 采集与绑定检查：新增 `CollectTransactionFacts` 纯函数，只从显式快照生成 I21-B 的 Baseline/ActionPreview。Plan 必须仍有效；Lock、Inventory、唯一 active Homebrew 身份/版本/目标架构、brew 路径、观察时间、目标 macOS/bottle tag、官方 catalog 原始 digest 和 source 均严格绑定；其他架构的 formula 不计为已满足。输入及嵌套数据保持不变。
- 配置检查：解析环境及 system/prefix/user 三层固定 `brew.env` 内容，不执行 Shell 或扩展；未批准 Homebrew/proxy/SUDO 配置、错误安全值、重复/非法行、NUL/CR、超大文件/值/键数均停止。结果仅保留规范化配置摘要与 safe 事实，不回显值。
- 事务闭包检查：catalog 允许前向新增 JSON 字段，完整文档只要求有界、名称有效且唯一；`homebrew/core` 核心字段与严格版本规则只约束 Git/CMake 的可达闭包，因此无关 formula 的合法非数字前缀版本不会误阻断。按目标 variation 覆盖 required/recommended 依赖，formula revision 进入精确版本。当前 Inventory 的同 manager/目标架构精确版本才标记 satisfied；目标架构已有其他版本则停止，完全缺失的 root/依赖必须具有目标 bottle tag、catalog SHA-256 和已知非零大小。依赖缺失、循环、disabled、过大闭包、root 已安装/版本漂移、依赖版本冲突、artifact 缺失/多余/重复/零大小/digest 不符全部拒绝。
- 隐私与执行隔离：输出序列化断言不含 brew 路径、官方 catalog URI、配置键值、镜像/凭据样本；实现不导入或调用 os/exec/net/http，不含 Registry、CommandSpec、文件写入或进程入口。CLI 与既有执行注册表均未修改，Homebrew Action 继续为 unregistered。
- 提交前审查：以 2026-08-12 的官方 formula catalog 抽查发现 8540 个 formula 中有 6 个无关 formula 使用合法的非数字前缀版本；据此将严格版本规则收窄到 Git/CMake 可达闭包并增加双向回归测试。审查同时补上 active Homebrew 目标架构绑定、其他架构 formula 不计为满足、目标架构旧版依赖冲突停止，以及不可显示配置键名不进入错误信息；未发现遗留阻断项。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、`go mod verify`、`GOSUMDB=off GOPROXY=off go test -count=1 ./internal/baseinstall` 以及 Linux/Windows amd64 目标构建均通过。`internal/baseinstall` 语句覆盖率为 86.8%，`git diff --check` 通过。
- 远程检查：首次分支 CI #31579880066 的 Ubuntu/macOS × Go 1.25/1.26 四项通过，Windows × Go 1.25/1.26 两项发现纯核心错误使用宿主 `filepath.IsAbs`，使 Windows 将目标 macOS 路径误判为相对路径。修复改用固定 POSIX/macOS 路径语义并增加跨平台回归测试；修复后的分支 CI #31580456682 在 Ubuntu、macOS、Windows × Go 1.25/1.26 六项全部通过。
- N/A：本增量不提供公开 CLI/Schema，不自行发现/读取路径，不调用 Homebrew/Shell/网络，不执行/注册 `brew install`，不写 Operation/Plan/Lock，不 bootstrap、不卸载、不生成执行后 Lock/diff，也不完成 I21。
- 结论：I21-C 本地与远端客观验收全部完成，阶段三完成并停在 PR/合并授权前。合并后才能冻结并进入 I21-D 固定 Homebrew 写适配器、重新采集/确认、失败记录和恢复边界；不得仅因事务事实已齐全就开放执行。

## I21-D 验收记录

- 增量：I21-D 固定 Homebrew 写适配器、重新采集与失败/恢复边界
- 开始日期：2026-08-12；中断后恢复与本地完成日期：2026-08-17；远端完成日期：2026-08-19
- 客观检查状态：Passed
- 维护者验收：Accepted（维护者于 2026-08-19 明确授权创建 PR 并合并；不包含发布授权）
- Plan 与确认检查：新增 `BindBaseTransactionReview`，保持 I21-A 候选 Plan 不变，为全部 Git/CMake Action 增加同一 Review ID 后重算最终 Plan ID。内部执行入口复用完整 I21-B 约束验证候选 Plan、Lock、Review 的内容与时间绑定，并核对派生 Plan 和 macOS 平台；确认必须同时绑定最终 Plan ID 与 Review ID，伪造、重复绑定、过期或错误确认均在历史及写进程前停止。
- 重新采集与配置检查：确认后使用当前显式 Inventory、brew 路径/字节、完整配置、catalog、bottle tag 和 artifact 重新执行 I21-C；除观察时间外全部事务事实必须与 Review 一致。HOME/TMPDIR 现要求绝对 POSIX 路径并进入配置摘要，适配器自身再次核对 executable/configuration digest，避免绕过编排层换用未审查环境。
- 固定执行检查：新增独立 `internal/adapter/homebrewinstall`，只生成 Review 恰好覆盖的 Git/CMake R2 Definition。写命令固定为 active 绝对 brew 路径及 `install --formula --force-bottle`；HOME、TMPDIR、最小 PATH 和七个 Homebrew 安全变量稳定排序，proxy 不继承，15 分钟超时并终止进程树。选项和嵌套 Plan/Review/configuration 均深复制。
- 预检、幂等与验证检查：恢复任务后的只读实机核对发现全量 `brew info --json=v2 --installed` 在当前开发机输出约 409 KB，会稳定超过执行器 64 KiB 上限；据此改为固定 `brew list --formula --versions` 与 `brew list --formula --full-name` 双探针，当前两个输出分别约 1.2 KB 与 0.6 KB。解析器拒绝失败/截断、非法或重复行、版本/full-name 不一致、非 core tap、无效版本、旧版和多版本并存。已满足依赖必须保持精确；根及整个依赖闭包精确满足时才幂等跳过；执行成功后根和全部依赖必须恰有一个 Review 精确版本。
- 历史与恢复检查：端到端测试覆盖成功、确认/平台/可执行文件/HOME/TMPDIR/catalog/artifact 漂移、旧版 preflight、竞态幂等跳过和写进程非零退出；Operation Record 保存最终 Plan/Review 绑定、固定调用、before/after/diff 和精确状态，且不泄漏 HOME/TMPDIR。已改变的成功记录通过固定检查点区分 current/drifted；可能已启动但无可证变更的失败保守标记 uncertain/manual，不执行卸载。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、`go mod verify`、`GOSUMDB=off GOPROXY=off` 相关包测试、Linux/Windows amd64 构建及 `git diff --check` 全部通过。
- 远程检查：首次分支 CI #32228847098 的 Ubuntu/macOS × Go 1.25/1.26 四项通过，Windows × Go 1.25/1.26 两项发现 macOS-only 执行 fixture 使用 POSIX 绝对路径，Windows 宿主执行器会按本机路径语义正确拒绝该测试 spec。修复仅在 Windows 跳过四项依赖 macOS 执行路径的集成 fixture，生产校验及其余纯逻辑测试保持不变；修复后的 CI #32229477623 六项全部通过。
- N/A：本增量不新增公开 Schema/CLI/Skill/MCP，不自行发现文件/环境/网络，不执行真实机器安装，不更新/bootstrap Homebrew，不接受任意 formula/版本，不换源、不提权、不卸载，也不生成最终 Lock/diff。
- 结论：阶段四 I21-D 已完成本地与远端客观验收并通过 PR #3 合并；I21 仍需后续最小增量完成执行后 Inventory/最终 Lock/diff 以及可恢复 macOS 环境验收，不能因内部适配器存在就宣称整体完成。

## I21-E 验收记录

- 增量：I21-E 执行后最终 Lock 与结构化差异收敛
- 开始、本地完成与检查日期：2026-08-19
- 客观检查状态：Passed
- 维护者验收：Accepted（维护者于 2026-08-19 明确允许远端验收并合并；不包含发布或真实机器写入授权）
- 用户价值与入口检查：新增内部纯 `Finalize`，只有完整有效的 I21-D Prepared、Completed Operation Record `0.4.0` 和同一 `finalized_at` 的显式执行后 Inventory 才能生成 Outcome；入口不发现文件、环境、历史或网络，也不调用 Homebrew/Runner/Store。
- 来源与时间检查：Prepared 必须可从 Candidate Plan、原 Lock 和 Review 逐字段重新派生；Record 必须保存完全相同的最终 confirmed Plan、恰好覆盖全部 Review Action 且结束时间不晚于收敛时间。旧 Schema、Failed、结束时间倒退、缺失 Before/After 或伪造初始/最终公式闭包均返回空结果。
- 执行后闭包检查：Inventory 必须通过当前 Schema，生成时间与收敛时间相同，系统目标与原 Lock 一致；唯一 active Homebrew installation 的 ID、版本、manager、路径和架构必须保持 Plan 绑定。Review 的全部 root/传递依赖在目标或 unknown 架构下必须各有且仅有一个精确 Homebrew 版本，缺失、旧版、重复、错误架构或身份漂移均停止；多项失败错误按公式名稳定返回。
- Lock 与差异检查：新增通用 `lockfile.DeriveState`，只改变既有 item 的 resolution evidence 并重新规范 summary/ID；Profile、target、sources、item identity 和 implementation 全部继承。I21-E 只把 Review 对应的 Git/CMake 从 install_required 更新为 satisfied，原本 satisfied 的 CMake 逐字段不变；Git+CMake 差异按 item ID 排序。使用最终 Lock 再准备 Base Plan 会明确得到无安装动作，证明终态收敛的幂等语义。
- 隐私与不可变检查：Finalize 对 Prepared/Record/Inventory 输入逐字节不变；Outcome 只含 Operation/Plan/Review ID、最终 Lock 和 capability/tool/manager/version/state 差异。序列化结果不含 HOME、TMPDIR、Homebrew/Cellar 路径、命令、参数、环境、Snapshot facts、stdout/stderr 或原始 Inventory。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、`go mod verify`、`GOSUMDB=off GOPROXY=off` 相关包测试、Linux/Windows amd64 构建及 `git diff --check` 全部通过。`internal/lockfile`、`internal/baseinstall` 和 `internal/baseapply` 语句覆盖率分别为 88.6%、87.2% 和 88.5%。
- 远程检查：[分支 CI #32232554955](https://github.com/gitbagHero/EnvMason/actions/runs/32232554955) 的 Ubuntu、macOS、Windows × Go 1.25/1.26 六项全部通过；每项均完成格式、全量测试、vet 和 CLI 构建。
- N/A：本增量不新增公开 Schema/CLI/Skill/MCP，不自动扫描/持久化 Inventory 或 Lock，不执行真实 Homebrew 安装，不生成失败后的部分 Lock，不恢复/卸载、不 bootstrap/更新/换源 Homebrew，也不完成可恢复 macOS 环境验收。
- 结论：I21-E 本地与远端客观验收完成，并已获得 PR/合并授权。I21 尚需在干净且可恢复的 macOS VM 完成真实配装与二次运行验收，当前不能进入 I22。

## I21-F1 验收记录

- 增量：默认禁用的可恢复 macOS 双阶段 live acceptance harness
- 开始、本地完成与检查日期：2026-08-19
- 客观检查状态：Passed
- 维护者验收：Accepted（维护者于 2026-08-19 明确授权提交和推送；不包含创建 PR、合并、发布、真实 Homebrew 写入或 I21-F2 授权）
- 默认隔离与入口检查：实机入口只存在于 `darwin && envmason_live_i21` 测试构建；普通 `go test ./...`、CLI 与发行构建均不可达。入口固定 `prepare|apply` 两种模式，并要求精确 disposable-VM 声明、仓库外绝对 `.json` bundle 和仍有效的当前时间；apply 另要求同时绑定最终 Plan ID 与 Review ID 的精确 token。默认排除和显式 tagged 包含已通过命令断言。
- 完整模拟检查：纯 fixture 从规范化 minimal Base Profile（显式启用 CMake、关闭 terminal）依次生成官方目录投影、Lock、Git+CMake 双动作候选 Plan、Git/gettext/CMake 闭包 artifact、Transaction Review、Review 绑定后的不同最终 Plan 和内容派生 bundle。Profile/Prepared/catalog/bottle tag/artifact 顺序、digest、大小或 bundle ID 任一篡改均被严格 codec 拒绝；Review 输出不含 HOME、brew/TMP 路径、环境、catalog URL、Inventory 或进程输出。
- 环境与私有证据检查：只接受原生 macOS arm64 `/opt/homebrew` 或 amd64 `/usr/local` 的固定系统 Homebrew 位置；目标架构 Git/CMake formula 任一已存在即停止。任何额外 `HOMEBREW_*`、proxy、`SUDO_ASKPASS`、`XDG_CONFIG_HOME` 或错误安全开关在 Homebrew/网络前停止且不回显值。bundle 父目录/文件严格要求真实目录或普通文件、`0700`/`0600`、仓库外解析路径、大小上限和无符号链接；写入使用不覆盖创建，公共目录权限保持不变而不是被入口改写。
- 网络与漂移检查：catalog 只允许固定官方 URL、GET、无 redirect 和 64 MiB 上限，原始 digest 绑定 Lock/bundle；可达闭包的 bottle URL 只允许精确 `ghcr.io/v2/homebrew/core/.../blobs/sha256:<digest>`。测试逐请求断言匿名 HEAD → 固定 realm/service/scope token GET → 携带内存 token 的授权 HEAD，只接受正 `Content-Length` 和匹配 `Docker-Content-Digest`；恶意 realm/host/scope、HTTP、公式或 digest 漂移在执行前拒绝。2026-08-19 只读抽查当前官方 Git/CMake/gettext/openssl@3 数据与 GHCR challenge 形状一致。
- prepare/apply 与收敛检查：prepare 在任何写能力前完成两次本机事实核对，只输出脱敏 Review、精确 token 和私有 bundle。apply 先重新校验 bundle/确认/有效期，再重新 GET catalog、HEAD bottle，并在网络后重新采集 Inventory、brew 字节和三层固定配置；全部事实匹配后才调用既有 I21-D `Service`。成功路径继续调用 I21-E `Finalize`，要求用同一 Profile/目录/执行后 Inventory 重解 Lock 与最终 Lock 逐字段一致，并确认第二次 Base Plan 为零动作；最终 Lock/Outcome 与 Operation 只进入 bundle 的私有目录。详细人工流程见 `docs/I21_LIVE_ACCEPTANCE.md`。
- 当前机安全拒绝：当前 macOS 15.7.4 arm64、Homebrew 6.0.12 开发机已有目标 Git/CMake。带完整 prepare mode、disposable 声明、隔离环境和新的仓库外不存在路径运行固定 test，按预期以“Git and CMake Homebrew formulae must be absent”非零退出；断言 bundle 及父目录均未创建。tagged 当前机安全测试同样通过，且该拒绝发生在 catalog/GHCR 与 Operation/写进程前。
- 自动检查：`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、`go mod verify`、tagged 普通/race/vet、`GOSUMDB=off GOPROXY=off` 相关包测试、Linux/Windows amd64 构建及 `git diff --check` 全部通过；无 TODO/FIXME/HACK/panic。
- 远程检查：Pending；本次授权包含分支推送，推送触发的远端 CI 结果待后续记录。
- N/A：F1 不创建、启动、恢复或销毁 VM，不执行真实 `brew install`，不 bootstrap/update/cleanup/uninstall/换源 Homebrew，不新增公开 CLI/Schema/Skill/MCP，不把环境变量本身视为用户授权，也不完成 I21 或进入 I22。
- 结论：I21-F1 的默认禁用入口、模拟、运行手册和当前机零写拒绝已完成本地客观验收。下一最小增量是 I21-F2：只能在 Git/CMake 均缺失的可恢复 VM 快照中先运行 prepare，维护者再对输出的精确 Plan/Review 给出新的 R2 明确确认后运行 apply，并以最终 Lock、二次零动作和 VM 恢复/销毁完成 I21。
