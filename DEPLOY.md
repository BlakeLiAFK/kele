# Kele 部署指南

## 🚀 本地开发

### 前置要求

- Go 1.25+
- CGO 支持（用于 SQLite）
- Git

### 快速开始

```bash
# 1. 克隆/进入项目
cd kele

# 2. 安装依赖
make deps

# 3. 设置环境变量
export OPENAI_API_BASE="https://api.z.ai/api/coding/paas/v4"
export OPENAI_API_KEY="your-api-key"

# 4. 运行
make run
```

## 📦 编译

### 本地编译

```bash
make build
```

生成文件：`bin/kele`

### 交叉编译

```bash
# Linux AMD64
GOOS=linux GOARCH=amd64 make build

# Linux ARM64
GOOS=linux GOARCH=arm64 make build

# macOS AMD64
GOOS=darwin GOARCH=amd64 make build

# macOS ARM64 (M1/M2)
GOOS=darwin GOARCH=arm64 make build
```

## 🐳 Docker 部署

### 创建 Dockerfile

```dockerfile
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache gcc musl-dev sqlite-dev git

WORKDIR /app
COPY . .

RUN go mod download
RUN CGO_ENABLED=1 go build -o kele ./cmd/kele

FROM alpine:latest

RUN apk add --no-cache sqlite-libs ca-certificates

WORKDIR /app
COPY --from=builder /app/kele .

# 数据持久化
VOLUME /app/.kele

ENV OPENAI_API_BASE=""
ENV OPENAI_API_KEY=""

CMD ["./kele"]
```

### 构建和运行

```bash
# 构建镜像
docker build -t kele:latest .

# 运行容器
docker run -it --rm \
  -e OPENAI_API_BASE="https://api.z.ai/api/coding/paas/v4" \
  -e OPENAI_API_KEY="your-api-key" \
  -v $(pwd)/.kele:/app/.kele \
  kele:latest
```

## 🖥️ 服务器部署

### Systemd 服务

创建 `/etc/systemd/system/kele.service`:

```ini
[Unit]
Description=Kele AI Assistant
After=network.target

[Service]
Type=simple
User=kele
WorkingDirectory=/opt/kele
ExecStart=/opt/kele/bin/kele

# 环境变量
Environment="OPENAI_API_BASE=https://api.z.ai/api/coding/paas/v4"
Environment="OPENAI_API_KEY=your-api-key"

# 自动重启
Restart=always
RestartSec=10

# 日志
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

### 启动服务

```bash
# 启用开机自启
sudo systemctl enable kele

# 启动服务
sudo systemctl start kele

# 查看状态
sudo systemctl status kele

# 查看日志
journalctl -u kele -f
```

## 🔧 配置管理

### 环境变量

推荐使用 `.env` 文件（不要提交到 Git）：

```bash
# .env
OPENAI_API_BASE=https://api.z.ai/api/coding/paas/v4
OPENAI_API_KEY=your-api-key
```

加载环境变量：

```bash
source .env
make run
```

### 配置文件

编辑 `.kele/config.yaml`:

```yaml
llm:
  provider: openai
  model: gpt-4o
  max_tokens: 4096
  temperature: 0.7

memory:
  enabled: true
  max_history: 20

tools:
  enabled:
    - bash
    - read
    - write
```

## 📊 监控

### 日志位置

- **开发模式**: 终端输出
- **生产模式**: systemd journal
- **Docker**: 容器日志

### 查看日志

```bash
# Systemd
journalctl -u kele -n 100

# Docker
docker logs -f kele-container

# 直接运行
./bin/kele 2>&1 | tee kele.log
```

### 数据文件

- `.kele/memory.db` - 记忆数据库
- `.kele/MEMORY.md` - 可读记忆
- `.kele/sessions/` - 会话日志

## 🔒 安全配置

### API Key 保护

```bash
# 使用环境变量，不要硬编码
export OPENAI_API_KEY="sk-..."

# 文件权限
chmod 600 .env
```

### 工具限制

编辑 `internal/tools/executor.go` 调整允许的命令。

### 数据加密

SQLite 数据库默认未加密，可以使用 SQLCipher：

```bash
go get github.com/mutecomm/go-sqlcipher/v4
```

## 🧪 测试部署

### 健康检查

```bash
# 编译测试
./test.sh

# 运行测试
make test

# 手动测试
./bin/kele
# 输入: /status
```

### 性能基准

```bash
# 启动时间
time ./bin/kele --help

# 内存占用
ps aux | grep kele

# 数据库大小
du -sh .kele/memory.db
```

## 🔄 更新部署

### 零停机更新

```bash
# 1. 编译新版本
make build

# 2. 备份数据
cp -r .kele .kele.backup

# 3. 停止旧版本
sudo systemctl stop kele

# 4. 替换二进制
sudo cp bin/kele /opt/kele/bin/

# 5. 启动新版本
sudo systemctl start kele
```

### 回滚

```bash
# 恢复旧版本
sudo cp /opt/kele/bin/kele.old /opt/kele/bin/kele

# 恢复数据
rm -rf .kele
mv .kele.backup .kele

# 重启
sudo systemctl restart kele
```

## 📦 备份策略

### 自动备份

```bash
#!/bin/bash
# backup.sh

DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_DIR="/backup/kele"

mkdir -p $BACKUP_DIR

# 备份数据库
cp .kele/memory.db $BACKUP_DIR/memory_$DATE.db

# 备份会话
tar -czf $BACKUP_DIR/sessions_$DATE.tar.gz .kele/sessions/

# 保留最近 7 天
find $BACKUP_DIR -mtime +7 -delete
```

### Cron 任务

```bash
# 每天凌晨 3 点备份
0 3 * * * /opt/kele/backup.sh
```

## 🔍 故障排查

### 常见问题

#### 1. 编译失败

```bash
# 检查 Go 版本
go version

# 检查 CGO
go env CGO_ENABLED

# 重新安装依赖
rm -rf vendor/ go.sum
make deps
```

#### 2. 连接错误

```bash
# 测试 API 连接
curl -X POST "$OPENAI_API_BASE/chat/completions" \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"test"}]}'
```

#### 3. SQLite 错误

```bash
# 检查数据库
sqlite3 .kele/memory.db "PRAGMA integrity_check;"

# 重建数据库
rm .kele/memory.db
./bin/kele
```

## 📈 扩展部署

### 负载均衡

可以运行多个实例，共享存储：

```
User → Nginx → Kele Instance 1 → Shared Storage
             → Kele Instance 2 → Shared Storage
```

### 集群部署

使用共享 Redis 存储会话状态（待实现）。

---

**需要帮助？** 查看 [USAGE.md](USAGE.md) 或提交 Issue。
