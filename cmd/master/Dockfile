# 使用官方Go镜像作为构建环境
FROM golang:1.23.4-alpine AS builder

# 设置工作目录
WORKDIR /app

# 安装必要的系统依赖
RUN apk add --no-cache git ca-certificates tzdata

# 复制go.mod和go.sum文件
COPY go.mod go.sum ./

# 设置Go代理（使用国内镜像源）
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=sum.golang.google.cn

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建应用程序
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./cmd/master

# 使用轻量级的alpine镜像作为运行环境
FROM alpine:latest

# 安装ca-certificates用于HTTPS请求
RUN apk --no-cache add ca-certificates tzdata

# 设置工作目录
WORKDIR /root/

# 从构建阶段复制二进制文件
COPY --from=builder /app/main .

# 创建配置文件目录
RUN mkdir -p config

# # 复制配置文件
# COPY cmd/master/config/config.yaml ./config/

# 创建日志目录
RUN mkdir -p log

# # 暴露端口（根据config.yaml中的gin.addr配置）
# EXPOSE ${SERVER_PORT:-8080}

# # 设置环境变量
# ENV GIN_MODE=release

# 运行应用程序
CMD ["./main", "--config", "./config/config.yaml"]