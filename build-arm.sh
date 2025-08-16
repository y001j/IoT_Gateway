#!/bin/bash

echo "========================================"
echo "   IoT Gateway ARM Linux Build Script"
echo "========================================"

# 检查Go是否安装
if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed. Please install Go 1.24+ first."
    exit 1
fi

# 创建输出目录
mkdir -p bin/arm

# 颜色输出函数
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

success() {
    echo -e "${GREEN}✓${NC} $1"
}

error() {
    echo -e "${RED}✗${NC} $1"
}

info() {
    echo -e "${YELLOW}ℹ${NC} $1"
}

# 检查交叉编译工具链 (可选，Go可以使用纯Go实现)
check_cross_compiler() {
    if command -v aarch64-linux-gnu-gcc &> /dev/null; then
        export CC_ARM64=aarch64-linux-gnu-gcc
        info "Found aarch64-linux-gnu-gcc for ARM64"
    else
        info "aarch64-linux-gnu-gcc not found, using Go's built-in CGO"
    fi

    if command -v arm-linux-gnueabihf-gcc &> /dev/null; then
        export CC_ARM=arm-linux-gnueabihf-gcc
        info "Found arm-linux-gnueabihf-gcc for ARM"
    else
        info "arm-linux-gnueabihf-gcc not found, using Go's built-in CGO"
    fi
}

# 构建函数
build_binary() {
    local name=$1
    local goos=$2
    local goarch=$3
    local goarm=$4
    local source=$5
    local output=$6
    
    echo
    echo "Building $name..."
    
    export GOOS=$goos
    export GOARCH=$goarch
    export CGO_ENABLED=0
    
    if [ ! -z "$goarm" ]; then
        export GOARM=$goarm
    fi
    
    if go build -ldflags="-w -s" -o "$output" "$source"; then
        success "$name build successful"
        # 显示文件大小
        local size=$(du -h "$output" | cut -f1)
        info "Binary size: $size"
    else
        error "$name build failed"
        return 1
    fi
}

# 检查交叉编译工具
check_cross_compiler

echo
echo "[1/5] Building for ARM64 (64-bit Raspberry Pi 4/5)..."
build_binary "ARM64 Gateway" "linux" "arm64" "" "cmd/gateway/main.go" "bin/arm/gateway-arm64"

echo
echo "[2/5] Building for ARMv7 (32-bit Raspberry Pi 3/4)..."
build_binary "ARMv7 Gateway" "linux" "arm" "7" "cmd/gateway/main.go" "bin/arm/gateway-armv7"

echo
echo "[3/5] Building for ARMv6 (Raspberry Pi Zero/1)..."
build_binary "ARMv6 Gateway" "linux" "arm" "6" "cmd/gateway/main.go" "bin/arm/gateway-armv6"

echo
echo "[4/5] Building frontend..."
if [ -d "web/frontend" ]; then
    cd web/frontend
    if command -v npm &> /dev/null; then
        npm install --silent
        npm run build
        success "Frontend build completed"
    else
        error "npm not found, skipping frontend build"
    fi
    cd ../..
else
    info "Frontend directory not found, skipping"
fi

echo
echo "[5/5] Creating deployment package..."
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
PACKAGE_NAME="iot-gateway-arm-$TIMESTAMP"

mkdir -p "$PACKAGE_NAME"/{bin,config,rules,docs,scripts}

