# Glaude

用 Go 语言实现的 AI Coding Agent，参照 Claude Code 架构设计。单二进制、零依赖、毫秒级启动。

## 特性

- **Agentic Loop** — LLM 自主决策工具调用，循环执行直到任务完成
- **8 种内置工具** — Bash、文件读写编辑、Glob、Grep、LS、子 Agent
- **多 Provider** — Anthropic / OpenAI / Ollama，统一接口 + 流式输出
- **权限治理** — 4 种安全模式 + 30+ 条危险命令检测规则
- **上下文压缩** — Token 预算管理 + 自动摘要，支持长对话
- **会话持久化** — JSONL append-only，支持中断恢复和会话切换
- **MCP 协议** — 连接外部 MCP 工具服务器，动态注册远端工具
- **生命周期 Hook** — 在工具执行前后运行自定义脚本（格式化、静态分析等）
- **记忆系统** — 四层指令文件（企业/用户/项目/本地）+ 文件快照回滚
- **终端 UI** — 基于 Bubble Tea 的交互界面，Markdown 渲染 + diff 展示

## 快速开始

### 安装

```bash
# 从源码构建
go build -o glaude ./cmd/glaude

# 或使用 Makefile
make build
```

### 配置 API Key

```bash
# Anthropic（默认）
export ANTHROPIC_API_KEY="sk-ant-..."

# OpenAI
export OPENAI_API_KEY="sk-..."

# Ollama（本地，无需 key）
# 默认连接 http://localhost:11434/v1
```

### 运行

```bash
# 交互模式（REPL）
./glaude

# 单次执行
./glaude -p "用 Go 写一个 HTTP server"

# 恢复上次会话
./glaude --continue

# 恢复指定会话
./glaude --resume <session-id>
```

### 项目初始化

```bash
# 交互式生成 .glaude.json 配置文件
./glaude init
```

## 使用指南

### CLI 参数

| 参数 | 短写 | 说明 |
|------|------|------|
| `--prompt` | `-p` | 运行单条 prompt 后退出 |
| `--continue` | `-c` | 恢复最近一次会话 |
| `--resume <id>` | | 恢复指定 session ID 的会话 |

子命令：

| 命令 | 说明 |
|------|------|
| `glaude init` | 交互式初始化项目配置 |
| `glaude version` | 打印版本信息 |

### REPL 斜杠命令

在交互模式下，输入以下命令：

| 命令 | 说明 |
|------|------|
| `/help` | 显示可用命令 |
| `/exit`, `/quit` | 退出 |
| `/clear` | 清空对话历史 |
| `/undo` | 撤销最近一次文件变更 |
| `/context` | 显示上下文信息（消息数、token 用量、undo 栈） |
| `/mode [name]` | 查看或切换权限模式 |

### 内置工具

Agent 在对话中自主调用这些工具：

| 工具 | 说明 | 只读 |
|------|------|------|
| `Bash` | 执行 shell 命令，持久化 shell 状态（cd、环境变量跨调用保留） | 否 |
| `Read` | 读取文件内容，支持行号分页，自动截断长行 | 是 |
| `Edit` | 精准字符串替换（str_replace），替换前自动快照 | 否 |
| `Write` | 创建或覆盖文件，自动创建父目录 | 否 |
| `Glob` | 文件模式匹配（`**/*.go`），按修改时间排序，上限 100 条 | 是 |
| `Grep` | 正则搜索文件内容，优先使用 ripgrep，支持多种输出模式 | 是 |
| `LS` | 列出目录内容（名称、类型、大小） | 是 |
| `Agent` | 派生子 Agent 处理独立子任务，仅返回结论 | 是 |

## 配置

### 配置文件层级

优先级从高到低：

| 层级 | 来源 | 路径 |
|------|------|------|
| 1 | 环境变量 | `GLAUDE_*` 前缀（如 `GLAUDE_MODEL=gpt-4o`） |
| 2 | 项目配置 | `.glaude.json`（当前目录） |
| 3 | 全局配置 | `~/.glaude/settings.json` |
| 4 | 默认值 | 内置 |

### 配置项

