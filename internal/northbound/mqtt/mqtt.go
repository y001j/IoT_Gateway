package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/northbound"
)

func init() {
	// 注册连接器工厂
	northbound.Register("mqtt", func() northbound.Sink {
		return NewMQTTSink()
	})
}

// NewMQTTSink 创建一个新的MQTT连接器
func NewMQTTSink() *MQTTSink {
	return &MQTTSink{
		BaseSink: northbound.NewBaseSink("mqtt"),
		stopCh:   make(chan struct{}),
	}
}

// MQTTSink 是一个MQTT连接器，用于发布数据到MQTT服务器
type MQTTSink struct {
	*northbound.BaseSink
	client   mqtt.Client
	topicTpl string
	qos      byte
	retained bool
	pointCh  chan model.Point
	stopCh   chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	mu       sync.Mutex
}

// MQTTConfig 是MQTT连接器的特定参数配置
type MQTTConfig struct {
	Broker   string     `json:"broker"`
	ClientID string     `json:"client_id"`
	Username string     `json:"username"`
	Password string     `json:"password"`
	TopicTpl string     `json:"topic_tpl"`
	QoS      byte       `json:"qos"`
	Retained bool       `json:"retained"`
	TLS      *TLSConfig `json:"tls,omitempty"`
}

// TLSConfig 是TLS配置
type TLSConfig struct {
	CACert     string `json:"ca_cert"`
	ClientCert string `json:"client_cert"`
	ClientKey  string `json:"client_key"`
}

// StandardConfig 是标准配置格式
type StandardConfig struct {
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	BatchSize  int               `json:"batch_size,omitempty"`
	BufferSize int               `json:"buffer_size,omitempty"`
	Tags       map[string]string `json:"tags,omitempty"`
	Params     json.RawMessage   `json:"params"` // 连接器特定的参数
}

// Init 初始化连接器
func (s *MQTTSink) Init(cfg json.RawMessage) error {
	fmt.Printf("!!!! MQTT DEBUG: Init被调用, cfg长度=%d !!!!\n", len(cfg))
	fmt.Printf("!!!! MQTT DEBUG: 接收到的配置内容=%s !!!!\n", string(cfg))
	
	// 使用标准化配置解析来设置基础属性
	if _, err := s.ParseStandardConfig(cfg); err != nil {
		return fmt.Errorf("解析MQTT sink标准配置失败: %w", err)
	}

	// 解析MQTT特定参数 - 直接从params字段解析
	var tempConfig struct {
		Params MQTTConfig `json:"params"`
	}
	if err := json.Unmarshal(cfg, &tempConfig); err != nil {
		fmt.Printf("!!!! MQTT DEBUG: JSON解析失败, 错误=%v !!!!\n", err)
		return fmt.Errorf("解析MQTT配置失败: %w", err)
	}
	mqttConfig := tempConfig.Params
	fmt.Printf("!!!! MQTT DEBUG: 解析得到的MQTT配置=%+v !!!!\n", mqttConfig)

	// 设置默认值
	s.topicTpl = mqttConfig.TopicTpl
	if s.topicTpl == "" {
		s.topicTpl = "iot/data/%s/%s" // 默认主题模板: iot/data/{deviceID}/{key}
	}
	s.qos = mqttConfig.QoS
	s.retained = mqttConfig.Retained

	// 创建MQTT客户端选项
	opts := mqtt.NewClientOptions()
	opts.AddBroker(mqttConfig.Broker)
	opts.SetClientID(mqttConfig.ClientID)
	opts.SetUsername(mqttConfig.Username)
	opts.SetPassword(mqttConfig.Password)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetMaxReconnectInterval(30 * time.Second)
	opts.SetCleanSession(true)
	opts.SetOrderMatters(false)
	
	// 保存名称到本地变量，避免闭包问题
	name := s.Name()
	opts.SetOnConnectHandler(func(client mqtt.Client) {
		log.Info().Str("name", name).Msg("MQTT连接成功")
	})
	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		s.HandleError(err, "MQTT连接断开")
	})

	// 配置TLS（如果有）
	if mqttConfig.TLS != nil {
		// TODO: 实现TLS配置
		log.Warn().Str("name", s.Name()).Msg("TLS配置暂未实现")
	}

	// 创建MQTT客户端
	s.client = mqtt.NewClient(opts)
	s.pointCh = make(chan model.Point, s.GetBufferSize())

	log.Info().
		Str("name", s.Name()).
		Str("broker", mqttConfig.Broker).
		Str("topic_tpl", s.topicTpl).
		Int("batch_size", s.GetBatchSize()).
		Int("buffer_size", s.GetBufferSize()).
		Msg("MQTT连接器初始化完成")

	return nil
}

