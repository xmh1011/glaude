# CLAUDE.md — glaude 项目指导

## 项目简介

glaude 是用 Go 实现的 AI Coding Agent，参照 Claude Code 架构。目标：单二进制、零依赖、毫秒级启动。

## ⚠️ 工作流协议（每个 Phase 必须遵循）

1. **开始前**：读 `docs/plan.md` 中对应 Phase 章节，获取设计目标、设计细节和参考文档链接
2. **读参考文档**：按 plan.md 中 `📖 参考文档` 指引，读对应的 `docs/reference/*.md` 参考分析文档（含 Claude Code 源码链接）
3. **读 Claude Code 源码**：参考文档中标注的源码路径（如 `src/tools/BashTool/BashTool.tsx:45-54`）均相对于 `/Users/xiaominghao/code/claude-code`，读对应源码理解设计意图
4. **用 Go 惯用方式实现**：不要逐行翻译 TypeScript，理解后重写
5. **完成子任务后**：更新本文件中对应 `[ ]` → `[x]`，然后 `git commit`
6. **严格按顺序推进，不要跳过任何 Phase**

## Go 编码规范

- 遵循 Effective Go，导出符号必须有 godoc 注释
- 错误用 `fmt.Errorf("context: %w", err)` 包装，禁止 `panic`
- 禁止 `init()`，禁止 CGO，保持交叉编译
- 包名小写单词不用复数（`tool` 不是 `tools`）
- 接口名动词+er（`Provider`），结构体名词（`Registry`）
- 方法接收者用类型首字母（`func (r *Registry) Get(...)`）
- 每个包必须有 `_test.go`，table-driven tests，LLM 调用用接口 mock

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
- 每次完成子项后更新本文件 `[ ]` → `[x]`
- 添加新工具时同时添加：实现、prompt、测试、Registry 注册
- 修改 Agent Loop 前先写测试覆盖现有行为
- commit 格式：`<type>(<scope>): <description>`，每次只做一件事

## 文档地图（按需读取，不要一次全读）

| 文档 | 用途 |
|------|------|
| `docs/plan.md` | **任务执行主文档**，每个 Phase 的设计细节和参考文档指引 |
| `docs/design.md` | Glaude 自身架构设计：模块划分、数据流、接口设计、测试策略 |
| `docs/reference/00-overview.md` | 全阶段参考：Claude Code 完整架构总览（17 篇分析浓缩版） |
| `docs/reference/01-query-engine.md` | Phase 1/6/9 参考：查询引擎核心循环、流式处理、错误恢复 |
| `docs/reference/02-tool-system.md` | Phase 2/3 参考：42 模块统一接口、装配流水线、延迟加载 |
| `docs/reference/03-coordinator.md` | Phase 8 参考：多智能体协调器、Worker 隔离、四阶段工作流 |
| `docs/reference/04-plugin-system.md` | Phase 8 参考：插件全生命周期、Skill 系统、MCP 集成 |
| `docs/reference/05-hook-system.md` | Phase 11 参考：20 种 Hook 事件、匹配机制、聚合策略 |
| `docs/reference/06-bash-engine.md` | Phase 2/7 参考：Bash 纵深防御、Shell 生命周期、OS 沙箱 |
| `docs/reference/07-permission-pipeline.md` | Phase 7 参考：权限流水线、纵深防御、分层治理链 |
| `docs/reference/08-agent-swarms.md` | Phase 8 参考：Agent 集群、团队协调、Fork 路径 |
| `docs/reference/09-session-persistence.md` | Phase 4/9 参考：会话持久化、DAG 分支、序列化协议 |
| `docs/reference/10-context-assembly.md` | Phase 3/6 参考：上下文装配、Prompt 动态组装、缓存策略 |
| `docs/reference/11-compact-system.md` | Phase 6 参考：三层压缩架构、微压缩、自动压缩熔断 |
| `docs/reference/12-startup-bootstrap.md` | Phase 0/1/10/11 参考：启动引导、三层建制、生命周期 |
| `docs/reference/13-bridge-system.md` | Phase 8 参考：桥接系统、远程控制协议、多端会话 |
| `docs/reference/14-ui-state-management.md` | Phase 5/10 参考：UI 状态管理、事件驱动架构 |
| `docs/reference/14-ui-state-rendering.md` | Phase 5 参考：UI 渲染管线、布局系统、Vim 模式 |
| `docs/reference/15-services-api-layer.md` | Phase 1/9/10 参考：queryModel 引擎、重试策略、流式处理 |
| `docs/reference/16-infrastructure-config.md` | Phase 0/4 参考：Bootstrap 单例、五层设置合并、安全存储 |
| `docs/reference/17-telemetry-privacy-operations.md` | Phase 0 参考：双通道遥测、模型代号、远程控制 |

