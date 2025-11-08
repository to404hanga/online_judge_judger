# Go编译和运行镜像
FROM golang:1.22-alpine

# 设置环境变量
ENV GO111MODULE=on
ENV CGO_ENABLED=0

# 安装必要的工具
RUN apk add --no-cache \
    git \
    ca-certificates

# 创建工作目录
WORKDIR /app

# 创建非root用户用于运行代码（安全考虑）
RUN adduser -D -s /bin/sh runner && \
    chown -R runner:runner /app

# 切换到非root用户
USER runner

# 默认命令
CMD ["/bin/sh"]