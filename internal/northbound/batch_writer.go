package northbound

import (
	"context"
	"errors"

	"github.com/y001j/iot-gateway/internal/model"
)

// BatchWriter 同步写入接口（本地暂存/断网续传使用）。
//
// 返回 nil 表示目标系统已确认接收（落库或 broker 确认），这是 ACK 本地游标的唯一依据。
// Publish 可以是异步的（例如 InfluxDB 非阻塞 WriteAPI、MQTT 发布队列），无法据此判断是否写入成功，
// 所以开启缓冲的 Sink 应实现 BatchWriter；未实现时退回调用 Publish（只能做到"尽力而为"）。
type BatchWriter interface {
	WriteBatch(ctx context.Context, batch []model.Point) error
}

// HealthChecker 目标不健康时，缓冲运行器暂停拉取并退避，避免空转重试
type HealthChecker interface {
	Healthy() error
}

// ErrPermanent 永久错误（例如数据格式被目标系统拒绝）：重试也不会成功，
// 缓冲运行器会丢弃该批数据并计数，而不是无限重试阻塞后续数据。
// 用法：return fmt.Errorf("%w: %v", northbound.ErrPermanent, err)
var ErrPermanent = errors.New("permanent sink error")

// IsPermanent 判断是否为永久错误
func IsPermanent(err error) bool { return errors.Is(err, ErrPermanent) }
