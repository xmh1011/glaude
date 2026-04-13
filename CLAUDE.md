# CLAUDE.md — glaude 项目指导

## 项目简介

glaude 是用 Go 实现的 AI Coding Agent，参照 Claude Code 架构。目标：单二进制、零依赖、毫秒级启动。

## ⚠️ 工作流协议（每个 Phase 必须遵循）

1. **读计划文档**：读 `docs/plan.md` 中对应 Phase 章节，获取设计目标、设计细节和参考文档链接
2. **读参考文档**：按 plan.md 中 `📖 参考文档` 指引，读对应的 `docs/reference/*.md` 参考分析文档
3. **⛔ 读 Claude Code 源码（不可跳过）**：
   - plan.md 每个 Phase 的 `📂 必读源码` 列出了该阶段必读的源文件清单
   - 所有路径相对于 `/Users/xiaominghao/code/claude-code`，**必须用 Read 工具实际打开**
   - 不能仅依赖参考文档中的代码片段，参考文档是索引，源码是实现依据
   - 开始写代码前，先列出你已阅读的 Claude Code 源文件路径，确认覆盖了必读清单
4. **用 Go 惯用方式实现**：理解 TypeScript 源码的设计意图后，用 Go 惯用模式重写，不要逐行翻译
5. **完成子任务后**：更新本文件中对应 `[ ]` → `[x]`，然后 `git commit`
6. **严格按顺序推进，不要跳过任何 Phase**

## Go 编码规范

- 遵循 Effective Go，导出符号必须有 godoc 注释
- 错误用 `fmt.Errorf("context: %w", err)` 包装，禁止 `panic`
- 禁止 `init()`，禁止 CGO，保持交叉编译
- 单个函数尽量不超过 50 行，单个文件尽量不超过 500 行，注意逻辑的解耦
- 包名小写单词不用复数（`tool` 不是 `tools`）
- 接口名动词+er（`Provider`），结构体名词（`Registry`）
- 方法接收者用类型首字母（`func (r *Registry) Get(...)`）
- 每个包必须有 `_test.go`，table-driven tests，LLM 调用用接口 mock，使用 `github.com/stretchr/testify/assert` 断言

## 核心接口约束（修改需谨慎）

- **Provider**：统一消息类型，API 格式差异在实现内部转换，不泄漏到上层
- **Tool**：Execute 返回纯文本，工具间不直接引用，通过 Registry 间接调用
- **Compactor**：不修改输入返回新切片；MicroCompact 禁止调用 LLM API
- **Transport**（MCP）：JSON-RPC 2.0，必须支持优雅关闭，处理子进程意外退出

## 工具实现要点

- **BashTool**：持久 shell，sentinel 分隔输出，截断 100KB，安全检查宁误拦不漏放
- **FileEditTool**：str_replace 语义，old_str 必须唯一，替换前 Checkpoint.Save
- **GlobTool/GrepTool**：排除 `.git`/`node_modules`/`vendor`，遵循 `.gitignore`，上限 200 条

## Agent 行为约束

- 实现模块前**必读** `docs/plan.md` 对应章节及其参考文档链接
- ⛔ **必须用 Read 工具读取 Claude Code 源码**（`/Users/xiaominghao/code/claude-code` 下的 `.ts`/`.tsx` 文件），参考文档只是索引，源码才是实现的第一手依据
- 每次完成子项后更新本文件 `[ ]` → `[x]`
- 添加新工具时同时添加：实现、prompt、测试、Registry 注册
- 修改 Agent Loop 前先写测试覆盖现有行为
- commit 格式：`<type>(<scope>): <description>`，每次只做一件事

## 文档地图（按需读取，不要一次全读）

