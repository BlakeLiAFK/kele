# 定时任务系统

> 创建: 2026-02-07
> 状态: 进行中

## 目标

实现全局定时任务系统，支持标准 cron 语法，持久化到 SQLite，通过 tool call 让 AI 增删改查。

## 架构设计

```
internal/cron/
  parser.go      -- cron 表达式解析器（5字段标准格式 + 快捷方式）
  scheduler.go   -- 调度器 + SQLite 持久化 + CRUD 方法
```

### 数据模型
- cron_jobs 表：id, name, schedule, command, enabled, last_run, next_run, last_result, last_error
- cron_logs 表：job_id, run_at, output, error, duration_ms

### Tool 定义（5个）
- cron_create(name, schedule, command) — 创建任务
- cron_list() — 列出所有任务
- cron_get(id) — 查看详情+最近日志
- cron_update(id, name?, schedule?, command?, enabled?) — 更新任务
- cron_delete(id) — 删除任务

### 调度机制
- 分钟级 ticker，对齐到分钟边界
- 每次 tick 加载所有启用任务，匹配 cron 表达式
- 命令执行带 5 分钟超时 + 危险命令过滤
- 执行日志保留最近 50 条

## 步骤

- [ ] 1. 创建 cron 表达式解析器 (parser.go)
- [ ] 2. 创建调度器 + 持久化 (scheduler.go)
- [ ] 3. 修改 tools/executor.go 注册 5 个 cron 工具
- [ ] 4. 修改 agent/brain.go 传递 scheduler + 更新 system prompt
- [ ] 5. 修改 tui/session.go 传递 scheduler
- [ ] 6. 修改 tui/app.go 创建 scheduler 生命周期
- [ ] 7. 修改 tui/commands.go 添加 /cron 命令
- [ ] 8. 编写测试
- [ ] 9. 全量测试 + 提交

## 完成标准

- [ ] cron 表达式解析正确（含 */N, N-M, N,M 等语法）
- [ ] 定时任务持久化到 SQLite
- [ ] AI 可通过 tool call 完成增删改查
- [ ] 调度器分钟级自动触发
- [ ] 所有测试通过
