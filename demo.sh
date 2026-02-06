#!/bin/bash

# Kele 演示脚本
set -e

echo "🦞 Kele - TUI 版 OpenClaw 演示"
echo "================================"
echo ""

# 检查环境变量
if [ -z "$OPENAI_API_KEY" ]; then
    echo "❌ 错误: 需要设置 OPENAI_API_KEY"
    echo ""
    echo "运行以下命令:"
    echo '  export OPENAI_API_BASE="https://api.z.ai/api/coding/paas/v4"'
    echo '  export OPENAI_API_KEY="your-key"'
    exit 1
fi

echo "✅ 环境检查通过"
echo ""

# 显示功能介绍
echo "📋 功能特性:"
echo "  • 流式响应 (打字机效果)"
echo "  • 工具调用 (bash/read/write)"
echo "  • 记忆系统 (SQLite + Markdown)"
echo "  • Slash 命令 (/help, /status, etc.)"
echo ""

# 显示文件信息
echo "📊 项目统计:"
echo "  • Go 代码: ~1500 行"
echo "  • 文档: ~1000 行"
echo "  • 二进制大小: $(ls -lh bin/kele 2>/dev/null | awk '{print $5}' || echo '未编译')"
echo ""

# 显示快速命令
echo "🚀 快速开始:"
echo "  1. make run       # 直接运行"
echo "  2. ./bin/kele     # 运行已编译版本"
echo "  3. make build     # 重新编译"
echo ""

echo "💡 使用提示:"
echo "  • 输入消息开始对话"
echo "  • 输入 /help 查看命令"
echo "  • 按 Ctrl+C 退出"
echo ""

# 询问是否运行

echo ""
echo "🦞 启动 Kele..."
echo "================================"
sleep 1
make run