## 🚀 任务跟踪

### Phase 0: 项目基石与统一抽象
> 📖 开始前读：`docs/plan.md` Phase 0 + `docs/reference/00-overview.md` + `docs/reference/12-startup-bootstrap.md` + `docs/reference/16-infrastructure-config.md` + `docs/reference/17-telemetry-privacy-operations.md`
- [x] 初始化目录结构与 `go mod`
- [x] 引入 `cobra` 与 `viper`，确立 CLI 路由树与配置读取策略
- [x] 引入 `logrus` 与 `lumberjack`，实现幽灵日志 (`telemetry`) 与双轨输出
- [x] 在主入口捕获 `SIGINT/SIGTERM`，建立全局 `context.Context` 级联取消树

### Phase 1: 生命周期与查询引擎
> 📖 开始前读：`docs/plan.md` Phase 1 + `docs/reference/01-query-engine.md` + `docs/reference/12-startup-bootstrap.md` + `docs/reference/15-services-api-layer.md`
- [x] 定义通用 `Message` 与 `ContentBlock` 模型
- [x] 定义 `Provider` 接口，实现 `AnthropicProvider` (带错误解析)
- [x] 实现 `MockProvider`，读取本地静态 JSON 剧本以供低成本测试
- [x] 实现基础 Agent 状态机 (`while` 循环：请求 -> 判断 `stop_reason` -> 退出)

### Phase 2: 核心工具与持久化沙箱
> 📖 开始前读：`docs/plan.md` Phase 2 + `docs/reference/02-tool-system.md` + `docs/reference/06-bash-engine.md`
- [x] 定义带 `ctx` 的 `Tool` 接口，实现 `Registry` 注册表
- [x] 实现 `FileReadTool` 与 `FileEditTool` (精准 str_replace)
- [x] 实现持久化 `BashTool` (后台 `bash --norc` 进程，基于 UUID Sentinel 截断输出)
- [x] 将工具执行逻辑整合进 Agent 主循环，并将执行错误作为上下文回调给 LLM

### Phase 3: 工具集扩展与提示词工程
> 📖 开始前读：`docs/plan.md` Phase 3 + `docs/reference/02-tool-system.md` + `docs/reference/10-context-assembly.md`
- [ ] 实现 `GlobTool` 与 `GrepTool`，强制引入 `.gitignore` 过滤机制与最大行数限制
- [ ] 实现 `FileWriteTool` 与 `LSTool`
- [ ] 建立 System Prompt 动态组装流水线 (组合系统信息与各工具 Schema/Prompt)

### Phase 4: 记忆系统与快照回滚
> 📖 开始前读：`docs/plan.md` Phase 4 + `docs/reference/09-session-persistence.md` + `docs/reference/16-infrastructure-config.md`
- [ ] 抽象 `MemoryStore` 接口，实现基于 Markdown 的本地存储
- [ ] 实现指令级联合并 (合并 `~/.glaude/GLAUDE.md` 与项目根目录指令)
- [ ] 完善 `Checkpoint` 引擎，实现栈式的跨文件内存快照与 `Undo()` 撤销功能

