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

### 2.1 项目概况

**代码规模**：54 个源文件 + 36 个测试文件，分布在 13 个 `internal/` 子包中。

**运行模式**：
* **One-shot 模式** (`glaude -p "..."`)：执行单条 prompt 后退出，适合脚本集成。
* **REPL 模式** (默认)：启动终端 UI，支持多轮对话、流式输出、权限交互、会话恢复。
* **会话恢复**：`--continue` 恢复最近会话，`--resume <id>` 恢复指定会话。

### 2.2 模块划分原则
按**职责边界**而非技术分层划分模块。每个模块回答一个问题：

| 模块           | 回答的问题                     |
|:--------------|:--------------------------|
| `agent`       | 如何驱动 LLM 循环执行任务？        |
| `llm`         | 如何与不同的 LLM 提供商通信？       |
| `tool`        | 如何定义、注册、执行工具？           |
| `compact`     | 如何管理有限的上下文窗口？           |
| `memory`      | 如何在会话间保持知识和文件快照？        |
| `session`     | 如何持久化和恢复对话历史？           |
| `permission`  | 如何对工具调用实施分层安全检查？        |
| `hook`        | 如何在生命周期事件上触发外部脚本？       |
| `prompt`      | 如何动态组装 System Prompt？    |
| `config`      | 如何管理分层配置？               |
| `mcp`         | 如何连接外部 MCP 服务？          |
| `ui`          | 如何与用户交互？                |
| `telemetry`   | 如何记录内部状态和追踪成本？          |

### 2.3 核心接口设计
整个系统围绕五个核心接口构建：

* **Provider** (`llm.Provider`)：抽象 LLM 通信。统一消息模型，屏蔽 Anthropic/OpenAI/Ollama API 差异。
* **StreamingProvider** (`llm.StreamingProvider`)：扩展 Provider，支持 SSE 流式事件通道。
* **Tool** (`tool.Tool`)：抽象工具能力。暴露 `Name()`、`Description()`、`InputSchema()`、`IsReadOnly()`、`Execute(ctx, input)`。
* **Store** (`memory.Store`)：抽象记忆的读写策略。初期实现为本地 Markdown，预留向量数据库接口。
* **Transport** (`mcp.Transport`)：抽象 MCP 传输。支持 stdio（本地子进程），上层代码不关心底层通信方式。

### 2.4 数据流

```text
用户输入 "fix the bug in auth.go"
    │
    ▼
┌──────────────────────────────────────────────────────────────┐
│ Agent Loop (agent.RunStream)                                  │
│                                                               │
│  1. 组装消息列表                                               │
│     [system prompt + GLAUDE.md 指令 + 环境信息]               │
│     [历史消息...]                                              │
│     [user: "fix the bug in auth.go"]                          │
│                                                               │
│  2. MicroCompact（每轮运行）                                   │
│     清理旧 tool_result（保留最近 5 条）                         │
│     截断超 100KB 的工具输出                                    │
│                                                               │
│  3. 上下文预算检查                                              │
│     budget.NeedsCompact()?                                    │
│     是 → AutoCompact：LLM 摘要压缩（含熔断器）                │
│                                                               │
│  4. 流式调用 LLM                                               │
│     provider.CompleteStream(ctx, req) ────────────────────────┼──→ LLM API
│     ← StreamEvent 通道                                        │
│     text_delta → 实时回调 UI 渲染                              │
│     tool_use 碎片 → 缓冲至 content_block_stop                 │
│                                                               │
│  5. 解析响应                                                    │
│     stop_reason == "tool_use"?                                │
│     是 → 提取 tool_use blocks                                 │
│                                                               │
│  6. Hook: PreToolUse                                          │
│     hook.Engine.Dispatch(PreToolUse) → 外部脚本可否决/修改      │
│                                                               │
│  7. 权限检查                                                    │
│     permission.Gate.Evaluate()                                │
│     ├── 会话规则匹配（deny > ask > allow）                     │
│     ├── 只读工具 → 放行                                        │
│     ├── Bash 命令 → 危险特征扫描（30+ 正则）                   │
│     └── 需确认 → 暂停，UI 弹窗 ──────────────────────────────┼──→ 用户 y/n
│                                                               │
│  8. 执行工具 (ctx 透传)                                         │
│     tool.Execute(ctx, input)                                  │
│     ├── Read → 记录 FileState → 返回内容                      │
│     ├── Edit → 检查 Staleness → Checkpoint.Save → 替换        │
│     ├── Write → 检查 Staleness → Checkpoint.Save → 写入       │
│     ├── Bash → 持久 Shell → UUID Sentinel 截断输出             │
│     ├── Glob → doublestar 匹配 → .gitignore 过滤              │
│     ├── Grep → ripgrep/grep → 结果上限 250 条                 │
│     ├── LS → 列出目录内容                                      │
│     └── Agent → 递归创建隔离子 Agent（半预算、20 轮上限）       │
│                                                               │
│  9. Hook: PostToolUse                                         │
│     hook.Engine.Dispatch(PostToolUse)                         │
│                                                               │
│ 10. 会话持久化                                                  │
│     session.Store.Append(entry) → JSONL 追加写                │
│                                                               │
│ 11. 回传结果                                                    │
│     messages.append(tool_result)                              │
│     → 回到步骤 2，继续循环                                     │
│                                                               │
│ 12. 循环终止                                                    │
│     stop_reason == "end_turn" → 输出给用户                     │
└──────────────────────────────────────────────────────────────┘
```

