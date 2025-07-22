#!/bin/bash

# IoT Gateway 规则引擎实际测试脚本
# 使用真实的网关配置和规则进行测试

echo "🚀 IoT Gateway 规则引擎实际测试"
echo "================================="
echo ""

# 检查环境
echo "📋 检查运行环境..."

if ! command -v go &> /dev/null; then
    echo "❌ Go 未安装或不在PATH中"
    exit 1
fi

GO_VERSION=$(go version | cut -d' ' -f3)
echo "✅ Go 版本: $GO_VERSION"
echo ""

# 创建必要的目录
echo "📂 创建必要的目录..."
mkdir -p logs
mkdir -p rules
echo "✅ 目录创建完成"
echo ""

# 复制规则文件
echo "📄 设置规则文件..."
if [[ -f "test_rules_simple.json" ]]; then
    cp test_rules_simple.json rules/
    echo "✅ 测试规则文件已复制到 rules/ 目录"
else
    echo "❌ 测试规则文件不存在"
    exit 1
fi
echo ""

# 验证配置文件
echo "⚙️  验证配置文件..."
if [[ -f "config_rule_engine_test.yaml" ]]; then
    echo "✅ 配置文件存在"
else
    echo "❌ 配置文件不存在: config_rule_engine_test.yaml"
    exit 1
fi

# 验证规则文件JSON格式
if command -v python3 &> /dev/null; then
    if python3 -m json.tool rules/test_rules_simple.json > /dev/null 2>&1; then
        echo "✅ 规则文件JSON格式正确"
    else
        echo "❌ 规则文件JSON格式错误"
        exit 1
    fi
fi
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

echo "📊 测试配置概览："
echo "   🌡️  温度传感器: temp_sensor_01 (1秒间隔)"
echo "   💨 压力传感器: pressure_sensor_01 (2秒间隔)" 
echo "   📳 振动传感器: vibration_sensor_01 (100ms间隔)"
echo ""

echo "📋 测试规则："
echo "   🔥 温度报警: >35°C"
echo "   📊 湿度统计: 5个数据点平均值"
echo "   ⚠️  振动检查: >7.0g"
echo "   🔧 压力过滤: 950-1050 hPa范围"
echo ""

echo "🌐 监控端点："
echo "   Web UI: http://localhost:8081"
echo "   WebSocket: ws://localhost:8090/ws/rules"
echo "   健康检查: http://localhost:8080/health (如果启用)"
echo ""

# 提供运行选项
echo "🎯 选择测试模式："
echo "   1) 启动网关并运行1分钟测试 (推荐)"
echo "   2) 仅启动网关服务 (手动控制)"
echo "   3) 检查配置和规则"
echo ""

