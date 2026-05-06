# Agent Gateway - 项目进度

## Phase 1: 核心框架

| 任务 | 状态 | 备注 |
|------|------|------|
| 项目结构初始化 | ✅ 完成 | Go module, 目录结构, Makefile |
| 设计文档 | ✅ 完成 | doc/design.md |
| 统一数据模型 | ✅ 完成 | internal/model/ (7 文件) |
| Provider 接口定义 | ✅ 完成 | internal/provider/provider.go |
| Gateway 核心逻辑 | ✅ 完成 | internal/gateway/ (gateway.go + registry.go) |
| HTTP/WS/SSE Server | ✅ 完成 | internal/server/server.go |
| 认证中间件 | ⬜ 待开始 | 内置 CORS，JWT/API Key 待实现 |
| SQLite Store | ✅ 完成 | internal/store/ (store.go + util.go) |
| 配置管理 | ✅ 完成 | internal/config/config.go + gateway.yaml |
| 程序入口 | ✅ 完成 | cmd/gateway/main.go |
| ID 生成工具 | ✅ 完成 | internal/util/id.go |
| 项目技能文件 | ✅ 完成 | .trae/rules/project_rules.md |

## Phase 2: Provider 适配器

| 任务 | 状态 | 备注 |
|------|------|------|
| LangGraph Provider | ✅ 完成 | HTTP/SSE 完整实现 |
| Dify Provider | 🔄 占位 | stub 实现，待完整实现 |
| OpenClaw Provider | 🔄 占位 | stub 实现，待完整实现 |
| Hermes Provider | 🔄 占位 | stub 实现，待完整实现 |
| Generic A2A Provider | 🔄 占位 | stub 实现，待完整实现 |

## Phase 3: 协议兼容层

| 任务 | 状态 | 备注 |
|------|------|------|
| A2A handler | ⬜ 待开始 | internal/protocol/a2a/ |
| ACP handler | ⬜ 待开始 | internal/protocol/acp/ |
| MCP handler | ⬜ 待开始 | internal/protocol/mcp/ |
| OpenAI 兼容 API | ⬜ 待开始 | /v1/chat/completions, /v1/responses |

## Phase 4: 高级特性

| 任务 | 状态 | 备注 |
|------|------|------|
| 跨 Agent 路由与编排 | ⬜ 待开始 | |
| Approval 统一 | ⬜ 待开始 | |
| MCP 工具桥接 | ⬜ 待开始 | |
| Dashboard API | ⬜ 待开始 | |
| 可观测性 | ⬜ 待开始 | |

## 待办事项
- [ ] 安装 Go 1.23+ 并运行 `cd src && go mod tidy` 拉取依赖
- [ ] 实现认证中间件 (JWT + API Key)
- [ ] 完善 Dify Provider
- [ ] 完善 OpenClaw Provider
- [ ] 完善 Hermes Provider
- [ ] 实现 A2A 协议 handler
- [ ] 实现 MCP 协议 handler
- [ ] 编写测试代码到 test/
