# Glaude 项目实现计划

## 🚀 详细实施计划 (按架构设计展开)

### Phase 0 — 项目基石与统一抽象

* **设计目标**：确立系统的配置中心化管理、环境感知能力以及幽灵日志的异步数据流。
* **设计细节**：
    * **配置路由设计**：建立层级配置合并策略（环境变量 \> 项目级 `.glaude.json` \> 全局 `settings.json`）。确立 CLI 的命令树（Command Tree）拓扑结构。
    * **遥测数据流 (Telemetry Flow)**：设计"双轨"日志隔离。前台仅保留极简的 UI 状态流转；后台设计基于 JSONL 的结构化日志管道，记录完整的 HTTP 载荷、工具调用耗时与内部状态机跃迁，配合轮转机制防止磁盘雪崩。
    * **级联取消树 (Cancellation Tree)**：在主入口点定义全局 Context，制定明确的规则：任何阻塞型操作（网络 IO、外部子进程计算）必须监听取消信号并实施资源回收。
* **📖 参考文档**：[`reference/00-overview.md`](./reference/00-overview.md), [`reference/12-startup-bootstrap.md`](./reference/12-startup-bootstrap.md), [`reference/16-infrastructure-config.md`](./reference/16-infrastructure-config.md), [`reference/17-telemetry-privacy-operations.md`](./reference/17-telemetry-privacy-operations.md)

### Phase 1 — 生命周期与查询引擎

* **设计目标**：统一各大模型 API 的差异，确立 Agent 的主循环机制。
* **设计细节**：
    * **大模型防腐层 (ACL)**：设计通用的 `Message` 与 `ContentBlock` 模型，屏蔽外部 API（Anthropic/OpenAI）的特定数据结构。
    * **Agent 状态机模型**：定义核心的单向循环逻辑：`观察上下文 -> 构建请求 -> 引擎推理 -> 解析意图 (StopReason) -> 触发工具/结束回应`。
    * **测试替身机制 (Mock Harness)**：设计一个无副作用的 Mock 引擎，允许通过读取本地静态 JSON 剧本来驱动 Agent 状态机，用于高速验证业务逻辑。
* **📖 参考文档**：[`reference/01-query-engine.md`](./reference/01-query-engine.md), [`reference/12-startup-bootstrap.md`](./reference/12-startup-bootstrap.md), [`reference/15-services-api-layer.md`](./reference/15-services-api-layer.md)

### Phase 2 — 核心工具与持久化沙箱

* **设计目标**：建立工具注册与调用的契约，解决 Agent 对本地环境状态的持续感知问题。
* **设计细节**：
    * **工具注册表契约 (Registry)**：定义统一的 Tool 抽象接口，强制声明输入 Schema（用于 LLM 认知）、执行逻辑和权限标识（用于安全网）。
    * **持久化 Shell 架构**：抛弃单次 `exec` 模型。设计一个基于管道 (Pipe) 的守护子进程模型，通过注入唯一的 UUID Sentinel（哨兵标记）来实现命令输出的精确截断，确保 `cd`、`export` 等上下文状态在多次请求间得以保留。
    * **进程组隔离**：确立子进程的进程组（PGID）管理机制，确保在超时或强制退出时，能以进程树为单位进行物理级抹杀。
* **📖 参考文档**：[`reference/02-tool-system.md`](./reference/02-tool-system.md), [`reference/06-bash-engine.md`](./reference/06-bash-engine.md)

### Phase 3 — 工具集扩展与提示词工程

* **设计目标**：扩展 Agent 的环境探索能力，并将经验规则编码入 Prompt。
* **设计细节**：
    * **搜索边界设计**：为 Glob 和 Grep 工具设计硬性保护边界（如强制忽略 `.git`, `node_modules`，限制最大返回行数），防止大仓库检索导致内存溢出 (OOM) 或 Token 爆仓。
    * **Prompt 组装管道**：设计一条动态的 System Prompt 生成流水线。策略为：`身份定义 -> 动态环境信息 (OS/Path) -> 活跃工具 Schema 列表 -> 工具避坑指南 (Best Practices)`。
* **📖 参考文档**：[`reference/02-tool-system.md`](./reference/02-tool-system.md), [`reference/10-context-assembly.md`](./reference/10-context-assembly.md)

### Phase 4 — 记忆系统与快照回滚

