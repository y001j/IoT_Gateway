package mqtt_sub

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/config"
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
	parser   *config.ConfigParser[config.MQTTSubConfig]
}

// TopicConfig 定义了要订阅的MQTT主题配置
type TopicConfig struct {
	Topic     string                     `json:"topic"`                // 要订阅的主题
	QoS       byte                       `json:"qos"`                  // 服务质量
	Key       string                     `json:"key"`                  // 数据点标识符
	Type      string                     `json:"type"`                 // 数据类型: int, float, bool, string, location, vector3d, color
	Path      string                     `json:"path"`                 // JSON路径，用于从消息中提取值 (基础类型)
	DeviceID  string                     `json:"device_id"`            // 设备ID，如果为空则使用适配器默认值
	Tags      map[string]string          `json:"tags,omitempty"`       // 附加标签
	Composite *CompositeExtractConfig    `json:"composite,omitempty"`  // 复合数据提取配置
}

// CompositeExtractConfig 复合数据提取配置
type CompositeExtractConfig struct {
	Location *LocationExtractConfig `json:"location,omitempty"`
	Vector3D *Vector3DExtractConfig `json:"vector3d,omitempty"`
	Color    *ColorExtractConfig    `json:"color,omitempty"`
}

// LocationExtractConfig GPS位置提取配置
type LocationExtractConfig struct {
	LatitudePath  string `json:"latitude_path"`
	LongitudePath string `json:"longitude_path"`
	AltitudePath  string `json:"altitude_path,omitempty"`
	AccuracyPath  string `json:"accuracy_path,omitempty"`
	SpeedPath     string `json:"speed_path,omitempty"`
	HeadingPath   string `json:"heading_path,omitempty"`
}

// Vector3DExtractConfig 3D向量提取配置
type Vector3DExtractConfig struct {
	XPath string `json:"x_path"`
	YPath string `json:"y_path"`
	ZPath string `json:"z_path"`
}

// ColorExtractConfig 颜色提取配置
type ColorExtractConfig struct {
	RedPath   string `json:"red_path"`
	GreenPath string `json:"green_path"`
	BluePath  string `json:"blue_path"`
	AlphaPath string `json:"alpha_path,omitempty"`
}


// Name 返回适配器名称
func (a *MQTTSubAdapter) Name() string {
	return a.BaseAdapter.Name()
}

// Init 初始化适配器
func (a *MQTTSubAdapter) Init(cfg json.RawMessage) error {
	// 创建配置解析器
	a.parser = config.NewParserWithDefaults(config.GetDefaultMQTTSubConfig())
	
	// 解析配置
	mqttConfig, err := a.parser.Parse(cfg)
	if err != nil {
		return fmt.Errorf("解析MQTT订阅配置失败: %w", err)
	}

	return a.initWithConfig(mqttConfig)
}

