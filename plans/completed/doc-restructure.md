# 文档重构：拆分与索引

> 创建: 2026-02-06
> 状态: 进行中

## 目标

将 readme.md 的内容拆分成结构化的多文档体系，放入 docs 目录，并创建索引文件。

## 步骤

- [ ] 分析 readme.md 结构，确定拆分方案
- [ ] 创建文档目录结构
- [ ] 拆分核心架构文档
- [ ] 拆分 Go 实现方案文档
- [ ] 创建总索引文件
- [ ] 完善和细化各模块内容

## 完成标准

- [ ] docs 目录结构清晰
- [ ] 每个功能点独立成文件
- [ ] 有清晰的索引文件
- [ ] 内容完整且格式规范

## 文档拆分方案

### 目录结构
```
docs/
├── README.md                    # 总索引
├── 01-overview/                 # 概述
│   ├── executive-summary.md     # 执行摘要
│   └── introduction.md          # 引言
├── 02-architecture/             # 架构设计
│   ├── gateway.md               # 网关设计
│   ├── concurrency-model.md     # 泳道并发模型
│   ├── chat-adapters.md         # 聊天软件适配器
│   └── llm-orchestration.md     # LLM 编排
├── 03-core-features/            # 核心功能
│   ├── autonomous-runtime.md    # 自主运行机制
│   ├── heartbeat.md             # 心跳机制
│   ├── memory-system.md         # 记忆系统
│   └── security.md              # 安全架构
├── 04-go-implementation/        # Go 实现
│   ├── architecture.md          # 系统架构
│   ├── gateway-impl.md          # 网关实现
│   ├── chat-adapters-impl.md    # 聊天适配器实现
│   ├── memory-impl.md           # 记忆系统实现
│   ├── heartbeat-impl.md        # 心跳实现
│   └── security-impl.md         # 安全模块实现
└── 05-roadmap/                  # 路线图
    └── implementation-plan.md   # 实施计划
```