* **设计目标**：建立系统级的代码防呆机制（回滚）和跨会话的知识沉淀。
* **设计细节**：
    * **Checkpoint 树设计**：设计基于内存的栈式文件快照结构。每次写操作前压栈旧内容，支持基于 UUID 的事务级 Undo 操作。
    * **记忆存储抽象 (MemoryStore)**：将记忆的"读写策略"与"存储介质"解耦。初期设计基于本地 Markdown 的存储实现，为后期扩展向量数据库预留接口。
    * **级联指令合并**：设计指令文件的向上回溯加载算法，确保全局规范与项目局部规范的正确优先级合并。
* **📖 参考文档**：[`reference/09-session-persistence.md`](./reference/09-session-persistence.md), [`reference/16-infrastructure-config.md`](./reference/16-infrastructure-config.md)

### Phase 5 — 终端 UI 架构层

* **设计目标**：将后台的复杂逻辑映射为无阻塞、高响应的终端极客界面。
* **设计细节**：
    * **MVU 单向数据流**：彻底分离 UI 状态 (Model)、事件处理 (Update) 与渲染层 (View)。所有的异步操作（网络请求、底层命令执行）均通过全局事件总线（消息/命令）抛给主 Update 函数处理，避免并发读写冲突。
    * **富文本与 Diff 渲染机制**：设计统一的文本着色器，对文件变更执行 Diff 算法计算增删块，映射为终端的安全色盘输出。
* **📖 参考文档**：[`reference/14-ui-state.md`](./reference/14-ui-state.md)

### Phase 6 — 上下文预算与压缩调度

* **设计目标**：在有限的上下文窗口内，实现信息密度的最大化保留。
* **设计细节**：
    * **Token 预算分配器**：建立刚性预算模型（System Prompt 预留、工具 Schema 预留、响应空间预留），动态计算历史对话的可用余量。
    * **多级降级压缩 (Compaction Pipeline)**：
        * 第一级 (Micro)：设计本地正则清洗引擎，抹除过期文件读取内容和超长堆栈。
        * 第二级 (Auto)：设计旁路的 LLM 摘要任务，将冗长的交互历史折叠为"当前状态与目标"的结构化短语。
* **📖 参考文档**：[`reference/10-context-assembly.md`](./reference/10-context-assembly.md), [`reference/11-compact-system.md`](./reference/11-compact-system.md)

### Phase 7 — 安全边界与权限矩阵

* **设计目标**：建立绝对的用户掌控权，防止 LLM 的幻觉导致破坏性系统操作。
* **设计细节**：
    * **权限降级漏斗**：设计工具调用的三道安全门：`静态白名单匹配 -> 危险特征正则扫描 (管道/重定向漏洞) -> UI 阻断拦截`。
    * **模式状态机**：设计 4 层安全配置模型（Default / Auto-edit / Plan-only / Auto full）。在 `Plan-only` 模式下，直接从工具注册表中卸载（Unregister）所有具备写权限的工具。
* **📖 参考文档**：[`reference/07-permission-pipeline.md`](./reference/07-permission-pipeline.md), [`reference/06-bash-engine.md`](./reference/06-bash-engine.md)

### Phase 8 — 多智能体分层与 MCP 桥接

* **设计目标**：突破单 Agent 的上下文污染瓶颈，对接外部能力生态。
* **设计细节**：
    * **子 Agent 隔离沙箱**：设计一种 `AgentTool`。当主循环触发此工具时，实例化一个具有全新 Token 预算、全新消息队列但继承当前环境变量的子实例。确立通信契约：子实体仅返回单一的结论字符串，抛弃中间推理过程。
    * **MCP 桥接协议 (Bridge)**：设计外部进程生命周期管理器 (Stdio) 和网络长连接保持器 (SSE)。将 MCP 的 `tools/list` 协议动态映射为本地的 Registry 结构，实现外部工具的透明化调用。
* **📖 参考文档**：[`reference/03-coordinator.md`](./reference/03-coordinator.md), [`reference/08-agent-swarms.md`](./reference/08-agent-swarms.md), [`reference/04-plugin-system.md`](./reference/04-plugin-system.md), [`reference/13-bridge-system.md`](./reference/13-bridge-system.md)

### Phase 9 — 会话路由与模型适配层

* **设计目标**：确立会话状态的持久化协议，兼容各种不稳定的第三方模型格式。
* **设计细节**：
    * **会话序列化协议**：定义包含 `SessionID`, `ParentID`, `Message Tree`, `CWD` 的标准化 JSON 存储格式，支持 DAG（有向无环图）式的会话分支 (Fork) 与回溯 (Resume)。
    * **模型方言修复器 (Dialect Fixer)**：针对开源模型（如 Ollama），在适配器层设计 JSON 语法修正和结构容错逻辑，将残缺的 `tool_calls` 强行拉平为标准 `ContentBlock`。