// initWithConfig 使用新配置格式初始化
func (a *MQTTSubAdapter) initWithConfig(config *config.MQTTSubConfig) error {
	// 初始化BaseAdapter
	a.BaseAdapter = southbound.NewBaseAdapter(config.Name, "mqtt_sub")
	a.stopCh = make(chan struct{})

	// 转换新配置格式到内部TopicConfig格式
	a.topics = make([]TopicConfig, len(config.Topics))
	for i, topicConfig := range config.Topics {
		// 设置QoS，如果没有指定则使用默认值
		qos := topicConfig.QoS
		if qos == 0 && config.DefaultQoS > 0 {
			qos = config.DefaultQoS
		}

		// 设置数据类型，默认为string
		dataType := topicConfig.Type
		if dataType == "" {
			dataType = "string"
		}

		topic := TopicConfig{
			Topic:    topicConfig.Topic,
			QoS:      qos,
			Key:      topicConfig.Key,
			Type:     dataType,
			Path:     topicConfig.Path,
			DeviceID: topicConfig.DeviceID,
			Tags:     topicConfig.Tags,
		}
		
		// 转换复合对象配置
		if topicConfig.Composite != nil {
			topic.Composite = &CompositeExtractConfig{}
			
			if topicConfig.Composite.Location != nil {
				topic.Composite.Location = &LocationExtractConfig{
					LatitudePath:  topicConfig.Composite.Location.LatitudePath,
					LongitudePath: topicConfig.Composite.Location.LongitudePath,
					AltitudePath:  topicConfig.Composite.Location.AltitudePath,
					AccuracyPath:  topicConfig.Composite.Location.AccuracyPath,
					SpeedPath:     topicConfig.Composite.Location.SpeedPath,
					HeadingPath:   topicConfig.Composite.Location.HeadingPath,
				}
			}
			
			if topicConfig.Composite.Vector3D != nil {
				topic.Composite.Vector3D = &Vector3DExtractConfig{
					XPath: topicConfig.Composite.Vector3D.XPath,
					YPath: topicConfig.Composite.Vector3D.YPath,
					ZPath: topicConfig.Composite.Vector3D.ZPath,
				}
			}
			
			if topicConfig.Composite.Color != nil {
				topic.Composite.Color = &ColorExtractConfig{
					RedPath:   topicConfig.Composite.Color.RedPath,
					GreenPath: topicConfig.Composite.Color.GreenPath,
					BluePath:  topicConfig.Composite.Color.BluePath,
					AlphaPath: topicConfig.Composite.Color.AlphaPath,
				}
			}
		}

		a.topics[i] = topic
	}

	// 设置默认设备ID（如果配置中有的话）
	if len(a.topics) > 0 && a.topics[0].DeviceID != "" {
		a.deviceID = a.topics[0].DeviceID
	} else {
		a.deviceID = config.Name // 使用适配器名称作为默认设备ID
	}
	
	// 创建MQTT客户端选项
	opts := mqtt.NewClientOptions()
	opts.AddBroker(config.Broker)
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
	if config.TLS != nil {
		if err := a.configureTLS(opts, config.TLS); err != nil {
			return fmt.Errorf("配置TLS失败: %w", err)
		}
	}

	// 创建MQTT客户端
	a.client = mqtt.NewClient(opts)

	log.Info().
		Str("name", a.Name()).
		Str("broker", config.Broker).
		Str("device_id", a.deviceID).
		Int("topics", len(a.topics)).
		Uint8("default_qos", uint8(config.DefaultQoS)).
		Bool("tls_enabled", config.TLS != nil).
		Msg("MQTT订阅适配器初始化完成 - 新格式")

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

		if err != nil {
			// 如果不是JSON，直接使用整个消息作为字符串
			value = string(msg.Payload())
			dataType = model.TypeString
		} else {
			// 根据类型处理数据提取
			switch topicCfg.Type {
			case "location":
				value, dataType, err = a.extractLocationData(jsonData, topicCfg)
			case "vector3d":
				value, dataType, err = a.extractVector3DData(jsonData, topicCfg)
			case "color":
				value, dataType, err = a.extractColorData(jsonData, topicCfg)
			default:
				// 处理基础数据类型
				if topicCfg.Path != "" {
					// 如果指定了路径，从JSON中提取值
					extractedValue, extractErr := a.extractValue(jsonData, topicCfg.Path)
					if extractErr != nil {
						log.Error().
							Err(extractErr).
							Str("name", a.Name()).
							Str("topic", msg.Topic()).
							Str("path", topicCfg.Path).
							Msg("从JSON中提取值失败")
						return
					}
					value = extractedValue
				} else {
					// 如果未指定路径，使用整个JSON
					value = jsonData
				}
				
				// 转换基础数据类型
				dataType, value, err = a.convertBasicType(topicCfg.Type, value)
			}
		}

		if err != nil {
			log.Error().
				Err(err).
				Str("name", a.Name()).
				Str("topic", msg.Topic()).
				Str("key", topicCfg.Key).
				Msg("数据处理失败")
			return
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

// configureTLS 配置TLS选项
func (a *MQTTSubAdapter) configureTLS(opts *mqtt.ClientOptions, tlsConfig *config.TLSConfig) error {
	if tlsConfig == nil {
		return nil
	}

	// 创建TLS配置
	config := &tls.Config{
		InsecureSkipVerify: tlsConfig.SkipVerify,
	}

	// 加载CA证书
	if tlsConfig.CACert != "" {
		caCert, err := os.ReadFile(tlsConfig.CACert)
		if err != nil {
			return fmt.Errorf("读取CA证书文件失败: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return fmt.Errorf("解析CA证书失败")
		}
		config.RootCAs = caCertPool
	}

	// 加载客户端证书
	if tlsConfig.ClientCert != "" && tlsConfig.ClientKey != "" {
		cert, err := tls.LoadX509KeyPair(tlsConfig.ClientCert, tlsConfig.ClientKey)
		if err != nil {
			return fmt.Errorf("加载客户端证书失败: %w", err)
		}
		config.Certificates = []tls.Certificate{cert}
	}

	// 设置TLS配置
	opts.SetTLSConfig(config)
	
	log.Info().
		Str("name", a.Name()).
		Bool("skip_verify", tlsConfig.SkipVerify).
		Bool("has_ca_cert", tlsConfig.CACert != "").
		Bool("has_client_cert", tlsConfig.ClientCert != "").
		Msg("TLS配置完成")

	return nil
}

// convertBasicType 转换基础数据类型
func (a *MQTTSubAdapter) convertBasicType(dataType string, value interface{}) (model.DataType, interface{}, error) {
	switch dataType {
	case "int":
		switch v := value.(type) {
		case float64:
			return model.TypeInt, int(v), nil
		case string:
			var intVal int
			if _, err := fmt.Sscanf(v, "%d", &intVal); err != nil {
				return "", nil, fmt.Errorf("无法将字符串 '%s' 转换为整数", v)
			}
			return model.TypeInt, intVal, nil
		case int:
			return model.TypeInt, v, nil
		default:
			return "", nil, fmt.Errorf("无法将类型 %T 转换为整数", v)
		}
		
	case "float":
		switch v := value.(type) {
		case float64:
			return model.TypeFloat, v, nil
		case int:
			return model.TypeFloat, float64(v), nil
		case string:
			var floatVal float64
			if _, err := fmt.Sscanf(v, "%f", &floatVal); err != nil {
				return "", nil, fmt.Errorf("无法将字符串 '%s' 转换为浮点数", v)
			}
			return model.TypeFloat, floatVal, nil
		default:
			return "", nil, fmt.Errorf("无法将类型 %T 转换为浮点数", v)
		}
		
	case "bool":
		switch v := value.(type) {
		case bool:
			return model.TypeBool, v, nil
		case string:
			switch v {
			case "true", "1", "on", "yes":
				return model.TypeBool, true, nil
			case "false", "0", "off", "no":
				return model.TypeBool, false, nil
			default:
				return "", nil, fmt.Errorf("无法将字符串 '%s' 转换为布尔值", v)
			}
		case float64:
			return model.TypeBool, v != 0, nil
		case int:
			return model.TypeBool, v != 0, nil
		default:
			return "", nil, fmt.Errorf("无法将类型 %T 转换为布尔值", v)
		}
		
	case "string":
		if s, ok := value.(string); ok {
			return model.TypeString, s, nil
		}
		return model.TypeString, fmt.Sprintf("%v", value), nil
		
	default:
		// 默认为字符串类型
		if s, ok := value.(string); ok {
			return model.TypeString, s, nil
		}
		return model.TypeString, fmt.Sprintf("%v", value), nil
	}
}

// extractValue 从JSON中提取值，支持嵌套路径如 "data.temperature"
func (a *MQTTSubAdapter) extractValue(data map[string]interface{}, path string) (interface{}, error) {
	if path == "" {
		return nil, fmt.Errorf("路径不能为空")
	}
	
	// 处理嵌套路径，如 "data.temperature"
	parts := strings.Split(path, ".")
	current := data

	// 遍历路径的每一部分，直到最后一个
	for i, part := range parts {
		if i == len(parts)-1 {
			// 最后一个部分，返回值
			if value, ok := current[part]; ok {
				return value, nil
			}
			return nil, fmt.Errorf("路径 %s 不存在", path)
		}

		// 不是最后一个部分，继续向下遍历
		next, ok := current[part]
		if !ok {
			return nil, fmt.Errorf("路径 %s 的部分 %s 不存在", path, part)
		}

		// 确保下一级是一个对象
		nextMap, ok := next.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("路径 %s 的部分 %s 不是一个对象", path, part)
		}

		current = nextMap
	}

	return nil, fmt.Errorf("无效的路径: %s", path)
}

// extractFloatValue 从JSON中提取浮点数值
func (a *MQTTSubAdapter) extractFloatValue(data map[string]interface{}, path string) (float64, error) {
	if path == "" {
		return 0, nil // 可选字段为空时返回0
	}
	
	value, err := a.extractValue(data, path)
	if err != nil {
		return 0, err
	}
	
	switch v := value.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case string:
		var floatVal float64
		if _, err := fmt.Sscanf(v, "%f", &floatVal); err != nil {
			return 0, fmt.Errorf("无法将字符串 '%s' 转换为浮点数", v)
		}
		return floatVal, nil
	default:
		return 0, fmt.Errorf("无法将类型 %T 转换为浮点数", v)
	}
}

// extractLocationData 提取GPS位置数据
func (a *MQTTSubAdapter) extractLocationData(data map[string]interface{}, topicCfg TopicConfig) (interface{}, model.DataType, error) {
	if topicCfg.Composite == nil || topicCfg.Composite.Location == nil {
		return nil, "", fmt.Errorf("location类型数据点缺少复合配置")
	}
	
	config := topicCfg.Composite.Location
	
	// 提取必需字段
	lat, err := a.extractFloatValue(data, config.LatitudePath)
	if err != nil {
		return nil, "", fmt.Errorf("提取纬度失败: %w", err)
	}
	
	lon, err := a.extractFloatValue(data, config.LongitudePath)
	if err != nil {
		return nil, "", fmt.Errorf("提取经度失败: %w", err)
	}
	
	location := &model.LocationData{
		Latitude:  lat,
		Longitude: lon,
	}
	
	// 提取可选字段
	if config.AltitudePath != "" {
		if alt, err := a.extractFloatValue(data, config.AltitudePath); err == nil {
			location.Altitude = alt
		}
	}
	
	if config.AccuracyPath != "" {
		if acc, err := a.extractFloatValue(data, config.AccuracyPath); err == nil {
			location.Accuracy = acc
		}
	}
	
	if config.SpeedPath != "" {
		if speed, err := a.extractFloatValue(data, config.SpeedPath); err == nil {
			location.Speed = speed
		}
	}
	
	if config.HeadingPath != "" {
		if heading, err := a.extractFloatValue(data, config.HeadingPath); err == nil {
			location.Heading = heading
		}
	}
	
	// 验证数据
	if err := location.Validate(); err != nil {
		return nil, "", fmt.Errorf("GPS位置数据验证失败: %w", err)
	}
	
	return location, model.TypeLocation, nil
}

// extractVector3DData 提取3D向量数据
func (a *MQTTSubAdapter) extractVector3DData(data map[string]interface{}, topicCfg TopicConfig) (interface{}, model.DataType, error) {
	if topicCfg.Composite == nil || topicCfg.Composite.Vector3D == nil {
		return nil, "", fmt.Errorf("vector3d类型数据点缺少复合配置")
	}
	
	config := topicCfg.Composite.Vector3D
	
	// 提取XYZ坐标
	x, err := a.extractFloatValue(data, config.XPath)
	if err != nil {
		return nil, "", fmt.Errorf("提取X坐标失败: %w", err)
	}
	
	y, err := a.extractFloatValue(data, config.YPath)
	if err != nil {
		return nil, "", fmt.Errorf("提取Y坐标失败: %w", err)
	}
	
	z, err := a.extractFloatValue(data, config.ZPath)
	if err != nil {
		return nil, "", fmt.Errorf("提取Z坐标失败: %w", err)
	}
	
	vector := &model.Vector3D{
		X: x,
		Y: y,
		Z: z,
	}
	
	// 验证数据
	if err := vector.Validate(); err != nil {
		return nil, "", fmt.Errorf("3D向量数据验证失败: %w", err)
	}
	
	return vector, model.TypeVector3D, nil
}

// extractColorData 提取颜色数据
func (a *MQTTSubAdapter) extractColorData(data map[string]interface{}, topicCfg TopicConfig) (interface{}, model.DataType, error) {
	if topicCfg.Composite == nil || topicCfg.Composite.Color == nil {
		return nil, "", fmt.Errorf("color类型数据点缺少复合配置")
	}
	
	config := topicCfg.Composite.Color
	
	// 提取RGB值
	red, err := a.extractFloatValue(data, config.RedPath)
	if err != nil {
		return nil, "", fmt.Errorf("提取红色分量失败: %w", err)
	}
	
	green, err := a.extractFloatValue(data, config.GreenPath)
	if err != nil {
		return nil, "", fmt.Errorf("提取绿色分量失败: %w", err)
	}
	
	blue, err := a.extractFloatValue(data, config.BluePath)
	if err != nil {
		return nil, "", fmt.Errorf("提取蓝色分量失败: %w", err)
	}
	
	// 将0-255范围的RGB值转换为uint8
	r := uint8(math.Min(255, math.Max(0, red)))
	g := uint8(math.Min(255, math.Max(0, green)))
	b := uint8(math.Min(255, math.Max(0, blue)))
	
	color := &model.ColorData{
		R: r,
		G: g,
		B: b,
	}
	
	// 提取Alpha值 (可选)，将0-1范围转换为0-255
	if config.AlphaPath != "" {
		if alpha, err := a.extractFloatValue(data, config.AlphaPath); err == nil {
			color.A = uint8(math.Min(255, math.Max(0, alpha*255)))
		} else {
			color.A = 255 // 默认不透明
		}
	} else {
		color.A = 255 // 默认不透明
	}
	
	// 验证数据
	if err := color.Validate(); err != nil {
		return nil, "", fmt.Errorf("颜色数据验证失败: %w", err)
	}
	
	return color, model.TypeColor, nil
}

// NewAdapter 创建一个新的MQTT订阅适配器实例
func NewAdapter() southbound.Adapter {
	return &MQTTSubAdapter{}
}
