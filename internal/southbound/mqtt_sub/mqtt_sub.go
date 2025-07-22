package mqtt_sub

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/southbound"
)

func init() {
	// 注册适配器工厂
	southbound.Register("mqtt_sub", func() southbound.Adapter {
		return &MQTTSubAdapter{}
	})
}

// MQTTSubAdapter 是一个MQTT订阅适配器，用于从外部MQTT代理订阅数据
type MQTTSubAdapter struct {
	*southbound.BaseAdapter
	client   mqtt.Client
	topics   []TopicConfig
	deviceID string
	stopCh   chan struct{}
	mutex    sync.Mutex
	running  bool
}

// TopicConfig 定义了要订阅的MQTT主题配置
type TopicConfig struct {
	Topic    string            `json:"topic"`          // 要订阅的主题
	QoS      byte              `json:"qos"`            // 服务质量
	Key      string            `json:"key"`            // 数据点标识符
	Type     string            `json:"type"`           // 数据类型: int, float, bool, string
	Path     string            `json:"path"`           // JSON路径，用于从消息中提取值
	DeviceID string            `json:"device_id"`      // 设备ID，如果为空则使用适配器默认值
	Tags     map[string]string `json:"tags,omitempty"` // 附加标签
}

// MQTTSubConfig 是MQTT订阅适配器的配置
type MQTTSubConfig struct {
	Name      string        `json:"name"`
	DeviceID  string        `json:"device_id"`  // 默认设备ID
	BrokerURL string        `json:"broker_url"` // MQTT代理URL
	ClientID  string        `json:"client_id"`  // 客户端ID
	Username  string        `json:"username"`   // 用户名
	Password  string        `json:"password"`   // 密码
	Topics    []TopicConfig `json:"topics"`     // 要订阅的主题列表
	// TLS配置（可选）
	CACert     string `json:"ca_cert,omitempty"`
	ClientCert string `json:"client_cert,omitempty"`
	ClientKey  string `json:"client_key,omitempty"`
}

// Name 返回适配器名称
func (a *MQTTSubAdapter) Name() string {
	return a.BaseAdapter.Name()
}

// Init 初始化适配器
func (a *MQTTSubAdapter) Init(cfg json.RawMessage) error {
	var config MQTTSubConfig
	if err := json.Unmarshal(cfg, &config); err != nil {
		return err
	}

	// 初始化BaseAdapter
	a.BaseAdapter = southbound.NewBaseAdapter(config.Name, "mqtt_sub")
	a.deviceID = config.DeviceID
	a.topics = config.Topics
	a.stopCh = make(chan struct{})

	// 创建MQTT客户端选项
	opts := mqtt.NewClientOptions()
	opts.AddBroker(config.BrokerURL)
	opts.SetClientID(config.ClientID)
	opts.SetUsername(config.Username)
	opts.SetPassword(config.Password)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetMaxReconnectInterval(30 * time.Second)
	opts.SetCleanSession(true)
	opts.SetOrderMatters(false)

	// 保存名称到本地变量，避免闭包问题
	name := a.Name()
	opts.SetOnConnectHandler(func(client mqtt.Client) {
		log.Info().Str("name", name).Msg("MQTT订阅适配器连接成功")
	})
	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		log.Error().Err(err).Str("name", name).Msg("MQTT订阅适配器连接断开")
	})

	// 配置TLS（如果有）
	if config.CACert != "" || config.ClientCert != "" || config.ClientKey != "" {
		// TODO: 实现TLS配置
		log.Warn().Str("name", a.Name()).Msg("TLS配置暂未实现")
	}

	// 创建MQTT客户端
	a.client = mqtt.NewClient(opts)

	log.Info().
		Str("name", a.Name()).
		Str("broker", config.BrokerURL).
		Int("topics", len(a.topics)).
		Msg("MQTT订阅适配器初始化完成")

	return nil
}

// Start 启动适配器
func (a *MQTTSubAdapter) Start(ctx context.Context, ch chan<- model.Point) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.running {
		return nil
	}
	a.running = true

	// 连接MQTT代理
	if token := a.client.Connect(); token.Wait() && token.Error() != nil {
		a.running = false
		return fmt.Errorf("连接MQTT代理失败: %w", token.Error())
	}

	// 订阅所有配置的主题
	for _, topicCfg := range a.topics {
		// 创建消息处理函数
		handler := a.createMessageHandler(topicCfg, ch)

		// 订阅主题
		if token := a.client.Subscribe(topicCfg.Topic, topicCfg.QoS, handler); token.Wait() && token.Error() != nil {
			log.Error().
				Err(token.Error()).
				Str("name", a.Name()).
				Str("topic", topicCfg.Topic).
				Msg("订阅MQTT主题失败")
		} else {
			log.Info().
				Str("name", a.Name()).
				Str("topic", topicCfg.Topic).
				Uint8("qos", uint8(topicCfg.QoS)).
				Msg("订阅MQTT主题成功")
		}
	}

	// 监听停止信号
	go func() {
		select {
		case <-a.stopCh:
			// 取消所有订阅
			for _, topicCfg := range a.topics {
				if token := a.client.Unsubscribe(topicCfg.Topic); token.Wait() && token.Error() != nil {
					log.Error().
						Err(token.Error()).
						Str("name", a.Name()).
						Str("topic", topicCfg.Topic).
						Msg("取消订阅MQTT主题失败")
				}
			}
			a.client.Disconnect(250) // 等待250ms完成断开
			log.Info().Str("name", a.Name()).Msg("MQTT订阅适配器停止")
		case <-ctx.Done():
			// 取消所有订阅
			for _, topicCfg := range a.topics {
				if token := a.client.Unsubscribe(topicCfg.Topic); token.Wait() && token.Error() != nil {
					log.Error().
						Err(token.Error()).
						Str("name", a.Name()).
						Str("topic", topicCfg.Topic).
						Msg("取消订阅MQTT主题失败")
				}
			}
			a.client.Disconnect(250) // 等待250ms完成断开
			log.Info().Str("name", a.Name()).Msg("MQTT订阅适配器上下文取消")
		}
	}()

	log.Info().Str("name", a.Name()).Msg("MQTT订阅适配器启动")
	return nil
}

