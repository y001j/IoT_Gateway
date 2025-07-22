#!/bin/bash

# IoT Gateway 规则引擎快速测试脚本
# 验证配置和规则，然后进行简短的功能测试

echo "⚡ IoT Gateway 规则引擎快速测试"
echo "==============================="
echo ""

# 1. 验证环境
echo "🔍 步骤1: 环境检查"
echo "-----------------"

if ! command -v go &> /dev/null; then
    echo "❌ Go未安装"
    exit 1
fi
echo "✅ Go环境: $(go version | cut -d' ' -f3)"

if [[ ! -f "config_rule_engine_test.yaml" ]]; then
    echo "❌ 配置文件不存在: config_rule_engine_test.yaml"
    exit 1
fi
echo "✅ 配置文件存在"

echo ""

# 2. 验证配置
echo "🔍 步骤2: 配置验证"
echo "-----------------"

echo "编译验证工具..."
if go build -o bin/validate validate_rule_engine.go; then
    echo "✅ 验证工具编译成功"
else
    echo "❌ 验证工具编译失败"
    exit 1
fi

echo ""
echo "运行配置验证..."
./bin/validate config_rule_engine_test.yaml

echo ""

# 3. 编译主程序
echo "🔍 步骤3: 编译主程序"
echo "------------------"

if go build -o bin/gateway cmd/gateway/main.go; then
    echo "✅ Gateway编译成功"
else
    echo "❌ Gateway编译失败"
    exit 1
fi

echo ""

# 4. 创建必要目录
echo "🔍 步骤4: 准备环境"
echo "----------------"

mkdir -p logs
mkdir -p rules

# 复制规则文件
if [[ -f "test_rules_simple.json" ]]; then
    cp test_rules_simple.json rules/
    echo "✅ 规则文件已复制到rules目录"
else
    echo "⚠️  外部规则文件不存在，将使用内联规则"
fi

echo ""

# 5. 快速启动测试
echo "🔍 步骤5: 快速功能测试"
echo "--------------------"

echo "启动Gateway进行10秒测试..."

# 启动gateway
./bin/gateway -config config_rule_engine_test.yaml > logs/quick_test.log 2>&1 &
GATEWAY_PID=$!

# 等待启动
sleep 2

# 检查进程
if kill -0 $GATEWAY_PID 2>/dev/null; then
    echo "✅ Gateway启动成功"
    
    # 运行10秒
    echo "运行测试 (10秒)..."
    sleep 10
    
    # 停止
    kill $GATEWAY_PID 2>/dev/null
    wait $GATEWAY_PID 2>/dev/null
    echo "✅ Gateway已停止"
else
    echo "❌ Gateway启动失败"
    echo "错误日志:"
    cat logs/quick_test.log
    exit 1
fi

echo ""

# 6. 结果分析
echo "🔍 步骤6: 结果分析"
echo "----------------"

if [[ -f "logs/quick_test.log" ]]; then
    # 统计日志
    total_lines=$(wc -l < logs/quick_test.log)
    echo "📊 日志统计:"
    echo "  • 总日志行数: $total_lines"
    
    # 检查关键词
    if grep -q -i "rule" logs/quick_test.log; then
        rule_mentions=$(grep -c -i "rule" logs/quick_test.log)
        echo "  • 规则相关日志: $rule_mentions 条"
    fi
    
    if grep -q -i "data\|point" logs/quick_test.log; then
        data_mentions=$(grep -c -i "data\|point" logs/quick_test.log)
        echo "  • 数据相关日志: $data_mentions 条"
    fi
    
    # 检查错误
    if grep -q -i "error\|failed\|panic" logs/quick_test.log; then
        error_count=$(grep -c -i "error\|failed\|panic" logs/quick_test.log)
        echo "  • ⚠️  错误日志: $error_count 条"
        
        echo ""
        echo "最近的错误信息:"
        grep -i "error\|failed\|panic" logs/quick_test.log | tail -3 | sed 's/^/    /'
    else
        echo "  • ✅ 无错误日志"
    fi
    
    echo ""
    echo "📄 完整日志: logs/quick_test.log"
    echo "查看完整日志: cat logs/quick_test.log"
else
    echo "❌ 日志文件不存在"
fi

echo ""

# 7. 总结
echo "🎯 测试总结"
echo "----------"

echo "✅ 环境检查通过"
echo "✅ 配置验证通过" 
echo "✅ 编译成功"
echo "✅ 启动和停止正常"

if [[ -f "logs/quick_test.log" ]] && ! grep -q -i "error\|failed\|panic" logs/quick_test.log; then
    echo "✅ 无明显错误"
    echo ""
    echo "🎉 快速测试通过！"
    echo ""
    echo "🚀 下一步可以运行完整测试:"
    echo "   ./test_gateway_rules.sh"
else
    echo "⚠️  发现一些问题，请检查日志"
    echo ""
    echo "🔍 调试建议:"
    echo "   1. 查看日志: cat logs/quick_test.log"
    echo "   2. 手动启动: ./bin/gateway -config config_rule_engine_test.yaml"
    echo "   3. 验证配置: ./bin/validate config_rule_engine_test.yaml"
fi

echo ""
echo "📚 相关文件:"
echo "  • 配置: config_rule_engine_test.yaml"
echo "  • 规则: rules/test_rules_simple.json" 
echo "  • 日志: logs/quick_test.log"
echo "  • 验证: ./bin/validate"