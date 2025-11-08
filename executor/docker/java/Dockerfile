# Java编译和运行镜像
# 使用 Eclipse Temurin 替代 openjdk，避免阿里云镜像403问题
FROM eclipse-temurin:17-jdk

# 设置环境变量
ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=UTC

# 安装必要的工具
RUN apt-get update && apt-get install -y \
    procps \
    && rm -rf /var/lib/apt/lists/*

# 创建工作目录
WORKDIR /app

# 创建非root用户用于运行代码（安全考虑）
RUN useradd -m -s /bin/bash runner && \
    chown -R runner:runner /app

# 切换到非root用户
USER runner

# 默认命令
CMD ["/bin/bash"]