### 2.5 级联取消与生命周期管理 (Context Cancellation)
Glaude 强依赖 Go 的 `context.Context` 进行整个 Agent Loop 的生命周期管理：
* **信号处理**：主入口捕获 `SIGINT`/`SIGTERM`，第一次信号触发 context cancel 优雅关闭，第二次信号强制退出。
* **LLM 请求中断**：用户按下 `Ctrl+C` 时，触发 Context Cancel，`llm.Provider` 立即中止 HTTP 请求和流式输出。
* **工具执行中断**：Context 透传给 `Tool.Execute(ctx)`。`BashTool` 捕获 cancel 信号后向底层进程组发送 `SIGTERM`，防止孤儿进程。
* **优雅退出**：系统保证 Session Store 和 Checkpoint 被安全落盘后再退出。

### 2.6 Agent 测试分层策略
纯大模型调用的集成测试不仅慢且昂贵。Glaude 采用严格的分层测试（36 个测试文件）：
* **工具层 (Unit Test)**：最核心的保障。Table-driven tests 覆盖 `BashTool` 的各种场景、`FileEditTool` 的字符串定位边界、`Scanner` 的 30+ 种危险模式识别、`Rules` 的模式匹配、`Checkpoint` 的栈操作。
* **Provider 抽象层 (Mock Test)**：`MockProvider` 和 `MockStreamEvents` 通过预置剧本模拟 LLM 返回 `tool_use` 或 `end_turn`，验证 Agent Loop 状态机、上下文压缩触发、权限拦截、流式事件分拣。
* **集成测试**：Hook 引擎的端到端测试（外部脚本执行→JSON stdin/stdout→聚合决策）、MCP 协议测试、Session 的 DAG 链重建和恢复测试。

---

## 三、各模块实现原理

### 3.1 Agent Loop（`internal/agent/`）

Agent Loop 是系统的心脏。`Agent` 结构体持有 provider、model、system prompt、消息列表、token 预算、auto compactor、权限门、会话存储和 hook 引擎。

**两种执行模式**：
* `Run(ctx, prompt)` — 同步模式：整轮完成后返回。
* `RunStream(ctx, prompt, callback)` — 流式模式：`text_delta` 实时回调 UI 渲染；`tool_use` 碎片缓冲至 `content_block_stop` 后批量执行。

**循环内部流程**：每轮迭代先运行 MicroCompact 清理旧数据，检查是否需要 AutoCompact 摘要压缩，然后调用 provider。工具执行时按序运行 PreToolUse hook → 权限检查 → `tool.Execute` → PostToolUse hook，并将每条消息追加到 Session Store。

**会话恢复**：`RestoreFrom(entries)` 加载 JSONL 中保存的对话，支持 `--continue` 和 `--resume` 两种恢复方式。

### 3.2 LLM 抽象层（`internal/llm/`）

**统一类型系统**（`types.go`）：`Message`、`ContentBlock`（text/tool_use/tool_result）、`Request`、`Response`、`StreamEvent`（6 种事件类型）。所有 API 差异在此层消化，不泄漏到上层。

**三个后端实现**：
* `AnthropicProvider` — 使用官方 `anthropic-sdk-go`，支持同步和流式。
* `OpenAIProvider` — 使用官方 `openai-go`，支持同步和流式，可配置自定义 base URL。
* `OllamaProvider` — 复用 OpenAI 兼容接口，指向 `localhost:11434/v1`。

