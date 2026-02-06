#!/bin/bash

# Kele 验证脚本

set -e

echo "========================================="
echo "  Kele v0.1.0 验证脚本"
echo "========================================="
echo ""

# 1. 检查编译
echo "✓ 检查编译..."
if [ ! -f "bin/kele" ]; then
    echo "✗ 可执行文件不存在，正在编译..."
    make build
fi
echo "  ✓ 可执行文件存在: $(ls -lh bin/kele | awk '{print $5}')"
echo ""

# 2. 检查源代码
echo "✓ 检查源代码..."
GO_FILES=$(find internal cmd -name "*.go" | wc -l | tr -d ' ')
echo "  ✓ Go 源文件: $GO_FILES 个"
GO_LINES=$(find internal cmd -name "*.go" -exec wc -l {} + | tail -1 | awk '{print $1}')
echo "  ✓ 代码行数: $GO_LINES 行"
echo ""

# 3. 检查依赖
echo "✓ 检查依赖..."
if go mod verify > /dev/null 2>&1; then
    echo "  ✓ 依赖验证通过"
else
    echo "  ✗ 依赖验证失败"
    exit 1
fi
echo ""

# 4. 检查数据库初始化
echo "✓ 检查数据库初始化..."
rm -rf .kele/memory.db
export OPENAI_API_BASE="https://api.z.ai/api/coding/paas/v4"
export OPENAI_API_KEY="test"

# 启动程序然后立即关闭，只为创建数据库
(./bin/kele < /dev/null > /dev/null 2>&1 &)
sleep 1
killall kele 2>/dev/null || true

if [ -f ".kele/memory.db" ]; then
    DB_SIZE=$(ls -lh .kele/memory.db | awk '{print $5}')
    echo "  ✓ 数据库创建成功: $DB_SIZE"
else
    echo "  ✗ 数据库创建失败"
    exit 1
fi
echo ""

# 5. 检查数据库表结构
echo "✓ 检查数据库表结构..."
TABLES=$(sqlite3 .kele/memory.db "SELECT name FROM sqlite_master WHERE type='table';" 2>&1)
if echo "$TABLES" | grep -q "messages"; then
    echo "  ✓ messages 表存在"
else
    echo "  ✗ messages 表不存在"
    exit 1
fi
if echo "$TABLES" | grep -q "memory_entries"; then
    echo "  ✓ memory_entries 表存在"
else
    echo "  ✗ memory_entries 表不存在"
    exit 1
fi
echo ""

# 6. 检查配置文件
echo "✓ 检查配置文件..."
if [ -f ".kele/config.yaml" ]; then
    echo "  ✓ config.yaml 存在"
else
    echo "  ⚠ config.yaml 不存在（可选）"
fi
if [ -f ".kele/MEMORY.md" ]; then
    echo "  ✓ MEMORY.md 存在"
else
    echo "  ⚠ MEMORY.md 不存在（运行时创建）"
fi
echo ""

# 7. 检查文档
echo "✓ 检查文档..."
DOC_FILES="README.md QUICKSTART.md USAGE.md FEATURES.md DEPLOY.md CHANGELOG.md"
for doc in $DOC_FILES; do
    if [ -f "$doc" ]; then
        echo "  ✓ $doc"
    else
        echo "  ✗ $doc 缺失"
    fi
done
echo ""

# 8. 统计信息
echo "========================================="
echo "  统计信息"
echo "========================================="
echo "源代码文件: $GO_FILES 个"
echo "代码行数: $GO_LINES 行"
echo "二进制大小: $(ls -lh bin/kele | awk '{print $5}')"
echo "文档文件: $(ls *.md 2>/dev/null | wc -l | tr -d ' ') 个"
echo ""

echo "========================================="
echo "  ✓ 验证完成"
echo "========================================="
echo ""
echo "使用方法："
echo "  export OPENAI_API_BASE=\"https://api.z.ai/api/coding/paas/v4\""
echo "  export OPENAI_API_KEY=\"your-key-here\""
echo "  ./bin/kele"
echo ""