// Start 启动连接器
func (s *MQTTSink) Start(ctx context.Context) error {
	s.SetRunning(true)
	s.ctx, s.cancel = context.WithCancel(ctx)

	// 连接MQTT服务器
	if token := s.client.Connect(); token.Wait() && token.Error() != nil {
		s.HandleError(token.Error(), "连接MQTT服务器")
		return fmt.Errorf("连接MQTT服务器失败: %w", token.Error())
	}

	// 启动后台发布协程
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.publishLoop()
	}()

	log.Info().Str("name", s.Name()).Msg("MQTT连接器启动")
	return nil
}

// Publish 发布数据点
func (s *MQTTSink) Publish(batch []model.Point) error {
	if !s.IsRunning() {
		return fmt.Errorf("MQTT连接器未启动")
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

		for _, point := range batch {
			select {
			case s.pointCh <- point:
				// 成功加入缓冲区
				log.Debug().
					Str("name", s.Name()).
					Str("device_id", point.DeviceID).
					Str("key", point.Key).
					Msg("数据点已加入发布队列")
			default:
				return fmt.Errorf("MQTT缓冲区已满，丢弃数据点")
			}
		}

		return nil
	}, publishStart)
}

// Stop 停止连接器
func (s *MQTTSink) Stop() error {
	s.SetRunning(false)

	if s.cancel != nil {
		s.cancel()
	}

	// 关闭通道
	close(s.stopCh)

	// 断开MQTT连接
	if s.client != nil && s.client.IsConnected() {
		s.client.Disconnect(250) // 等待250ms完成断开
	}

	// 等待协程完成
	s.wg.Wait()

	log.Info().Str("name", s.Name()).Msg("MQTT连接器停止")
	return nil
}

// publishLoop 是后台发布循环
func (s *MQTTSink) publishLoop() {
	log.Info().Str("name", s.Name()).Msg("启动MQTT发布循环")
	
	for {
		select {
		case point := <-s.pointCh:
			// 构建主题
			topic := fmt.Sprintf(s.topicTpl, point.DeviceID, point.Key)
			
			// 根据数据点类型处理值
			var finalValue interface{}
			switch point.Type {
			case model.TypeInt:
				switch v := point.Value.(type) {
				case float64:
					finalValue = int(v)
				case int:
					finalValue = v
				default:
					finalValue = 0
				}
			case model.TypeFloat:
				switch v := point.Value.(type) {
				case float64:
					finalValue = v
				case int:
					finalValue = float64(v)
				default:
					finalValue = 0.0
				}
			case model.TypeBool:
				if v, ok := point.Value.(bool); ok {
					finalValue = v
				} else {
					finalValue = false
				}
			case model.TypeString:
				if v, ok := point.Value.(string); ok {
					finalValue = v
				} else {
					finalValue = fmt.Sprintf("%v", point.Value)
				}
			default:
				finalValue = point.Value
			}
			
			// 创建完整的数据结构，包含所有字段
			fullData := map[string]interface{}{
				"device_id": point.DeviceID,
				"key":       point.Key,
				"value":     finalValue,
				"type":      point.Type,
				"timestamp": point.Timestamp,
				"tags":      point.GetTagsCopy(), // 保留tags信息
			}
			
			// 序列化完整数据结构
			payload, err := json.Marshal(fullData)
			if err != nil {
				s.HandleError(err, "序列化数据点值")
				continue
			}

			// 发布消息
			token := s.client.Publish(topic, s.qos, s.retained, payload)
			if token.Wait() && token.Error() != nil {
				s.HandleError(token.Error(), "发布MQTT消息")
			} else {
				log.Debug().
					Str("name", s.Name()).
					Str("device_id", point.DeviceID).
					Str("key", point.Key).
					Str("topic", topic).
					Interface("tags", point.GetTagsCopy()).
					Interface("full_data", fullData).
					Msg("成功发布数据点到MQTT")
			}

		case <-s.stopCh:
			return

		case <-s.ctx.Done():
			return
		}
	}
}

// Healthy 检查连接器健康状态
func (s *MQTTSink) Healthy() error {
	if !s.IsRunning() {
		return fmt.Errorf("MQTT连接器未运行")
	}
	if s.client == nil || !s.client.IsConnected() {
		return fmt.Errorf("MQTT客户端未连接")
	}
	return nil
}