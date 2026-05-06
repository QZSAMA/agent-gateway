# Agent Gateway

通用 AI Agent 网关 — 一套 API 接入所有主流 Agent 平台。

## 特性

- **统一接入** — 通过一套 REST/SSE/WebSocket API 接入所有 Agent 平台
- **协议兼容** — 原生支持 A2A、ACP、MCP、OpenAI 四大协议
- **可插拔架构** — 每个 Agent 平台是一个 Provider，按需启用
- **跨平台协作** — 不同平台的 Agent 可以互相发现和协作
- **流式支持** — SSE 和 WebSocket 双模式流式输出

## 支持的 Agent 平台

| 平台 | 传输层 | 会话模型 | 状态 |
|------|--------|---------|------|
| LangGraph | HTTP + SSE | Thread + Run | ✅ 已实现 |
| Dify | HTTP + SSE | Conversation | 🔄 开发中 |
| OpenClaw | WebSocket + HTTP | Workspace + Session | 🔄 开发中 |
| Hermes | stdio + HTTP | Session | 🔄 开发中 |
| 通用 A2A | HTTP + SSE | Task | 🔄 开发中 |

## 架构

```
                          Client Layer
                        (API / SDK / Dashboard)
                               │
                    Unified Gateway API
                     (REST + SSE + WebSocket)
                               │
              ┌────────────────┼────────────────┐
              │                │                │
       Protocol           Session          Routing &
      Translation        Management      Orchestration
              │                │                │
     ┌────────┼────────────────┼────────────────┼──────┐
     │        │                │                │      │
  A2A       ACP             MCP           OpenAI    Custom
  Adapter  Adapter        Adapter       Compat    Adapter
     │        │                │                │      │
     └────────┴────────────────┴────────────────┴──────┘
                          │
               Agent Provider Layer
     ┌──────────┬──────────┬──────────┬──────────┐
     │ LangGraph│   Dify   │ OpenClaw │  Hermes   │
     │ Provider │ Provider │ Provider │ Provider  │
     └──────────┴──────────┴──────────┴──────────┘
```

## 前置要求

- Go 1.23+
- SQLite3 (CGO 依赖，需安装 C 编译器)
- 至少一个 Agent 平台的运行实例 (如 LangGraph Server)

## 快速开始

### 1. 安装依赖

```bash
cd src
go mod tidy
```

### 2. 配置

编辑 `src/configs/gateway.yaml`，启用你要使用的 Agent 平台：

```yaml
providers:
  langgraph:
    enabled: true
    endpoint: "http://localhost:2024"
    auth:
      api_key: "your-langgraph-api-key"
    options:
      default_assistant: "agent"
```

### 3. 启动

```bash
# 使用 Makefile
make run

# 或直接运行
cd src && go run ./cmd/gateway/

# 指定配置文件
cd src && go run ./cmd/gateway/ -config /path/to/gateway.yaml
```

网关默认监听 `http://0.0.0.0:8080`。

### 4. 验证

```bash
curl http://localhost:8080/v1/health
```

## API 使用

### Agent 发现

```bash
# 列出所有已注册的 Agent
curl http://localhost:8080/v1/agents

# 获取单个 Agent 详情
curl http://localhost:8080/v1/agents/langgraph:agent

# 获取 A2A Agent Card
curl http://localhost:8080/v1/agents/langgraph:agent/card
```

### 创建会话

```bash
curl -X POST http://localhost:8080/v1/sessions \
  -H "Content-Type: application/json" \
  -d '{
    "agentId": "langgraph:agent",
    "options": {
      "metadata": {}
    }
  }'
```

响应：

```json
{
  "id": "thread_abc123",
  "agentId": "langgraph:agent",
  "provider": "langgraph",
  "status": "active",
  "createdAt": "2026-05-06T12:00:00Z",
  "updatedAt": "2026-05-06T12:00:00Z"
}
```

### 发送消息 (阻塞)

```bash
curl -X POST http://localhost:8080/v1/sessions/{sessionId}/messages \
  -H "Content-Type: application/json" \
  -d '{
    "role": "user",
    "content": [{"type": "text", "text": "Hello, what can you do?"}]
  }'
```