| 键 | 默认值 | 说明 |
|----|--------|------|
| `provider` | `"anthropic"` | LLM 提供方：`anthropic` / `openai` / `ollama` |
| `model` | `"claude-sonnet-4-20250514"` | 模型名称 |
| `api_key` | | API 密钥（也可通过环境变量设置） |
| `base_url` | | API 端点（也可通过环境变量设置） |
| `permission_mode` | `"default"` | 权限模式（见下文） |
| `log_dir` | `~/.glaude/logs` | 日志目录 |
| `log_level` | `"info"` | 日志级别 |

### 配置文件示例

```json
{
  "provider": "openai",
  "model": "gpt-4o",
  "api_key": "sk-xxx",
  "base_url": "https://api.openai.com/v1",
  "permission_mode": "auto-edit",
  "mcp_servers": [
    {
      "name": "github",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": ["GITHUB_TOKEN=ghp_xxx"]
    }
  ],
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          { "type": "command", "command": "python3 validate.py", "timeout": 5000 }
        ]
      }
    ]
  }
}
```

## 权限模式

4 种安全模式控制 Agent 的行为边界：

| 模式 | 配置值 | 行为 |
|------|--------|------|
| **Default** | `"default"` | 每个修改操作都需要用户确认 |
| **Auto-Edit** | `"auto-edit"` | 文件编辑自动批准，Bash 仍需确认 |
| **Plan-Only** | `"plan-only"` | 所有修改被拒绝，仅允许只读工具 |
| **Auto-Full** | `"auto-full"` | 所有操作自动批准（请谨慎使用） |

无论哪种模式，内置的危险命令扫描器都会拦截高危操作（`rm -rf`、`sudo`、`git push --force` 等）并要求用户确认。

## 指令文件（GLAUDE.md）

Glaude 从四个层级加载指令文件，为 Agent 提供项目上下文和行为约束：

| 层级 | 路径 | 说明 |
|------|------|------|
| 企业 | `/etc/glaude/GLAUDE.md` | 组织统一规范 |
| 用户 | `~/.glaude/GLAUDE.md` | 个人全局偏好 |
| 项目 | `<project>/GLAUDE.md` | 项目编码规范、架构说明 |
| 本地 | `<project>/GLAUDE.local.md` | 个人项目配置（应 gitignore） |

每个层级还支持 `.glaude/rules/*.md` 规则目录。文件内支持 `@./path` 语法引用其他文件。

## 生命周期 Hook

Hook 允许在 Agent 执行的关键节点运行外部脚本：

| 事件 | 触发时机 | 能力 |
|------|----------|------|
| `PreToolUse` | 工具执行前 | 批准/拒绝/修改输入 |
| `PostToolUse` | 工具执行后 | 读取结果、记录日志 |
| `SessionStart` | 会话开始（仅一次） | 环境初始化 |
| `Stop` | Agent 即将结束回合 | 清理、校验 |

### Hook 通信协议

- 通过 `sh -c <command>` 执行，输入以 JSON 写入 stdin
- **退出码**：`0` = 成功，`1` = 非阻塞错误，`2` = 阻塞（阻止工具执行）
- stdout 输出 JSON 可控制行为：

```json
{
  "decision": "deny",
  "message": "此操作被安全策略禁止",
  "updatedInput": { "command": "echo safe" },
  "continue": false
}
```

- 非 JSON 输出视为纯文本消息
- 多个 Hook 匹配时：deny > allow（最严格优先），`updatedInput` 最后一个生效

### Matcher 模式

| 模式 | 示例 | 说明 |
|------|------|------|
| `*` 或空 | `"*"` | 匹配所有工具 |
| 精确匹配 | `"Bash"` | 仅匹配 Bash 工具 |
| 管道分隔 | `"Write\|Edit"` | 匹配 Write 或 Edit |
| Glob | `"File*"` | 匹配 FileRead、FileWrite 等 |

