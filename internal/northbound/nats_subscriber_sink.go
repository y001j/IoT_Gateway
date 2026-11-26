package northbound

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
)

// NATSSubscriberSink 是一个可配置的NATS订阅器sink
// 它可以从NATS总线订阅不同类型的数据：原始数据、规则数据、告警数据
type NATSSubscriberSink struct {
	*BaseSink
	conn          *nats.Conn
	subscriptions []*nats.Subscription
	config        *NATSSubscriberConfig
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	dataChan      chan model.Point
	targetSinks   []Sink // 目标sink列表
	mu            sync.RWMutex
}

// NATSSubscriberConfig NATS订阅器配置
type NATSSubscriberConfig struct {
	// 订阅配置
	Subscriptions []SubscriptionConfig `json:"subscriptions"`

	// 数据处理配置
	BufferSize    int           `json:"buffer_size"`    // 内部缓冲区大小
	BatchSize     int           `json:"batch_size"`     // 批处理大小
	FlushInterval time.Duration `json:"flush_interval"` // 刷新间隔

	// 目标sink配置
	TargetSinks []SinkConfig `json:"target_sinks"` // 目标sink列表

	// 消息过滤配置
	FilterRules []FilterRule `json:"filter_rules,omitempty"` // 消息过滤规则
}

// SubscriptionConfig 订阅配置
type SubscriptionConfig struct {
	Subject    string            `json:"subject"`               // NATS主题
	QueueGroup string            `json:"queue_group,omitempty"` // 队列组名
	DataType   string            `json:"data_type"`             // 数据类型: "raw", "rule", "alert", "system"
	Enabled    bool              `json:"enabled"`               // 是否启用
	Transform  *TransformConfig  `json:"transform,omitempty"`   // 数据转换配置
	Tags       map[string]string `json:"tags,omitempty"`        // 附加标签
}

// TransformConfig 数据转换配置
type TransformConfig struct {
	DeviceIDField  string            `json:"device_id_field,omitempty"` // 设备ID字段映射
	KeyField       string            `json:"key_field,omitempty"`       // 键字段映射
	ValueField     string            `json:"value_field,omitempty"`     // 值字段映射
	TimestampField string            `json:"timestamp_field,omitempty"` // 时间戳字段映射
	StaticTags     map[string]string `json:"static_tags,omitempty"`     // 静态标签
}

// SinkConfig 目标sink配置
type SinkConfig struct {
	Name   string          `json:"name"`
	Type   string          `json:"type"`
	Params json.RawMessage `json:"params"`
}

// FilterRule 过滤规则
type FilterRule struct {
	Field    string      `json:"field"`    // 字段名
	Operator string      `json:"operator"` // 操作符: "eq", "ne", "gt", "lt", "contains", "regex"
	Value    interface{} `json:"value"`    // 比较值
	Action   string      `json:"action"`   // 动作: "include", "exclude"
}

func init() {
	// 注册NATS订阅器sink工厂
	Register("nats_subscriber", func() Sink {
		return NewNATSSubscriberSink()
	})
}

// NewNATSSubscriberSink 创建新的NATS订阅器sink
func NewNATSSubscriberSink() *NATSSubscriberSink {
	return &NATSSubscriberSink{
		BaseSink: NewBaseSink("nats_subscriber"),
		dataChan: make(chan model.Point, 1000),
	}
}

// Init 初始化NATS订阅器sink
func (s *NATSSubscriberSink) Init(cfg json.RawMessage) error {
	// 解析标准配置
	standardConfig, err := s.ParseStandardConfig(cfg)
	if err != nil {
		return fmt.Errorf("解析NATS订阅器配置失败: %w", err)
	}

	// 解析NATS订阅器特定配置
	var natsConfig NATSSubscriberConfig
	if err := json.Unmarshal(standardConfig.Params, &natsConfig); err != nil {
		return fmt.Errorf("解析NATS订阅器特定配置失败: %w", err)
	}

	s.config = &natsConfig

	// 设置默认值
	if s.config.BufferSize <= 0 {
		s.config.BufferSize = 1000
	}
	if s.config.BatchSize <= 0 {
		s.config.BatchSize = 50
	}
	if s.config.FlushInterval <= 0 {
		s.config.FlushInterval = 1 * time.Second
	}

	// 创建目标sink
	s.targetSinks = make([]Sink, 0, len(s.config.TargetSinks))
	for _, sinkConfig := range s.config.TargetSinks {
		targetSink, exists := Create(sinkConfig.Type)
		if !exists {
			return fmt.Errorf("未知的sink类型: %s", sinkConfig.Type)
		}

		// 创建目标sink的配置
		targetConfig := Config{
			Name:   sinkConfig.Name,
			Type:   sinkConfig.Type,
			Params: sinkConfig.Params,
		}

		targetConfigData, err := json.Marshal(targetConfig)
		if err != nil {
			return fmt.Errorf("序列化目标sink配置失败: %w", err)
		}

		if err := targetSink.Init(targetConfigData); err != nil {
			return fmt.Errorf("初始化目标sink %s 失败: %w", sinkConfig.Name, err)
		}

		s.targetSinks = append(s.targetSinks, targetSink)
	}

	log.Info().
		Str("name", s.Name()).
		Int("subscriptions", len(s.config.Subscriptions)).
		Int("target_sinks", len(s.targetSinks)).
		Msg("NATS订阅器sink初始化完成")

	return nil
}

