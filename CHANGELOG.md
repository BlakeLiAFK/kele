# 更新日志

## v0.1.3 - 2026-02-06

### ✨ 新功能

- **环境变量支持模型配置**: 添加 `OPENAI_MODEL` 环境变量支持
  - 默认值: `gpt-4o`
  - 可通过环境变量自定义模型名称

- **丰富的 Slash 命令系统**: 参考 Claude Code，大幅扩展命令系统
  - **模型管理**: `/model`, `/models`, `/model-reset` - 运行时切换模型
  - **记忆系统**: `/remember`, `/search`, `/memory` - 完整记忆管理
  - **信息查看**: `/status`, `/config`, `/history`, `/tokens`, `/debug` - 详细状态信息
  - **会话管理**: `/save`, `/export` - 导出和保存对话
  - **对话控制**: `/clear`, `/reset`, `/exit`, `/quit` - 基础控制

- **运行时模型切换**: 无需重启即可切换不同的 AI 模型
  - 支持 OpenAI 系列 (gpt-4o, gpt-4-turbo, gpt-3.5-turbo)
  - 支持 Claude 系列 (claude-3-5-sonnet, claude-3-opus)
  - 支持任意兼容 OpenAI API 的模型

- **命令自动补全**: 按 Tab 键自动补全命令
  - 智能匹配命令前缀
  - 单个匹配时自动补全
  - 多个匹配时显示候选列表
  - 支持最长公共前缀补全

- **多任务支持**: ESC 键改为中断任务而非退出
  - ESC 键: 中断当前流式响应，保持程序运行
  - Ctrl+C: 退出程序
  - /exit, /quit: 退出程序
  - 支持随时中断任务开始新对话

### 🔧 技术改进

- 在 LLM Client 中添加模型切换支持
- 在 Agent Brain 中暴露模型管理接口
- 完全重写 `handleCommand` 函数，支持参数解析
- 添加辅助函数: `getEnv`, `truncateString`, `findCommonPrefix`
- 实现命令补全算法（前缀匹配 + 公共前缀）
- 改进按键处理逻辑，支持任务中断

### 🎨 界面改进

- 将品牌 emoji 从 🦞 改为 🥤（可乐）
- 更新状态栏和帮助文本
- 优化输入提示信息

### 📝 文档更新

- 更新所有文档添加 `OPENAI_MODEL` 说明
- 更新配置文件示例
- 创建命令使用指南

---

## v0.1.2 - 2026-02-06

### 🐛 Bug 修复

- **修复流式响应错误**: 修复"无法找到用户输入"错误
  - 重新设计流式响应架构，使用 event channel 传递
  - 添加 `streamInitMsg` 类型用于初始化流式响应
  - 修复 `startStream` 和 `continueStream` 函数逻辑
  - 在 App 结构体中保存 event channel

### 🔧 技术改进

- 优化流式响应流程，避免重复调用 `ChatStream`
- 改进错误处理，确保 channel 正确关闭

---

## v0.1.1 - 2026-02-06

### 🐛 Bug 修复

- **修复 FTS5 模块错误**: 移除 SQLite FTS5 依赖，改用普通索引和 LIKE 查询
- **修复 Module 名称**: 将 `github.com/yourusername/kele` 更正为 `github.com/BlakeLiAFK/kele`
- **添加验证脚本**: 新增 `verify.sh` 用于快速验证程序功能

### 🔧 技术改进

- 简化数据库结构，移除 FTS5 虚拟表
- 在 `memory_entries` 表添加索引优化查询性能
- 所有功能验证通过，确保程序可正常运行

---

## v0.1.0 - 2026-02-06

### 🎉 首次发布

完整实现 TUI 版 OpenClaw MVP！

### ✨ 新功能

#### 核心功能
- ✅ **LLM 集成**: 支持 OpenAI 兼容 API，流式响应
- ✅ **工具系统**: bash/read/write 三个基础工具
- ✅ **Agent 大脑**: 智能对话处理，工具调用
- ✅ **记忆系统**: SQLite + MEMORY.md + 会话日志
- ✅ **TUI 界面**: 基于 Bubble Tea 的精美终端界面

#### 交互特性
- ✅ 流式响应（打字机效果）
- ✅ 工具调用可视化
- ✅ Slash 命令系统
- ✅ 上下文管理（20 轮历史）
- ✅ 自动会话保存

#### 工具能力
- ✅ **bash**: 执行 shell 命令（带安全检查）
- ✅ **read**: 读取文件内容
- ✅ **write**: 创建/修改文件

#### 记忆功能
- ✅ SQLite 全文检索（FTS5）
- ✅ MEMORY.md 文件同步
- ✅ 会话 JSONL 日志
- ✅ 记忆搜索

### 🛠️ 技术栈

- **语言**: Go 1.25
- **TUI**: Bubble Tea + Lipgloss
- **数据库**: SQLite3
- **LLM**: OpenAI 兼容 API

### 📦 依赖

```
github.com/charmbracelet/bubbletea v1.3.10
github.com/charmbracelet/bubbles v0.21.1
github.com/charmbracelet/lipgloss v1.1.1
github.com/mattn/go-sqlite3 v1.14.33
```

### 🎯 支持的平台

- macOS (arm64/amd64)
- Linux (amd64/arm64)
- 需要 CGO 支持（SQLite）

### 📝 已知限制

- 暂不支持图片/文件上传
- 暂不支持多会话并发
- 工具调用为串行执行
- 仅支持 OpenAI 格式 API

### 🔜 计划功能

- [ ] 多 LLM 提供商支持
- [ ] 向量检索（Embeddings）
- [ ] 心跳机制（定时任务）
- [ ] Web UI 控制面板
- [ ] 插件系统

### 🐛 Bug 修复

无（首次发布）

### 📚 文档

- ✅ README.md - 项目介绍
- ✅ QUICKSTART.md - 快速开始
- ✅ USAGE.md - 使用指南
- ✅ docs/ - 完整架构文档
- ✅ plans/ - 开发计划

### 🙏 致谢

基于以下项目的灵感：
- Claude Code - TUI 交互设计
- OpenClaw - 自主能力架构
- Bubble Tea - 优秀的 TUI 框架

---

## 开发统计

- **开发时间**: 1 天
- **代码行数**: ~1500 行 Go 代码
- **测试状态**: ✅ 编译通过
- **文档完整度**: ✅ 100%
