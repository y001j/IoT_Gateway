package jetstream

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/northbound"
)

func init() {
	// 注册JetStream sink工厂
	northbound.Register("jetstream", func() northbound.Sink {
		return NewJetStreamSink()
	})
}

// NewJetStreamSink 创建一个新的JetStream连接器
func NewJetStreamSink() *JetStreamSink {
	return &JetStreamSink{
		BaseSink: northbound.NewBaseSink("jetstream"),
	}
}

// JetStreamSink 实现了使用NATS JetStream进行数据点持久化的sink
type JetStreamSink struct {
	*northbound.BaseSink
	conn       *nats.Conn
	js         nats.JetStreamContext
	streamName string
	subject    string
	buffer     []model.Point
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	mu         sync.Mutex
}

// JetStreamConfig 是JetStream连接器的特定参数配置
type JetStreamConfig struct {
	URL        string        `json:"url"`         // NATS服务器URL
	StreamName string        `json:"stream_name"` // JetStream流名称
	Subject    string        `json:"subject"`     // 发布主题
	MaxAge     time.Duration `json:"max_age"`     // 消息最大保留时间
	MaxBytes   int64         `json:"max_bytes"`   // 流最大存储字节数
	Replicas   int           `json:"replicas"`    // 副本数量
}

// Init 初始化JetStream sink
func (s *JetStreamSink) Init(cfg json.RawMessage) error {
	// 使用标准化配置解析
	standardConfig, err := s.ParseStandardConfig(cfg)
	if err != nil {
		return fmt.Errorf("解析JetStream sink配置失败: %w", err)
	}

	// 解析JetStream特定参数
	var jsConfig JetStreamConfig
	if err := json.Unmarshal(standardConfig.Params, &jsConfig); err != nil {
		return fmt.Errorf("解析JetStream特定参数失败: %w", err)
	}

	s.streamName = jsConfig.StreamName
	s.subject = jsConfig.Subject

	// 初始化缓冲区
	s.buffer = make([]model.Point, 0, s.GetBatchSize())

	// 连接NATS服务器
	url := jsConfig.URL
	if url == "" {
		url = nats.DefaultURL
	}

	s.conn, err = nats.Connect(url,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(10),
		nats.ReconnectWait(5*time.Second),
	)
	if err != nil {
		return fmt.Errorf("连接NATS服务器失败: %w", err)
	}

	// 创建JetStream上下文
	s.js, err = s.conn.JetStream()
	if err != nil {
		s.conn.Close()
		return fmt.Errorf("创建JetStream上下文失败: %w", err)
	}

	// 确保流存在
	maxAge := jsConfig.MaxAge
	if maxAge <= 0 {
		maxAge = 24 * time.Hour // 默认保留24小时
	}

	maxBytes := jsConfig.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 1024 * 1024 * 1024 // 默认1GB
	}

	replicas := jsConfig.Replicas
	if replicas <= 0 {
		replicas = 1 // 默认1个副本
	}

	// 创建或更新流
	_, err = s.js.StreamInfo(s.streamName)
	if err != nil {
		// 流不存在，创建新流
		_, err = s.js.AddStream(&nats.StreamConfig{
			Name:     s.streamName,
			Subjects: []string{s.subject},
			MaxAge:   maxAge,
			MaxBytes: maxBytes,
			Replicas: replicas,
			Storage:  nats.FileStorage,
		})
		if err != nil {
			s.conn.Close()
			return fmt.Errorf("创建JetStream流失败: %w", err)
		}
		log.Info().
			Str("name", s.Name()).
			Str("stream", s.streamName).
			Str("subject", s.subject).
			Dur("max_age", maxAge).
			Int64("max_bytes", maxBytes).
			Int("replicas", replicas).
			Msg("创建JetStream流")
	} else {
		log.Info().
			Str("name", s.Name()).
			Str("stream", s.streamName).
			Str("subject", s.subject).
			Msg("使用现有JetStream流")
	}

	log.Info().
		Str("name", s.Name()).
		Str("url", url).
		Str("stream", s.streamName).
		Str("subject", s.subject).
		Int("batch_size", s.GetBatchSize()).
		Int("buffer_size", s.GetBufferSize()).
		Msg("JetStream连接器初始化完成")

	return nil
}

// Start 启动JetStream sink
func (s *JetStreamSink) Start(ctx context.Context) error {
	s.SetRunning(true)
	s.ctx, s.cancel = context.WithCancel(ctx)

	// 启动后台刷新协程
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := s.flush(); err != nil {
					s.HandleError(err, "刷新JetStream缓冲区")
				}
			case <-s.ctx.Done():
				// 确保最后一次刷新
				if err := s.flush(); err != nil {
					s.HandleError(err, "最终刷新JetStream缓冲区")
				}
				return
			}
		}
	}()

	log.Info().Str("name", s.Name()).Msg("JetStream连接器启动")
	return nil
}