**两个装饰器**：
* `DialectFixer` — 修复开源模型的畸形响应：移除 BOM、修正尾逗号、闭合未关闭的括号。`SafeParseJSON` 实现多阶段 JSON 修复。
* `RetryProvider` — 指数退避 + 抖动重试。对 429/529/5xx/连接错误自动重试，最多 5 次，基础延迟 1s，最大 30s。

**工厂函数**（`factory.go`）：`NewProvider(name, model)` 按配置创建后端，非 Anthropic 的自动套 DialectFixer，所有后端统一套 RetryProvider。

### 3.3 工具系统（`internal/tool/`）

**核心接口** (`tool.go`)：

```go
type Tool interface {
    Name() string
    Description() string
    InputSchema() map[string]any
    IsReadOnly() bool
    Execute(ctx context.Context, input map[string]any) (string, error)
}
```

**Registry** (`registry.go`)：按名称索引的工具注册表，`Definitions()` 生成 LLM 可消费的 `[]llm.ToolDefinition`。

**FileStateCache** (`filestate.go`)：记录文件读取时的内容哈希和 mtime，用于 Edit/Write 前的 staleness 检查，防止覆盖未读取的文件。

**8 个内置工具**：

| 工具 | 包 | 只读 | 核心特性 |
|:-----|:---|:----:|:--------|
| Read | `tool/fileread` | ✓ | offset/limit 分段读取，记录 FileState |
| Edit | `tool/fileedit` | ✗ | str_replace 精确替换（old_str 必须唯一），支持 replace_all；Staleness 检查 → Checkpoint.Save → 替换 |
| Write | `tool/filewrite` | ✗ | 自动创建父目录；已有文件需 Staleness 检查 → Checkpoint.Save |
| Bash | `tool/bash` | ✗ | 持久 `bash --norc` 子进程，UUID Sentinel 截断输出，100KB 上限，默认 120s 超时（最大 10min） |
| Glob | `tool/glob` | ✓ | `doublestar` 递归匹配，mtime 倒序排列，100 条上限，排除 `.git`/`node_modules`/`vendor` |
| Grep | `tool/grep` | ✓ | 优先使用 ripgrep，降级为 grep；支持 content/files_with_matches/count 三种输出模式，250 条上限 |
| LS | `tool/ls` | ✓ | 非递归目录列表，显示名称/类型/大小 |
| Agent | `tool/subagent` | ✓ | 派生隔离子 Agent：半预算、20 轮上限、无 AutoCompact、从 Registry 中移除自身防止递归 |

### 3.4 上下文预算与压缩（`internal/compact/`）

**TokenCounter** (`budget.go`)：基于 `tiktoken-go`（cl100k_base 编码）估算 token 数，降级为 `len(text)/4`。支持对消息列表和工具定义的批量计数。

**Budget** (`budget.go`)：刚性预算模型，跟踪上下文窗口各部分的分配：

| 分区 | 说明 |
|:-----|:-----|
| ContextWindow | 默认 200,000 tokens |
| SystemPrompt | System Prompt 占用 |
| Tools | 工具定义占用 |
| Messages | 历史消息占用 |
| Reserved | 响应预留 16,000 tokens |

* `NeedsCompact()` — 当可用空间 < 13,000 tokens 时触发。
* `NeedsWarning()` — 当可用空间 < 20,000 tokens 时警告。
* `CalibrateFromAPI()` — 用首次 API 返回的真实 token 数校准估算偏差。
* `FormatBudgetBar()` — 生成文本进度条，显示在 UI 状态栏。

**MicroCompact** (`microcompact.go`)：每轮迭代运行，零 API 成本：
* 清除旧 tool_result 内容（保留最近 5 条）。
* 截断超过 100KB 的单条工具输出。
* 纯函数，返回新切片，不修改输入。

**AutoCompact** (`autocompact.go`)：Budget 报警时触发，调用 LLM 摘要压缩：
* 保留最近约 30% 的消息不动，对更早的消息生成结构化摘要。
* 正确处理 tool_use/tool_result 配对边界，避免切断消息对。
* 内置熔断器：连续 3 次失败后停止尝试，防止无限消耗。

### 3.5 记忆系统（`internal/memory/`）

**Store 接口**：`Load(projectRoot) (string, error)` / `Save(projectRoot, content) error`，解耦读写策略与存储介质。

**FileStore**（`filestore.go`）：从四层目录加载 GLAUDE.md 指令文件：

