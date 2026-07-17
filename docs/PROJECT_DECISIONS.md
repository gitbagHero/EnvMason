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

## 已规划、尚未决定的事项

| 事项 | 最迟决策增量 |
|---|---|
| YAML Profile 正式 Schema | I19 |
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