read -p "请选择 (1-3, 默认1): " choice
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
        sleep 3
        
        # 检查进程状态
        if kill -0 $GATEWAY_PID 2>/dev/null; then
            echo "✅ 网关进程正在运行"
        else
            echo "❌ 网关启动失败，请检查日志"
            cat logs/gateway.log
            exit 1
        fi
        
        echo ""
        echo "📊 开始数据监控 (60秒)..."
        echo "   查看实时日志: tail -f logs/gateway.log"
        echo ""
        
        # 监控60秒
        start_time=$(date +%s)
        while [ $(($(date +%s) - start_time)) -lt 60 ]; do
            if ! kill -0 $GATEWAY_PID 2>/dev/null; then
                echo "❌ 网关进程意外退出"
                break
            fi
            
            # 显示进度
            elapsed=$(($(date +%s) - start_time))
            printf "\r⏱️  运行时间: %d/60秒" $elapsed
            sleep 1
        done
        
        echo ""
        echo ""
        echo "🛑 停止网关..."
        kill $GATEWAY_PID 2>/dev/null
        wait $GATEWAY_PID 2>/dev/null
        echo "✅ 网关已停止"
        
        echo ""
        echo "📊 测试结果分析："
        echo "查看日志文件以分析结果:"
        echo "  • 主日志: logs/gateway.log"
        
        # 简单的日志分析
        if [[ -f "logs/gateway.log" ]]; then
            echo ""
            echo "📈 快速统计："
            
            # 统计规则相关的日志条目
            if grep -q "rule" logs/gateway.log; then
                rule_count=$(grep -c "rule" logs/gateway.log)
                echo "  • 规则相关日志条目: $rule_count"
            fi
            
            # 统计数据相关的日志条目
            if grep -q "data" logs/gateway.log; then
                data_count=$(grep -c "data" logs/gateway.log)
                echo "  • 数据相关日志条目: $data_count"
            fi
            
            # 检查错误
            if grep -q -i "error\|failed\|panic" logs/gateway.log; then
                error_count=$(grep -c -i "error\|failed\|panic" logs/gateway.log)
                echo "  • 错误/失败条目: $error_count"
                echo ""
                echo "⚠️  发现错误，最近的错误信息:"
                grep -i "error\|failed\|panic" logs/gateway.log | tail -3
            else
                echo "  • 错误条目: 0 ✅"
            fi
            
            echo ""
            echo "💡 查看完整日志: cat logs/gateway.log"
        fi
        ;;
        
    2)
        echo ""
        echo "🚀 启动网关服务..."
        echo "==================="
        echo "使用 Ctrl+C 停止服务"
        echo ""
        
        # 前台运行网关
        echo "执行命令: ./bin/gateway -config config_rule_engine_test.yaml"
        ./bin/gateway -config config_rule_engine_test.yaml
        ;;
        
    3)
        echo ""
        echo "⚙️  配置和规则检查..."
        echo "===================="
        echo ""
        
        # 检查配置文件关键部分
        echo "📋 配置文件检查:"
        if grep -q "rule_engine:" config_rule_engine_test.yaml; then
            echo "✅ 规则引擎配置存在"
        else
            echo "❌ 规则引擎配置缺失"
        fi
        
        if grep -q "southbound:" config_rule_engine_test.yaml; then
            echo "✅ 南向适配器配置存在"
        else
            echo "❌ 南向适配器配置缺失"
        fi
        
        if grep -q "northbound:" config_rule_engine_test.yaml; then
            echo "✅ 北向输出配置存在"
        else
            echo "❌ 北向输出配置缺失"
        fi
        
        echo ""
        echo "📄 规则文件检查:"
        if [[ -f "rules/test_rules_simple.json" ]]; then
            rule_count=$(cat rules/test_rules_simple.json | python3 -c "import sys, json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "解析失败")
            echo "✅ 规则文件存在，包含 $rule_count 个规则"
            
            echo ""
            echo "规则详情:"
            if command -v jq &> /dev/null; then
                cat rules/test_rules_simple.json | jq -r '.[] | "  • \(.id): \(.name) (\(.enabled // false | if . then "启用" else "禁用" end))"'
            else
                python3 -c "
import json, sys
with open('rules/test_rules_simple.json') as f:
    rules = json.load(f)
    for rule in rules:
        status = '启用' if rule.get('enabled', False) else '禁用'
        print(f'  • {rule[\"id\"]}: {rule[\"name\"]} ({status})')
" 2>/dev/null || echo "  无法解析规则详情"
            fi
        else
            echo "❌ 规则文件不存在"
        fi
        
        echo ""
        echo "🔧 建议的启动命令:"
        echo "  ./bin/gateway -config config_rule_engine_test.yaml"
        ;;
        
    *)
        echo "❌ 无效选择"
        exit 1
        ;;
esac

echo ""
echo "📚 更多信息："
echo "   • 查看日志: tail -f logs/gateway.log"
echo "   • 配置文件: config_rule_engine_test.yaml"
echo "   • 规则文件: rules/test_rules_simple.json"
echo "   • Web界面: http://localhost:8081 (如果启用)"
echo ""
echo "🎉 测试完成！"