| 层级 | 路径 | 优先级 |
|:-----|:-----|:------:|
| 企业管控 | `/etc/glaude/` | 最高 |
| 用户全局 | `~/.glaude/GLAUDE.md` | 高 |
| 项目级 | `<root>/GLAUDE.md`、`<root>/.glaude/GLAUDE.md`、`<root>/.glaude/rules/*.md` | 中 |
| 项目本地 | `<root>/GLAUDE.local.md`（gitignored） | 低 |

* 支持 `@path` include 指令（递归加载，最大深度 5，环引用检测）。
* 自动剥离 HTML 注释。

**Checkpoint**（`checkpoint.go`）：栈式文件快照系统：
* `Transaction` — 一组 `Snapshot{Path, Content, Mode}`，对应一次逻辑操作。
* `Save(txID, path)` — 写操作前压栈文件旧内容。
* `Undo()` — 弹出栈顶事务，按逆序恢复所有文件（新建的文件则删除）。
* 并发安全（`sync.Mutex` 保护）。

### 3.6 会话持久化（`internal/session/`）

**JSONL 追加写**（`store.go`）：每条 `Entry` 包含 Type（user/assistant/summary/title/last-prompt/tag）、UUID、ParentUUID（DAG 链接）、SessionID、CWD、Timestamp、Message。文件路径：`~/.glaude/projects/{sanitized_cwd}/{sessionID}.jsonl`。懒创建文件，10MB 单行上限保护。

**DAG 链重建**（`chain.go`）：`BuildChain(entries)` 从叶节点（无子节点的 Entry）沿 ParentUUID 回溯到根节点，重建线性对话序列。支持 Fork 分支和环检测。`ToMessages(chain)` 将链转换为 `[]llm.Message` 用于 Agent 恢复。

**会话列表**（`list.go`）：扫描 session 目录，通过读取头尾各 64KB 快速提取元数据（标题、最后 prompt、时间戳），无需完整加载。`MostRecentSession(cwd)` 支持 `--continue` 快速恢复。

### 3.7 权限系统（`internal/permission/`）

**四种安全模式**（`mode.go`）：

| 模式 | 行为 |
|:-----|:-----|
| `Default` | 所有写操作需用户确认 |
| `AutoEdit` | 文件编辑自动放行，Bash 仍需确认 |
| `PlanOnly` | 拒绝所有写操作 |
| `AutoFull` | 所有操作自动放行 |

**规则引擎**（`rules.go`）：支持三种匹配模式——精确匹配、前缀通配（`cmd:*`）、通配符（`git * --force`）。规则按 deny > ask > allow 优先级求值。命令归一化处理：allow 规则保守剥离（仅安全包装器），deny/ask 规则激进剥离（所有环境变量）。支持复合命令拆分（`&&`、`||`、`;`、`|`）。

**危险特征扫描器**（`scanner.go`）：30+ 条正则模式，覆盖 6 类威胁：

| 类别 | 示例模式 |
|:-----|:---------|
| 破坏性操作 | `rm -rf`、`mkfs`、`dd` |
| Shell 配置篡改 | `.bashrc`、`.zshrc` 修改 |
| 权限提升 | `sudo`、`chmod +s` |
| 网络渗出 | `curl POST`、`nc -l` |
| 管道注入 | `\| sh`、`eval` |
| Git 敏感操作 | `force push`、`hard reset` |

**权限门**（`gate.go`）：组合 Checker + DangerScanner + PromptFunc，形成完整的评估流水线：模式检查 → 规则匹配 → Bash 命令扫描 → 需要确认时调用 UI 弹窗。

### 3.8 生命周期 Hook（`internal/hook/`）

**四种事件**：`PreToolUse`、`PostToolUse`、`SessionStart`、`Stop`。

**配置结构**：`HookConfig` = `map[Event][]HookGroup`，每个 `HookGroup` 绑定一个工具名模式（支持 `*`、精确匹配、pipe 分隔、glob）到一组 `HookEntry{Type, Command, Timeout}`。

**执行协议**：Hook 以 `sh -c <command>` 执行，通过 stdin 接收 JSON 格式的 `HookInput`（含工具名、输入参数、session ID），通过 stdout 返回 `HookOutput`。退出码协议：0=成功、1=非阻塞错误、2=阻塞错误。默认 10 秒超时。

**聚合策略**（`engine.go`）：多个 Hook 的结果合并时，deny 优先于 allow；最后一个 updatedInput 生效；任一 Hook 可设置 stopSession 终止会话。

### 3.9 System Prompt 组装（`internal/prompt/`）

`Builder` 按固定顺序拼接四个段落：

