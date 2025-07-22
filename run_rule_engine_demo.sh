#!/bin/bash

# IoT Gateway 规则引擎功能演示脚本
# 使用测试配置启动网关并演示规则引擎功能

echo "🚀 IoT Gateway 规则引擎功能演示"
echo "================================="
echo ""

# 检查环境
echo "📋 检查运行环境..."

# 检查Go环境
if ! command -v go &> /dev/null; then
    echo "❌ Go 未安装或不在PATH中"
    echo "   请安装 Go 1.19+ 版本"
    exit 1
fi

GO_VERSION=$(go version | cut -d' ' -f3)
echo "✅ Go 版本: $GO_VERSION"

# 检查NATS服务器（可选）
if command -v nats-server &> /dev/null; then
    echo "✅ NATS Server 可用"
    NATS_AVAILABLE=true
else
    echo "⚠️  NATS Server 未安装，将使用嵌入式NATS"
    NATS_AVAILABLE=false
fi

echo ""

# 创建必要的目录
echo "📂 创建必要的目录..."
mkdir -p logs
mkdir -p plugins
mkdir -p rules
echo "✅ 目录创建完成"
echo ""

# 编译网关
echo "🔨 编译 IoT Gateway..."
if go build -o bin/gateway cmd/gateway/main.go; then
    echo "✅ 编译成功"
else
    echo "❌ 编译失败"
    exit 1
fi
echo ""

# 验证配置文件
echo "⚙️  验证配置文件..."
if [[ -f "config_rule_engine_test.yaml" ]]; then
    echo "✅ 测试配置文件存在"
else
    echo "❌ 测试配置文件不存在: config_rule_engine_test.yaml"
    exit 1
fi

if [[ -f "rules/test_comprehensive_rules.json" ]]; then
    echo "✅ 测试规则文件存在"
else
    echo "❌ 测试规则文件不存在: rules/test_comprehensive_rules.json"
    exit 1
fi

# 验证JSON格式
if command -v python3 &> /dev/null; then
    if python3 -m json.tool rules/test_comprehensive_rules.json > /dev/null 2>&1; then
        echo "✅ 规则文件JSON格式正确"
    else
        echo "❌ 规则文件JSON格式错误"
        exit 1
    fi
fi
echo ""

# 启动外部NATS服务器（如果可用且需要）
if [ "$NATS_AVAILABLE" = true ]; then
    echo "🔧 检查NATS服务器状态..."
    if ! pgrep -f "nats-server" > /dev/null; then
        echo "启动NATS服务器..."
        nats-server --port 4222 --http_port 8222 > logs/nats-server.log 2>&1 &
        NATS_PID=$!
        echo "✅ NATS服务器已启动 (PID: $NATS_PID)"
        sleep 2
    else
        echo "✅ NATS服务器已在运行"
    fi
else
    echo "💡 将使用嵌入式NATS服务器"
fi
echo ""

# 显示启动信息
echo "📊 演示配置信息："
echo "   🌐 Web UI: http://localhost:8081"
echo "   📡 WebSocket监控: ws://localhost:8090/ws/rules"
echo "   📈 Prometheus指标: http://localhost:9090/metrics"
echo "   🔍 性能分析: http://localhost:6060/debug/pprof/"
echo "   🏥 健康检查: http://localhost:8080/health"
echo ""

echo "📋 测试规则概览："
echo "   🌡️  温度高温报警 (>40°C)"
echo "   💧 湿度范围检查 (30%-70%)"
echo "   📊 温度数据聚合统计"
echo "   ⚠️  压力异常检测"
echo "   📳 振动阈值监控 (>8g)"
echo "   🔗 多传感器关联分析"
echo "   💓 设备健康监控"
echo "   ✅ 数据质量检查"
echo "   🏃‍♂️ 性能基准测试"
echo "   🧪 复杂表达式测试"
echo ""

# 提供运行选项
echo "🎯 选择演示模式："
echo "   1) 启动网关并运行测试 (推荐)"
echo "   2) 仅启动网关服务"
echo "   3) 运行规则引擎测试套件"
echo "   4) 显示配置信息"
echo ""

read -p "请选择 (1-4, 默认1): " choice
choice=${choice:-1}

case $choice in
    1)
        echo ""
        echo "🚀 启动网关并运行测试..."
        echo "================================="
        
        # 启动网关（后台运行）
        echo "启动 IoT Gateway..."
        ./bin/gateway -config config_rule_engine_test.yaml > logs/gateway.log 2>&1 &
        GATEWAY_PID=$!
        echo "✅ Gateway 已启动 (PID: $GATEWAY_PID)"
        
        # 等待服务启动
        echo "等待服务启动..."
        sleep 5
        
        # 检查服务状态
        if curl -s http://localhost:8080/health > /dev/null; then
            echo "✅ 网关健康检查通过"
        else
            echo "⚠️  网关可能未完全启动，请检查日志"
        fi
        
        echo ""
        echo "📊 实时监控信息："
        echo "   查看日志: tail -f logs/gateway.log"
        echo "   查看规则数据: tail -f logs/rule_engine_test_data.log"
        echo "   Web界面: http://localhost:8081"
        echo ""
        
        echo "⏱️  演示将运行2分钟，然后自动停止..."
        sleep 120
        
        echo ""
        echo "🛑 停止演示..."
        kill $GATEWAY_PID 2>/dev/null
        if [ -n "$NATS_PID" ]; then
            kill $NATS_PID 2>/dev/null
        fi
        echo "✅ 演示完成"
        ;;
        
    2)
        echo ""
        echo "🚀 启动网关服务..."
        echo "==================="
        echo "使用 Ctrl+C 停止服务"
        echo ""
        
        # 前台运行网关
        ./bin/gateway -config config_rule_engine_test.yaml
        ;;
        
    3)
        echo ""
        echo "🧪 运行规则引擎测试套件..."
        echo "=========================="
        ./run_rule_engine_tests.sh
        ;;
        
    4)
        echo ""
        echo "⚙️  配置信息详情..."
        echo "=================="
        echo ""
        echo "📁 文件结构："
        echo "   config_rule_engine_test.yaml     - 主配置文件"
        echo "   rules/test_comprehensive_rules.json - 测试规则"
        echo "   logs/                            - 日志目录"
        echo "   bin/gateway                      - 编译后的可执行文件"
        echo ""
        echo "🔧 配置亮点："
        echo "   • 工作池: 8个并行工作器"
        echo "   • 表达式缓存: 10,000个表达式"
        echo "   • 监控: 全链路性能追踪"
        echo "   • 数据源: 3种Mock适配器"
        echo "   • 输出: 控制台、文件、WebSocket"
        echo ""
        echo "📊 测试数据："
        echo "   • 温度传感器: 10个设备，1秒间隔"
        echo "   • 压力传感器: 5个设备，2秒间隔"
        echo "   • 振动传感器: 20个设备，100ms间隔（高频）"
        echo ""
        echo "🎯 规则类型："
        echo "   • 阈值报警 (温度、压力、振动)"
        echo "   • 数据聚合 (统计分析)"
        echo "   • 异常检测 (数据质量)"
        echo "   • 关联分析 (多传感器)"
        echo "   • 性能测试 (高频处理)"
        echo ""
        ;;
        
    *)
        echo "❌ 无效选择"
        exit 1
        ;;
esac

echo ""
echo "📚 更多信息："
echo "   文档: docs/rule_engine/"
echo "   测试报告: RULE_ENGINE_TEST_REPORT.md"
echo "   配置参考: config_rule_engine_test.yaml"
echo ""
echo "🎉 感谢使用 IoT Gateway 规则引擎演示！"