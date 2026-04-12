# Glaude

用 Go 语言实现的 AI Coding Agent，参照 Claude Code 架构设计。目标：单二进制、零依赖、毫秒级启动。

## 项目概览

Glaude 不是 Claude Code 的逐行翻译，而是以 Go 惯用方式重新实现其核心架构思想：Agentic Loop、分层工具系统、上下文压缩、权限治理、多智能体协作和记忆持久化。

### 核心架构

| 模块 | 职责 |
|------|------|
| `agent` | 驱动 LLM 循环执行任务（Agentic Loop） |
| `llm` | 统一多 Provider 通信（Anthropic/OpenAI/Ollama） |
| `tool` | 工具定义、注册、执行与权限声明 |
| `context` | 三层上下文压缩（MicroCompact → AutoCompact → FullCompact） |
| `memory` | 跨会话记忆（GLAUDE.md、Checkpoint、MemoryStore） |
| `config` | 用户配置与分层权限规则 |
| `mcp` | Model Context Protocol 客户端（stdio / SSE） |
| `ui` | 基于 Bubble Tea 的终端交互界面 |
| `telemetry` | Ghost Logging、Token 计量与成本追踪 |

### 技术选型

| 职责 | 选型 |
|------|------|
| TUI 框架 | `bubbletea`（Elm 架构） |
| CLI / 配置 | `cobra` + `viper` |
| 日志 | `go.uber.org/zap` |
| LLM SDK | `anthropic-sdk-go` / `go-openai` |
| Token 计数 | `tiktoken-go` |
| Glob 搜索 | `doublestar` |
| Diff 对比 | `go-diff` |

## 设计文档

`docs/reference/` 目录收录了来自 [claude-reviews-claude](https://github.com/openedclaude/claude-reviews-claude) 项目的源码级架构分析（含 Claude Code 源码链接），作为 Glaude 实现的参考蓝本：

| 文档 | 主题 | 关联阶段 |
|------|------|----------|
| [00-overview.md](docs/reference/00-overview.md) | 完整架构总览（17 篇浓缩版） | 全阶段通用 |
| [01-query-engine.md](docs/reference/01-query-engine.md) | 查询引擎：核心循环与流式处理 | Phase 1 |
| [02-tool-system.md](docs/reference/02-tool-system.md) | 工具系统：42 模块统一接口 | Phase 2/3 |
| [03-coordinator.md](docs/reference/03-coordinator.md) | 多智能体协调器 | Phase 8 |
| [04-plugin-system.md](docs/reference/04-plugin-system.md) | 插件系统：全生命周期管理 | Phase 8 |
| [05-hook-system.md](docs/reference/05-hook-system.md) | Hook 系统：20 种事件类型 | Phase 11 |
| [06-bash-engine.md](docs/reference/06-bash-engine.md) | Bash 执行引擎：沙箱与管道 | Phase 2/7 |
| [07-permission-pipeline.md](docs/reference/07-permission-pipeline.md) | 权限流水线：纵深防御 | Phase 7 |
| [08-agent-swarms.md](docs/reference/08-agent-swarms.md) | Agent 集群：团队协调 | Phase 8 |
| [09-session-persistence.md](docs/reference/09-session-persistence.md) | 会话持久化 | Phase 4/9 |
| [10-context-assembly.md](docs/reference/10-context-assembly.md) | 上下文装配 | Phase 3/6 |
| [11-compact-system.md](docs/reference/11-compact-system.md) | 压缩系统：多层架构 | Phase 6 |
| [12-startup-bootstrap.md](docs/reference/12-startup-bootstrap.md) | 启动与引导 | Phase 0/1/10/11 |
| [13-bridge-system.md](docs/reference/13-bridge-system.md) | 桥接系统：远程控制协议 | Phase 8 |
| [14-ui-state.md](docs/reference/14-ui-state.md) | UI 与状态管理 | Phase 5/10 |
| [15-services-api-layer.md](docs/reference/15-services-api-layer.md) | 服务层与 API 架构 | Phase 1/9/10 |
| [16-infrastructure-config.md](docs/reference/16-infrastructure-config.md) | 基础设施与配置 | Phase 0/4 |
| [17-telemetry-privacy-operations.md](docs/reference/17-telemetry-privacy-operations.md) | 遥测、隐私与运营 | Phase 0 |

Glaude 自身的架构设计详见 [docs/design.md](docs/design.md)，实施计划详见 [docs/plan.md](docs/plan.md)。

## 开发顺序

严格按阶段推进：

1. **Phase 0** — 项目骨架，`go build` 通过
2. **Phase 1** — 纯文本对话，`glaude -p "hello"` 跑通
3. **Phase 2** — 核心工具（Bash/FileRead/FileEdit）+ Context Cancel
4. **Phase 3** — 扩展工具（FileWrite/Glob/Grep/LS）
5. **Phase 4** — 记忆系统（GLAUDE.md 加载、Checkpoint、MicroCompact）
6. **Phase 5** — 终端 UI（Bubble Tea 交互界面）
7. **Phase 6** — 权限系统（命令白名单、安全拦截）
8. **Phase 7** — 子 Agent & MCP
9. **Phase 8** — 多 Provider、Session 持久化
10. **Phase 9** — 发布打磨、E2E 测试、文档

## 快速开始

```bash
# 构建
go build -o glaude ./cmd/glaude

# 运行
./glaude -p "hello"
```

## 参考资料

- **[claude-reviews-claude](https://github.com/openedclaude/claude-reviews-claude)** — Claude Code 深度架构解析系列（17 篇源码分析，中英双语），是 Glaude 参考分析文档的上游来源。

## 许可证

MIT