1. **Identity** — "You are an AI coding agent..."
2. **Rules** — 工具使用指南、安全规范。
3. **Custom Instructions** — 从 `memory.FileStore` 加载的 GLAUDE.md 指令。
4. **Environment** — 自动检测 OS、Shell、CWD、Git 分支。

### 3.10 终端 UI（`internal/ui/`）

**MVU 架构**（`model.go`）：基于 `bubbletea` 的 Model-View-Update 单向数据流。Model 持有 agent、checkpoint、textarea、spinner、glamour renderer、消息历史、流式状态和权限弹窗状态。

**消息类型**：
* 用户输入 — `KeyMsg`（Ctrl+D 退出、Ctrl+C 取消/退出、Enter 发送、Alt+Enter 换行）
* 窗口调整 — `WindowSizeMsg`
* 流式事件 — `streamTextMsg`（实时文本）、`streamToolStartMsg`（工具开始）、`streamDoneMsg`（流结束）
* Agent 结果 — `agentDoneMsg`
* 权限请求 — `permissionRequestMsg`

**流式渲染**：通过 `programRef` 共享指针实现 bubbletea 值拷贝安全的 `p.Send()`，text_delta 实时追加到 View 并显示闪烁光标。

**状态栏**：底部实时显示权限模式、消息数量、上下文使用百分比、Token 计数。

**斜杠命令**（`commands.go`）：`/exit`、`/quit`、`/clear`、`/undo`、`/context`、`/mode [mode]`、`/help`。

**权限桥接**（`permission.go`）：`WirePermissionGate` 将 Agent 的权限检查请求通过 `p.Send(permissionRequestMsg)` 转发到 UI，用户通过 channel 回复 y/n。

**Diff 渲染**（`diff.go`）：基于 `go-diff` 的 Myers 算法，输出带颜色的统一 diff 格式。

### 3.11 MCP 集成（`internal/mcp/`）

**JSON-RPC 2.0 协议**（`protocol.go`）：完整的线格式定义——Request、Response、RPCError、Notification，以及 MCP 特有的握手和工具发现结构体。

**Stdio 传输**（`transport.go`）：管理 MCP 服务器子进程的 stdin/stdout 通信，维护 pending request/response channel map，后台读循环分发响应。

**客户端**（`client.go`）：管理连接生命周期——`Initialize`（协议握手）→ `ListTools`（发现工具）→ `CallTool`（调用工具）→ `Close`。工具名命名空间化为 `mcp__{serverName}__{toolName}`。

**Registry 集成**（`tool.go`、`config.go`）：`MCPTool` 适配 MCP 工具到标准 `tool.Tool` 接口。`Manager` 管理多个 MCP 服务器连接。`LoadFromConfig` 从 viper 读取 `mcp_servers` 配置，自动连接并注册到全局 Registry。

### 3.12 配置管理（`internal/config/`）

四层配置合并（优先级从高到低）：

```
环境变量 (GLAUDE_ 前缀) > 项目 .glaude.json > 全局 ~/.glaude/settings.json > 默认值
```

默认值：`provider=anthropic`、`model=claude-sonnet-4-20250514`、`permission_mode=default`。

### 3.13 可观测性（`internal/telemetry/`）

**幽灵日志 (Ghost Logging)**：基于 `logrus` + `lumberjack` 的结构化 JSONL 日志，写入 `~/.glaude/logs/glaude.jsonl`。日志轮转策略：50MB 单文件上限、3 个备份、7 天保留期。默认丢弃（测试安全），`Init()` 后激活。绝不干扰终端 UI。

---

## 四、分阶段实现路线

> 详细的设计细节、参考文档和必读源码清单见 [`docs/plan.md`](./plan.md)。

