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
	"github.com/y001j/iot-gateway/internal/southbound/mock"
	"github.com/y001j/iot-gateway/internal/model"
)

func main() {
	// 设置日志级别为debug
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	// 创建一个模拟适配器
	adapter := &mock.MockAdapter{}

	// 准备配置
	config := map[string]interface{}{
		"device_id":   "test-device",
		"interval_ms": 1000,
		"points": []map[string]interface{}{
			{
				"key":      "temperature",
				"min":      20.0,
				"max":      30.0,
				"type":     "float",
				"variance": 0.05,
			},
			{
				"key":      "humidity",
				"min":      40.0,
				"max":      60.0,
				"type":     "float",
				"variance": 0.02,
			},
			{
				"key":      "status",
				"min":      0,
				"max":      1,
				"type":     "int",
				"variance": 0.1,
			},
			{
				"key":      "online",
				"constant": true,
				"type":     "bool",
			},
			{
				"key":      "device_name",
				"constant": "测试设备",
				"type":     "string",
			},
		},
	}

	// 序列化配置
	configBytes, err := json.Marshal(config)
	if err != nil {
		log.Fatal().Err(err).Msg("序列化配置失败")
	}

	// 初始化适配器
	if err := adapter.Init(configBytes); err != nil {
		log.Fatal().Err(err).Msg("初始化适配器失败")
	}

	// 创建数据通道
	dataCh := make(chan model.Point, 100)

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动适配器
	if err := adapter.Start(ctx, dataCh); err != nil {
		log.Fatal().Err(err).Msg("启动适配器失败")
	}

	// 处理数据点
	go func() {
		for point := range dataCh {
			log.Info().
				Str("key", point.Key).
				Str("device_id", point.DeviceID).
				Str("type", string(point.Type)).
				Interface("value", point.Value).
				Msg("收到数据点")

			// 打印值的实际Go类型
			switch point.Value.(type) {
			case int:
				fmt.Printf("值 %v 的Go类型: int\n", point.Value)
			case float64:
				fmt.Printf("值 %v 的Go类型: float64\n", point.Value)
			case bool:
				fmt.Printf("值 %v 的Go类型: bool\n", point.Value)
			case string:
				fmt.Printf("值 %v 的Go类型: string\n", point.Value)
			default:
				fmt.Printf("值 %v 的Go类型: %T\n", point.Value, point.Value)
			}
		}
	}()

	// 等待中断信号
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Info().Msg("收到中断信号，正在停止...")
}
