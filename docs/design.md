# Glaude 架构设计与实现原理

## Go 语言实现的 AI Coding Agent — 参照 Claude Code 架构

---

## 一、Claude Code 架构逆向分析

### 1.1 核心本质
Claude Code 不是一个"聊天工具"，而是一个 **agentic harness（智能体运行框架）**。它的核心工作是：给 LLM 装上"手脚"（工具），让 LLM 能与真实世界交互，然后在一个循环中不断让 LLM 决策→执行→观察→再决策。

LLM 本身只能输出文本。Claude Code 的价值在于把 LLM 的文本输出**翻译成真实的系统操作**（读文件、写代码、跑命令），再把操作结果**翻译回文本**喂给 LLM，形成闭环。

### 1.2 Agentic Loop 原理
整个系统的心脏是一个极其简单的 while 循环：

```text
while true:
    response = llm.call(messages, tools)
    messages.append(assistant: response)

    if response.stop_reason == "end_turn":
        break   // LLM 认为任务完成，退出

    for each tool_call in response:
        result = execute_tool(tool_call)
        messages.append(user: tool_result(result))

    // 继续循环，让 LLM 看到工具结果后做下一步决策
```

这个循环的精髓在于：**LLM 既是决策者也是终止判断者**。它自己决定调用什么工具、自己判断何时任务完成。框架只负责忠实地执行工具并回传结果。

Claude Code 在此基础上叠加了三个增强：
* **上下文压缩**：循环跑久了消息列表会撑爆上下文窗口，需要在每次迭代前检查并压缩。
* **权限拦截**：工具执行前插入权限检查，危险操作暂停循环等用户确认。
* **错误恢复**：工具执行失败不终止循环，而是把错误信息作为 tool_result 回传，让 LLM 自行修正。

### 1.3 工具系统设计哲学
**工具是 LLM 的感官和肢体**。分为三类能力：
* **感知类**（只读）：FileRead、Glob、Grep、LS — LLM 用这些"看"代码库。
* **行动类**（写操作）：FileEdit、FileWrite、Bash — LLM 用这些"改"代码。
* **元能力类**：AgentTool（派生子 Agent）、WebFetch（访问网络）。

**每个工具由三部分组成**：
1.  **Schema**：JSON Schema 描述输入格式，这是 LLM 看到的"工具说明书"。
2.  **Prompt**：嵌入 system prompt 的使用指引，教 LLM 何时用、怎么用这个工具。
3.  **Executor**：实际执行逻辑。

**工具的 prompt 比执行逻辑更重要**。Claude Code 的 BashTool 有 1,143 行代码，但 prompt 有 6,500 tokens。prompt 里编码了大量从生产事故中总结的反模式："不要用 cat 读大文件"、"不要用 sed 编辑文件"、"git hook 失败后不要 --amend"。这些 prompt 是让 Agent 可靠工作的关键。

### 1.4 上下文管理——最被低估的复杂度
上下文窗口是有限资源，而 Agent 的每次工具调用都在消耗它。Claude Code 用三层策略应对：
* **第一层：MicroCompact（本地裁剪，零 API 成本）**：截断超长输出，删除被覆盖的文件读取结果，折叠重复错误。
* **第二层：AutoCompact（LLM 辅助摘要）**：超过阈值时，调用 LLM 生成对话摘要，保留关键代码和决策。
* **第三层：Full Compact（完整重建）**：极端情况下压缩整个对话，选择性重新注入关键文件内容。

### 1.5 子 Agent（AgentTool）——递归问题分解
AgentTool 是最巧妙的设计。它让 Agent 能"派生"一个子 Agent 来处理子任务。子 Agent 拥有独立的上下文窗口和受限的范围，只返回最终结果给主 Agent。这完美解决了上下文污染和复杂任务分解的问题。

---

## 二、Glaude 架构设计

### 2.1 模块划分原则
按**职责边界**而非技术分层划分模块。每个模块回答一个问题：