| 阶段 | 目标与里程碑 | 状态 |
| :--- | :--- | :---: |
| **Phase 0** | **项目基石**。初始化目录结构、cobra/viper CLI、logrus 幽灵日志、SIGINT/SIGTERM 级联取消树。 | ✅ |
| **Phase 1** | **生命周期与查询引擎**。通用 Message/ContentBlock 模型、Provider 接口、AnthropicProvider、MockProvider、基础 Agent 状态机。 | ✅ |
| **Phase 2** | **核心工具与持久化沙箱**。Tool 接口与 Registry、FileReadTool、FileEditTool、持久化 BashTool（UUID Sentinel）。 | ✅ |
| **Phase 3** | **工具集扩展与提示词工程**。GlobTool、GrepTool、FileWriteTool、LSTool、System Prompt 动态组装管线。 | ✅ |
| **Phase 4** | **记忆系统与快照回滚**。MemoryStore 接口、四层 GLAUDE.md 加载、Checkpoint 栈式快照与 Undo。 | ✅ |
| **Phase 5** | **终端 UI 架构层**。bubbletea MVU 状态机、glamour Markdown 渲染、go-diff 补丁渲染、斜杠命令。 | ✅ |
| **Phase 6** | **上下文预算与压缩调度**。tiktoken-go 本地估算、MicroCompact 降级、AutoCompact LLM 摘要（含熔断器）。 | ✅ |
| **Phase 7** | **安全边界与权限矩阵**。四层安全模式、Bash 危险特征正则扫描（30+ 模式）、UI 阻断拦截弹窗。 | ✅ |
| **Phase 8** | **多智能体分层与 MCP 桥接**。AgentTool 隔离子 Agent、MCP Stdio 传输与协议、动态工具注册。 | ✅ |
| **Phase 9** | **会话路由与模型适配层**。OpenAI/Ollama Provider、DialectFixer 装饰器、JSONL DAG 会话持久化、--continue/--resume。 | ✅ |
| **Phase 10** | **流式状态机与 UI 联动**。StreamingProvider 接口、SSE 流解析引擎、流式事件分拣器、异步 UI 更新机制。 | ✅ |
| **Phase 11** | **自动化钩子与交付**。生命周期 Hook 点、`/init` 交互式引导、CI/CD 多架构交叉编译。 | 🚧 |

---

## 五、关键技术选型

| 职责 | 选型 | 选择理由 |
| :--- | :--- | :--- |
| TUI 框架 | `bubbletea` + `bubbles` | Go 生态事实标准 TUI 框架，Elm 架构适合并发状态机 |
| Markdown 渲染 | `glamour` | 终端 Markdown 渲染，与 bubbletea 生态无缝集成 |
| 终端样式 | `lipgloss` | 声明式终端样式，定义颜色、边框、布局 |
| CLI / 配置 | `cobra` + `viper` | Kubernetes/Docker 同款，支持多种配置文件和环境变量 |
| 日志遥测 | `logrus` + `lumberjack` | 结构化 JSON 日志 + 自动轮转，适合 Ghost Logging |
| LLM SDK | `anthropic-sdk-go` / `openai-go` | 官方维护，类型安全，SSE 流式支持 |
| Token 计数 | `tiktoken-go` | cl100k_base 编码，精度高 |
| Glob 搜索 | `doublestar` | 支持 `**` 递归匹配，规避标准库限制 |
| Diff 对比 | `go-diff` | Myers diff 算法，稳定可靠 |
| UUID 生成 | `google/uuid` | Session ID、Transaction ID、Sentinel 标记 |
| 测试断言 | `testify` | assert/require 断言，提升测试可读性 |

---

## 六、风险与应对策略

1.  **Context 泄漏与僵尸进程**
    * *风险*：用户频繁中断可能导致后台 Bash 进程残留。
    * *应对*：严格的 `context.WithCancel` 树 + `SysProcAttr` 设置进程组 ID（PGID），`kill -负PID` 清理整棵进程树。BashTool 在 cancel 时主动发送 SIGTERM。
2.  **大仓库工具回填导致 OOM**
    * *风险*：`GrepTool` 或 `BashTool` 返回巨量输出，撑爆内存。
    * *应对*：工具层硬性截断（Bash 100KB、Grep 250 条、Glob 100 条），MicroCompact 进一步清理超 100KB 的 tool_result。
3.  **不同模型的工具调用幻觉**
    * *风险*：开源模型（如 Ollama）的 tool_calls JSON 格式不稳定。
    * *应对*：`DialectFixer` 装饰器在 Provider 层实施多阶段 JSON 修复（BOM 移除→尾逗号修正→括号闭合），`SafeParseJSON` 确保上层拿到合法结构。
4.  **上下文压缩失败导致死循环**
    * *风险*：AutoCompact 反复失败，Token 预算耗尽后无法继续对话。
    * *应对*：`AutoCompactor` 内置熔断器，连续 3 次失败后停止压缩尝试，改为依赖 MicroCompact 维持基本运转。
5.  **会话文件损坏**
    * *风险*：进程异常退出时 JSONL 文件可能写入不完整。
    * *应对*：append-only 写入模式，每条 Entry 独立一行。`LoadEntries` 在解析失败时跳过损坏行，最大化恢复可读数据。
