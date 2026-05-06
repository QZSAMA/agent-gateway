# Agent Gateway - 设计方案

## 1. 项目定位

Agent Gateway 是一个通用 AI Agent 网关，提供以下核心能力：

1. **统一接入**：通过一套 API 接入所有主流 Agent 平台
2. **协议翻译**：原生兼容 A2A / ACP / MCP / OpenAI 四大协议
3. **跨平台协作**：不同平台的 Agent 可以互相发现和协作
4. **可插拔架构**：每个 Agent 平台是一个 Provider，按需启用

## 2. 支持的 Agent 平台

| 平台 | 传输层 | 消息格式 | 会话模型 |
|------|--------|---------|---------|
| OpenClaw | WebSocket + HTTP | JSON RPC v3 | Workspace + Session |
| Hermes Agent | stdio + HTTP | ACP NDJSON | SQLite Session |
| LangGraph | HTTP + SSE | Agent Protocol | Thread + Run |
| Dify | HTTP + SSE | REST API | Conversation |
| 通用 A2A | HTTP + SSE | JSON-RPC 2.0 | Task |
| 通用 MCP | HTTP/stdio | JSON-RPC 2.0 | 无状态 |

## 3. 核心架构

```
Client Layer (API/SDK/Dashboard)
         │
  Unified Gateway API (REST + SSE + WS)
         │
  ┌──────┼──────────┬──────────────┐
  │      │          │              │
Protocol  Session   Routing &    Auth &
Translation Mgmt   Orchestration  Middleware
  │      │          │
  └──────┼──────────┼──────────────┐
         │          │              │
  ┌──────▼──┐ ┌────▼─────┐ ┌─────▼──────┐
  │ A2A     │ │ ACP      │ │ MCP        │ │ OpenAI  │
  │ Adapter │ │ Adapter  │ │ Adapter    │ │ Compat  │
  └──────┬──┘ └────┬─────┘ └─────┬──────┘ └────┬────┘
         │         │             │              │
  ┌──────▼─────────▼─────────────▼──────────────▼────┐
  │            Agent Provider Layer                   │
  │  OpenClaw | Hermes | LangGraph | Dify | Generic  │
  └──────────────────────────────────────────────────┘
```

## 4. 统一数据模型

### Agent
- id, name, description, provider
- capabilities (streaming, toolUse, humanInTheLoop, etc.)
- skills, authSchemes, metadata

### Session
- id, agentId, provider, status
- createdAt, metadata

### Message
- id, sessionId, role (user/agent/system)
- content: ContentBlock[] (text/image/audio/file/data)
- timestamp, metadata

### Task (A2A 兼容)
- id, sessionId, agentId, status
- input/output: Message
- artifacts: Artifact[]

### StreamEvent
- message_chunk / thought_chunk / tool_call / tool_call_update
- task_status / artifact / approval_request / error

### Tool (MCP 兼容)
- name, description, inputSchema, outputSchema

### Approval
- id, sessionId, agentId, actionType
- title, description, riskLevel

## 5. Provider Adapter 接口

```go
type AgentProviderAdapter interface {
    Name() string
    Initialize(config ProviderConfig) error
    Shutdown() error
    HealthCheck(ctx context.Context) error

    ListAgents(ctx context.Context) ([]AgentDescriptor, error)
    GetAgent(ctx context.Context, agentID string) (*AgentDescriptor, error)

    CreateSession(ctx context.Context, agentID string, opts *SessionOptions) (*Session, error)
    GetSession(ctx context.Context, sessionID string) (*Session, error)
    ListSessions(ctx context.Context, agentID string) ([]Session, error)
    CloseSession(ctx context.Context, sessionID string) error

    SendMessage(ctx context.Context, sessionID string, msg *Message, opts *SendOptions) (*Message, error)
    StreamMessage(ctx context.Context, sessionID string, msg *Message, opts *SendOptions) (<-chan StreamEvent, error)

    Cancel(ctx context.Context, sessionID string) error

    ListTools(ctx context.Context, agentID string) ([]ToolDefinition, error)
    InvokeTool(ctx context.Context, agentID string, toolName string, input map[string]any) (any, error)

    RespondApproval(ctx context.Context, approvalID string, resp *ApprovalResponse) error
    ResumeTask(ctx context.Context, sessionID string, resumeData any) (*Message, error)

    GetHistory(ctx context.Context, sessionID string) ([]Message, error)
}
```

## 6. API 设计

### REST API
- GET/POST /v1/agents - Agent 发现
- POST/GET/DELETE /v1/sessions - 会话管理
- POST /v1/sessions/{id}/messages - 阻塞消息
- POST /v1/sessions/{id}/stream - SSE 流式消息
- POST /v1/tasks/send - A2A 兼容任务发送
- POST /v1/tasks/sendSubscribe - A2A 流式任务
- GET /v1/agents/{id}/tools - 工具列表
- POST /v1/agents/{id}/tools/invoke - 工具调用
- GET/POST /v1/approvals - 人机协作

### 协议端点
- POST /a2a - A2A JSON-RPC
- POST /acp - ACP JSON-RPC (HTTP)
- POST /mcp - MCP Streamable HTTP
- POST /v1/chat/completions - OpenAI 兼容
- POST /v1/responses - OpenAI Responses 兼容

## 7. 技术选型

| 组件 | 选型 |
|------|------|
| 语言 | Go 1.23+ |
| HTTP | chi |
| WebSocket | gorilla/websocket |
| 配置 | Viper + YAML |
| 日志 | zerolog |
| 存储 | SQLite (default) + PostgreSQL (optional) |
| 认证 | JWT + API Key |

## 8. 开发路线图

### Phase 1: 核心框架
- 统一数据模型
- Gateway 核心结构
- Provider 接口
- Session Manager / Agent Registry
- HTTP + WS + SSE Server
- 认证中间件
- SQLite Store
- 配置管理

### Phase 2: Provider 适配器
- LangGraph Provider
- Dify Provider
- OpenClaw Provider
- Hermes Provider
- Generic A2A Provider

### Phase 3: 协议兼容层
- A2A handler
- ACP handler
- MCP handler
- OpenAI 兼容 API

### Phase 4: 高级特性
- 跨 Agent 路由与编排
- Approval 统一
- MCP 工具桥接
- Dashboard API
- 可观测性
