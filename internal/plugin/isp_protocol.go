package plugin

import (
	"encoding/json"
	"time"
)

// ISP (IoT Sidecar Protocol) 消息类型
const (
	MessageTypeConfig    = "CONFIG"
	MessageTypeData      = "DATA"
	MessageTypeStatus    = "STATUS"
	MessageTypeHeartbeat = "HEARTBEAT"
	MessageTypeResponse  = "RESPONSE"
	MessageTypeMetrics   = "METRICS"
)

// ISPMessage ISP协议基础消息结构
type ISPMessage struct {
	Type      string          `json:"type"`              // 消息类型
	ID        string          `json:"id,omitempty"`      // 消息ID（用于请求/响应匹配）
	Timestamp int64           `json:"timestamp"`         // 时间戳
	Payload   json.RawMessage `json:"payload,omitempty"` // 消息载荷
}

// ConfigPayload 配置消息载荷
type ConfigPayload struct {
	Mode       string                 `json:"mode"`            // tcp/rtu
	Address    string                 `json:"address"`         // 连接地址
	TimeoutMS  int                    `json:"timeout_ms"`      // 超时时间
	IntervalMS int                    `json:"interval_ms"`     // 采样间隔
	Registers  []RegisterConfig       `json:"registers"`       // 寄存器配置
	Extra      map[string]interface{} `json:"extra,omitempty"` // 扩展配置
}

// RegisterConfig 寄存器配置
type RegisterConfig struct {
	Key       string            `json:"key"`                  // 数据点标识符
	Address   uint16            `json:"address"`              // 寄存器地址
	Quantity  uint16            `json:"quantity"`             // 读取数量
	Type      string            `json:"type"`                 // 数据类型
	Function  uint8             `json:"function"`             // 功能码
	Scale     float64           `json:"scale"`                // 缩放因子
	ByteOrder string            `json:"byte_order,omitempty"` // 字节序
	BitOffset int               `json:"bit_offset,omitempty"` // 位偏移
	DeviceID  byte              `json:"device_id"`            // 设备ID
	Tags      map[string]string `json:"tags,omitempty"`       // 标签
}

// DataPayload 数据消息载荷
type DataPayload struct {
	Points []DataPoint `json:"points"` // 数据点数组
}

// DataPoint 数据点
type DataPoint struct {
	Key       string            `json:"key"`            // 数据点标识符
	Source    string            `json:"source"`         // 数据源
	Timestamp int64             `json:"timestamp"`      // 时间戳（纳秒）
	Value     interface{}       `json:"value"`          // 数据值
	Type      string            `json:"type"`           // 数据类型
	Quality   int               `json:"quality"`        // 质量码
	Tags      map[string]string `json:"tags,omitempty"` // 标签
}

// GetTagsSafe 安全获取所有标签（ISP DataPoint专用）
func (dp *DataPoint) GetTagsSafe() map[string]string {
	if dp.Tags == nil {
		return make(map[string]string)
	}
	// ISP DataPoint的Tags通常在单线程环境下使用，但为安全起见创建副本
	result := make(map[string]string, len(dp.Tags))
	for k, v := range dp.Tags {
		result[k] = v
	}
	return result
}

// GetTag 安全获取单个标签值
func (dp *DataPoint) GetTag(key string) (string, bool) {
	if dp.Tags == nil {
		return "", false
	}
	value, exists := dp.Tags[key]
	return value, exists
}

// AddTag 添加标签
func (dp *DataPoint) AddTag(key, value string) {
	if dp.Tags == nil {
		dp.Tags = make(map[string]string)
	}
	dp.Tags[key] = value
}

// StatusPayload 状态查询载荷（空结构体）
type StatusPayload struct{}

// ResponsePayload 响应消息载荷
type ResponsePayload struct {
	Success bool        `json:"success"`         // 是否成功
	Error   string      `json:"error,omitempty"` // 错误信息
	Data    interface{} `json:"data,omitempty"`  // 响应数据
}

// StatusData 状态响应数据
type StatusData struct {
	Name      string `json:"name"`              // 插件名称
	Running   bool   `json:"running"`           // 是否运行中
	Connected bool   `json:"connected"`         // 是否连接正常
	Health    string `json:"health"`            // 健康状态
	Message   string `json:"message,omitempty"` // 状态消息
}

