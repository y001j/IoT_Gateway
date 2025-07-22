package bus

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
)

// 批量发布优化的对象池
var (
	messagePool = sync.Pool{
		New: func() interface{} {
			return make([]*nats.Msg, 0, 100)
		},
	}
	
	subjectPool = sync.Pool{
		New: func() interface{} {
			return make([]string, 0, 100)
		},
	}
)

// Bus 定义了内部消息总线接口
type Bus interface {
	// Publish 发布消息到指定主题
	Publish(subject string, v interface{}) error
	
	// PublishBatch 批量发布消息以提高性能
	PublishBatch(subjects []string, messages []interface{}) error
	
	// PublishAsync 异步发布消息
	PublishAsync(subject string, v interface{}) error
	
	// Subscribe 订阅指定主题的消息
	Subscribe(subject string, handler MsgHandler) (Subscription, error)
	
	// Close 关闭总线连接
	Close() error
}

// MsgHandler 是消息处理函数类型
type MsgHandler func(msg []byte) error

// Subscription 表示一个订阅
type Subscription interface {
	// Unsubscribe 取消订阅
	Unsubscribe() error
}

// NatsBus 是基于NATS的消息总线实现
type NatsBus struct {
	conn *nats.Conn
	js   nats.JetStreamContext
	mu   sync.RWMutex
	
	// 批量发布优化
	batchChannel chan batchMessage
	batchSize    int
	flushTimeout time.Duration
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

// batchMessage 批量消息结构
type batchMessage struct {
	subject string
	data    []byte
	reply   chan error
}

// NewNatsBus 创建一个新的NATS消息总线
func NewNatsBus(url string) (*NatsBus, error) {
	if url == "" {
		url = nats.DefaultURL
	}
	
	// 连接NATS服务器
	nc, err := nats.Connect(url,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(10),
		nats.ReconnectWait(5*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("连接NATS失败: %w", err)
	}
	
	// 创建JetStream上下文
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("创建JetStream失败: %w", err)
	}
	
	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	
	bus := &NatsBus{
		conn:         nc,
		js:           js,
		batchChannel: make(chan batchMessage, 1000),
		batchSize:    50,  // 默认批量大小
		flushTimeout: 50 * time.Millisecond, // 默认刷新超时
		ctx:          ctx,
		cancel:       cancel,
	}
	
	// 启动批量处理器
	bus.wg.Add(1)
	go bus.batchProcessor()
	
	return bus, nil
}

// Publish 实现Bus接口
func (b *NatsBus) Publish(subject string, v interface{}) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	if b.conn == nil {
		return fmt.Errorf("总线未连接")
	}
	
	var data []byte
	var err error
	
	switch msg := v.(type) {
	case []byte:
		data = msg
	case string:
		data = []byte(msg)
	default:
		data, err = json.Marshal(v)
		if err != nil {
			return fmt.Errorf("消息序列化失败: %w", err)
		}
	}
	
	return b.conn.Publish(subject, data)
}

// PublishBatch 批量发布消息以提高性能
func (b *NatsBus) PublishBatch(subjects []string, messages []interface{}) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	if b.conn == nil {
		return fmt.Errorf("总线未连接")
	}
	
	if len(subjects) != len(messages) {
		return fmt.Errorf("主题和消息数量不匹配")
	}
	
	// 批量序列化和发布
	for i, subject := range subjects {
		var data []byte
		var err error
		
		switch msg := messages[i].(type) {
		case []byte:
			data = msg
		case string:
			data = []byte(msg)
		default:
			data, err = json.Marshal(messages[i])
			if err != nil {
				log.Error().Err(err).Str("subject", subject).Msg("消息序列化失败")
				continue
			}
		}
		
		// 使用异步发布提高性能
		if err := b.conn.Publish(subject, data); err != nil {
			log.Error().Err(err).Str("subject", subject).Msg("发布消息失败")
		}
	}
	
	// 刷新连接确保消息发送
	return b.conn.Flush()
}