### 发送消息 (SSE 流式)

```bash
curl -X POST http://localhost:8080/v1/sessions/{sessionId}/stream \
  -H "Content-Type: application/json" \
  -d '{
    "role": "user",
    "content": [{"type": "text", "text": "Hello, what can you do?"}]
  }'
```

SSE 事件流：

```
event: message_chunk
data: {"type":"message_chunk","sessionId":"...","role":"agent","content":{"type":"text","text":"I can help"}}

event: message_chunk
data: {"type":"message_chunk","sessionId":"...","role":"agent","content":{"type":"text","text":" you with..."}}

event: tool_call
data: {"type":"tool_call","sessionId":"...","toolCallId":"tc1","toolName":"search","toolKind":"search","toolInput":{}}

event: task_status
data: {"type":"task_status","sessionId":"...","taskStatus":"completed"}

event: done
data: {}
```

### A2A 兼容任务发送

```bash
# 阻塞模式
curl -X POST http://localhost:8080/v1/tasks/send \
  -H "Content-Type: application/json" \
  -d '{
    "agentId": "langgraph:agent",
    "message": {
      "role": "user",
      "content": [{"type": "text", "text": "Analyze this data"}]
    }
  }'

# 流式模式
curl -X POST http://localhost:8080/v1/tasks/sendSubscribe \
  -H "Content-Type: application/json" \
  -d '{
    "agentId": "langgraph:agent",
    "message": {
      "role": "user",
      "content": [{"type": "text", "text": "Analyze this data"}]
    }
  }'
```

### WebSocket

连接 `ws://localhost:8080/ws`，发送 JSON 消息：

```json
// 创建会话
{ "type": "req", "id": "1", "method": "session.create", "params": { "agentId": "langgraph:agent" } }

// 发送消息 (自动流式返回)
{ "type": "req", "id": "2", "method": "session.send", "params": {
  "sessionId": "thread_abc123",
  "message": { "role": "user", "content": [{"type": "text", "text": "Hello"}] }
}}

// 响应审批
{ "type": "req", "id": "3", "method": "approval.respond", "params": {
  "approvalId": "appr_xxx",
  "decision": "approved"
}}
```

接收事件：

```json
{ "type": "event", "event": "stream.message_chunk", "payload": {...}, "timestamp": "..." }
{ "type": "event", "event": "stream.tool_call", "payload": {...}, "timestamp": "..." }
{ "type": "res", "id": "2", "ok": true, "payload": { "status": "completed" } }
```

### 会话管理

```bash
# 列出会话
curl http://localhost:8080/v1/sessions

# 按 Agent 过滤
curl http://localhost:8080/v1/sessions?agentId=langgraph:agent

# 获取会话详情
curl http://localhost:8080/v1/sessions/{sessionId}

# 关闭会话
curl -X DELETE http://localhost:8080/v1/sessions/{sessionId}

# 获取历史消息
curl http://localhost:8080/v1/sessions/{sessionId}/history

# 取消进行中的任务
curl -X POST http://localhost:8080/v1/sessions/{sessionId}/cancel
```

### 工具调用

```bash
# 列出 Agent 可用工具
curl http://localhost:8080/v1/agents/langgraph:agent/tools

# 调用工具
curl -X POST http://localhost:8080/v1/agents/langgraph:agent/tools/invoke \
  -H "Content-Type: application/json" \
  -d '{
    "toolName": "search",
    "input": {"query": "golang generics"}
  }'
```

### 人机协作

```bash
# 查看待审批
curl http://localhost:8080/v1/approvals/pending

# 响应审批
curl -X POST http://localhost:8080/v1/approvals/{approvalId}/respond \
  -H "Content-Type: application/json" \
  -d '{ "decision": "approved" }'

# 恢复中断的任务
curl -X POST http://localhost:8080/v1/sessions/{sessionId}/resume \
  -H "Content-Type: application/json" \
  -d '{ "resumeData": { "approved": true } }'
```