# 复制二进制文件
cp bin/arm/* "$PACKAGE_NAME/bin/" 2>/dev/null || true

# 复制配置文件
cp config*.yaml "$PACKAGE_NAME/config/" 2>/dev/null || true

# 复制规则文件
cp -r rules/* "$PACKAGE_NAME/rules/" 2>/dev/null || true

# 复制文档
cp -r docs "$PACKAGE_NAME/" 2>/dev/null || true
cp README*.md LICENSE* "$PACKAGE_NAME/" 2>/dev/null || true

# 复制前端文件
if [ -d "web/frontend/dist" ]; then
    mkdir -p "$PACKAGE_NAME/web/frontend"
    cp -r web/frontend/dist "$PACKAGE_NAME/web/frontend/"
fi

echo
echo "Creating startup script..."
cat > "$PACKAGE_NAME/scripts/start.sh" << 'EOF'
#!/bin/bash

# IoT Gateway 启动脚本 - 自动检测ARM架构

# 获取当前脚本目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

# 检测ARM架构
ARCH=$(uname -m)
case $ARCH in
    aarch64)
        BINARY="gateway-arm64"
        echo "🔍 检测到 ARM64 架构 (64位)"
        ;;
    armv7l)
        BINARY="gateway-armv7"
        echo "🔍 检测到 ARMv7 架构 (32位)"
        ;;
    armv6l)
        BINARY="gateway-armv6"
        echo "🔍 检测到 ARMv6 架构 (Pi Zero)"
        ;;
    *)
        echo "❌ 不支持的架构: $ARCH"
        echo "支持的架构: aarch64, armv7l, armv6l"
        exit 1
        ;;
esac

# 检查二进制文件是否存在
BINARY_PATH="$PROJECT_DIR/bin/$BINARY"
if [ ! -f "$BINARY_PATH" ]; then
    echo "❌ 二进制文件不存在: $BINARY_PATH"
    echo "请确保已正确编译对应架构的程序"
    exit 1
fi

# 检查配置文件
CONFIG_FILE="$PROJECT_DIR/config/config.yaml"
if [ ! -f "$CONFIG_FILE" ]; then
    # 尝试使用测试配置
    CONFIG_FILE="$PROJECT_DIR/config/config_rule_engine_test.yaml"
    if [ ! -f "$CONFIG_FILE" ]; then
        echo "❌ 配置文件不存在"
        echo "请确保存在 config/config.yaml 或 config/config_rule_engine_test.yaml"
        exit 1
    fi
fi

echo "🚀 启动 IoT Gateway ($ARCH)..."
echo "📁 工作目录: $PROJECT_DIR"
echo "⚙️ 配置文件: $CONFIG_FILE"

# 切换到项目目录
cd "$PROJECT_DIR"

# 设置权限
chmod +x "$BINARY_PATH"

# 启动程序
exec "$BINARY_PATH" -config "$CONFIG_FILE"
EOF

chmod +x "$PACKAGE_NAME/scripts/start.sh"

echo
echo "Creating systemd service..."
cat > "$PACKAGE_NAME/scripts/iot-gateway.service" << 'EOF'
[Unit]
Description=IoT Gateway Service
Documentation=https://github.com/y001j/IoT_Gateway
After=network.target
Wants=network.target

[Service]
Type=simple
User=pi
Group=pi
WorkingDirectory=/opt/iot-gateway
ExecStart=/opt/iot-gateway/scripts/start.sh
ExecReload=/bin/kill -HUP $MAINPID
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

# 环境变量
Environment=PATH=/usr/local/bin:/usr/bin:/bin
Environment=HOME=/home/pi

# 安全设置
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ReadWritePaths=/opt/iot-gateway

[Install]
WantedBy=multi-user.target
EOF

# 创建安装脚本
cat > "$PACKAGE_NAME/install.sh" << 'EOF'
#!/bin/bash

# IoT Gateway 安装脚本

INSTALL_DIR="/opt/iot-gateway"
SERVICE_NAME="iot-gateway"

echo "🚀 正在安装 IoT Gateway..."

# 检查是否为root用户
if [ "$EUID" -ne 0 ]; then
    echo "❌ 请使用 sudo 运行此脚本"
    exit 1
fi

# 停止现有服务（如果存在）
if systemctl is-active --quiet $SERVICE_NAME; then
    echo "🛑 停止现有服务..."
    systemctl stop $SERVICE_NAME
fi

# 创建安装目录
echo "📁 创建安装目录: $INSTALL_DIR"
mkdir -p $INSTALL_DIR

# 复制文件
echo "📋 复制文件..."
cp -r * $INSTALL_DIR/

# 设置所有者和权限
echo "🔐 设置权限..."
chown -R pi:pi $INSTALL_DIR
chmod +x $INSTALL_DIR/bin/*
chmod +x $INSTALL_DIR/scripts/*

# 安装systemd服务
echo "⚙️ 安装systemd服务..."
cp $INSTALL_DIR/scripts/iot-gateway.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable $SERVICE_NAME

echo "✅ 安装完成!"
echo
echo "🎯 后续操作:"
echo "  启动服务: sudo systemctl start iot-gateway"
echo "  查看状态: sudo systemctl status iot-gateway"
echo "  查看日志: sudo journalctl -u iot-gateway -f"
echo "  访问界面: http://$(hostname -I | awk '{print $1}'):8081"
echo
echo "🔧 手动运行: cd $INSTALL_DIR && ./scripts/start.sh"
EOF

chmod +x "$PACKAGE_NAME/install.sh"

# 创建压缩包
echo "📦 Creating package..."
tar -czf "$PACKAGE_NAME.tar.gz" "$PACKAGE_NAME"

# 清理临时目录
rm -rf "$PACKAGE_NAME"

echo
echo "========================================"
echo "✅ Build Summary"
echo "========================================"
ls -la bin/arm/
echo
success "Package created: $PACKAGE_NAME.tar.gz"
echo
echo "🚀 部署到树莓派:"
echo "  1. 传输文件: scp $PACKAGE_NAME.tar.gz pi@your-pi:/home/pi/"
echo "  2. 解压安装: tar -xzf $PACKAGE_NAME.tar.gz && cd ${PACKAGE_NAME%.*} && sudo ./install.sh"
echo "  3. 启动服务: sudo systemctl start iot-gateway"
echo
echo "🔍 手动运行测试:"
echo "  # 64位树莓派: ./gateway-arm64 -config config.yaml"
echo "  # 32位树莓派: ./gateway-armv7 -config config.yaml"
echo "  # 树莓派Zero: ./gateway-armv6 -config config.yaml"
echo
echo "📱 访问Web界面: http://树莓派IP:8081"