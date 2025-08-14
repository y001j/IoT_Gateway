# 多阶段构建 Dockerfile for IoT Gateway

# 阶段1: 构建前端
FROM node:18-alpine AS frontend-builder

WORKDIR /app/frontend

# 复制前端依赖文件
COPY web/frontend/package*.json ./

# 安装依赖
RUN npm ci --only=production

# 复制前端源代码
COPY web/frontend/ .

# 构建前端
RUN npm run build

# 阶段2: 构建后端
FROM golang:1.24-alpine AS backend-builder

# 安装必要的包
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# 复制 Go 模块文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 复制前端构建结果
COPY --from=frontend-builder /app/frontend/dist ./web/frontend/dist

# 构建应用
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o gateway cmd/gateway/main.go

# 阶段3: 运行环境
FROM alpine:latest

# 安装必要的包
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# 创建非root用户
RUN addgroup -g 1001 -S iot && \
    adduser -S -D -H -u 1001 -h /app -s /sbin/nologin -G iot -g iot iot

# 复制二进制文件
COPY --from=backend-builder /app/gateway .

# 复制配置文件示例
COPY --from=backend-builder /app/config_rule_engine_test.yaml ./config.yaml

# 复制规则文件
COPY --from=backend-builder /app/rules ./rules

# 创建必要的目录
RUN mkdir -p logs data plugins && \
    chown -R iot:iot /app

# 切换到非root用户
USER iot

# 暴露端口
EXPOSE 8080 8081

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# 启动命令
CMD ["./gateway", "-config", "config.yaml"]