// SetNATSConnection 设置NATS连接
func (s *NATSSubscriberSink) SetNATSConnection(conn *nats.Conn) {
	s.conn = conn
}

// Start 启动NATS订阅器sink
func (s *NATSSubscriberSink) Start(ctx context.Context) error {
	if s.conn == nil {
		return fmt.Errorf("NATS连接未设置")
	}

	s.SetRunning(true)
	s.ctx, s.cancel = context.WithCancel(ctx)

	// 启动所有目标sink
	for _, targetSink := range s.targetSinks {
		if err := targetSink.Start(s.ctx); err != nil {
			return fmt.Errorf("启动目标sink失败: %w", err)
		}
	}

	// 创建订阅
	if err := s.createSubscriptions(); err != nil {
		return fmt.Errorf("创建NATS订阅失败: %w", err)
	}

	// 启动数据处理协程
	s.wg.Add(1)
	go s.processData()

	log.Info().
		Str("name", s.Name()).
		Int("subscriptions", len(s.subscriptions)).
		Msg("NATS订阅器sink启动完成")

	return nil
}

// createSubscriptions 创建NATS订阅
func (s *NATSSubscriberSink) createSubscriptions() error {
	for _, subConfig := range s.config.Subscriptions {
		if !subConfig.Enabled {
			continue
		}

		var sub *nats.Subscription
		var err error

		handler := s.createMessageHandler(subConfig)

		if subConfig.QueueGroup != "" {
			// 队列订阅
			sub, err = s.conn.QueueSubscribe(subConfig.Subject, subConfig.QueueGroup, handler)
		} else {
			// 普通订阅
			sub, err = s.conn.Subscribe(subConfig.Subject, handler)
		}

		if err != nil {
			return fmt.Errorf("订阅主题 %s 失败: %w", subConfig.Subject, err)
		}

		s.subscriptions = append(s.subscriptions, sub)

		log.Info().
			Str("name", s.Name()).
			Str("subject", subConfig.Subject).
			Str("queue_group", subConfig.QueueGroup).
			Str("data_type", subConfig.DataType).
			Msg("创建NATS订阅成功")
	}

	return nil
}

// createMessageHandler 创建消息处理器
func (s *NATSSubscriberSink) createMessageHandler(subConfig SubscriptionConfig) nats.MsgHandler {
	return func(msg *nats.Msg) {
		// 🔍 调试：记录接收到的原始NATS消息内容
		log.Debug().
			Str("name", s.Name()).
			Str("subject", msg.Subject).
			Str("raw_data", string(msg.Data)).
			Int("data_size", len(msg.Data)).
			Msg("🔍 NATS订阅器接收到原始消息")

		// 根据数据类型解析消息
		point, err := s.parseMessage(msg.Data, subConfig)
		if err != nil {
			s.HandleError(err, fmt.Sprintf("解析%s消息", subConfig.DataType))
			return
		}

		// 应用过滤规则
		if !s.shouldProcessPoint(point) {
			return
		}

		// 发送到数据处理通道
		select {
		case s.dataChan <- point:
		case <-s.ctx.Done():
			return
		default:
			// 通道满，记录警告
			log.Warn().
				Str("name", s.Name()).
				Str("subject", subConfig.Subject).
				Msg("数据处理通道已满，丢弃消息")
		}
	}
}