| 文档                                                  | 用途                                       |
|-----------------------------------------------------|------------------------------------------|
| `docs/plan.md`                                      | **任务执行主文档**，每个 Phase 的设计细节和参考文档指引        |
| `docs/design.md`                                    | Glaude 自身架构设计：模块划分、数据流、接口设计、测试策略         |
| `docs/reference/00-overview.md`                     | 全阶段参考：Claude Code 完整架构总览（17 篇分析浓缩版）      |
| `docs/reference/01-query-engine.md`                 | Phase 1/6/9 参考：查询引擎核心循环、流式处理、错误恢复        |
| `docs/reference/02-tool-system.md`                  | Phase 2/3 参考：42 模块统一接口、装配流水线、延迟加载        |
| `docs/reference/03-coordinator.md`                  | Phase 8 参考：多智能体协调器、Worker 隔离、四阶段工作流      |
| `docs/reference/04-plugin-system.md`                | Phase 8/11/12 参考：插件全生命周期、Skill 系统、MCP 集成 |
| `docs/reference/05-hook-system.md`                  | Phase 12 参考：20 种 Hook 事件、匹配机制、聚合策略       |
| `docs/reference/06-bash-engine.md`                  | Phase 2/7 参考：Bash 纵深防御、Shell 生命周期、OS 沙箱  |
| `docs/reference/07-permission-pipeline.md`          | Phase 7 参考：权限流水线、纵深防御、分层治理链              |
| `docs/reference/08-agent-swarms.md`                 | Phase 8 参考：Agent 集群、团队协调、Fork 路径         |
| `docs/reference/09-session-persistence.md`          | Phase 4/9 参考：会话持久化、DAG 分支、序列化协议          |
| `docs/reference/10-context-assembly.md`             | Phase 3/6 参考：上下文装配、Prompt 动态组装、缓存策略      |
| `docs/reference/11-compact-system.md`               | Phase 6 参考：三层压缩架构、微压缩、自动压缩熔断             |
| `docs/reference/12-startup-bootstrap.md`            | Phase 0/1/10/12 参考：启动引导、三层建制、生命周期        |
| `docs/reference/13-bridge-system.md`                | Phase 8 参考：桥接系统、远程控制协议、多端会话              |
| `docs/reference/14-ui-state.md`                     | Phase 5/10 参考：UI 状态管理、渲染管线、事件系统、Vim 模式   |
| `docs/reference/15-services-api-layer.md`           | Phase 1/9/10 参考：queryModel 引擎、重试策略、流式处理  |
| `docs/reference/16-infrastructure-config.md`        | Phase 0/4 参考：Bootstrap 单例、五层设置合并、安全存储    |
| `docs/reference/17-telemetry-privacy-operations.md` | Phase 0 参考：双通道遥测、模型代号、远程控制               |

## 🚀 任务跟踪

### Phase 0: 项目基石与统一抽象
> 📖 开始前读：`docs/plan.md` Phase 0（含参考文档 + 必读源码清单）
- [x] 初始化目录结构与 `go mod`
- [x] 引入 `cobra` 与 `viper`，确立 CLI 路由树与配置读取策略
- [x] 引入 `logrus` 与 `lumberjack`，实现幽灵日志 (`telemetry`) 与双轨输出
- [x] 在主入口捕获 `SIGINT/SIGTERM`，建立全局 `context.Context` 级联取消树

### Phase 1: 生命周期与查询引擎
> 📖 开始前读：`docs/plan.md` Phase 1（含参考文档 + 必读源码清单）
- [x] 定义通用 `Message` 与 `ContentBlock` 模型
- [x] 定义 `Provider` 接口，实现 `AnthropicProvider` (带错误解析)
- [x] 实现 `MockProvider`，读取本地静态 JSON 剧本以供低成本测试
- [x] 实现基础 Agent 状态机 (`while` 循环：请求 -> 判断 `stop_reason` -> 退出)

### Phase 2: 核心工具与持久化沙箱
> 📖 开始前读：`docs/plan.md` Phase 2（含参考文档 + 必读源码清单）
- [x] 定义带 `ctx` 的 `Tool` 接口，实现 `Registry` 注册表
- [x] 实现 `FileReadTool` 与 `FileEditTool` (精准 str_replace)
- [x] 实现持久化 `BashTool` (后台 `bash --norc` 进程，基于 UUID Sentinel 截断输出)
- [x] 将工具执行逻辑整合进 Agent 主循环，并将执行错误作为上下文回调给 LLM

### Phase 3: 工具集扩展与提示词工程
> 📖 开始前读：`docs/plan.md` Phase 3（含参考文档 + 必读源码清单）
- [x] 实现 `GlobTool` 与 `GrepTool`，强制引入 `.gitignore` 过滤机制与最大行数限制
- [x] 实现 `FileWriteTool` 与 `LSTool`
- [x] 建立 System Prompt 动态组装流水线 (组合系统信息与各工具 Schema/Prompt)

### Phase 4: 记忆系统与快照回滚
> 📖 开始前读：`docs/plan.md` Phase 4（含参考文档 + 必读源码清单）
- [x] 抽象 `MemoryStore` 接口，实现基于 Markdown 的本地存储
- [x] 实现指令级联合并 (合并 `~/.glaude/GLAUDE.md` 与项目根目录指令)
- [x] 完善 `Checkpoint` 引擎，实现栈式的跨文件内存快照与 `Undo()` 撤销功能

### Phase 5: 终端 UI 架构层
> 📖 开始前读：`docs/plan.md` Phase 5（含参考文档 + 必读源码清单）
- [x] 引入 `bubbletea` 建立 MVU 架构状态机
- [x] 引入 `glamour` 渲染 LLM 输出，实现状态指示器 (Spinner/执行摘要)
- [x] 引入 `go-diff` 与 `lipgloss` 渲染文件变更补丁
- [x] 解析内部斜杠命令 (`/undo`, `/clear`, `/context`, `/exit`)

