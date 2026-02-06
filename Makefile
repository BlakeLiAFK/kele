.PHONY: build run clean install deps test

# 构建
build:
	go build -o bin/kele ./cmd/kele

# 运行（开发模式）
run:
	go run ./cmd/kele

# 安装依赖
deps:
	go get github.com/charmbracelet/bubbletea
	go get github.com/charmbracelet/bubbles
	go get github.com/charmbracelet/lipgloss
	go get github.com/charmbracelet/glamour
	go mod tidy

# 清理
clean:
	rm -rf bin/
	go clean

# 测试
test:
	go test -v ./...

# 安装到系统
install: build
	cp bin/kele /usr/local/bin/

# 格式化代码
fmt:
	go fmt ./...

# 代码检查
lint:
	golangci-lint run ./...

# 快速开始
quickstart: deps run
