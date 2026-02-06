# GoClaw 文档中心

OpenClaw AI 智能体架构深度解析与 Go 语言复刻实施指南

## 📚 文档导航

### 1. 概述

- [执行摘要](01-overview/executive-summary.md) - 项目背景与价值主张
- [引言：从对话机器人到数字劳动力](01-overview/introduction.md) - AI 范式转变与 OpenClaw 定位

### 2. 架构设计

- [网关架构](02-architecture/gateway.md) - 单一事实来源的控制平面
- [泳道并发模型](02-architecture/concurrency-model.md) - 高并发会话隔离机制
- [聊天软件适配器](02-architecture/chat-adapters.md) - 多平台接入实现
- [LLM 编排系统](02-architecture/llm-orchestration.md) - 多模型调度与路由

### 3. 核心功能

- [自主运行机制](03-core-features/autonomous-runtime.md) - 守护进程与持久化
- [心跳系统](03-core-features/heartbeat.md) - 主动监控与决策
- [记忆系统](03-core-features/memory-system.md) - 混合检索与上下文管理
- [安全架构](03-core-features/security.md) - 配对机制与沙箱

### 4. Go 语言实现

- [系统架构概览](04-go-implementation/architecture.md) - GoClaw 分层设计
- [网关实现](04-go-implementation/gateway-impl.md) - 事件总线与泳道调度
- [聊天适配器实现](04-go-implementation/chat-adapters-impl.md) - WhatsApp/Telegram/Discord 集成
- [记忆系统实现](04-go-implementation/memory-impl.md) - SQLite 混合搜索
- [心跳实现](04-go-implementation/heartbeat-impl.md) - Ticker 与主动性
- [安全模块实现](04-go-implementation/security-impl.md) - 配对中间件

### 5. 实施路线图

- [实施计划](05-roadmap/implementation-plan.md) - 分阶段开发指南

## 🎯 快速开始

如果你是：

- **产品经理/架构师**：先读 [执行摘要](01-overview/executive-summary.md) 和 [引言](01-overview/introduction.md)
- **后端工程师**：重点关注 [架构设计](02-architecture/) 和 [Go 实现](04-go-implementation/)
- **开发者**：直接跳到 [实施计划](05-roadmap/implementation-plan.md) 开始编码

## 📖 阅读顺序建议

1. **理解概念**：01-overview → 02-architecture → 03-core-features
2. **实战开发**：04-go-implementation → 05-roadmap
3. **深入专题**：根据兴趣点选择性阅读

## 🔗 外部资源

- [OpenClaw 官方仓库](https://github.com/openclaw)
- [Go 并发模式](https://go.dev/blog/pipelines)
- [whatsmeow 文档](https://github.com/tulir/whatsmeow)
- [SQLite FTS5](https://www.sqlite.org/fts5.html)

---

*最后更新：2026-02-06*