### Phase 5: 终端 UI 架构层
> 📖 开始前读：`docs/plan.md` Phase 5 + `docs/reference/14-ui-state-management.md` + `docs/reference/14-ui-state-rendering.md`
- [ ] 引入 `bubbletea` 建立 MVU 架构状态机
- [ ] 引入 `glamour` 渲染 LLM 输出，实现状态指示器 (Spinner/执行摘要)
- [ ] 引入 `go-diff` 与 `lipgloss` 渲染文件变更补丁
- [ ] 解析内部斜杠命令 (`/undo`, `/clear`, `/context`, `/exit`)

### Phase 6: 上下文预算与压缩调度
> 📖 开始前读：`docs/plan.md` Phase 6 + `docs/reference/10-context-assembly.md` + `docs/reference/11-compact-system.md`
- [ ] 引入 `tiktoken-go` 进行本地 Token 估算，并在 UI 底部展示预算水位
- [ ] 实现 `MicroCompact` 降级策略 (截短超长工具输出，清理无效重试)
- [ ] 实现 `AutoCompact` LLM 摘要压缩机制

### Phase 7: 安全边界与权限矩阵
> 📖 开始前读：`docs/plan.md` Phase 7 + `docs/reference/07-permission-pipeline.md` + `docs/reference/06-bash-engine.md`
- [ ] 读取配置文件设定四层安全模式 (Default/Auto-edit/Plan-only/Auto full)
- [ ] 在 BashTool 执行前实现危险特征正则扫描引擎 (拦截管道重定向等风险指令)
- [ ] 接入 UI 阻断拦截弹窗 (请求用户 `y/n` 授权)

### Phase 8: 多智能体分层与 MCP 桥接
> 📖 开始前读：`docs/plan.md` Phase 8 + `docs/reference/03-coordinator.md` + `docs/reference/08-agent-swarms.md` + `docs/reference/04-plugin-system.md` + `docs/reference/13-bridge-system.md`
- [ ] 实现 `AgentTool` (实例化具有独立预算和消息队列的子 Agent，仅返回结论)
- [ ] 搭建 MCP 桥接协议核心 (Stdio 子进程管理器与 SSE 监听)
- [ ] 实现动态路由，将远端 MCP 工具注册进本地 Registry

### Phase 9: 会话路由与模型适配层
> 📖 开始前读：`docs/plan.md` Phase 9 + `docs/reference/01-query-engine.md` + `docs/reference/09-session-persistence.md` + `docs/reference/15-services-api-layer.md`
- [ ] 实现 `OpenAIProvider` 与 `OllamaProvider` (针对弱模型的 JSON 格式修复)
- [ ] 制定会话序列化格式，落地 Session 数据到本地目录
- [ ] 支撑 `--continue` 恢复与 `--fork` 分支逻辑

### Phase 10: 流式状态机与 UI 联动
> 📖 开始前读：`docs/plan.md` Phase 10 + `docs/reference/12-startup-bootstrap.md` + `docs/reference/14-ui-state-management.md` + `docs/reference/15-services-api-layer.md`
- [ ] 实现 Provider 层 SSE 流解析引擎
- [ ] 构建流式事件分拣器 (文本碎片实时送达 UI 渲染，JSON 工具碎片缓存阻断)
- [ ] 联调异步 UI 更新机制

### Phase 11: 自动化钩子与交付
> 📖 开始前读：`docs/plan.md` Phase 11 + `docs/reference/05-hook-system.md` + `docs/reference/12-startup-bootstrap.md`
- [ ] 落地生命周期 Hook 点 (如 `post_edit`)
- [ ] 编写 `/init` 交互式引导生成 `.glaude.json`
- [ ] 配置 CI/CD 多架构交叉编译与全量测试验证