// createMessageHandler 创建MQTT消息处理函数
func (a *MQTTSubAdapter) createMessageHandler(topicCfg TopicConfig, ch chan<- model.Point) mqtt.MessageHandler {
	return func(client mqtt.Client, msg mqtt.Message) {
		// 记录消息处理开始时间
		messageStart := time.Now()
		
		log.Debug().
			Str("name", a.Name()).
			Str("topic", msg.Topic()).
			Str("payload", string(msg.Payload())).
			Msg("收到MQTT消息")

		// 解析消息内容
		var value interface{}
		var dataType model.DataType

		// 确定设备ID
		deviceID := topicCfg.DeviceID
		if deviceID == "" {
			deviceID = a.deviceID
		}

		// 尝试解析JSON
		var jsonData map[string]interface{}
		err := json.Unmarshal(msg.Payload(), &jsonData)

		if err == nil && topicCfg.Path != "" {
			// 如果是JSON且指定了路径，尝试从JSON中提取值
			if pathValue, ok := jsonData[topicCfg.Path]; ok {
				value = pathValue
			} else {
				log.Error().
					Str("name", a.Name()).
					Str("topic", msg.Topic()).
					Str("path", topicCfg.Path).
					Msg("在JSON中找不到指定路径")
				return
			}
		} else {
			// 如果不是JSON或未指定路径，直接使用整个消息
			value = string(msg.Payload())
		}

		// 根据配置的类型转换值
		switch topicCfg.Type {
		case "int":
			switch v := value.(type) {
			case float64:
				value = int(v)
			case string:
				var intVal int
				if _, err := fmt.Sscanf(v, "%d", &intVal); err == nil {
					value = intVal
				} else {
					log.Error().
						Err(err).
						Str("name", a.Name()).
						Str("topic", msg.Topic()).
						Str("value", v).
						Msg("无法将字符串转换为整数")
					return
				}
			}
			dataType = model.TypeInt
		case "float":
			switch v := value.(type) {
			case string:
				var floatVal float64
				if _, err := fmt.Sscanf(v, "%f", &floatVal); err == nil {
					value = floatVal
				} else {
					log.Error().
						Err(err).
						Str("name", a.Name()).
						Str("topic", msg.Topic()).
						Str("value", v).
						Msg("无法将字符串转换为浮点数")
					return
				}
			}
			dataType = model.TypeFloat
		case "bool":
			switch v := value.(type) {
			case string:
				switch v {
				case "true", "1", "on", "yes":
					value = true
				case "false", "0", "off", "no":
					value = false
				default:
					log.Error().
						Str("name", a.Name()).
						Str("topic", msg.Topic()).
						Str("value", v).
						Msg("无法将字符串转换为布尔值")
					return
				}
			case float64:
				value = v != 0
			case int:
				value = v != 0
			}
			dataType = model.TypeBool
		case "string":
			if _, ok := value.(string); !ok {
				value = fmt.Sprintf("%v", value)
			}
			dataType = model.TypeString
		default:
			// 默认为字符串类型
			if _, ok := value.(string); !ok {
				value = fmt.Sprintf("%v", value)
			}
			dataType = model.TypeString
		}

		// 创建数据点
		point := model.NewPoint(topicCfg.Key, deviceID, value, dataType)

		// 添加标签
		point.AddTag("source", "mqtt_sub")
		point.AddTag("topic", msg.Topic())

		// 添加自定义标签
		for k, v := range topicCfg.Tags {
			point.AddTag(k, v)
		}

		// 使用BaseAdapter的SafeSendDataPoint方法，自动处理统计
		a.SafeSendDataPoint(ch, point, messageStart)
		
		log.Debug().
			Str("name", a.Name()).
			Str("key", topicCfg.Key).
			Str("topic", msg.Topic()).
			Interface("value", value).
			Str("type", string(dataType)).
			Msg("发送MQTT订阅数据点")
	}
}

// Stop 停止适配器
func (a *MQTTSubAdapter) Stop() error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if !a.running {
		return nil
	}

	close(a.stopCh)
	a.running = false
	return nil
}

// NewAdapter 创建一个新的MQTT订阅适配器实例
func NewAdapter() southbound.Adapter {
	return &MQTTSubAdapter{}
}
