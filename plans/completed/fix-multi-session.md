# 修复多会话系统

> 创建: 2026-02-07
> 状态: 已完成

## 目标

修复多会话系统的 7 个 Bug，确保快捷键在 macOS 终端正常工作，会话切换时输入框内容持久化，流式响应绑定到发起会话。

## Bug 清单

1. [x] BUG-1 (CRITICAL): switchSession 不保存/恢复 textarea 内容 → 添加 draftInput 字段
2. [x] BUG-2 (CRITICAL): 流式响应绑定到 currentSession 而非发起会话 → 添加 sessionID 绑定
3. [x] BUG-3 (CRITICAL): Alt+number 在 macOS 终端不生效 → 添加 Ctrl+] 替代方案
4. [x] BUG-4 (SIGNIFICANT): 无备用快捷键 → Ctrl+] 循环切换
5. [x] BUG-5 (SIGNIFICANT): 流式响应中切换会话可能损坏数据 → 按 sessionID 隔离
6. [x] BUG-6 (MINOR): 会话 ID 关闭后新建可能重复 → 全局递增 nextSessionID
7. [x] BUG-7 (MINOR): 关闭会话跳到右边而非左边 → 优先切到左边

## 变更文件

- `session.go`: 添加 draftInput 字段
- `app.go`: nextSessionID, findSession, sessionID 绑定, switchSession 保存/恢复, closeSession 优先左切
- `keys.go`: Ctrl+] 循环切换会话
- `commands.go`: 更新帮助文本
- `view.go`: 更新设置面板快捷键显示
- `app_test.go`: 新增 3 个测试（TextareaPersistence, SessionIDUniqueness, Ctrl+] 切换）

## 测试

- 19/19 通过（含新增 3 个）