| 模块 | 回答的问题 |
| :--- | :--- |
| `agent` | 如何驱动 LLM 循环执行任务？ |
| `llm` | 如何与不同的 LLM 提供商通信？ |
| `tool` | 如何定义、注册、执行工具？ |
| `context` | 如何管理有限的上下文窗口？ |
| `memory` | 如何在会话间保持知识？ |
| `config` | 如何管理用户配置和权限规则？ |
| `mcp` | 如何连接外部 MCP 服务？ |
| `ui` | 如何与用户交互？ |
| `telemetry` | 如何记录内部状态和追踪成本？ |

### 2.2 核心接口设计
整个系统围绕四个核心接口构建：

* **LLM Provider**：抽象 LLM 通信。统一消息模型，屏蔽不同 API（Anthropic/OpenAI/Ollama）的差异。
* **Tool**：抽象工具能力。暴露名称、描述、JSON Schema、执行方法和权限属性。
* **Compactor**：抽象上下文压缩。不同压缩策略实现同一接口，在管道中组合使用。
* **Transport**：抽象 MCP 传输。支持 stdio 和 SSE，上层代码不关心底层通信方式。

### 2.3 数据流

```text
用户输入 "fix the bug in auth.go"
    │
    ▼
┌─────────────────────────────────────────────────┐
│ Agent Loop                                       │
│                                                  │
│  1. 组装消息列表                                  │
│     [system prompt + GLAUDE.md + memory]         │
│     [历史消息...]                                 │
│     [user: "fix the bug in auth.go"]             │
│                                                  │
│  2. 上下文检查                                    │
│     token_count(messages) > threshold?           │
│     是 → 触发 Compactor 压缩                     │
│                                                  │
│  3. 调用 LLM (ctx 控制)                           │
│     provider.Complete(ctx, messages, tools) ─────┼──→ LLM API
│                                                  │     ← response
│  4. 解析响应                                      │
│     stop_reason == "tool_use"?                   │
│     是 → 提取 tool_use blocks                    │
│                                                  │
│  5. 权限检查                                      │
│     tool.NeedsPermission()?                      │
│     是 → 暂停，询问用户 ─────────────────────────┼──→ UI 层
│         用户确认 ←───────────────────────────────┼──
│                                                  │
│  6. 执行工具 (ctx 透传)                           │
│     tool.Execute(ctx, input)                     │
│     ├── FileRead → 读文件                        │
│     ├── FileEdit → 创建 checkpoint → 编辑        │
│     ├── Bash → 安全检查 → 执行命令               │
│     └── AgentTool → 递归创建子 Agent Loop        │
│                                                  │
│  7. 回传结果                                      │
│     messages.append(tool_result)                 │
│     → 回到步骤 2，继续循环                        │
│                                                  │
│  8. 循环终止                                      │
│     stop_reason == "end_turn" → 输出给用户        │
└─────────────────────────────────────────────────┘
```

### 2.4 级联取消与生命周期管理 (Context Cancellation)
Glaude 强依赖 Go 的 `context.Context` 进行整个 Agent Loop 的生命周期管理：
* **LLM 请求中断**：用户在终端按下 `Ctrl+C` 时，触发 Context Cancel，`llm.Provider` 必须立即中止 HTTP 请求，停止 token 流式输出。
* **工具执行中断**：Context 必须透传给 `Tool.Execute(ctx)`。特别是在 `BashTool` 中，如果 Agent 跑了一个死循环脚本（如无限重试的 curl），Cancel 信号必须能被捕获，并优雅地向底层的 Bash 进程发送 `SIGINT` 或 `SIGTERM`，防止孤儿进程遗留在系统中。
* **优雅退出**：接收到中断信号后，系统需保证当前的会话状态（Session Messages）和内存快照（Checkpoint）被安全落盘后再彻底退出，绝不能造成文件损坏。

