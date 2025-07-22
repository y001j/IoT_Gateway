package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/southbound/modbus"
)

func init() {
	// 配置日志
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
}

func main() {
	fmt.Println("Modbus测试工具")
	fmt.Println("=============")

	// 创建Modbus适配器
	adapter := modbus.NewAdapter()
	if adapter == nil {
		log.Fatal().Msg("创建Modbus适配器失败")
	}

	// 准备配置
	config := map[string]interface{}{
		"mode": "tcp",
		"address": "127.0.0.1:502",
		"timeout_ms": 5000,
		"interval_ms": 2000,
		"registers": []map[string]interface{}{
			{
				"key":       "temperature",
				"device_id": uint8(1),
				"function":  uint8(3), // 3 = 读保持寄存器
				"address":   uint16(0),
				"quantity":  uint16(1),
				"type":      "float",
				"scale":     0.1,
				"tags": map[string]string{
					"description": "温度传感器",
				},
			},
			{
				"key":       "humidity",
				"device_id": uint8(1),
				"function":  uint8(3), // 3 = 读保持寄存器
				"address":   uint16(1),
				"quantity":  uint16(1),
				"type":      "float",
				"scale":     0.1,
				"tags": map[string]string{
					"description": "湿度传感器",
				},
			},
			{
				"key":       "pressure",
				"device_id": uint8(1),
				"function":  uint8(3), // 3 = 读保持寄存器
				"address":   uint16(2),
				"quantity":  uint16(1),
				"type":      "float",
				"scale":     1.0,
				"tags": map[string]string{
					"description": "压力传感器",
				},
			},
			{
				"key":       "status",
				"device_id": uint8(1),
				"function":  uint8(1), // 1 = 读线圈
				"address":   uint16(0),
				"quantity":  uint16(1),
				"type":      "bool",
				"tags": map[string]string{
					"description": "设备状态",
				},
			},
			{
				"key":       "alarm",
				"device_id": uint8(1),
				"function":  uint8(2), // 2 = 读离散输入
				"address":   uint16(0),
				"quantity":  uint16(1),
				"type":      "bool",
				"tags": map[string]string{
					"description": "报警状态",
				},
			},
		},
	}

	// 序列化配置
	configData, err := json.Marshal(config)
	if err != nil {
		log.Fatal().Err(err).Msg("序列化配置失败")
	}

	// 初始化适配器
	fmt.Println("初始化Modbus适配器...")
	if err := adapter.Init(configData); err != nil {
		log.Fatal().Err(err).Msg("初始化Modbus适配器失败")
	}

	// 创建数据通道
	dataChan := make(chan model.Point, 100)

	// 创建上下文，支持取消
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动适配器
	fmt.Println("启动Modbus适配器...")
	if err := adapter.Start(ctx, dataChan); err != nil {
		log.Fatal().Err(err).Msg("启动Modbus适配器失败")
	}

	// 处理数据点
	go func() {
		for {
			select {
			case point := <-dataChan:
				// 格式化时间
				timeStr := point.Timestamp.Format("2006-01-02 15:04:05.000")
				
				// 格式化标签
				tagsStr := ""
				for k, v := range point.Tags {
					tagsStr += fmt.Sprintf("%s=%s ", k, v)
				}
				
				// 打印数据点
				fmt.Printf("[%s] %s.%s = %v (%s) %s\n", 
					timeStr, 
					point.DeviceID, 
					point.Key, 
					point.Value, 
					point.Type, 
					tagsStr)
			case <-ctx.Done():
				return
			}
		}
	}()

	// 等待中断信号
	fmt.Println("按Ctrl+C停止...")
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	// 停止适配器
	fmt.Println("停止Modbus适配器...")
	if err := adapter.Stop(); err != nil {
		log.Error().Err(err).Msg("停止Modbus适配器失败")
	}

	fmt.Println("测试完成")
}