// parseMessage 解析消息
func (s *NATSSubscriberSink) parseMessage(data []byte, subConfig SubscriptionConfig) (model.Point, error) {
	var point model.Point

	switch subConfig.DataType {
	case "raw":
		// 原始数据点格式
		if err := json.Unmarshal(data, &point); err != nil {
			return point, fmt.Errorf("解析原始数据失败: %w", err)
		}

	case "rule":
		// 规则处理后的数据 - 解析规则引擎事件格式
		var ruleEvent map[string]interface{}
		if err := json.Unmarshal(data, &ruleEvent); err != nil {
			return point, fmt.Errorf("解析规则事件数据失败: %w", err)
		}

		// 规则事件（iot.rules.matched 等）把数据点放在 data_point 字段中；
		// 规则输出（iot.rules.forward.* / iot.rules.aggregate.* / transform 输出）是扁平的数据点格式
		if dataPointData, exists := ruleEvent["data_point"]; exists {
			dataPointMap, ok := dataPointData.(map[string]interface{})
			if !ok {
				return point, fmt.Errorf("规则事件中data_point格式无效")
			}
			point = s.convertMapToPoint(dataPointMap)
		} else if _, hasKey := ruleEvent["key"]; hasKey {
			point = s.convertMapToPoint(ruleEvent)
		} else {
			return point, fmt.Errorf("规则消息中既没有data_point字段也不是数据点格式")
		}

	case "alert":
		// 告警数据
		var alertData map[string]interface{}
		if err := json.Unmarshal(data, &alertData); err != nil {
			return point, fmt.Errorf("解析告警数据失败: %w", err)
		}

		// 将告警数据转换为数据点
		point = s.convertAlertToPoint(alertData)

	case "system":
		// 系统数据
		var systemData map[string]interface{}
		if err := json.Unmarshal(data, &systemData); err != nil {
			return point, fmt.Errorf("解析系统数据失败: %w", err)
		}

		// 将系统数据转换为数据点
		point = s.convertSystemToPoint(systemData)

	default:
		return point, fmt.Errorf("未知的数据类型: %s", subConfig.DataType)
	}

	// 应用数据转换
	if subConfig.Transform != nil {
		point = s.applyTransform(point, subConfig.Transform)
	}

	// 添加订阅配置中的标签
	if subConfig.Tags != nil {
		// 记录标签合并过程

		for k, v := range subConfig.Tags {
			// Go 1.24安全：使用AddTag方法替代直接Tags[]访问
			point.AddTag(k, v)
		}

		// 记录合并后的标签
		mergedTags := point.GetTagsCopy()
		log.Debug().
			Str("name", s.Name()).
			Str("subject", subConfig.Subject).
			Interface("merged_tags", mergedTags).
			Msg("🏷️ NATS订阅器标签合并完成")
	}

	return point, nil
}

// convertMapToPoint 将map数据转换为model.Point结构
func (s *NATSSubscriberSink) convertMapToPoint(dataMap map[string]interface{}) model.Point {
	point := model.Point{
		Timestamp: time.Now(), // 默认时间戳
	}

	// 提取基础字段
	if deviceID, ok := dataMap["device_id"].(string); ok {
		point.DeviceID = deviceID
	}
	if key, ok := dataMap["key"].(string); ok {
		point.Key = key
	}
	if value := dataMap["value"]; value != nil {
		point.Value = value
	}
	if pointType, ok := dataMap["type"].(string); ok {
		point.Type = model.DataType(pointType)
	}

	// 处理时间戳
	if timestampStr, ok := dataMap["timestamp"].(string); ok {
		if parsedTime, err := time.Parse(time.RFC3339Nano, timestampStr); err == nil {
			point.Timestamp = parsedTime
		}
	}

	// 处理标签
	if tagsData, exists := dataMap["tags"]; exists {
		if tagsMap, ok := tagsData.(map[string]interface{}); ok {
			for k, v := range tagsMap {
				if strVal, ok := v.(string); ok {
					point.AddTag(k, strVal)
				} else {
					// 对于非string类型的标签值，转换为字符串
					point.AddTag(k, fmt.Sprintf("%v", v))
				}
			}
		}
	}

	log.Debug().
		Str("name", s.Name()).
		Str("device_id", point.DeviceID).
		Str("key", point.Key).
		Interface("value", point.Value).
		Interface("tags", point.GetTagsCopy()).
		Msg("✅ 成功转换规则数据为数据点")

	return point
}