### 2.5 Agent 测试分层策略
纯大模型调用的集成测试不仅慢且昂贵。Glaude 采用严格的分层测试：
* **工具层 (Unit Test)**：最核心的保障。使用 Table-driven tests 覆盖 `BashTool` 的各种恶意注入场景，覆盖 `FileEditTool` 的字符串定位边界条件。这些测试完全在本地运行，零 LLM 依赖。
* **Provider 抽象层 (Mock Test)**：实现一个 `MockProvider`，通过预置剧本（Scripted Responses）模拟 LLM 返回 `tool_use` 或 `end_turn`，验证 Agent Loop 的状态机是否正确运转、上下文压缩是否按预期触发、权限拦截是否生效。
* **黄金路径测试 (E2E Test)**：每天在 CI 中跑一次真实的 API 调用。给定一个预设的有 Bug 的代码库，断言 Glaude 能否在 5 轮对话内成功修复并跑通 `go test`。

---

## 三、各模块实现原理

### 3.1 Agent Loop（`internal/agent/`）
Agent Loop 本质是一个状态机。消息列表是唯一的状态，不维护额外的任务状态。工具执行是串行的。工具执行失败时，将错误信息作为 tool_result 回传，让 LLM 自行修正。支持 Session 的 Resume 和 Fork。

### 3.2 LLM 抽象层（`internal/llm/`）
使用统一的 Message 和 ContentBlock 模型。支持 SSE 流式输出，内置流式状态机以解析增量到达的 `tool_use` 块。

### 3.3 工具系统（`internal/tool/`）
* **Registry**：动态维护工具表，支持 MCP 工具延迟加载 (ToolSearch)。
* **BashTool**：分语法分析、黑名单、白名单、确认四层安全检查。维护持久的后台 shell 会话。
* **FileEditTool**：采用 `str_replace` 语义精确替换，替换前自动创建可回滚的内存 Checkpoint。
* **Glob/GrepTool**：默认排除 vendor/node_modules，遵循 `.gitignore`，保障大仓库下的搜索性能。

### 3.4 上下文管理（`internal/context/`）
客户端使用 tiktoken 近似估算 token。严格分配上下文预算（为 System prompt、Tool definition、响应预留空间）。

### 3.5 记忆系统（`internal/memory/`）
* **GLAUDE.md 加载链**：按 全局 -> 项目 -> 当前目录 优先级合并指令，注入 System Prompt。
* **MemoryStore 接口抽象**：记忆系统通过抽象的 `MemoryStore` 接口与核心业务解耦。早期实现为本地 Markdown 文件读写（`LocalMarkdownStore`，如 `MEMORY.md`）。这为未来引入高级记忆框架预留空间——例如，可平滑接入 PostgreSQL/RDS 向量数据库进行相似度检索，或引入长期记忆反思机制，以支持在大规模集群项目中保持连贯上下文。

### 3.6 权限系统（`internal/config/`）
分 Default、Auto-edit、Plan only、Auto full 四种模式。命令按风险分为绿（只读）、黄（受控写入）、红（危险），红色命令在任何模式下均需显式确认。

### 3.7 MCP 集成（`internal/mcp/`）
实现 Model Context Protocol。支持 stdio (本地进程) 和 SSE (远程 HTTP) 传输。工具注册表代理转发 MCP 请求，使外部能力与内置工具在使用体验上完全一致。

### 3.8 终端 UI（`internal/ui/`）
基于 `bubbletea` 的 Elm 架构。并发协调 LLM 流式输出、工具异步执行与用户输入。支持 Diff 语法高亮展示。

### 3.9 可观测性与成本追踪（`internal/telemetry/`）
Agent 开发的 Debug 极度依赖内部状态透视：
* **幽灵日志 (Ghost Logging)**：所有内部状态迁移、API 耗时、详细的 HTTP 请求/响应（包含完整的 System Prompt和 Tool Context），通过 `zap/slog` 异步写入 `~/.glaude/logs/` 的 JSONL 文件中，绝不干扰终端 UI。
* **Token & 成本记账本**：在 Session 级别维护计量器。解析 Provider 返回的 usage 字段，实时在 TUI 底部状态栏显示本次会话消耗的 Token 数量和预估美元成本。
* **Dump & Trace**：提供一个隐藏斜杠命令（如 `/dump`），一键打包当前上下文、本地记忆和近期错误日志，方便进行本地 Prompt 回放和 Debug。

