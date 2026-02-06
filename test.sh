#!/bin/bash

# 测试脚本
set -e

echo "🧪 开始测试 Kele..."

# 检查环境变量
if [ -z "$OPENAI_API_KEY" ]; then
    echo "❌ 错误: 需要设置 OPENAI_API_KEY 环境变量"
    exit 1
fi

echo "✅ 环境变量检查通过"

# 编译
echo "🔨 编译程序..."
make build

if [ ! -f "bin/kele" ]; then
    echo "❌ 编译失败"
    exit 1
fi

echo "✅ 编译成功"

# 检查可执行文件
echo "📦 检查二进制文件..."
file bin/kele
ls -lh bin/kele

echo ""
echo "✅ 所有测试通过！"
echo ""
echo "运行程序："
echo "  make run"
echo ""
echo "或者："
echo "  ./bin/kele"