// convertAlertToPoint 将告警数据转换为数据点
func (s *NATSSubscriberSink) convertAlertToPoint(alertData map[string]interface{}) model.Point {
	point := model.Point{
		DeviceID:  "system",
		Key:       "alert",
		Type:      "alert",
		Timestamp: time.Now(),
	}
	// 初始化SafeTags并添加标签
	point.AddTag("source", "alert")
	point.AddTag("data_type", "alert")

	// 提取告警信息
	if id, ok := alertData["id"].(string); ok {
		point.DeviceID = id
	}
	if level, ok := alertData["level"].(string); ok {
		// Go 1.24安全：使用AddTag方法替代直接Tags[]访问
		point.AddTag("level", level)
	}
	if message, ok := alertData["message"].(string); ok {
		point.Value = message
	}
	if deviceID, ok := alertData["device_id"].(string); ok && deviceID != "" {
		point.DeviceID = deviceID
	}

	return point
}

// convertSystemToPoint 将系统数据转换为数据点
func (s *NATSSubscriberSink) convertSystemToPoint(systemData map[string]interface{}) model.Point {
	point := model.Point{
		DeviceID:  "system",
		Key:       "system_event",
		Type:      "system",
		Timestamp: time.Now(),
	}
	// 初始化SafeTags并添加标签
	point.AddTag("source", "system")
	point.AddTag("data_type", "system")

	// 提取系统事件信息
	if eventType, ok := systemData["event_type"].(string); ok {
		point.Key = eventType
	}
	if message, ok := systemData["message"].(string); ok {
		point.Value = message
	}

	return point
}

// applyTransform 应用数据转换
func (s *NATSSubscriberSink) applyTransform(point model.Point, transform *TransformConfig) model.Point {
	// 应用静态标签
	if transform.StaticTags != nil {
		for k, v := range transform.StaticTags {
			// Go 1.24安全：使用AddTag方法替代直接Tags[]访问
			point.AddTag(k, v)
		}
	}

	// 其他转换逻辑可以在这里实现
	return point
}

// shouldProcessPoint 检查数据点是否应该被处理
func (s *NATSSubscriberSink) shouldProcessPoint(point model.Point) bool {
	for _, rule := range s.config.FilterRules {
		if !s.evaluateFilterRule(point, rule) {
			return false
		}
	}
	return true
}

// evaluateFilterRule 评估过滤规则
func (s *NATSSubscriberSink) evaluateFilterRule(point model.Point, rule FilterRule) bool {
	// 获取字段值
	var fieldValue interface{}
	switch rule.Field {
	case "device_id":
		fieldValue = point.DeviceID
	case "key":
		fieldValue = point.Key
	case "value":
		fieldValue = point.Value
	case "type":
		fieldValue = point.Type
	default:
		// Go 1.24安全：使用GetTag方法替代直接Tags[]访问
		if val, exists := point.GetTag(rule.Field); exists {
			fieldValue = val
		}
	}

	// 评估条件（以前 contains 实际做的是相等比较；eq 用于 type 字段时类型不同永远不相等）
	match := false
	fs, rs := fmt.Sprintf("%v", fieldValue), fmt.Sprintf("%v", rule.Value)
	fn, fok := ToFloat64(fieldValue)
	rn, rok := ToFloat64(rule.Value)
	switch rule.Operator {
	case "eq":
		if fok && rok {
			match = fn == rn
		} else {
			match = fieldValue != nil && fs == rs
		}
	case "ne":
		if fok && rok {
			match = fn != rn
		} else {
			match = fieldValue == nil || fs != rs
		}
	case "gt":
		match = fok && rok && fn > rn
	case "lt":
		match = fok && rok && fn < rn
	case "contains":
		match = fieldValue != nil && strings.Contains(fs, rs)
	case "regex":
		if re, err := regexp.Compile(rs); err == nil && fieldValue != nil {
			match = re.MatchString(fs)
		}
	}

	// 根据动作返回结果
	if rule.Action == "include" {
		return match
	} else if rule.Action == "exclude" {
		return !match
	}

	return true
}