### 示例：自动格式化

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Write|Edit",
        "hooks": [
          { "type": "command", "command": "gofmt -w $(cat /dev/stdin | jq -r '.tool_input.file_path')" }
        ]
      }
    ]
  }
}
```

## MCP 集成

通过 [Model Context Protocol](https://modelcontextprotocol.io/) 连接外部工具服务器：

```json
{
  "mcp_servers": [
    {
      "name": "github",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": ["GITHUB_TOKEN=ghp_xxx"]
    }
  ]
}
```

MCP 工具自动注册到 Agent，命名格式为 `mcp__{server}__{tool}`。支持 Stdio 传输协议，JSON-RPC 2.0 通信。

## 会话持久化

所有对话自动保存为 JSONL 文件：

```
~/.glaude/projects/{sanitized-cwd}/{session-id}.jsonl
```

- `--continue` — 恢复最近的会话
- `--resume <id>` — 恢复指定会话
- 基于 UUID DAG 链的消息关联，支持分支和恢复

## 多 Provider 支持

| Provider | 环境变量 | 特性 |
|----------|----------|------|
| **Anthropic** | `ANTHROPIC_API_KEY` | 官方 SDK，流式输出 |
| **OpenAI** | `OPENAI_API_KEY`，可选 `OPENAI_BASE_URL` | 官方 SDK，兼容 API |
| **Ollama** | 可选 `OLLAMA_BASE_URL`（默认 `localhost:11434`） | 本地模型，复用 OpenAI 兼容接口 |

所有 Provider 支持：
- 流式输出（实时文本 + 工具调用碎片缓冲）
- 指数退避重试（429 / 5xx，最多 5 次）
- 弱模型 JSON 修复（`DialectFixer` 自动修复尾逗号、未闭合括号等）

## 项目结构

```
glaude/
├── cmd/glaude/          # CLI 入口（main.go, init.go）
├── internal/
│   ├── agent/           # Agentic Loop 状态机
│   ├── llm/             # Provider 抽象层
│   ├── tool/            # 工具接口 + Registry
│   │   ├── bash/        #   持久化 Shell
│   │   ├── fileedit/    #   精准字符串替换
│   │   ├── fileread/    #   文件读取
│   │   ├── filewrite/   #   文件写入
│   │   ├── glob/        #   模式匹配
│   │   ├── grep/        #   内容搜索
│   │   ├── ls/          #   目录列表
│   │   └── subagent/    #   子 Agent
│   ├── hook/            # 生命周期 Hook 引擎
│   ├── permission/      # 权限矩阵 + 危险扫描
│   ├── compact/         # 上下文预算 + 压缩
│   ├── session/         # 会话持久化（JSONL）
│   ├── memory/          # 指令文件 + Checkpoint
│   ├── mcp/             # MCP 客户端
│   ├── prompt/          # System Prompt 组装
│   ├── config/          # 分层配置
│   ├── ui/              # 终端 UI（Bubble Tea）
│   └── telemetry/       # 结构化日志
├── docs/                # 设计文档 + 参考分析
├── Makefile             # 构建、测试、交叉编译
└── .github/workflows/   # CI/CD
```

## 构建

```bash
# 构建当前平台
make build

# 运行测试（含 race detector）
make test

# 代码检查
make lint

# 交叉编译（linux/darwin × amd64/arm64）
make cross
# 输出到 dist/ 目录
```

## 技术选型

| 职责 | 选型 |
|------|------|
| TUI 框架 | [bubbletea](https://github.com/charmbracelet/bubbletea)（Elm MVU 架构） |
| Markdown 渲染 | [glamour](https://github.com/charmbracelet/glamour) |
| CLI / 配置 | [cobra](https://github.com/spf13/cobra) + [viper](https://github.com/spf13/viper) |
| 日志 | [logrus](https://github.com/sirupsen/logrus) + [lumberjack](https://github.com/natefinch/lumberjack) |
| LLM SDK | [anthropic-sdk-go](https://github.com/anthropics/anthropic-sdk-go) / [openai-go](https://github.com/openai/openai-go) |
| Token 计数 | [tiktoken-go](https://github.com/pkoukk/tiktoken-go) |
| Diff 对比 | [go-diff](https://github.com/sergi/go-diff) |

## 设计文档

- [docs/design.md](docs/design.md) — Glaude 架构设计
- [docs/plan.md](docs/plan.md) — 实施计划（Phase 0-11）
- [docs/reference/](docs/reference/) — Claude Code 源码级架构分析（17 篇），来自 [claude-reviews-claude](https://github.com/openedclaude/claude-reviews-claude)

## 许可证

[Apache License 2.0](LICENSE)