## API 端点总览

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v1/health` | 健康检查 |
| GET | `/v1/agents` | 列出所有 Agent |
| GET | `/v1/agents/{agentId}` | 获取 Agent 详情 |
| GET | `/v1/agents/{agentId}/card` | 获取 A2A Agent Card |
| GET | `/v1/agents/{agentId}/tools` | 列出可用工具 |
| POST | `/v1/agents/{agentId}/tools/invoke` | 调用工具 |
| POST | `/v1/sessions` | 创建会话 |
| GET | `/v1/sessions` | 列出会话 |
| GET | `/v1/sessions/{sessionId}` | 获取会话 |
| DELETE | `/v1/sessions/{sessionId}` | 关闭会话 |
| POST | `/v1/sessions/{sessionId}/messages` | 发送消息 (阻塞) |
| POST | `/v1/sessions/{sessionId}/stream` | 发送消息 (SSE 流式) |
| GET | `/v1/sessions/{sessionId}/history` | 获取历史消息 |
| POST | `/v1/sessions/{sessionId}/cancel` | 取消任务 |
| POST | `/v1/sessions/{sessionId}/resume` | 恢复中断任务 |
| POST | `/v1/tasks/send` | A2A 兼容任务发送 |
| POST | `/v1/tasks/sendSubscribe` | A2A 兼容任务发送 (流式) |
| GET | `/v1/tasks/{taskId}` | 获取任务状态 |
| POST | `/v1/tasks/{taskId}/cancel` | 取消任务 |
| GET | `/v1/approvals/pending` | 查看待审批 |
| POST | `/v1/approvals/{approvalId}/respond` | 响应审批 |
| GET | `/ws` | WebSocket 连接 |

## 配置

配置文件位于 `src/configs/gateway.yaml`，主要配置项：

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  websocket:
    enabled: true
    path: "/ws"
  cors:
    allowed_origins: ["*"]

auth:
  jwt:
    secret: "change-me-in-production"
    expiry: "24h"
  api_keys: []

store:
  type: "sqlite"
  sqlite:
    path: "./data/gateway.db"

providers:
  langgraph:
    enabled: true
    endpoint: "http://localhost:2024"
    auth:
      api_key: ""
    options:
      default_assistant: "agent"

  dify:
    enabled: true
    endpoint: "http://localhost/v1"
    auth:
      api_key: ""
    options:
      default_app_type: "chat"

logging:
  level: "info"
  format: "json"

health:
  check_interval: "30s"
  unhealthy_threshold: 3
```

### Agent ID 格式

Agent ID 使用 `{provider}:{local_id}` 格式，例如：

- `langgraph:agent` — LangGraph 平台的 agent 助手
- `dify:chat-app-xxx` — Dify 平台的聊天应用
- `openclaw:main` — OpenClaw 平台的主 Agent

## 开发

```bash
# 构建
make build

# 运行
make run

# 测试
make test

# 详细测试
make test-verbose

# 代码检查
make lint

# 格式化
make fmt

# 清理
make clean

# 更新依赖
make tidy
```

### 项目结构

```
agent-gateway/
├── doc/                        # 设计文档、进度跟踪
│   ├── design.md
│   └── progress.md
├── test/                       # 测试代码
├── src/
│   ├── cmd/gateway/main.go     # 程序入口
│   ├── configs/gateway.yaml    # 默认配置
│   └── internal/
│       ├── config/             # 配置管理
│       ├── model/              # 统一数据模型
│       ├── provider/           # Provider 适配器
│       │   ├── provider.go     # 接口定义
│       │   ├── langgraph/      # LangGraph 实现
│       │   ├── dify/           # Dify 实现
│       │   ├── openclaw/       # OpenClaw 实现
│       │   ├── hermes/         # Hermes 实现
│       │   └── generic/        # 通用 A2A 实现
│       ├── gateway/            # 网关核心逻辑
│       ├── server/             # HTTP/WS/SSE 服务
│       ├── store/              # 持久化层
│       └── util/               # 工具函数
├── Makefile
└── README.md
```

### 添加新的 Provider

1. 在 `src/internal/provider/` 下创建新目录
2. 实现 `AgentProviderAdapter` 接口 (定义在 `provider/provider.go`)
3. 在 `cmd/gateway/main.go` 的 `createProvider` 函数中注册
4. 在 `gateway.yaml` 中添加配置项

## License

MIT