// processData 处理数据
func (s *NATSSubscriberSink) processData() {
	defer s.wg.Done()

	batch := make([]model.Point, 0, s.config.BatchSize)
	ticker := time.NewTicker(s.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case point := <-s.dataChan:
			batch = append(batch, point)

			// 检查是否达到批处理大小
			if len(batch) >= s.config.BatchSize {
				s.publishBatch(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			// 定期刷新
			if len(batch) > 0 {
				s.publishBatch(batch)
				batch = batch[:0]
			}

		case <-s.ctx.Done():
			// 最后一次刷新
			if len(batch) > 0 {
				s.publishBatch(batch)
			}
			return
		}
	}
}

// publishBatch 发布批量数据到目标sink
func (s *NATSSubscriberSink) publishBatch(batch []model.Point) {
	if len(batch) == 0 {
		return
	}

	// 发送到所有目标sink
	for _, targetSink := range s.targetSinks {
		// 记录发送前的数据点详情
		for _, point := range batch {
			log.Debug().
				Str("name", s.Name()).
				Str("target_sink", targetSink.Name()).
				Str("device_id", point.DeviceID).
				Str("key", point.Key).
				Interface("tags", point.GetTagsCopy()).
				Interface("value", point.Value).
				Msg("发送数据点到目标sink")
		}

		// 每个目标 Sink 使用独立副本（Sink 会修改标签，调用方还会复用 batch）
		if err := targetSink.Publish(cloneBatch(batch)); err != nil {
			s.HandleError(err, fmt.Sprintf("发送数据到目标sink %s", targetSink.Name()))
		}
	}

	// 更新统计信息
	s.IncrementMessageCount()

	log.Debug().
		Str("name", s.Name()).
		Int("batch_size", len(batch)).
		Int("target_sinks", len(s.targetSinks)).
		Msg("批量数据发送完成")
}

// Publish 发布数据点（NATSSubscriberSink主要用于订阅，但也可以接受数据）
func (s *NATSSubscriberSink) Publish(batch []model.Point) error {
	// NATS订阅器sink主要设计用于订阅数据
	// 但为了兼容插件管理器的统一接口，我们可以接受数据但不做任何处理
	// 或者将数据转发到配置的目标sinks

	if len(batch) == 0 {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// 如果有配置目标sinks，将数据转发到它们
	if len(s.targetSinks) > 0 {
		for _, targetSink := range s.targetSinks {
			if err := targetSink.Publish(cloneBatch(batch)); err != nil {
				log.Error().
					Err(err).
					Str("target_sink", targetSink.Name()).
					Str("subscriber_sink", s.Name()).
					Msg("转发数据到目标sink失败")
			}
		}
		return nil
	}

	// 如果没有目标sinks，只记录调试信息并忽略数据
	log.Debug().
		Str("name", s.Name()).
		Int("batch_size", len(batch)).
		Msg("NATS订阅器sink收到数据，但没有配置目标sinks，忽略数据")

	return nil
}

// Stop 停止NATS订阅器sink
func (s *NATSSubscriberSink) Stop() error {
	s.SetRunning(false)

	// 取消订阅
	for _, sub := range s.subscriptions {
		if err := sub.Unsubscribe(); err != nil {
			log.Error().Err(err).Msg("取消NATS订阅失败")
		}
	}

	// 停止上下文
	if s.cancel != nil {
		s.cancel()
	}

	// 等待处理协程完成
	s.wg.Wait()

	// 停止所有目标sink
	for _, targetSink := range s.targetSinks {
		if err := targetSink.Stop(); err != nil {
			log.Error().Err(err).Str("sink", targetSink.Name()).Msg("停止目标sink失败")
		}
	}

	log.Info().Str("name", s.Name()).Msg("NATS订阅器sink停止完成")
	return nil
}

// Healthy 检查sink健康状态
func (s *NATSSubscriberSink) Healthy() error {
	if !s.IsRunning() {
		return fmt.Errorf("NATS订阅器sink未运行")
	}

	if s.conn == nil {
		return fmt.Errorf("NATS连接未设置")
	}

	if !s.conn.IsConnected() {
		return fmt.Errorf("NATS连接已断开")
	}

	// 检查目标sink健康状态
	for _, targetSink := range s.targetSinks {
		if extendedSink, ok := targetSink.(ExtendedSink); ok {
			if err := extendedSink.Healthy(); err != nil {
				return fmt.Errorf("目标sink %s 不健康: %w", targetSink.Name(), err)
			}
		}
	}

	return nil
}

// cloneBatch 深拷贝一批数据点（包括标签容器）
func cloneBatch(batch []model.Point) []model.Point {
	out := make([]model.Point, len(batch))
	for i := range batch {
		out[i] = batch[i].Clone()
	}
	return out
}

// subscribedMessageParser 只用于解析消息（parseMessage 不读写连接器状态）
var subscribedMessageParser = NewNATSSubscriberSink()

// ParseSubscribedMessage 把订阅到的 NATS 消息解析为数据点。dataType 为 raw（数据点 JSON）、
// rule（规则输出或规则事件）、alert（告警）或 system，规则与 nats_subscriber 一致。
// 普通连接器配置 subscriptions 时使用。
func ParseSubscribedMessage(data []byte, dataType string) (model.Point, error) {
	return subscribedMessageParser.parseMessage(data, SubscriptionConfig{DataType: dataType})
}
