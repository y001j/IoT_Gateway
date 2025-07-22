package northbound

import (
	"context"
	"encoding/json"

	"github.com/nats-io/nats.go"
	"github.com/y001j/iot-gateway/internal/model"
)

// Sink 定义了所有北向连接器必须实现的接口
type Sink interface {
	// Name 返回连接器的唯一名称
	Name() string
	
	// Init 初始化连接器，传入JSON格式的配置
	Init(cfg json.RawMessage) error
	
	// Start 启动连接器，准备发送数据
	Start(ctx context.Context) error
	
	// Publish 发布一批数据点到目标系统
	Publish(batch []model.Point) error
	
	// Stop 停止连接器，释放资源
	Stop() error
}


// NATSAwareSink 定义了需要NATS连接的连接器接口
// 这是一个可选接口，只有需要从NATS接收数据的连接器才需要实现
type NATSAwareSink interface {
	Sink
	
	// SetNATSConnection 设置共享的NATS连接
	SetNATSConnection(conn *nats.Conn)
}

// Config 是连接器配置的基础结构
type Config struct {
	Name     string          `json:"name"`
	Type     string          `json:"type"`
	Params   json.RawMessage `json:"params"`
	BatchSize int            `json:"batch_size,omitempty"` // 批量发送大小
	BufferSize int           `json:"buffer_size,omitempty"` // 内存缓冲大小
	Tags     map[string]string `json:"tags,omitempty"`    // 附加标签
}

// SinkFactory 定义了创建连接器实例的工厂函数类型
type SinkFactory func() Sink

// Registry 维护所有已注册的连接器工厂
var Registry = make(map[string]SinkFactory)

// Register 注册一个连接器工厂到全局注册表
func Register(typeName string, factory SinkFactory) {
	Registry[typeName] = factory
}

// Create 根据类型名创建连接器实例
func Create(typeName string) (Sink, bool) {
	factory, exists := Registry[typeName]
	if !exists {
		return nil, false
	}
	return factory(), true
}

// CreateSink 根据类型名创建连接器实例（向后兼容）
func CreateSink(typeName string) Sink {
	sink, _ := Create(typeName)
	return sink
}
