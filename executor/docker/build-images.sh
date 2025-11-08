#!/bin/bash

# Docker镜像构建脚本
# 用于构建在线判题系统的各种编程语言镜像

set -e

echo "开始构建在线判题系统Docker镜像..."

# 获取脚本所在目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOCKER_DIR="$(dirname "$SCRIPT_DIR")"

# 构建函数
build_image() {
    local language=$1
    local image_name=$2
    local dockerfile_path="$DOCKER_DIR/$language"
    
    echo "正在构建 $language 镜像: $image_name..."
    
    if [ -d "$dockerfile_path" ]; then
        docker build -t "$image_name" "$dockerfile_path"
        echo "✅ $language 镜像构建成功: $image_name"
    else
        echo "❌ 错误: 找不到 $language 的Dockerfile目录: $dockerfile_path"
        exit 1
    fi
}

# 构建各个语言的镜像
echo "构建C++镜像..."
build_image "cpp" "judge-cpp:latest"

echo "构建C镜像..."
build_image "c" "judge-c:latest"

echo "构建Java镜像..."
build_image "java" "judge-java:latest"

echo "构建Python镜像..."
build_image "python" "judge-python:latest"

echo "构建Go镜像..."
build_image "go" "judge-go:latest"

echo "🎉 所有镜像构建完成！"
echo ""
echo "已构建的镜像列表:"
docker images | grep "judge-"