// MetricsPayload 指标消息载荷
type MetricsPayload struct {
	DataPointsCollected int64         `json:"data_points_collected"` // 采集的数据点总数
	ErrorsCount         int64         `json:"errors_count"`          // 错误总数
	ConnectionUptime    int64         `json:"connection_uptime"`     // 连接正常运行时间（秒）
	LastError           string        `json:"last_error,omitempty"`  // 最后错误信息
	AverageResponseTime float64       `json:"average_response_time"` // 平均响应时间（毫秒）
	StartTime           int64         `json:"start_time"`            // 启动时间（纳秒时间戳）
	LastDataTime        int64         `json:"last_data_time"`        // 最后数据时间（纳秒时间戳）
	Extra               map[string]interface{} `json:"extra,omitempty"` // 扩展指标
}

// NewConfigMessage 创建配置消息
func NewConfigMessage(id string, config ConfigPayload) (*ISPMessage, error) {
	payload, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	return &ISPMessage{
		Type:      MessageTypeConfig,
		ID:        id,
		Timestamp: time.Now().UnixNano(),
		Payload:   payload,
	}, nil
}

// NewDataMessage 创建数据消息
func NewDataMessage(points []DataPoint) (*ISPMessage, error) {
	payload, err := json.Marshal(DataPayload{Points: points})
	if err != nil {
		return nil, err
	}

	return &ISPMessage{
		Type:      MessageTypeData,
		Timestamp: time.Now().UnixNano(),
		Payload:   payload,
	}, nil
}

// NewStatusMessage 创建状态查询消息
func NewStatusMessage(id string) *ISPMessage {
	return &ISPMessage{
		Type:      MessageTypeStatus,
		ID:        id,
		Timestamp: time.Now().UnixNano(),
	}
}

// NewResponseMessage 创建响应消息
func NewResponseMessage(id string, success bool, errMsg string, data interface{}) (*ISPMessage, error) {
	response := ResponsePayload{
		Success: success,
		Error:   errMsg,
		Data:    data,
	}

	payload, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}

	return &ISPMessage{
		Type:      MessageTypeResponse,
		ID:        id,
		Timestamp: time.Now().UnixNano(),
		Payload:   payload,
	}, nil
}

// NewHeartbeatMessage 创建心跳消息
func NewHeartbeatMessage() *ISPMessage {
	return &ISPMessage{
		Type:      MessageTypeHeartbeat,
		Timestamp: time.Now().UnixNano(),
	}
}

// NewMetricsMessage 创建指标消息
func NewMetricsMessage(metrics MetricsPayload) (*ISPMessage, error) {
	payload, err := json.Marshal(metrics)
	if err != nil {
		return nil, err
	}

	return &ISPMessage{
		Type:      MessageTypeMetrics,
		Timestamp: time.Now().UnixNano(),
		Payload:   payload,
	}, nil
}

// NewMetricsRequestMessage 创建指标请求消息
func NewMetricsRequestMessage(id string) *ISPMessage {
	return &ISPMessage{
		Type:      MessageTypeMetrics,
		ID:        id,
		Timestamp: time.Now().UnixNano(),
	}
}

// ParseConfigPayload 解析配置载荷
func (msg *ISPMessage) ParseConfigPayload() (*ConfigPayload, error) {
	var config ConfigPayload
	err := json.Unmarshal(msg.Payload, &config)
	return &config, err
}

// ParseDataPayload 解析数据载荷
func (msg *ISPMessage) ParseDataPayload() (*DataPayload, error) {
	var data DataPayload
	err := json.Unmarshal(msg.Payload, &data)
	return &data, err
}

// ParseResponsePayload 解析响应载荷
func (msg *ISPMessage) ParseResponsePayload() (*ResponsePayload, error) {
	var response ResponsePayload
	err := json.Unmarshal(msg.Payload, &response)
	return &response, err
}

// ParseMetricsPayload 解析指标载荷
func (msg *ISPMessage) ParseMetricsPayload() (*MetricsPayload, error) {
	var metrics MetricsPayload
	err := json.Unmarshal(msg.Payload, &metrics)
	return &metrics, err
}

// ToJSON 将消息序列化为JSON字符串
func (msg *ISPMessage) ToJSON() ([]byte, error) {
	return json.Marshal(msg)
}

// FromJSON 从JSON字符串反序列化消息
func FromJSON(data []byte) (*ISPMessage, error) {
	var msg ISPMessage
	err := json.Unmarshal(data, &msg)
	return &msg, err
}
