# ARM Linux 部署指南

> 版本：v1.0 &nbsp;&nbsp; 作者：IoT Gateway Team &nbsp;&nbsp; 日期：2025-08-15

本文档介绍如何将IoT Gateway编译为Linux ARM架构程序，特别适用于树莓派等ARM设备的部署。

## 1. 环境要求

### 开发环境
- **Go版本**: 1.24+ (支持交叉编译)
- **操作系统**: Windows, macOS, 或 Linux
- **Node.js**: 18+ (用于前端构建)

### 目标ARM设备
- **树莓派**: 3B+, 4B, 5, Zero 2W等
- **操作系统**: Raspberry Pi OS, Ubuntu, Debian ARM版本
- **架构**: ARMv6, ARMv7 (32位), ARMv8 (64位)

## 2. 交叉编译方法

### 2.1 后端Go程序编译

#### 编译到ARM64 (64位树莓派) - 推荐方式
```bash
# 设置环境变量 (纯Go编译，无需GCC)
export GOOS=linux
export GOARCH=arm64
export CGO_ENABLED=0

# 编译主程序 (包含嵌入式Web服务器)
go build -ldflags="-w -s" -o bin/gateway-arm64 cmd/gateway/main.go
```

> 💡 **为什么使用 CGO_ENABLED=0？**
> - 无需安装ARM交叉编译工具链 (gcc-aarch64-linux-gnu)
> - 生成静态链接的二进制文件，更容易部署
> - IoT Gateway所有依赖都有纯Go实现（包括SQLite）

#### 编译到ARMv7 (32位树莓派)
```bash
# 设置环境变量 (纯Go编译)
export GOOS=linux
export GOARCH=arm
export GOARM=7
export CGO_ENABLED=0

# 编译主程序 (包含嵌入式Web服务器)
go build -ldflags="-w -s" -o bin/gateway-armv7 cmd/gateway/main.go
```

#### 编译到ARMv6 (树莓派Zero)
```bash
# 设置环境变量 (纯Go编译)
export GOOS=linux
export GOARCH=arm
export GOARM=6
export CGO_ENABLED=0

# 编译主程序 (包含嵌入式Web服务器)
go build -ldflags="-w -s" -o bin/gateway-armv6 cmd/gateway/main.go
```

### 2.2 Windows下交叉编译脚本

创建 `build-arm.bat` 脚本：
```batch
@echo off
echo Building IoT Gateway for ARM Linux (Pure Go)...

REM 创建输出目录
if not exist "bin\arm" mkdir "bin\arm"

REM 编译ARM64版本 (64位树莓派) - 无需GCC
echo Building ARM64 version...
set GOOS=linux
set GOARCH=arm64
set CGO_ENABLED=0
go build -ldflags="-w -s" -o bin/arm/gateway-arm64 cmd/gateway/main.go

REM 编译ARMv7版本 (32位树莓派) - 无需GCC
echo Building ARMv7 version...
set GOOS=linux
set GOARCH=arm
set GOARM=7
set CGO_ENABLED=0
go build -ldflags="-w -s" -o bin/arm/gateway-armv7 cmd/gateway/main.go

REM 编译ARMv6版本 (树莓派Zero) - 无需GCC
echo Building ARMv6 version...
set GOOS=linux
set GOARCH=arm
set GOARM=6
set CGO_ENABLED=0
go build -ldflags="-w -s" -o bin/arm/gateway-armv6 cmd/gateway/main.go

echo ARM builds completed! All binaries are statically linked.
echo Note: Web server is embedded in gateway binary.
pause
```

### 2.3 Linux/macOS下交叉编译脚本

创建 `build-arm.sh` 脚本：
```bash
#!/bin/bash

echo "Building IoT Gateway for ARM Linux..."

# 创建输出目录
mkdir -p bin/arm

# 检查交叉编译工具链
if ! command -v aarch64-linux-gnu-gcc &> /dev/null; then
    echo "Warning: aarch64-linux-gnu-gcc not found. Installing..."
    # Ubuntu/Debian
    sudo apt-get update
    sudo apt-get install -y gcc-aarch64-linux-gnu gcc-arm-linux-gnueabihf
fi

# 编译ARM64版本
echo "Building ARM64 version..."
export GOOS=linux
export GOARCH=arm64
export CGO_ENABLED=1
export CC=aarch64-linux-gnu-gcc
go build -ldflags="-w -s" -o bin/arm/gateway-arm64 cmd/gateway/main.go
go build -ldflags="-w -s" -o bin/arm/server-arm64 cmd/server/main.go

# 编译ARMv7版本
echo "Building ARMv7 version..."
export GOOS=linux
export GOARCH=arm
export GOARM=7
export CGO_ENABLED=1
export CC=arm-linux-gnueabihf-gcc
go build -ldflags="-w -s" -o bin/arm/gateway-armv7 cmd/gateway/main.go
go build -ldflags="-w -s" -o bin/arm/server-armv7 cmd/server/main.go

# 编译ARMv6版本
echo "Building ARMv6 version..."
export GOOS=linux
export GOARCH=arm
export GOARM=6
export CGO_ENABLED=1
export CC=arm-linux-gnueabihf-gcc
go build -ldflags="-w -s" -o bin/arm/gateway-armv6 cmd/gateway/main.go

echo "ARM builds completed!"
ls -la bin/arm/
```