* **📖 参考文档**：[`reference/09-session-persistence.md`](./reference/09-session-persistence.md), [`reference/15-services-api-layer.md`](./reference/15-services-api-layer.md)

### Phase 10 — 流式状态机与 UI 联动

* **设计目标**：实现"打字机"般的实时反馈，同时保证 JSON 结构体的完整解析。
* **设计细节**：
    * **流式事件分拣器**：设计底层的流式解析状态机。收到 `text_delta` 时，直接将数据块投递至 UI 渲染管线；收到 `tool_use_delta` 时，将数据截留至内存缓冲区，阻塞挂起，直至收到 `stop` 标志再进行完整性校验并投递给工具执行引擎。
* **📖 参考文档**：[`reference/12-startup-bootstrap.md`](./reference/12-startup-bootstrap.md), [`reference/14-ui-state.md`](./reference/14-ui-state.md), [`reference/15-services-api-layer.md`](./reference/15-services-api-layer.md)

### Phase 11 — 自动化钩子与交付

* **设计目标**：提供工程级的生命周期扩展点。
* **设计细节**：
    * **Lifecycle Hooks 设计**：在状态机的特定阶段（如 `PreEdit`, `PostEdit`, `PostCommand`）设计事件发布/订阅机制。允许用户通过配置触发外部脚本（如自动执行代码格式化或静态扫描）。
* **📖 参考文档**：[`reference/05-hook-system.md`](./reference/05-hook-system.md), [`reference/12-startup-bootstrap.md`](./reference/12-startup-bootstrap.md)

---

## 📚 参考文档索引

所有参考文档来自 [claude-reviews-claude](https://github.com/openedclaude/claude-reviews-claude) 项目，提供 Claude Code 各子系统的源码级架构解析（含源码链接），作为 Glaude 实现的参考蓝本。

| 文档 | 主题 | 关联阶段 |
|------|------|----------|
| [`reference/00-overview.md`](./reference/00-overview.md) | 完整架构总览（17 篇浓缩版） | 全阶段通用 |
| [`reference/01-query-engine.md`](./reference/01-query-engine.md) | 查询引擎：核心循环与流式处理 | Phase 1 |
| [`reference/02-tool-system.md`](./reference/02-tool-system.md) | 工具系统：42 模块统一接口 | Phase 2/3 |
| [`reference/03-coordinator.md`](./reference/03-coordinator.md) | 多智能体协调器 | Phase 8 |
| [`reference/04-plugin-system.md`](./reference/04-plugin-system.md) | 插件系统：全生命周期管理 | Phase 8 |
| [`reference/05-hook-system.md`](./reference/05-hook-system.md) | Hook 系统：20 种事件类型 | Phase 11 |
| [`reference/06-bash-engine.md`](./reference/06-bash-engine.md) | Bash 执行引擎：沙箱与管道 | Phase 2/7 |
| [`reference/07-permission-pipeline.md`](./reference/07-permission-pipeline.md) | 权限流水线：纵深防御 | Phase 7 |
| [`reference/08-agent-swarms.md`](./reference/08-agent-swarms.md) | Agent 集群：团队协调 | Phase 8 |
| [`reference/09-session-persistence.md`](./reference/09-session-persistence.md) | 会话持久化 | Phase 4/9 |
| [`reference/10-context-assembly.md`](./reference/10-context-assembly.md) | 上下文装配 | Phase 3/6 |
| [`reference/11-compact-system.md`](./reference/11-compact-system.md) | 压缩系统：多层架构 | Phase 6 |
| [`reference/12-startup-bootstrap.md`](./reference/12-startup-bootstrap.md) | 启动与引导 | Phase 0/1/10/11 |
| [`reference/13-bridge-system.md`](./reference/13-bridge-system.md) | 桥接系统：远程控制协议 | Phase 8 |
| [`reference/14-ui-state.md`](./reference/14-ui-state.md) | UI 与状态管理 | Phase 5/10 |
| [`reference/15-services-api-layer.md`](./reference/15-services-api-layer.md) | 服务层与 API 架构 | Phase 1/9/10 |
| [`reference/16-infrastructure-config.md`](./reference/16-infrastructure-config.md) | 基础设施与配置 | Phase 0/4 |
| [`reference/17-telemetry-privacy-operations.md`](./reference/17-telemetry-privacy-operations.md) | 遥测、隐私与运营 | Phase 0 |
