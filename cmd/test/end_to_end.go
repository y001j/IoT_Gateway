//go:build ignore

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/southbound/mock"
	"github.com/y001j/iot-gateway/internal/model"
	mqtt "github.com/y001j/iot-gateway/internal/northbound/mqtt"
)

func main() {
	// 设置日志级别为debug
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	// 连接到NATS服务器
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Fatal().Err(err).Msg("连接NATS服务器失败")
	}
	defer nc.Close()
	log.Info().Msg("成功连接到NATS服务器")

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建数据通道
	dataCh := make(chan model.Point, 100)

	// 设置MQTT Sink
	mqttSink := setupMQTTSink(nc)

	// 设置Mock Adapter
	mockAdapter := setupMockAdapter()

	// 启动MQTT Sink
	if err := mqttSink.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("启动MQTT Sink失败")
	}

	// 启动Mock Adapter
	if err := mockAdapter.Start(ctx, dataCh); err != nil {
		log.Fatal().Err(err).Msg("启动Mock Adapter失败")
	}

	// 转发数据点到NATS
	go func() {
		for point := range dataCh {
			// 打印数据点信息
			log.Info().
				Str("key", point.Key).
				Str("device_id", point.DeviceID).
				Str("type", string(point.Type)).
				Interface("value", point.Value).
				Msg("从适配器收到数据点")

			// 打印值的实际Go类型
			switch v := point.Value.(type) {
			case int:
				log.Info().Str("key", point.Key).Msgf("适配器值 %v 的Go类型: int", v)
			case float64:
				log.Info().Str("key", point.Key).Msgf("适配器值 %v 的Go类型: float64", v)
			case bool:
				log.Info().Str("key", point.Key).Msgf("适配器值 %v 的Go类型: bool", v)
			case string:
				log.Info().Str("key", point.Key).Msgf("适配器值 %v 的Go类型: string", v)
			default:
				log.Info().Str("key", point.Key).Msgf("适配器值 %v 的Go类型: %T", v, v)
			}

			// 序列化数据点
			data, err := json.Marshal(point)
			if err != nil {
				log.Error().Err(err).Msg("序列化数据点失败")
				continue
			}

			// 发布到NATS
			subject := fmt.Sprintf("iot.sink.%s", mqttSink.Name())
			log.Debug().Str("subject", subject).Msg("发布数据点到NATS")
			if err := nc.Publish(subject, data); err != nil {
				log.Error().Err(err).Msg("发布数据点到NATS失败")
			}
		}
	}()

	// 等待中断信号
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Info().Msg("收到中断信号，正在停止...")
}

func setupMQTTSink(nc *nats.Conn) *mqtt.MQTTSink {
	// 创建MQTT Sink
	sink := &mqtt.MQTTSink{}

	// 准备配置
	config := map[string]interface{}{
		"name":         "mqtt-test",
		"broker_url":   "tcp://localhost:1883",
		"client_id":    "iot-gateway-test",
		"topic_prefix": "iot/data",
		"username":     "",
		"password":     "",
		"qos":          1,
		"retained":     false,
	}

	// 序列化配置
	configBytes, err := json.Marshal(config)
	if err != nil {
		log.Fatal().Err(err).Msg("序列化MQTT配置失败")
	}

	// 初始化MQTT Sink
	if err := sink.Init(configBytes); err != nil {
		log.Fatal().Err(err).Msg("初始化MQTT Sink失败")
	}

	// 设置NATS总线
	sink.SetBus(nc)

	return sink
}

func setupMockAdapter() *mock.MockAdapter {
	// 创建Mock Adapter
	adapter := &mock.MockAdapter{}

	// 准备配置
	config := map[string]interface{}{
		"device_id":   "test-device",
		"interval_ms": 3000, // 3秒间隔
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
		log.Fatal().Err(err).Msg("序列化Mock配置失败")
	}

	// 初始化Mock Adapter
	if err := adapter.Init(configBytes); err != nil {
		log.Fatal().Err(err).Msg("初始化Mock Adapter失败")
	}

	return adapter
}