## 3. 前端构建

### 3.1 构建前端资源
```bash
cd web/frontend

# 安装依赖
npm install

# 构建生产版本
npm run build

# 前端资源会生成到 dist/ 目录
```

### 3.2 前端集成选项

#### 选项1: 嵌入式前端 (推荐)
Go程序已经内嵌了前端资源，无需额外操作。

#### 选项2: 独立前端服务
```bash
# 将前端资源复制到ARM设备
scp -r web/frontend/dist/ pi@raspberrypi:/opt/iot-gateway/web/
```

## 4. 完整打包脚本

### 4.1 自动化打包脚本 (`package-arm.sh`)
```bash
#!/bin/bash

VERSION=${1:-"1.0.0"}
BUILD_DATE=$(date +%Y%m%d_%H%M%S)

echo "Packaging IoT Gateway v$VERSION for ARM Linux..."

# 创建打包目录
PACKAGE_DIR="iot-gateway-arm-$VERSION"
mkdir -p $PACKAGE_DIR/{bin,config,rules,web,docs,scripts}

# 构建前端
echo "Building frontend..."
cd web/frontend
npm install
npm run build
cd ../..

# 编译ARM版本
echo "Building ARM binaries..."
./build-arm.sh

# 复制文件
echo "Copying files..."
cp bin/arm/* $PACKAGE_DIR/bin/
cp config*.yaml $PACKAGE_DIR/config/
cp -r rules/* $PACKAGE_DIR/rules/ 2>/dev/null || true
cp -r web/frontend/dist $PACKAGE_DIR/web/frontend/
cp -r docs $PACKAGE_DIR/
cp README.md LICENSE $PACKAGE_DIR/

# 创建启动脚本
cat > $PACKAGE_DIR/scripts/start.sh << 'EOF'
#!/bin/bash

# 检测ARM架构
ARCH=$(uname -m)
case $ARCH in
    aarch64)
        BINARY="gateway-arm64"
        ;;
    armv7l)
        BINARY="gateway-armv7"
        ;;
    armv6l)
        BINARY="gateway-armv6"
        ;;
    *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

echo "Starting IoT Gateway for $ARCH..."
cd "$(dirname "$0")/.."
./bin/$BINARY -config config/config.yaml
EOF

chmod +x $PACKAGE_DIR/scripts/start.sh

# 创建systemd服务文件
cat > $PACKAGE_DIR/scripts/iot-gateway.service << 'EOF'
[Unit]
Description=IoT Gateway Service
After=network.target

[Service]
Type=simple
User=pi
WorkingDirectory=/opt/iot-gateway
ExecStart=/opt/iot-gateway/scripts/start.sh
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# 创建安装脚本
cat > $PACKAGE_DIR/install.sh << 'EOF'
#!/bin/bash

INSTALL_DIR="/opt/iot-gateway"

echo "Installing IoT Gateway..."

# 创建安装目录
sudo mkdir -p $INSTALL_DIR

# 复制文件
sudo cp -r * $INSTALL_DIR/

# 设置权限
sudo chown -R pi:pi $INSTALL_DIR
sudo chmod +x $INSTALL_DIR/bin/*
sudo chmod +x $INSTALL_DIR/scripts/*

# 安装systemd服务
sudo cp $INSTALL_DIR/scripts/iot-gateway.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable iot-gateway

echo "Installation completed!"
echo "Start service: sudo systemctl start iot-gateway"
echo "Check status: sudo systemctl status iot-gateway"
EOF

chmod +x $PACKAGE_DIR/install.sh

# 打包
echo "Creating package..."
tar -czf $PACKAGE_DIR.tar.gz $PACKAGE_DIR
zip -r $PACKAGE_DIR.zip $PACKAGE_DIR

echo "Package created: $PACKAGE_DIR.tar.gz"
echo "Package created: $PACKAGE_DIR.zip"

# 清理
rm -rf $PACKAGE_DIR
```

