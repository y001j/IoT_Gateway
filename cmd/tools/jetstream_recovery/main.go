package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/northbound"
)

var (
	natsURL      = flag.String("nats", "nats://localhost:4222", "NATS服务器URL")
	streamName   = flag.String("stream", "iot_data", "JetStream流名称")
	subject      = flag.String("subject", "iot.data.>", "订阅主题")
	consumerName = flag.String("consumer", "recovery-consumer", "消费者名称")
	sinkType     = flag.String("sink", "", "目标sink类型 (influxdb, redis, websocket等)")
	sinkConfig   = flag.String("config", "", "目标sink配置文件路径")
	batchSize    = flag.Int("batch", 100, "批处理大小")
	logLevel     = flag.String("log", "info", "日志级别 (debug, info, warn, error)")
)

func main() {
	flag.Parse()

	// 设置日志
	level, err := zerolog.ParseLevel(*logLevel)
	if err != nil {
		fmt.Printf("无效的日志级别: %s\n", *logLevel)
		os.Exit(1)
	}
	zerolog.SetGlobalLevel(level)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	if *sinkType == "" || *sinkConfig == "" {
		log.Fatal().Msg("必须指定sink类型和配置文件")
	}

	// 读取sink配置
	configData, err := os.ReadFile(*sinkConfig)
	if err != nil {
		log.Fatal().Err(err).Str("path", *sinkConfig).Msg("读取配置文件失败")
	}

	// 创建目标sink
	sink, ok := northbound.Create(*sinkType)
	if !ok {
		log.Fatal().Str("type", *sinkType).Msg("不支持的sink类型")
	}

	// 初始化sink
	if err := sink.Init(configData); err != nil {
		log.Fatal().Err(err).Str("type", *sinkType).Msg("初始化sink失败")
	}

	// 启动sink
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sink.Start(ctx); err != nil {
		log.Fatal().Err(err).Str("type", *sinkType).Msg("启动sink失败")
	}
	defer sink.Stop()

	// 连接NATS
	nc, err := nats.Connect(*natsURL,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(10),
		nats.ReconnectWait(5*time.Second),
	)
	if err != nil {
		log.Fatal().Err(err).Str("url", *natsURL).Msg("连接NATS失败")
	}
	defer nc.Close()

	// 创建JetStream上下文
	js, err := nc.JetStream()
	if err != nil {
		log.Fatal().Err(err).Msg("创建JetStream上下文失败")
	}

	// 创建或获取消费者
	consumerConfig := &nats.ConsumerConfig{
		Durable:       *consumerName,
		DeliverPolicy: nats.DeliverAllPolicy,
		AckPolicy:     nats.AckExplicitPolicy,
		AckWait:       30 * time.Second,
		MaxDeliver:    -1, // 无限重试
		FilterSubject: *subject,
	}

	_, err = js.AddConsumer(*streamName, consumerConfig)
	if err != nil {
		log.Fatal().Err(err).Str("stream", *streamName).Str("consumer", *consumerName).Msg("创建消费者失败")
	}

	// 创建拉取订阅
	sub, err := js.PullSubscribe(*subject, *consumerName, nats.Bind(*streamName, *consumerName))
	if err != nil {
		log.Fatal().Err(err).Str("subject", *subject).Str("consumer", *consumerName).Msg("创建订阅失败")
	}

	log.Info().
		Str("nats_url", *natsURL).
		Str("stream", *streamName).
		Str("subject", *subject).
		Str("consumer", *consumerName).
		Str("sink_type", *sinkType).
		Msg("开始从JetStream恢复数据")

	// 处理信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// 统计信息
	var (
		processed  int64
		failed     int64
		lastReport time.Time = time.Now()
		mu         sync.Mutex
	)

	// 启动后台报告协程
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				mu.Lock()
				proc := processed
				fail := failed
				mu.Unlock()
				log.Info().
					Int64("processed", proc).
					Int64("failed", fail).
					Msg("恢复进度")
			case <-ctx.Done():
				return
			}
		}
	}()

	// 主处理循环
	batch := make([]model.Point, 0, *batchSize)
	running := true

	for running {
		select {
		case <-sigCh:
			log.Info().Msg("接收到终止信号，正在优雅关闭...")
			running = false
			continue
		default:
			// 继续处理
		}

		// 拉取消息
		msgs, err := sub.Fetch(*batchSize, nats.MaxWait(1*time.Second))
		if err != nil {
			if err == nats.ErrTimeout {
				// 超时是正常的，继续尝试
				continue
			}
			log.Error().Err(err).Msg("拉取消息失败")
			time.Sleep(1 * time.Second)
			continue
		}

		if len(msgs) == 0 {
			continue
		}

		// 处理消息
		batch = batch[:0] // 清空批处理
		for _, msg := range msgs {
			var point model.Point
			if err := json.Unmarshal(msg.Data, &point); err != nil {
				log.Error().Err(err).Msg("解析数据点失败")
				msg.Ack() // 确认消息以避免无限重试
				mu.Lock()
				failed++
				mu.Unlock()
				continue
			}

			batch = append(batch, point)

			// 如果批处理已满或这是最后一条消息，发送到sink
			if len(batch) >= *batchSize {
				if err := sink.Publish(batch); err != nil {
					log.Error().Err(err).Int("count", len(batch)).Msg("发布数据点到sink失败")
					// 不确认消息，让JetStream重新投递
					for _, m := range msgs {
						m.Nak()
					}
					mu.Lock()
					failed += int64(len(batch))
					mu.Unlock()
				} else {
					// 确认所有消息
					for _, m := range msgs {
						m.Ack()
					}
					mu.Lock()
					processed += int64(len(batch))
					mu.Unlock()
				}
				batch = batch[:0] // 清空批处理
			}
		}

		// 处理剩余的批处理
		if len(batch) > 0 {
			if err := sink.Publish(batch); err != nil {
				log.Error().Err(err).Int("count", len(batch)).Msg("发布数据点到sink失败")
				// 不确认消息，让JetStream重新投递
				for _, m := range msgs[len(msgs)-len(batch):] {
					m.Nak()
				}
				mu.Lock()
				failed += int64(len(batch))
				mu.Unlock()
			} else {
				// 确认所有消息
				for _, m := range msgs[len(msgs)-len(batch):] {
					m.Ack()
				}
				mu.Lock()
				processed += int64(len(batch))
				mu.Unlock()
			}
		}

		// 更新统计信息
		if time.Since(lastReport) > 5*time.Second {
			mu.Lock()
			proc := processed
			fail := failed
			mu.Unlock()
			log.Info().
				Int64("processed", proc).
				Int64("failed", fail).
				Msg("恢复进度")
			lastReport = time.Now()
		}
	}

	log.Info().Msg("恢复工具已完成")
}
