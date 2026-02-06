# Kele 快速开始指南 🚀

**版本**: v0.1.2
**更新时间**: 2026-02-06

---

## ✅ 当前状态

**MVP v0.1.2 已完成** - 功能完整可用！

你现在可以：
- ✅ 运行完整的 TUI 界面
- ✅ 与 AI 进行实时对话
- ✅ 使用工具系统（bash/read/write）
- ✅ 流式响应体验
- ✅ 记忆系统

---

## 🚀 5 分钟上手

### 步骤 1: 设置环境变量

```bash
# 必需：API 密钥
export OPENAI_API_KEY="your-api-key-here"

# 可选：自定义配置
export OPENAI_API_BASE="https://api.z.ai/api/coding/paas/v4"
export OPENAI_MODEL="gpt-4o"
```

### 步骤 2: 运行程序

```bash
# 方式一：直接运行（推荐）
./bin/kele

# 方式二：使用 make
make run

# 方式三：重新编译
make build && ./bin/kele
```

### 步骤 3: 开始对话

程序启动后，直接输入消息：

```
你好
```

按 `Enter` 发送，你将看到：
- 流式响应逐字显示
- 打字机效果
- 实时状态更新

---

## 🎮 基础操作

### 发送消息

直接输入文字后按 `Enter`:

```
帮我分析当前目录
```

### 使用工具

AI 会自动调用工具：

```
你: 列出当前目录的文件
AI: 🔧 调用工具: bash
    执行命令: ls -la
    [返回文件列表]
```

### 使用命令

以 `/` 开头的特殊命令：

```
/help     # 查看所有命令
/status   # 查看系统状态
/clear    # 清空对话历史
/quit     # 退出程序
```

**💡 提示**: 输入命令时按 `Tab` 键可以自动补全！

```
/mo<Tab>        → /model
/model-<Tab>    → /model-reset
```

### 退出程序

- 按 `Ctrl+C`
- 或输入 `/quit`

---

## 🖥️ 界面说明

```
┌─────────────────────────────────────────────────┐
│ 🥤 Kele v0.1.2 | 准备就绪 | 输入消息开始对话     │  ← 状态栏
├─────────────────────────────────────────────────┤
│                                                 │
│ You: 你好                                       │  ← 对话历史
│ Assistant: 你好！我是 Kele，很高兴见到你▋      │  ← 流式响应
│                                                 │
├─────────────────────────────────────────────────┤
│ 输入消息... (Enter 发送, Ctrl+C 退出)            │  ← 输入区域
│                                                 │
├─────────────────────────────────────────────────┤
│ 💡 /help 查看命令 | Ctrl+C 退出                 │  ← 帮助提示
└─────────────────────────────────────────────────┘
```

---

## 💡 使用技巧

### 1. 让 AI 执行命令

```
你: 帮我创建一个名为 test.txt 的文件，内容是 Hello World
AI: 🔧 调用工具: write
    写入文件: test.txt
    内容: Hello World
    ✓ 完成
```

### 2. 多轮对话

AI 会记住上下文（20 轮历史）：

```
你: 我叫 Blake
AI: 很高兴认识你，Blake！

你: 我叫什么名字？
AI: 你叫 Blake。
```

### 3. 查看文件内容

```
你: 读取 README.md 的内容
AI: 🔧 调用工具: read
    读取文件: README.md
    [显示文件内容]
```

### 4. 清空历史重新开始

```
/clear
```

---

## 📋 环境变量完整说明

| 变量名 | 必需 | 默认值 | 说明 |
|--------|------|--------|------|
| `OPENAI_API_KEY` | **是** | 无 | API 密钥 |
| `OPENAI_API_BASE` | 否 | https://api.openai.com/v1 | API 端点 |
| `OPENAI_MODEL` | 否 | gpt-4o | 模型名称 |

### 示例配置

**OpenAI 官方**:
```bash
export OPENAI_API_KEY="sk-xxx"
export OPENAI_MODEL="gpt-4o"
```

**自定义端点**:
```bash
export OPENAI_API_BASE="https://api.z.ai/api/coding/paas/v4"
export OPENAI_API_KEY="your-key"
export OPENAI_MODEL="gpt-4o"
```

**Claude (通过兼容接口)**:
```bash
export OPENAI_API_BASE="https://api.anthropic-proxy.com/v1"
export OPENAI_API_KEY="sk-ant-xxx"
export OPENAI_MODEL="claude-3-5-sonnet-20241022"
```

---

## ⚙️ 配置文件

配置文件位于 `.kele/config.yaml`，但当前版本主要使用环境变量。

配置文件中的设置会被环境变量覆盖。

---

## 🔍 常见问题

### Q: 提示 "OPENAI_API_KEY environment variable is required"

**A**: 设置环境变量：
```bash
export OPENAI_API_KEY="your-key"
```

### Q: 连接失败

**A**: 检查网络和 API 端点：
```bash
curl -H "Authorization: Bearer $OPENAI_API_KEY" \
     "$OPENAI_API_BASE/models"
```

### Q: 界面显示异常

**A**:
- 确保终端窗口足够大（至少 80x24）
- 确保终端支持 UTF-8 和 ANSI 颜色
- 推荐使用现代终端（iTerm2, Alacritty）

### Q: 如何查看日志

**A**:
- 会话日志: `.kele/sessions/*.jsonl`
- 记忆文件: `.kele/MEMORY.md`
- 数据库: `.kele/memory.db`

---

## 📚 下一步

- 查看完整功能: `FEATURES.md`
- 详细使用指南: `USAGE.md`
- 部署到服务器: `DEPLOY.md`
- 查看更新日志: `CHANGELOG.md`

---

## 🎉 开始使用

现在你已经准备好开始使用 Kele 了！

```bash
./bin/kele
```

享受与 AI 的交互吧！ 🥤

---

**提示**: 如果遇到问题，查看 `HOW_TO_TEST.md` 或提交 Issue。