## 5. 树莓派部署

### 5.1 传输文件到树莓派
```bash
# 通过SCP传输
scp iot-gateway-arm-1.0.0.tar.gz pi@raspberrypi:/home/pi/

# 或使用U盘/SD卡传输
```

### 5.2 在树莓派上安装
```bash
# 解压安装包
tar -xzf iot-gateway-arm-1.0.0.tar.gz
cd iot-gateway-arm-1.0.0

# 运行安装脚本
sudo ./install.sh

# 启动服务
sudo systemctl start iot-gateway

# 检查状态
sudo systemctl status iot-gateway
```

### 5.3 手动运行 (调试模式)
```bash
cd /opt/iot-gateway

# 直接运行 (根据架构选择)
# 64位树莓派:
./bin/gateway-arm64 -config config/config.yaml

# 32位树莓派:
./bin/gateway-armv7 -config config/config.yaml

# 树莓派Zero:
./bin/gateway-armv6 -config config/config.yaml
```

## 6. 性能优化

### 6.1 树莓派性能调优
```bash
# 修改配置文件适应ARM性能
sudo nano /opt/iot-gateway/config/config.yaml
```

ARM优化配置示例：
```yaml
gateway:
  name: "IoT Gateway ARM"
  log_level: "info"
  http_port: 8080
  nats_url: "embedded"

# 降低并发和缓冲以适应ARM性能
rule_engine:
  worker_pool:
    max_workers: 4      # ARM设备建议4个worker
    queue_size: 1000    # 减小队列大小
    batch_size: 10      # 减小批处理大小

# 优化内存使用
southbound:
  adapters:
    - name: "mock_sensors"
      type: "mock"
      config:
        interval: "5s"    # 增加采样间隔

northbound:
  sinks:
    - name: "console_output"
      type: "console"
      config:
        buffer_size: 100  # 减小缓冲区
```

### 6.2 内存和CPU监控
```bash
# 监控资源使用
htop

# 查看内存使用
free -h

# 查看IoT Gateway进程
ps aux | grep gateway

# 查看日志
journalctl -u iot-gateway -f
```

## 7. 故障排除

### 7.1 常见问题

#### 编译错误
```bash
# CGO编译错误 - 安装交叉编译工具链
sudo apt-get install gcc-aarch64-linux-gnu gcc-arm-linux-gnueabihf

# Go模块问题
go mod tidy
go mod download
```

#### 运行时错误
```bash
# 权限问题
sudo chmod +x /opt/iot-gateway/bin/*

# 端口占用
sudo netstat -tlnp | grep :8080
sudo systemctl stop iot-gateway
```

#### 性能问题
```bash
# 减少并发worker数量
# 增加数据采样间隔
# 关闭不必要的功能
```

### 7.2 调试命令
```bash
# 查看系统信息
uname -a
cat /proc/cpuinfo
cat /proc/meminfo

# 查看IoT Gateway版本
./bin/gateway-arm64 -version

# 检查配置文件语法
./bin/gateway-arm64 -config config/config.yaml -check
```

## 8. Docker容器部署 (可选)

### 8.1 多架构Docker构建
```dockerfile
# Dockerfile.arm
FROM --platform=linux/arm64 golang:1.24-alpine AS builder

WORKDIR /app
COPY . .
RUN go mod download
RUN CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -o gateway cmd/gateway/main.go

FROM --platform=linux/arm64 alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/gateway .
COPY config.yaml .
CMD ["./gateway", "-config", "config.yaml"]
```

### 8.2 Docker构建和运行
```bash
# 构建ARM64镜像
docker buildx build --platform linux/arm64 -t iot-gateway:arm64 -f Dockerfile.arm .

# 运行容器
docker run -d \
  --name iot-gateway \
  -p 8080:8080 \
  -p 8081:8081 \
  -v /opt/iot-gateway/config:/config \
  iot-gateway:arm64
```

## 9. 最佳实践

### 9.1 部署建议
1. **选择合适架构**: 根据树莓派型号选择对应的ARM版本
2. **性能调优**: 根据设备性能调整并发参数
3. **监控部署**: 配置systemd自动重启和日志管理
4. **数据备份**: 定期备份配置文件和规则文件

### 9.2 维护建议
1. **定期更新**: 保持系统和应用的最新版本
2. **日志管理**: 配置日志轮转避免磁盘满载
3. **性能监控**: 定期检查CPU和内存使用率
4. **远程管理**: 配置SSH密钥和VPN访问

---

通过以上指南，你可以成功将IoT Gateway部署到树莓派等ARM设备上，实现高效的物联网数据处理和管理。