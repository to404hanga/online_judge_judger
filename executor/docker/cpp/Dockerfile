# C++编译和运行镜像
FROM ubuntu:22.04

# 设置环境变量
ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=UTC

# 安装C++编译环境
RUN apt-get update && apt-get install -y \
    build-essential \
    gcc \
    g++ \
    gdb \
    cmake \
    make \
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