---

## 四、分阶段实现路线

| 阶段 | 目标与里程碑 | 预估耗时 |
| :--- | :--- | :--- |
| **Phase 0** | **最小可运行骨架**。搭建项目结构，定义核心接口，`go build` 通过。 | 1 天 |
| **Phase 1** | **纯文本对话**。实现 Provider 和极简 Loop。`glaude -p "hello"` 可跑通。 | 2-3 天 |
| **Phase 2** | **工具系统核心**。实现 Registry、BashTool、FileRead、FileEdit，引入 Context Cancel。 | 5-6 天 |
| **Phase 3** | **扩展工具集**。FileWrite、Glob、Grep、LS，重点调优 Tool Prompt。 | 3-4 天 |
| **Phase 4** | **记忆与上下文**。实现 GLAUDE.md 加载、Checkpoint、MicroCompact 压缩。 | 4-5 天 |
| **Phase 5** | **终端 UI**。引入 Bubble Tea，实现流式渲染和并发事件处理。 | 4-5 天 |
| **Phase 6** | **权限与观测**。四种权限模式、命令白名单、Ghost Logging 落地。 | 2-3 天 |
| **Phase 7** | **子 Agent & MCP**。实现递归 Agent Loop 和 MCP Client。 | 5-6 天 |
| **Phase 8** | **高级特性**。多 Provider (OpenAI/Ollama)、Session 持久化、云端记忆抽象。 | 4-5 天 |
| **Phase 9** | **发布打磨**。完善 E2E 测试、文档编写、goreleaser 发布。 | 3-4 天 |

---

## 五、关键技术选型

| 职责 | 选型 | 选择理由 |
| :--- | :--- | :--- |
| TUI 框架 | `bubbletea` | Go 生态事实上的标准 TUI 框架，Elm 架构适合并发状态机 |
| CLI / 配置 | `cobra` + `viper` | Kubernetes/Docker 同款，支持多种配置文件和环境变量 |
| 日志遥测 | `go.uber.org/zap` | 高性能结构化日志，适合落盘 Ghost Logging |
| LLM SDK | `anthropic-sdk-go` / `go-openai` | 官方/主流社区维护，类型安全，更新及时 |
| Token 计数 | `github.com/pkoukk/tiktoken-go` | 精度高，性能好 |
| Glob 搜索 | `github.com/bmatcuk/doublestar` | 支持 `**` 递归匹配，规避标准库能力不足 |
| Diff 对比 | `github.com/sergi/go-diff` | 稳定可靠的 Myers diff 算法实现 |

---

## 六、风险与应对策略

1.  **Context 泄漏与僵尸进程**
    * *风险*：用户频繁中断可能导致后台 Bash 进程残留。
    * *应对*：采用严格的 `context.WithCancel` 树，结合 `os/exec` 的 `SysProcAttr` 设置进程组 ID (PGID)，确保 `kill -负PID` 能一波带走整个进程树。
2.  **大仓库工具回填导致 OOM**
    * *风险*：`GrepTool` 或 `BashTool` 返回巨大日志，撑爆内存。
    * *应对*：在工具执行器层实施硬性截断（如超过 100KB 强制腰斩），并在 MicroCompact 阶段丢弃无价值信息。
3.  **不同模型的工具调用幻觉**
    * *风险*：接入开源模型（如 Ollama）时，工具调用格式不稳定。
    * *应对*：架构解耦发挥优势。在 `Provider` 层实现"格式修复拦截器"（Format Fixer），尝试通过正则自动纠正轻微的 JSON 语法错误，降低上层容错压力。