// PublishAsync 异步发布消息
func (b *NatsBus) PublishAsync(subject string, v interface{}) error {
	var data []byte
	var err error
	
	switch msg := v.(type) {
	case []byte:
		data = msg
	case string:
		data = []byte(msg)
	default:
		data, err = json.Marshal(v)
		if err != nil {
			return fmt.Errorf("消息序列化失败: %w", err)
		}
	}
	
	// 发送到批量处理通道
	reply := make(chan error, 1)
	select {
	case b.batchChannel <- batchMessage{subject: subject, data: data, reply: reply}:
		// 异步发送成功，不等待结果
		go func() {
			<-reply // 消费reply通道避免阻塞
		}()
		return nil
	case <-b.ctx.Done():
		return fmt.Errorf("总线已关闭")
	default:
		// 通道满了，回退到同步发布
		return b.Publish(subject, data)
	}
}

// batchProcessor 批量消息处理器
func (b *NatsBus) batchProcessor() {
	defer b.wg.Done()
	
	// 从对象池获取消息批次
	batch := messagePool.Get().([]*nats.Msg)
	batch = batch[:0]
	defer func() {
		batch = batch[:0]
		messagePool.Put(batch)
	}()
	
	ticker := time.NewTicker(b.flushTimeout)
	defer ticker.Stop()
	
	for {
		select {
		case <-b.ctx.Done():
			// 处理剩余消息
			if len(batch) > 0 {
				b.flushBatch(batch)
			}
			return
			
		case msg := <-b.batchChannel:
			// 构建NATS消息
			natsMsg := &nats.Msg{
				Subject: msg.subject,
				Data:    msg.data,
			}
			batch = append(batch, natsMsg)
			
			// 如果批次达到大小，立即发送
			if len(batch) >= b.batchSize {
				b.flushBatch(batch)
				batch = batch[:0]
			}
			
			// 异步回复（不阻塞）
			go func(reply chan error) {
				select {
				case reply <- nil:
				default:
				}
			}(msg.reply)
			
		case <-ticker.C:
			// 定时刷新批次
			if len(batch) > 0 {
				b.flushBatch(batch)
				batch = batch[:0]
			}
		}
	}
}

// flushBatch 刷新消息批次
func (b *NatsBus) flushBatch(batch []*nats.Msg) {
	if len(batch) == 0 {
		return
	}
	
	// 批量发布消息
	for _, msg := range batch {
		if err := b.conn.PublishMsg(msg); err != nil {
			log.Error().Err(err).Str("subject", msg.Subject).Msg("批量发布消息失败")
		}
	}
	
	// 刷新连接
	if err := b.conn.Flush(); err != nil {
		log.Error().Err(err).Msg("刷新NATS连接失败")
	}
	
	log.Debug().Int("count", len(batch)).Msg("批量发布消息完成")
}

// Subscribe 实现Bus接口
func (b *NatsBus) Subscribe(subject string, handler MsgHandler) (Subscription, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	if b.conn == nil {
		return nil, fmt.Errorf("总线未连接")
	}
	
	sub, err := b.conn.Subscribe(subject, func(msg *nats.Msg) {
		if err := handler(msg.Data); err != nil {
			log.Error().Err(err).Str("subject", subject).Msg("处理消息失败")
		}
	})
	
	if err != nil {
		return nil, fmt.Errorf("订阅主题失败: %w", err)
	}
	
	return sub, nil
}

// Close 实现Bus接口
func (b *NatsBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	// 取消上下文，停止批量处理器
	if b.cancel != nil {
		b.cancel()
	}
	
	// 等待批量处理器退出
	b.wg.Wait()
	
	// 关闭批量通道
	if b.batchChannel != nil {
		close(b.batchChannel)
	}
	
	// 关闭NATS连接
	if b.conn != nil {
		b.conn.Close()
		b.conn = nil
	}
	
	return nil
}

// PublishPoint 发布数据点到总线
func PublishPoint(bus Bus, point model.Point) error {
	subject := fmt.Sprintf("iot.data.%s.%s", point.DeviceID, point.Key)
	return bus.Publish(subject, point)
}

// SubscribePoints 订阅数据点
func SubscribePoints(ctx context.Context, bus Bus, deviceID string, handler func(point model.Point) error) (Subscription, error) {
	subject := fmt.Sprintf("iot.data.%s.>", deviceID)
	
	return bus.Subscribe(subject, func(data []byte) error {
		var point model.Point
		if err := json.Unmarshal(data, &point); err != nil {
			return fmt.Errorf("解析数据点失败: %w", err)
		}
		
		return handler(point)
	})
}
