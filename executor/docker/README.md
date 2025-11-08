# 在线判题系统Docker镜像

这个目录包含了在线判题系统所需的各种编程语言的Docker镜像。

## 镜像列表

- `judge-cpp:latest` - C++编译和运行环境
- `judge-c:latest` - C编译和运行环境  
- `judge-java:latest` - Java编译和运行环境
- `judge-python:latest` - Python运行环境
- `judge-go:latest` - Go编译和运行环境

## 快速开始

### 批量构建所有镜像

```bash
# 给构建脚本添加执行权限
chmod +x build-images.sh

# 执行构建脚本
./build-images.sh
```

### 单独构建某个镜像

```bash
# 构建C++镜像
docker build -t judge-cpp:latest ./cpp

# 构建Java镜像
docker build -t judge-java:latest ./java

# 构建Python镜像
docker build -t judge-python:latest ./python
```

## 镜像详情

### C++镜像 (judge-cpp:latest)
- 基础镜像: ubuntu:22.04
- 编译器: g++ (支持C++17标准)
- 构建命令: `g++ -std=c++17 -o /app/main /app/main.cpp`
- 运行命令: `/app/main`

### C镜像 (judge-c:latest)
- 基础镜像: ubuntu:22.04
- 编译器: gcc (支持C17标准)
- 构建命令: `gcc -std=c17 -o /app/main /app/main.c`
- 运行命令: `/app/main`

### Java镜像 (judge-java:latest)
- 基础镜像: openjdk:17-slim
- JDK版本: OpenJDK 17
- 构建命令: `javac -d /app /app/Main.java`
- 运行命令: `java -cp /app Main`

### Python镜像 (judge-python:latest)
- 基础镜像: python:3.11-slim
- Python版本: 3.11
- 运行命令: `python3 /app/main.py`

### Go镜像 (judge-go:latest)
- 基础镜像: golang:1.21-alpine
- Go版本: 1.21
- 构建命令: `go build -o /app/main /app/main.go`
- 运行命令: `/app/main`

## 安全特性

所有镜像都包含以下安全特性：
- 使用非root用户运行代码
- 限制容器权限
- 最小化基础镜像
- 移除不必要的软件包

## 使用示例

```bash
# 测试C++编译
docker run --rm -v $(pwd):/app judge-cpp:latest g++ -std=c++17 -o /app/main /app/main.cpp

# 测试Java编译和运行
docker run --rm -v $(pwd):/app judge-java:latest bash -c "javac -d /app /app/Main.java && java -cp /app Main"

# 测试Python运行
docker run --rm -v $(pwd):/app judge-python:latest python3 /app/main.py
```