// Publish 发布数据点到JetStream
func (s *JetStreamSink) Publish(batch []model.Point) error {
	if !s.IsRunning() {
		return fmt.Errorf("JetStream连接器未启动")
	}

	if len(batch) == 0 {
		return nil
	}

	// 记录发布操作开始时间
	publishStart := time.Now()

	// 使用BaseSink的SafePublishBatch方法，自动处理统计
	return s.SafePublishBatch(batch, func(batch []model.Point) error {
		// 使用基础方法添加标签
		s.AddTags(batch)

		s.mu.Lock()
		defer s.mu.Unlock()

		// 添加到缓冲区
		s.buffer = append(s.buffer, batch...)

		// 如果缓冲区超过批处理大小，立即刷新
		if len(s.buffer) >= s.GetBatchSize() {
			return s.flushLocked()
		}

		return nil
	}, publishStart)
}

// Stop 停止JetStream sink
func (s *JetStreamSink) Stop() error {
	s.SetRunning(false)

	if s.cancel != nil {
		s.cancel()
	}

	// 等待后台协程完成
	s.wg.Wait()

	// 关闭连接
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}

	log.Info().Str("name", s.Name()).Msg("JetStream连接器停止")
	return nil
}

// Healthy 检查连接器健康状态
func (s *JetStreamSink) Healthy() error {
	if !s.IsRunning() {
		return fmt.Errorf("JetStream连接器未运行")
	}
	if s.conn == nil {
		return fmt.Errorf("NATS连接未初始化")
	}
	if !s.conn.IsConnected() {
		return fmt.Errorf("NATS连接已断开")
	}
	if s.js == nil {
		return fmt.Errorf("JetStream上下文未初始化")
	}
	
	// 检查流是否存在
	_, err := s.js.StreamInfo(s.streamName)
	if err != nil {
		return fmt.Errorf("JetStream流不存在或无法访问: %w", err)
	}
	
	return nil
}

// flush 刷新缓冲区中的数据点到JetStream
func (s *JetStreamSink) flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.flushLocked()
}

// flushLocked 在已获取锁的情况下刷新缓冲区
func (s *JetStreamSink) flushLocked() error {
	if len(s.buffer) == 0 {
		return nil
	}

	// 发布所有数据点到JetStream
	for _, point := range s.buffer {
		data, err := json.Marshal(point)
		if err != nil {
			s.HandleError(err, "序列化数据点")
			continue
		}

		// 使用JetStream发布
		_, err = s.js.Publish(s.subject, data)
		if err != nil {
			s.HandleError(err, "发布数据点到JetStream")
			continue
		}

		log.Debug().
			Str("name", s.Name()).
			Str("key", point.Key).
			Str("device_id", point.DeviceID).
			Str("subject", s.subject).
			Msg("数据点已发布到JetStream")
	}

	// 清空缓冲区
	s.buffer = s.buffer[:0]
	return nil
}

// CreateConsumer 创建一个JetStream消费者
// 这个方法可以被外部调用，用于创建消费者从JetStream中读取数据
func (s *JetStreamSink) CreateConsumer(consumerName string, deliverSubject string) (*nats.ConsumerInfo, error) {
	if s.js == nil {
		return nil, fmt.Errorf("JetStream上下文未初始化")
	}

	// 创建推送型消费者
	consumerConfig := &nats.ConsumerConfig{
		Durable:        consumerName,
		DeliverSubject: deliverSubject,
		DeliverPolicy:  nats.DeliverAllPolicy,
		AckPolicy:      nats.AckExplicitPolicy,
		AckWait:        30 * time.Second,
		MaxDeliver:     -1, // 无限重试
	}

	return s.js.AddConsumer(s.streamName, consumerConfig)
}

// SubscribeConsumer 订阅一个JetStream消费者
// 返回一个订阅对象，可以用于取消订阅
func (s *JetStreamSink) SubscribeConsumer(consumerName string, deliverSubject string, handler func(point model.Point) error) (*nats.Subscription, error) {
	if s.conn == nil {
		return nil, fmt.Errorf("NATS连接未初始化")
	}

	// 确保消费者存在
	_, err := s.CreateConsumer(consumerName, deliverSubject)
	if err != nil {
		return nil, fmt.Errorf("创建或获取消费者失败: %w", err)
	}

	// 订阅消费者的投递主题
	sub, err := s.conn.Subscribe(deliverSubject, func(msg *nats.Msg) {
		var point model.Point
		if err := json.Unmarshal(msg.Data, &point); err != nil {
			s.HandleError(err, "解析数据点")
			// 即使解析失败，也确认消息以避免无限重试
			msg.Ack()
			return
		}

		// 处理数据点
		if err := handler(point); err != nil {
			s.HandleError(err, "处理数据点")
			// 处理失败，不确认消息，让JetStream重新投递
			msg.Nak()
			return
		}

		// 处理成功，确认消息
		msg.Ack()
	})

	if err != nil {
		return nil, fmt.Errorf("订阅消费者失败: %w", err)
	}

	log.Info().
		Str("name", s.Name()).
		Str("consumer", consumerName).
		Str("deliver_subject", deliverSubject).
		Msg("已订阅JetStream消费者")

	return sub, nil
}