### Phase 6: 上下文预算与压缩调度
> 📖 开始前读：`docs/plan.md` Phase 6（含参考文档 + 必读源码清单）
- [x] 引入 `tiktoken-go` 进行本地 Token 估算，并在 UI 底部展示预算水位
- [x] 实现 `MicroCompact` 降级策略 (截短超长工具输出，清理无效重试)
- [x] 实现 `AutoCompact` LLM 摘要压缩机制

### Phase 7: 安全边界与权限矩阵
> 📖 开始前读：`docs/plan.md` Phase 7（含参考文档 + 必读源码清单）
- [x] 读取配置文件设定四层安全模式 (Default/Auto-edit/Plan-only/Auto full)
- [x] 在 BashTool 执行前实现危险特征正则扫描引擎 (拦截管道重定向等风险指令)
- [x] 接入 UI 阻断拦截弹窗 (请求用户 `y/n` 授权)

### Phase 8: 多智能体分层与 MCP 桥接
> 📖 开始前读：`docs/plan.md` Phase 8（含参考文档 + 必读源码清单）
- [x] 实现 `AgentTool` (实例化具有独立预算和消息队列的子 Agent，仅返回结论)
- [x] 搭建 MCP 桥接协议核心 (Stdio 子进程管理器与 SSE 监听)
- [x] 实现动态路由，将远端 MCP 工具注册进本地 Registry

### Phase 9: 会话路由与模型适配层
> 📖 开始前读：`docs/plan.md` Phase 9（含参考文档 + 必读源码清单）
- [x] 实现 `OpenAIProvider`（官方 openai-go SDK）与 `OllamaProvider`（复用 OpenAI 兼容接口），`DialectFixer` 装饰器修复弱模型 JSON 格式，`NewProvider` 工厂函数按配置选择后端
- [x] 制定会话序列化格式（JSONL append-only，Entry 带 UUID/ParentUUID DAG 链），落地 `internal/session/` 包（store/chain/list），路径 `~/.glaude/projects/{sanitized_cwd}/{session-id}.jsonl`
- [x] Agent 集成 session.Store 自动持久化，CLI 支持 `--continue`（恢复最近会话）与 `--resume <id>`（恢复指定会话）

### Phase 10: 流式状态机与 UI 联动
> 📖 开始前读：`docs/plan.md` Phase 10（含参考文档 + 必读源码清单）
- [x] 实现 Provider 层 SSE 流解析引擎：新增 `StreamingProvider` 接口与 `StreamEvent` 类型（6 种事件），Anthropic/OpenAI 各自实现 `CompleteStream` 方法，DialectFixer/RetryProvider 流式穿透
- [x] 构建流式事件分拣器：Agent 新增 `RunStream` + `consumeStream`，`text_delta` 实时回调 UI，`tool_use` 碎片缓冲至 `content_block_stop` 后 `SafeParseJSON` 修复再执行
- [x] 联调异步 UI 更新机制：UI 新增 `streamTextMsg`/`streamToolStartMsg`/`streamDoneMsg` 消息类型，View 渲染流式文本+闪烁光标，通过 `programRef` 共享指针实现 bubbletea 值拷贝安全的 `p.Send()`

### Phase 11: Skill 系统与 Prompt-as-Code
> 📖 开始前读：`docs/plan.md` Phase 11（含参考文档 + 必读源码清单）
- [x] 定义 Skill Markdown + YAML Frontmatter 规范（name/description/allowedTools/whenToUse/executionContext），实现 Skill 解析器
- [x] 实现六层 Skill 加载链（项目本地 → 用户全局 → 插件 → 企业管控 → 内置 → MCP），按命名空间合并
- [x] 实现 `SkillTool`（注册进 Registry，支持 inline 注入和 fork 隔离执行），将 Skill 自动映射为斜杠命令
- [x] 实现 Skill 列表的 Token 预算控制（三级降级：完整描述 → 截断 → 仅名称）

### Phase 12: 自动化钩子与交付
> 📖 开始前读：`docs/plan.md` Phase 12（含参考文档 + 必读源码清单）
- [x] 落地生命周期 Hook 点 (如 `post_edit`)
- [x] 编写 `/init` 交互式引导生成 `.glaude.json`
- [x] 配置 CI/CD 多架构交叉编译与全量测试验证
- [ ] 整合 Plugin 三位一体：Skill (Phase 11) + Hook + MCP (Phase 8)，实现 plugin manifest 声明式配置
